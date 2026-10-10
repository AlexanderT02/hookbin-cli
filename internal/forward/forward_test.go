package forward

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hookbin/hookbin-cli/internal/protocol"
)

func TestForwardPreservesRequestAndDropsHopHeaders(t *testing.T) {
	type received struct {
		path, query, removed string
		ids                  []string
		body                 string
	}
	got := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- received{r.URL.Path, r.URL.RawQuery, r.Header.Get("X-Remove"),
			r.Header.Values("X-Event-ID"), string(body)}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	f, err := New(server.URL+"/webhook?token=1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.Send(context.Background(), protocol.Request{
		Method: "POST", Path: "/orders", Query: "source=test",
		Headers: map[string][]string{"Content-Type": {"application/json"},
			"X-Event-ID": {"one", "two"}, "Connection": {"X-Remove"}, "X-Remove": {"secret"}},
		BodyBase64: base64.StdEncoding.EncodeToString([]byte(`{"id":1}`)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != http.StatusAccepted {
		t.Fatalf("status = %d", result.Status)
	}
	r := <-got
	if r.path != "/webhook/orders" || r.query != "token=1&source=test" {
		t.Fatalf("path/query = %s?%s", r.path, r.query)
	}
	if r.removed != "" || len(r.ids) != 2 {
		t.Fatalf("removed = %q, ids = %#v", r.removed, r.ids)
	}
	if r.body != `{"id":1}` {
		t.Fatalf("body = %s", r.body)
	}
}

func TestRootPathUsesTargetPath(t *testing.T) {
	f, err := New("http://localhost:9000/webhook", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.URL("/", "")
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/webhook" {
		t.Fatalf("path = %s", u.Path)
	}
}

func TestSetTargetSwitchesUrlAndRejectsInvalid(t *testing.T) {
	f, err := New("http://localhost:3000", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetTarget("http://localhost:8080/hooks"); err != nil {
		t.Fatal(err)
	}
	u, err := f.URL("/stripe", "a=1")
	if err != nil || u.String() != "http://localhost:8080/hooks/stripe?a=1" {
		t.Fatalf("url = %v, %v", u, err)
	}
	for _, invalid := range []string{"localhost:3000", "file:///etc/passwd", "http://user:pass@localhost"} {
		if f.SetTarget(invalid) == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	if f.Target() != "http://localhost:8080/hooks" {
		t.Fatalf("target = %s", f.Target())
	}
}

func TestResponseBodyIsTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), MaxResponseBytes+100))
	}))
	defer server.Close()
	f, err := New(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.Send(context.Background(), protocol.Request{Method: "GET", Path: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Body) != MaxResponseBytes {
		t.Fatalf("body bytes = %d", len(result.Body))
	}
}
