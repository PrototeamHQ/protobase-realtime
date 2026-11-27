package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/socket"
)

// sseClient reads a server-sent event stream in the background, one event at a time.
type sseClient struct {
	t          *testing.T
	srv        *httptest.Server
	id, secret string
	// first is the connection event.
	first  string
	events chan string
	ended  chan struct{}
	cancel context.CancelFunc
}

// dialSSE opens a stream with the ticket and reads its connection event.
func dialSSE(t *testing.T, srv *httptest.Server, ticket string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+socket.EventsPath+"?vsn=2.0.0&ticket="+url.QueryEscape(ticket), nil)
	req.Header.Set("Accept", "text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("open: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	c := &sseClient{t: t, srv: srv, events: make(chan string, 64), ended: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(c.ended)
		defer res.Body.Close()
		lines := bufio.NewScanner(res.Body)
		var event strings.Builder
		for lines.Scan() {
			event.WriteString(lines.Text() + "\n")
			if lines.Text() == "" {
				c.events <- event.String()
				event.Reset()
			}
		}
	}()
	first := c.read()
	data, ok := strings.CutPrefix(first, "event: connection\ndata: ")
	var conn struct{ Connection, Secret string }
	if !ok || json.Unmarshal([]byte(data), &conn) != nil || conn.Connection == "" || conn.Secret == "" {
		t.Fatalf("first event: %q", first)
	}
	c.id, c.secret, c.first = conn.Connection, conn.Secret, first
	return c
}

// read returns the next event, or "" when none arrives within a short wait.
func (c *sseClient) read() string {
	c.t.Helper()
	select {
	case event := <-c.events:
		return event
	case <-time.After(time.Second):
		return ""
	}
}

// send posts a frame, or an array of frames, with the connection's secret and returns the status.
func (c *sseClient) send(body string) int {
	c.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, c.srv.URL+socket.SendPath(c.id), strings.NewReader(body))
	req.Header.Set(socket.SecretHeader, c.secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// waitEnded fails unless the relay ends the stream.
func (c *sseClient) waitEnded() {
	c.t.Helper()
	select {
	case <-c.ended:
	case <-time.After(5 * time.Second):
		c.t.Fatal("stream stayed open")
	}
}
