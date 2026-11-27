package tests

import (
	"flag"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// update rewrites the recorded stream: go test ./tests -run TestSSE -update
var update = flag.Bool("update", false, "rewrite tests/testdata from this run")

// sseStreamFixture is the stream TestSSEConnectionCarriesTheSocketProtocol gets, its random id and secret replaced, so
// a client transport can replay a real stream.
const sseStreamFixture = "testdata/sse-stream.txt"

func TestSSEConnectionCarriesTheSocketProtocol(t *testing.T) {
	srv := start(t)
	c := dialSSE(t, srv, issue(t, appA, person{"u1", "s1", []string{"room"}}))
	other := connect(t, srv, appA, person{"u2", "s2", []string{"room"}})
	other.join("room")
	recorded := []string{strings.NewReplacer(c.id, "<connection>", c.secret, "<secret>").Replace(c.first)}
	expect := func(want string) {
		t.Helper()
		got := c.read()
		if !strings.Contains(got, want) {
			t.Fatalf("got %q, want %s", got, want)
		}
		recorded = append(recorded, got)
	}

	if code := c.send(`["1","1","room","phx_join",{}]`); code != http.StatusNoContent {
		t.Fatalf("join: %d", code)
	}
	expect(`["1","1","room","phx_reply",{"response":{},"status":"ok"}]`)
	command(t, srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{"text":"hello"}}`)
	expect(`[null,null,"room","new",{"text":"hello"}]`)
	other.read()
	batch := `[[null,"2","phoenix","heartbeat",{}],["1","3","room","typing",{"thread":"t1"}]]`
	if code := c.send(batch); code != http.StatusNoContent {
		t.Fatalf("batch: %d", code)
	}
	expect(`[null,"2","phoenix","phx_reply",{"response":{},"status":"ok"}]`)
	expect(`["1","3","room","phx_reply",{"response":{},"status":"ok"}]`)
	if got := other.read(); !strings.Contains(got, `"event":"typing","payload":{"thread":"t1"}`) {
		t.Fatalf("other got %s", got)
	}
	command(t, srv, appA, "/sessions/close", `{"session":"s1"}`)
	expect("event: close\ndata: {\"code\":1000,")
	c.waitEnded()
	if code := c.send(`[null,"4","phoenix","heartbeat",{}]`); code != http.StatusNotFound {
		t.Fatalf("send after close: %d", code)
	}

	stream := strings.Join(recorded, "")
	if *update {
		if err := os.WriteFile(sseStreamFixture, []byte(stream), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fixture, err := os.ReadFile(sseStreamFixture)
	if err != nil {
		t.Fatal(err)
	}
	if string(fixture) != stream {
		t.Fatalf("the stream differs from %s, rewrite it with -update:\n%s", sseStreamFixture, stream)
	}
}

func TestSSEDroppedStreamUnregistersItsConnection(t *testing.T) {
	srv := start(t)
	c := dialSSE(t, srv, issue(t, appA, person{"u1", "s1", []string{"room"}}))
	c.send(`["1","1","room","phx_join",{}]`)
	c.read()
	c.cancel()
	c.waitEnded()
	// The relay notices the dropped request on its own time.
	deadline := time.Now().Add(5 * time.Second)
	for n := 1; command(t, srv, appA, "/publish", `{"id":"`+strconv.Itoa(n)+`","channel":"room","event":"new","payload":{}}`) != `{"sockets":0}`; n++ {
		if time.Now().After(deadline) {
			t.Fatal("the dropped stream's connection stayed registered")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSSERefusesWhatTheWebSocketRefuses(t *testing.T) {
	srv := start(t)
	ticket := issue(t, appA, person{"u1", "s1", []string{"room"}})
	cases := []struct {
		name, query, accept, origin string
		want                        int
	}{
		{"no event stream", "vsn=2.0.0&ticket=" + ticket, "application/json", "", http.StatusNotAcceptable},
		{"wrong version", "vsn=1.0.0&ticket=" + ticket, "text/event-stream", "", http.StatusBadRequest},
		{"bad ticket", "vsn=2.0.0&ticket=x", "text/event-stream", "", http.StatusUnauthorized},
		{"other origin", "vsn=2.0.0&ticket=" + ticket, "text/event-stream", "https://other.example", http.StatusForbidden},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/socket/sse?"+tc.query, nil)
		req.Header.Set("Accept", tc.accept)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, res.StatusCode, tc.want)
		}
	}
}
