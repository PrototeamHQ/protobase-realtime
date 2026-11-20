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

func testApp(id string, clientMessages bool) *App {
	return NewApp(config.App{ID: id, ClientMessages: clientMessages,
		Limits: config.Limits{Connections: 10, ChannelsPerTicket: 5, RequestBytes: 1024}})
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
