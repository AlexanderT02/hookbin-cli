package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hookbin/hookbin-cli/internal/forward"
	"github.com/hookbin/hookbin-cli/internal/protocol"
)

type sessionFixture struct {
	server *httptest.Server
	target string
	result chan []byte
	ping   bool
	expire bool
}

func (fixture *sessionFixture) withPing() *sessionFixture {
	fixture.ping = true
	return fixture
}

func (fixture *sessionFixture) withExpiry() *sessionFixture {
	fixture.expire = true
	return fixture
}

func newSessionFixture(t *testing.T, targetError bool) *sessionFixture {
	t.Helper()
	fixture := &sessionFixture{result: make(chan []byte, 1)}
	fixture.target = "http://127.0.0.1:1/webhook"
	if !targetError {
		local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Local", "yes")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("ok"))
		}))
		t.Cleanup(local.Close)
		fixture.target = local.URL + "/webhook"
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			fixture.result <- nil
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hello, _ := json.Marshal(protocol.Hello{Type: "hello", BinSlug: "abcdefghijklmnop",
			ProtocolVersion: 1, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
		if conn.Write(ctx, websocket.MessageText, hello) != nil {
			fixture.result <- nil
			return
		}
		if _, _, err := conn.Read(ctx); err != nil {
			fixture.result <- nil
			return
		} // ready
		if fixture.expire {
			_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"expired"}`))
			return
		}
		if fixture.ping {
			if conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`)) != nil {
				fixture.result <- nil
				return
			}
			_, pong, err := conn.Read(ctx)
			if err != nil || string(pong) != `{"type":"pong"}` {
				fixture.result <- nil
				return
			}
		}
		request, _ := json.Marshal(protocol.Request{Type: "request",
			AttemptID: "4a2d0000-0000-0000-0000-000000000000",
			RequestID: "3f1c0000-0000-0000-0000-000000000000", Method: "POST", Path: "/orders"})
		if conn.Write(ctx, websocket.MessageText, request) != nil {
			fixture.result <- nil
			return
		}
		_, result, err := conn.Read(ctx)
		if err != nil {
			fixture.result <- nil
			return
		}
		fixture.result <- result
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *sessionFixture) run(t *testing.T) error {
	t.Helper()
	f, err := forward.New(fixture.target, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return runSession(ctx, strings.Replace(fixture.server.URL, "http", "ws", 1),
		"abcdefghijklmnop", "secret", fixture.target, f, &sync.Once{})
}

func (fixture *sessionFixture) message(t *testing.T) map[string]any {
	t.Helper()
	var message map[string]any
	select {
	case data := <-fixture.result:
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("no CLI result")
	}
	return message
}

func TestReportsLocalResponse(t *testing.T) {
	fixture := newSessionFixture(t, false)
	fixture.run(t)
	if message := fixture.message(t); message["type"] != "response" || message["status"] != float64(201) || message["attemptId"] != "4a2d0000-0000-0000-0000-000000000000" {
		t.Fatalf("result = %#v", message)
	}
}

func TestReportsLocalError(t *testing.T) {
	fixture := newSessionFixture(t, true)
	fixture.run(t)
	if message := fixture.message(t); message["type"] != "error" || message["message"] == "" {
		t.Fatalf("result = %#v", message)
	}
}

func TestRepliesToServerPing(t *testing.T) {
	fixture := newSessionFixture(t, false).withPing()
	fixture.run(t)
	if message := fixture.message(t); message["type"] != "response" {
		t.Fatalf("result = %#v", message)
	}
}

func TestExpiryStopsSession(t *testing.T) {
	fixture := newSessionFixture(t, false).withExpiry()
	err := fixture.run(t)
	if _, ok := err.(permanentError); !ok {
		t.Fatalf("error = %v", err)
	}
}
