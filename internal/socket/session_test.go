package socket

import (
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

func newSession(t *testing.T, user string, channels ...string) *session {
	t.Helper()
	app := hub.NewApp(config.App{ID: "a", ClientEvents: &config.ClientEvents{AllowAny: true},
		Limits: config.Limits{Connections: 10, ChannelsPerTicket: 5, RequestBytes: 1024}}, nil)
	out := newSink(func() {})
	conn, err := app.Register(ticket.Ticket{App: "a", User: user, Anonymous: user == "", Session: "s1",
		Channels: channels}, out, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &session{app: app, conn: conn, out: out, now: func() time.Time { return time.UnixMilli(5) }}
}

// exchange handles one raw frame and returns the reply.
func exchange(t *testing.T, s *session, raw string) string {
	t.Helper()
	f, err := phoenix.Decode([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	s.handle(f)
	select {
	case reply := <-s.out.queue:
		return string(reply)
	default:
		t.Fatalf("no reply to %s", raw)
		return ""
	}
}

func TestSessionAnswersTheProtocol(t *testing.T) {
	s := newSession(t, "u1", "room")
	steps := []struct{ send, want string }{
		{`[null,"1","phoenix","heartbeat",{}]`, `[null,"1","phoenix","phx_reply",{"response":{},"status":"ok"}]`},
		{`["2","2","other","phx_join",{}]`, `["2","2","other","phx_reply",{"response":{"reason":"unauthorized"},"status":"error"}]`},
		{`["3","3","room","typing",{}]`, `["3","3","room","phx_reply",{"response":{"reason":"unmatched topic"},"status":"error"}]`},
		{`["4","4","room","phx_join",{"viewing":[""]}]`, `["4","4","room","phx_reply",{"response":{"reason":"invalid_interest"},"status":"error"}]`},
		{`["5","5","room","phx_join",{"viewing":["thread:1"]}]`, `["5","5","room","phx_reply",{"response":{},"status":"ok"}]`},
		{`["5","6","room","interest",{"viewing":["thread:2"]}]`, `["5","6","room","phx_reply",{"response":{},"status":"ok"}]`},
		{`["5","7","room","typing",{}]`, `["5","7","room","phx_reply",{"response":{},"status":"ok"}]`},
		{`["5","8","room","phx_leave",{}]`, `["5","8","room","phx_reply",{"response":{},"status":"ok"}]`},
		{`["5","9","room","typing",{}]`, `["5","9","room","phx_reply",{"response":{"reason":"unmatched topic"},"status":"error"}]`},
	}
	for _, step := range steps {
		if got := exchange(t, s, step.send); got != step.want {
			t.Errorf("%s:\n got %s\nwant %s", step.send, got, step.want)
		}
	}
}

func TestAnonymousSessionIsReceiveOnly(t *testing.T) {
	s := newSession(t, "", "room")
	exchange(t, s, `["1","1","room","phx_join",{}]`)
	want := `["1","2","room","phx_reply",{"response":{"reason":"receive_only"},"status":"error"}]`
	if got := exchange(t, s, `["1","2","room","typing",{}]`); got != want {
		t.Fatalf("got %s", got)
	}
}

func TestSessionServesTheRelayTopic(t *testing.T) {
	s := newSession(t, "u1", "room")
	steps := []struct{ send, want string }{
		{`["1","1","relay","presence",{"status":"away"}]`, `["1","1","relay","phx_reply",{"response":{"reason":"unmatched topic"},"status":"error"}]`},
		{`["1","2","relay","phx_join",{}]`, `["1","2","relay","phx_reply",{"response":{},"status":"ok"}]`},
		{`["1","3","relay","presence",{"status":"asleep"}]`, `["1","3","relay","phx_reply",{"response":{"reason":"invalid_presence"},"status":"error"}]`},
		{`["1","4","relay","presence",{"status":"away"}]`, `["1","4","relay","phx_reply",{"response":{},"status":"ok"}]`},
		{`["1","5","relay","ring",{"payload":{}}]`, `["1","5","relay","phx_reply",{"response":{"reason":"invalid_direct"},"status":"error"}]`},
		{`["1","6","relay","ring",{"to":"u2","payload":{}}]`, `["1","6","relay","phx_reply",{"response":{"reason":"unreachable"},"status":"error"}]`},
		{`["1","7","relay","ring",{"to":"u1","payload":{}}]`, `["1","7","relay","phx_reply",{"response":{},"status":"ok"}]`},
	}
	for _, step := range steps {
		if got := exchange(t, s, step.send); got != step.want {
			t.Errorf("%s:\n got %s\nwant %s", step.send, got, step.want)
		}
	}
}

func TestAnonymousSessionHasNoRelayTopic(t *testing.T) {
	s := newSession(t, "", "room")
	want := `["1","1","relay","phx_reply",{"response":{"reason":"receive_only"},"status":"error"}]`
	if got := exchange(t, s, `["1","1","relay","phx_join",{}]`); got != want {
		t.Fatalf("got %s", got)
	}
}
