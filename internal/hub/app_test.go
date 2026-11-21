package hub

import (
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

func TestRegisterEnforcesTheConnectionLimit(t *testing.T) {
	a := NewApp(config.App{ID: "a", Limits: config.Limits{Connections: 1, ChannelsPerTicket: 1, RequestBytes: 1}}, nil)
	c, _ := connect(t, a, "u1", "s1")
	if _, err := a.Register(ticket.Ticket{User: "u2", Session: "s2"}, &fakeSink{}, now); err != ErrFull {
		t.Fatalf("got %v", err)
	}
	a.Unregister(c)
	connect(t, a, "u2", "s2")
}

func TestRegisterEnforcesTheChannelLimit(t *testing.T) {
	a := testApp("a", nil)
	_, err := a.Register(ticket.Ticket{User: "u", Session: "s", Channels: []string{"1", "2", "3", "4", "5", "6"}}, &fakeSink{}, now)
	if err != ErrTooManyChannels {
		t.Fatalf("got %v", err)
	}
}

func TestUnregisterDropsEveryIndex(t *testing.T) {
	a := testApp("a", nil)
	c, _ := connect(t, a, "u1", "s1", "room")
	join(t, a, c, "room")
	a.Unregister(c)
	a.Unregister(c)
	if a.Join(c, "room", nil, nil) != ErrNotMember {
		t.Fatal("a closed socket joined")
	}
	if a.Connections() != 0 || len(a.byChannel) != 0 || len(a.byUser) != 0 || len(a.bySession) != 0 || len(a.people) != 0 {
		t.Fatalf("left %+v", a)
	}
}

func TestAppsWithEqualChannelNamesStayApart(t *testing.T) {
	h := New([]config.App{testApp("a", nil).Config, testApp("b", nil).Config}, nil)
	a, _ := h.App("a")
	b, _ := h.App("b")
	ca, sinkA := connect(t, a, "u1", "s1", "room")
	cb, sinkB := connect(t, b, "u1", "s1", "room")
	join(t, a, ca, "room")
	join(t, b, cb, "room")
	a.Publish(Publish{Channel: "room", Event: "new", Payload: []byte(`{}`)})
	if len(sinkA.frames) != 1 || len(sinkB.frames) != 0 {
		t.Fatalf("a got %v, b got %v", sinkA.frames, sinkB.frames)
	}
	b.CloseSession("s1", now)
	if sinkA.closed != "" {
		t.Fatal("closing a session in b closed a's socket")
	}
}

func TestCloseAllRestartsEverySocket(t *testing.T) {
	h := New([]config.App{testApp("a", nil).Config, testApp("b", nil).Config}, nil)
	a, _ := h.App("a")
	b, _ := h.App("b")
	_, sa := connect(t, a, "u1", "s1")
	_, sb := connect(t, b, "", "s2")
	h.CloseAll()
	if sa.closed != "restart" || sb.closed != "restart" {
		t.Fatal("a socket was not restarted")
	}
}
