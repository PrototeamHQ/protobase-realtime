package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

const (
	keyA = "app-a-api-key-0123456789abcdefgh"
	keyB = "app-b-api-key-0123456789abcdefgh"
)

type fakeSink struct {
	frames []string
	closed bool
}

func (s *fakeSink) Send(frame []byte) { s.frames = append(s.frames, string(frame)) }
func (s *fakeSink) Close(string)      { s.closed = true }
func (s *fakeSink) Restart()          { s.closed = true }

func setup(t *testing.T) (*httptest.Server, *hub.Hub) {
	t.Helper()
	limits := config.Limits{Connections: 10, ChannelsPerTicket: 5, RequestBytes: 512}
	h := hub.New([]config.App{
		{ID: "a", APIKeys: []string{"old-key-for-a-0123456789abcdefgh", keyA}, Limits: limits},
		{ID: "b", APIKeys: []string{keyB}, Limits: limits},
	}, nil)
	mux := http.NewServeMux()
	Mount(mux, h, time.Now)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, h
}

func joined(t *testing.T, h *hub.Hub, user, session, channel string) *fakeSink {
	t.Helper()
	app, _ := h.App("a")
	sink := &fakeSink{}
	c, err := app.Register(ticket.Ticket{App: "a", User: user, Session: session, Channels: []string{channel}}, sink, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Join(c, channel, nil, nil); err != nil {
		t.Fatal(err)
	}
	return sink
}

func post(t *testing.T, srv *httptest.Server, path, key, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, strings.TrimSpace(string(out))
}

func TestCommandsRequireTheAppsOwnKey(t *testing.T) {
	srv, _ := setup(t)
	for _, tc := range []struct{ path, key string }{
		{"/apps/a/publish", keyB}, {"/apps/a/publish", ""}, {"/apps/z/publish", keyA},
		{"/apps/a/channels/add", keyB}, {"/apps/a/channels/remove", keyB}, {"/apps/a/sessions/close", keyB},
		{"/apps/a/people/rules", keyB},
	} {
		if status, _ := post(t, srv, tc.path, tc.key, `{}`); status != http.StatusUnauthorized {
			t.Errorf("%s with %q: %d", tc.path, tc.key, status)
		}
	}
}

func TestPublishSendsOncePerId(t *testing.T) {
	srv, h := setup(t)
	sink := joined(t, h, "u1", "s1", "room")
	body := `{"id":"m1","channel":"room","event":"new","payload":{"n":1}}`
	if status, out := post(t, srv, "/apps/a/publish", keyA, body); status != 200 || out != `{"sockets":1}` {
		t.Fatalf("%d %s", status, out)
	}
	if status, out := post(t, srv, "/apps/a/publish", keyA, body); status != 200 || out != `{"sockets":0,"duplicate":true}` {
		t.Fatalf("%d %s", status, out)
	}
	if len(sink.frames) != 1 || sink.frames[0] != `[null,null,"room","new",{"n":1}]` {
		t.Fatalf("got %v", sink.frames)
	}
}

func TestPublishIdsArePerApp(t *testing.T) {
	srv, _ := setup(t)
	body := `{"id":"m1","channel":"room","event":"new","payload":{}}`
	post(t, srv, "/apps/a/publish", keyA, body)
	if _, out := post(t, srv, "/apps/b/publish", keyB, body); out != `{"sockets":0}` {
		t.Fatalf("got %s", out)
	}
}

func TestPublishRefusesInvalidRequests(t *testing.T) {
	srv, _ := setup(t)
	cases := map[string]int{
		`{"channel":"room","event":"new","payload":{}}`:                                                400,
		`{"id":"m","event":"new","payload":{}}`:                                                        400,
		`{"id":"m","channel":"room","event":"phx_close","payload":{}}`:                                 400,
		`{"id":"m","channel":"room","event":"client_event","payload":{}}`:                              400,
		`{"id":"m","channel":"relay","event":"new","payload":{}}`:                                      400,
		`{"id":"m","channel":"room","event":"new","payload":[1]}`:                                      400,
		`{"id":"m","channel":"room","event":"new","payload":{},"from":"x"}`:                            400,
		`{"id":"m","channel":"room","event":"new","payload":{"x":"` + strings.Repeat("a", 600) + `"}}`: 413,
	}
	for body, want := range cases {
		if status, _ := post(t, srv, "/apps/a/publish", keyA, body); status != want {
			t.Errorf("%.80s: %d, want %d", body, status, want)
		}
	}
}

func TestMembersAddAndRemove(t *testing.T) {
	srv, h := setup(t)
	sink := joined(t, h, "u1", "s1", "room")
	if _, out := post(t, srv, "/apps/a/channels/add", keyA, `{"channel":"news","users":["u1"]}`); out != `{"sockets":1}` {
		t.Fatalf("got %s", out)
	}
	for range 2 {
		if _, out := post(t, srv, "/apps/a/channels/remove", keyA, `{"channel":"room","users":["u1"]}`); out != `{"sockets":1}` {
			t.Fatalf("got %s", out)
		}
	}
	if len(sink.frames) != 1 || !strings.Contains(sink.frames[0], `"phx_close"`) {
		t.Fatalf("got %v", sink.frames)
	}
	if status, _ := post(t, srv, "/apps/a/channels/add", keyA, `{"users":["u1"]}`); status != 400 {
		t.Fatalf("no channel: %d", status)
	}
}

func TestCloseSession(t *testing.T) {
	srv, h := setup(t)
	closing := joined(t, h, "u1", "s1", "room")
	staying := joined(t, h, "u1", "s2", "room")
	if _, out := post(t, srv, "/apps/a/sessions/close", keyA, `{"session":"s1"}`); out != `{"sockets":1}` {
		t.Fatalf("got %s", out)
	}
	if !closing.closed || staying.closed {
		t.Fatal("wrong sockets closed")
	}
	if status, _ := post(t, srv, "/apps/a/sessions/close", keyA, `{}`); status != 400 {
		t.Fatalf("no session: %d", status)
	}
}

func TestRulesReplaceThePersonsRules(t *testing.T) {
	srv, h := setup(t)
	joined(t, h, "u1", "s1", "room")
	if _, out := post(t, srv, "/apps/a/people/rules", keyA, `{"user":"u1","blocked":["u2"]}`); out != `{"sockets":1}` {
		t.Fatalf("got %s", out)
	}
	if _, out := post(t, srv, "/apps/a/people/rules", keyA, `{"user":"gone","friends_only":true}`); out != `{"sockets":0}` {
		t.Fatalf("got %s", out)
	}
	if status, _ := post(t, srv, "/apps/a/people/rules", keyA, `{"blocked":["u2"]}`); status != 400 {
		t.Fatalf("no user: %d", status)
	}
}
