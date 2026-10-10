package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/hookbin/hookbin-cli/internal/forward"
	"github.com/hookbin/hookbin-cli/internal/protocol"
)

var Version = "dev"

type permanentError string

func (e permanentError) Error() string { return string(e) }

func Listen(ctx context.Context, server, slug, secret, target string, timeout time.Duration, insecure bool) error {
	f, err := forward.NewWithOptions(target, timeout, insecure)
	if err != nil {
		return err
	}
	u, err := url.Parse(server)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("--server must be an absolute http(s) URL")
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/ws/agent"
	u.RawQuery = url.Values{"slug": {slug}}.Encode()
	backoff := time.Second
	var warned sync.Once
	for {
		started := time.Now()
		err := runSession(ctx, u.String(), slug, secret, target, f, &warned)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return nil
		}
		var permanent permanentError
		if errors.As(err, &permanent) {
			return err
		}
		if time.Since(started) > 30*time.Second {
			backoff = time.Second
		}
		fmt.Printf("Disconnected: %v; reconnecting in %s\n", err, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func runSession(ctx context.Context, endpoint, slug, secret, target string, f *forward.Forwarder, warned *sync.Once) error {
	conn, response, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + secret}, "Hookbin-Protocol": {"1"}},
	})
	if err != nil {
		if response != nil {
			switch response.StatusCode {
			case http.StatusUnauthorized:
				return permanentError("invalid bin secret")
			case http.StatusGone:
				return permanentError("bin expired; create a new one at https://hookbin.madebyflow.de")
			case http.StatusNotFound:
				return permanentError("unknown bin")
			case http.StatusUpgradeRequired:
				return permanentError("protocol unsupported; update hookbin")
			}
		}
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(2 * 1024 * 1024)
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("read hello: %w", err)
	}
	if messageType != websocket.MessageText {
		return permanentError("expected hello text frame")
	}
	var hello protocol.Hello
	if err := json.Unmarshal(data, &hello); err != nil || hello.Type != "hello" || hello.ProtocolVersion != 1 {
		return permanentError("invalid or unsupported server hello")
	}
	expiresAt, err := time.Parse(time.RFC3339, hello.ExpiresAt)
	if err != nil {
		return permanentError("invalid bin expiry from server")
	}
	if !expiresAt.After(time.Now()) {
		return permanentError("bin expired; create a new one at https://hookbin.madebyflow.de")
	}
	sessionCtx, cancel := context.WithDeadline(ctx, expiresAt)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	go warnBeforeExpiry(sessionCtx, expiresAt, warned)
	fmt.Printf("Connected to bin %s -> forwarding to %s (expires in %s)\n",
		hello.BinSlug, target, time.Until(expiresAt).Round(time.Minute))
	var writeMu sync.Mutex
	write := func(message any) error {
		payload, err := json.Marshal(message)
		if err != nil {
			return err
		}
		writeCtx, done := context.WithTimeout(sessionCtx, 5*time.Second)
		defer done()
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.Write(writeCtx, websocket.MessageText, payload)
	}
	if err := write(protocol.Ready{Type: "ready", CliVersion: Version, TargetURL: target}); err != nil {
		return fmt.Errorf("send ready: %w", err)
	}
	slots := make(chan struct{}, 8)
	for {
		messageType, data, err = conn.Read(sessionCtx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(sessionCtx.Err(), context.DeadlineExceeded) {
				return permanentError("bin expired; create a new one at https://hookbin.madebyflow.de")
			}
			return fmt.Errorf("connection closed: %w", err)
		}
		if messageType != websocket.MessageText {
			continue
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			continue
		}
		switch envelope.Type {
		case "ping":
			if err := write(map[string]string{"type": "pong"}); err != nil {
				return err
			}
		case "expired":
			return permanentError("bin expired; create a new one at https://hookbin.madebyflow.de")
		case "request":
			var request protocol.Request
			if json.Unmarshal(data, &request) != nil || request.AttemptID == "" || request.RequestID == "" {
				continue
			}
			select {
			case slots <- struct{}{}:
				workers.Add(1)
				go func() {
					defer workers.Done()
					defer func() { <-slots }()
					if err := handleRequest(sessionCtx, f, request, write); err != nil {
						conn.CloseNow()
					}
				}()
			default:
				_ = write(protocol.Error{Type: "error", AttemptID: request.AttemptID,
					RequestID: request.RequestID, Message: "CLI is busy", DurationMs: 0})
			}
		}
	}
}

func handleRequest(ctx context.Context, f *forward.Forwarder, request protocol.Request, write func(any) error) error {
	result, err := f.Send(ctx, request)
	label := request.Path
	if request.Replay {
		label += " (replay)"
	}
	if err != nil {
		fmt.Printf("%s  %s %s  ->  %v\n", time.Now().Format("15:04:05"), request.Method, label, err)
		return write(protocol.Error{Type: "error", AttemptID: request.AttemptID,
			RequestID: request.RequestID, Message: err.Error(), DurationMs: result.Duration.Milliseconds()})
	}
	fmt.Printf("%s  %s %s  ->  %d  %d ms\n", time.Now().Format("15:04:05"),
		request.Method, label, result.Status, result.Duration.Milliseconds())
	return write(protocol.Response{Type: "response", AttemptID: request.AttemptID,
		RequestID: request.RequestID, Status: result.Status, Headers: result.Headers,
		BodyBase64: base64.StdEncoding.EncodeToString(result.Body), DurationMs: result.Duration.Milliseconds()})
}

func warnBeforeExpiry(ctx context.Context, expiry time.Time, once *sync.Once) {
	wait := time.Until(expiry.Add(-10 * time.Minute))
	if wait < 0 {
		wait = 0
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		once.Do(func() { fmt.Println("Warning: bin expires in less than 10 minutes") })
	case <-ctx.Done():
	}
}
