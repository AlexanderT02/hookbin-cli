package forward

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hookbin/hookbin-cli/internal/protocol"
)

var hopByHop = map[string]bool{
	"connection": true, "keep-alive": true, "proxy-authenticate": true,
	"proxy-authorization": true, "proxy-connection": true, "te": true,
	"trailer": true, "transfer-encoding": true, "upgrade": true,
	"host": true, "content-length": true,
}

type Forwarder struct {
	mu     sync.RWMutex
	target *url.URL // switched by SetTarget while requests are in flight
	client *http.Client
}

const MaxResponseBytes = 256 * 1024

type Result struct {
	Status   int
	Headers  map[string][]string
	Body     []byte
	Duration time.Duration
}

func New(target string, timeout time.Duration) (*Forwarder, error) {
	return NewWithOptions(target, timeout, false)
}

func parseTarget(target string) (*url.URL, error) {
	u, err := url.Parse(target)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("--forward-to must be an absolute http(s) URL")
	}
	if u.User != nil || u.Fragment != "" {
		return nil, errors.New("--forward-to cannot contain credentials or a fragment")
	}
	return u, nil
}

func NewWithOptions(target string, timeout time.Duration, insecure bool) (*Forwarder, error) {
	u, err := parseTarget(target)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		return nil, errors.New("--timeout must be positive")
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	if insecure {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Explicit --insecure for local HTTPS.
		client.Transport = transport
	}
	return &Forwarder{target: u, client: client}, nil
}

// Target is the local URL requests are forwarded to.
func (f *Forwarder) Target() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.target.String()
}

// SetTarget switches the local URL for every request from now on. Requests already being sent keep the old one.
func (f *Forwarder) SetTarget(target string) error {
	u, err := parseTarget(target)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.target = u
	f.mu.Unlock()
	return nil
}

func (f *Forwarder) URL(path, query string) (*url.URL, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("invalid request path")
	}
	if _, err := url.ParseRequestURI(path); err != nil {
		return nil, fmt.Errorf("invalid request path: %w", err)
	}
	f.mu.RLock()
	u := *f.target
	f.mu.RUnlock()
	basePath := strings.TrimSuffix(u.EscapedPath(), "/")
	rawPath := basePath + path
	if path == "/" && basePath != "" {
		rawPath = basePath
	}
	decoded, err := url.PathUnescape(rawPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path escape: %w", err)
	}
	u.Path = decoded
	u.RawPath = rawPath
	if u.RawQuery != "" && query != "" {
		u.RawQuery += "&" + query
	} else if query != "" {
		u.RawQuery = query
	}
	return &u, nil
}

func (f *Forwarder) Send(ctx context.Context, incoming protocol.Request) (Result, error) {
	start := time.Now()
	u, err := f.URL(incoming.Path, incoming.Query)
	if err != nil {
		return Result{Duration: time.Since(start)}, err
	}
	body, err := base64.StdEncoding.DecodeString(incoming.BodyBase64)
	if err != nil {
		return Result{Duration: time.Since(start)}, fmt.Errorf("invalid base64 body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, incoming.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return Result{Duration: time.Since(start)}, err
	}
	blocked := make(map[string]bool, len(hopByHop))
	for name, value := range hopByHop {
		blocked[name] = value
	}
	for name, values := range incoming.Headers {
		if strings.EqualFold(name, "Connection") {
			for _, value := range values {
				for _, token := range strings.Split(value, ",") {
					blocked[strings.ToLower(strings.TrimSpace(token))] = true
				}
			}
		}
	}
	for name, values := range incoming.Headers {
		if blocked[strings.ToLower(name)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	response, err := f.client.Do(req)
	if err != nil {
		return Result{Duration: time.Since(start)}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return Result{Duration: time.Since(start)}, err
	}
	if len(responseBody) > MaxResponseBytes {
		responseBody = responseBody[:MaxResponseBytes]
	}
	return Result{Status: response.StatusCode, Headers: filteredHeaders(response.Header),
		Body: responseBody, Duration: time.Since(start)}, nil
}

func filteredHeaders(source http.Header) map[string][]string {
	blocked := make(map[string]bool, len(hopByHop))
	for name, value := range hopByHop {
		blocked[name] = value
	}
	for name, values := range source {
		if strings.EqualFold(name, "Connection") {
			for _, value := range values {
				for _, token := range strings.Split(value, ",") {
					blocked[strings.ToLower(strings.TrimSpace(token))] = true
				}
			}
		}
	}
	filtered := make(map[string][]string)
	for name, values := range source {
		if !blocked[strings.ToLower(name)] {
			filtered[name] = append([]string(nil), values...)
		}
	}
	return filtered
}
