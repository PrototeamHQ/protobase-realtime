package hub

import (
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

type fakeSink struct {
	frames []string
	closed string
}

func (s *fakeSink) Send(frame []byte)   { s.frames = append(s.frames, string(frame)) }
func (s *fakeSink) Close(reason string) { s.closed = reason }
func (s *fakeSink) Restart()            { s.closed = "restart" }

var now = time.Unix(1_800_000_000, 0)

// allowAny lets every valid event through with the relay's fixed limits.
var allowAny = &config.ClientEvents{AllowAny: true}

func testApp(id string, events *config.ClientEvents) *App {
	return NewApp(config.App{ID: id, ClientEvents: events,
		Limits: config.Limits{Connections: 10, ChannelsPerTicket: 5, RequestBytes: 1024}}, nil)
}

func connect(t *testing.T, a *App, user, session string, channels ...string) (*Conn, *fakeSink) {
	t.Helper()
	sink := &fakeSink{}
	c, err := a.Register(ticket.Ticket{App: a.Config.ID, User: user, Anonymous: user == "", Session: session,
		Channels: channels}, sink, now)
	if err != nil {
		t.Fatal(err)
	}
	return c, sink
}

func join(t *testing.T, a *App, c *Conn, channel string) {
	t.Helper()
	ref := "1"
	if err := a.Join(c, channel, &ref, nil); err != nil {
		t.Fatal(err)
	}
}

// declared lets clients send only typing, with a schema, a 64-byte cap and a rate of 1 a second in bursts of 2.
func declared(t *testing.T) *config.ClientEvents {
	t.Helper()
	e := &config.Event{Schema: []byte(`{"type":"object","properties":{"thread":{"type":"string"}},` +
		`"required":["thread"],"additionalProperties":false}`), MaxBytes: 64, PerSecond: 1, Burst: 2}
	if err := e.Compile(); err != nil {
		t.Fatal(err)
	}
	return &config.ClientEvents{Events: map[string]*config.Event{"typing": e}}
}

func connectWithRules(t *testing.T, a *App, user string, rules ticket.Rules, channels ...string) (*Conn, *fakeSink) {
	t.Helper()
	sink := &fakeSink{}
	c, err := a.Register(ticket.Ticket{App: a.Config.ID, User: user, Session: user + "-session", Channels: channels,
		Rules: rules}, sink, now)
	if err != nil {
		t.Fatal(err)
	}
	return c, sink
}

func joinRelay(t *testing.T, a *App, c *Conn) {
	t.Helper()
	if err := a.JoinRelay(c, nil); err != nil {
		t.Fatal(err)
	}
}
