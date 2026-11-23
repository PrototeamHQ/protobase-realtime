package hub

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

func testConfig(id string, connections int) config.App {
	return config.App{ID: id, Limits: config.Limits{Connections: connections, ChannelsPerTicket: 5, RequestBytes: 1024}}
}

func TestReloadKeepsAnUnchangedAppsSockets(t *testing.T) {
	h := New([]config.App{testConfig("a", 10)}, nil)
	before, _ := h.App("a")
	c, sink := connect(t, before, "u1", "s1", "room")
	join(t, before, c, "room")
	h.Reload([]config.App{testConfig("a", 10)}, nil)
	after, _ := h.App("a")
	if after != before {
		t.Fatal("an unchanged app got a new hub")
	}
	if n := after.Publish(Publish{Channel: "room", Event: "new", Payload: []byte(`{}`)}); n != 1 || sink.closed != "" {
		t.Fatalf("published to %d sockets, closed %q", n, sink.closed)
	}
}

func TestReloadClosesARemovedAppsSocketsAndRefusesNewOnes(t *testing.T) {
	h := New([]config.App{testConfig("a", 10), testConfig("b", 10)}, nil)
	a, _ := h.App("a")
	b, _ := h.App("b")
	_, sinkA := connect(t, a, "u1", "s1")
	_, sinkB := connect(t, b, "u1", "s1")
	h.Reload([]config.App{testConfig("b", 10)}, nil)
	if _, ok := h.App("a"); ok {
		t.Fatal("a removed app is still served")
	}
	if sinkA.closed != "app removed" || sinkB.closed != "" {
		t.Fatalf("a closed %q, b closed %q", sinkA.closed, sinkB.closed)
	}
	if _, err := a.Register(ticket.Ticket{User: "u2", Session: "s2"}, &fakeSink{}, now); err != ErrAppRemoved {
		t.Fatalf("got %v", err)
	}
}

func TestReloadToNoAppsClosesTheLastAppsSockets(t *testing.T) {
	h := New([]config.App{testConfig("a", 10)}, nil)
	a, _ := h.App("a")
	_, sink := connect(t, a, "u1", "s1")
	h.Reload(nil, nil)
	if _, ok := h.App("a"); ok {
		t.Fatal("the last app is still served")
	}
	if sink.closed != "app removed" {
		t.Fatalf("closed %q", sink.closed)
	}
}

func TestReloadAddsANewApp(t *testing.T) {
	h := New([]config.App{testConfig("a", 10)}, nil)
	h.Reload([]config.App{testConfig("a", 10), testConfig("b", 10)}, nil)
	b, ok := h.App("b")
	if !ok {
		t.Fatal("the new app is not served")
	}
	connect(t, b, "u1", "s1")
	if keys, ok := h.TicketKeys("b"); !ok || keys != nil {
		t.Fatalf("got %v %v", keys, ok)
	}
}

func TestReloadAppliesChangedClientEventsToOpenSockets(t *testing.T) {
	h := New([]config.App{testConfig("a", 10)}, nil)
	a, _ := h.App("a")
	sender, _ := connect(t, a, "u1", "s1", "conversations/1")
	other, otherSink := connect(t, a, "u2", "s2", "conversations/1")
	join(t, a, sender, "conversations/1")
	join(t, a, other, "conversations/1")
	if got := a.Relay(sender, "conversations/1", "typing", []byte(`{"thread":"t1"}`), now); got != RefusedNotAllowed {
		t.Fatalf("before: got %q", got)
	}
	changed := testConfig("a", 10)
	changed.ClientEvents = declared(t)
	h.Reload([]config.App{changed}, nil)
	if got := a.Relay(sender, "conversations/1", "typing", []byte(`{"thread":"t1"}`), now); got != "" {
		t.Fatalf("after: got %q", got)
	}
	if len(otherSink.frames) != 1 || a.MaxEventBytes() != 64 {
		t.Fatalf("got %d frames, max %d bytes", len(otherSink.frames), a.MaxEventBytes())
	}
}

func TestReloadAppliesAChangedRateToAnExistingLimiter(t *testing.T) {
	events := declared(t)
	cfg := testConfig("a", 10)
	cfg.ClientEvents = events
	h := New([]config.App{cfg}, nil)
	a, _ := h.App("a")
	sender, _ := connect(t, a, "u1", "s1", "conversations/1")
	join(t, a, sender, "conversations/1")
	send := func(after time.Duration) string {
		return a.Relay(sender, "conversations/1", "typing", []byte(`{"thread":"t1"}`), now.Add(after))
	}
	send(0)
	send(0)
	if got := send(0); got != RefusedRate {
		t.Fatalf("past the burst of 2: got %q", got)
	}
	faster := *events.Channels["conversations"].Events["typing"]
	faster.PerSecond = 10
	cfg.ClientEvents = &config.ClientEvents{Channels: map[string]*config.ChannelEvents{
		"conversations": {Events: map[string]*config.Event{"typing": &faster}}}}
	h.Reload([]config.App{cfg}, nil)
	// SetLimitAt refills up to the event's time at the old rate before it switches, so half a second at the old rate
	// of 1 refills half a send; the new rate of 10 counts from that event on.
	if got := send(500 * time.Millisecond); got != RefusedRate {
		t.Fatalf("half a second on: got %q", got)
	}
	if got := send(600 * time.Millisecond); got != "" {
		t.Fatalf("a tenth of a second at the new rate: got %q", got)
	}
}

func TestReloadAppliesALowerConnectionLimitToNewSocketsOnly(t *testing.T) {
	h := New([]config.App{testConfig("a", 3)}, nil)
	a, _ := h.App("a")
	first, sink1 := connect(t, a, "u1", "s1")
	_, sink2 := connect(t, a, "u2", "s2")
	h.Reload([]config.App{testConfig("a", 1)}, nil)
	if sink1.closed != "" || sink2.closed != "" || a.Connections() != 2 {
		t.Fatal("a lower limit closed an open socket")
	}
	if _, err := a.Register(ticket.Ticket{User: "u3", Session: "s3"}, &fakeSink{}, now); err != ErrFull {
		t.Fatalf("got %v", err)
	}
	a.Unregister(first)
	if _, err := a.Register(ticket.Ticket{User: "u3", Session: "s3"}, &fakeSink{}, now); err != ErrFull {
		t.Fatalf("at the new limit: got %v", err)
	}
}

func TestReloadTellsNewPresenceWhoIsOnline(t *testing.T) {
	h := New([]config.App{testConfig("a", 10)}, nil)
	a, _ := h.App("a")
	connect(t, a, "u1", "s1")
	connect(t, a, "", "anon")
	log := &presenceLog{}
	h.Reload([]config.App{testConfig("a", 10)}, map[string]Presence{"a": log})
	h.Reload([]config.App{testConfig("a", 10)}, map[string]Presence{"a": log})
	if fmt.Sprint(*log) != "[u1 1/0]" {
		t.Fatalf("got %v", *log)
	}
	h.Reload([]config.App{testConfig("a", 10)}, nil)
	connect(t, a, "u2", "s2")
	if fmt.Sprint(*log) != "[u1 1/0]" {
		t.Fatalf("presence turned off still heard %v", *log)
	}
}

// lockedSink is a sink sockets on several goroutines may close at once.
type lockedSink struct {
	mu     sync.Mutex
	closed bool
}

func (s *lockedSink) Send([]byte) {}
func (s *lockedSink) Close(string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}
func (s *lockedSink) Restart() { s.Close("") }

// TestReloadRacesWithSockets is for the race detector: sockets connect, publish and leave while the table swaps.
func TestReloadRacesWithSockets(t *testing.T) {
	h := New([]config.App{testConfig("a", 100)}, nil)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			for j := range 50 {
				a, ok := h.App("a")
				if !ok {
					continue
				}
				session := fmt.Sprintf("s%d-%d", i, j)
				c, err := a.Register(ticket.Ticket{User: "u", Session: session, Channels: []string{"room"}},
					&lockedSink{}, now)
				if err != nil {
					continue
				}
				_ = a.Join(c, "room", nil, nil)
				a.Publish(Publish{Channel: "room", Event: "new", Payload: []byte(`{}`)})
				a.Unregister(c)
			}
		})
	}
	for i := range 50 {
		apps := []config.App{testConfig("a", 100+i)}
		if i%2 == 1 {
			apps = append(apps, testConfig("b", 10))
		}
		h.Reload(apps, map[string]Presence{})
	}
	wg.Wait()
}
