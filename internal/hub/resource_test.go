package hub

import (
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
)

// joinAll connects u1 and u2 and joins both to every channel.
func joinAll(t *testing.T, a *App, channels ...string) (*Conn, *fakeSink) {
	t.Helper()
	sender, _ := connect(t, a, "u1", "s1", channels...)
	other, otherSink := connect(t, a, "u2", "s2", channels...)
	for _, channel := range channels {
		join(t, a, sender, channel)
		join(t, a, other, channel)
	}
	return sender, otherSink
}

func TestChannelEventIsAllowedOnlyOnItsResource(t *testing.T) {
	a := testApp("a", declared(t))
	sender, otherSink := joinAll(t, a, "conversations/1", "rooms/1", "conversations")
	payload := []byte(`{"thread":"t1"}`)
	cases := map[string]string{"conversations/1": "", "rooms/1": RefusedNotAllowed, "conversations": RefusedNotAllowed}
	for channel, want := range cases {
		if got := a.Relay(sender, channel, "typing", payload, at); got != want {
			t.Errorf("%s: got %q, want %q", channel, got, want)
		}
	}
	if len(otherSink.frames) != 1 {
		t.Fatalf("delivered %d", len(otherSink.frames))
	}
}

func TestChannelWithoutResourceAllowsOnlyWithAllowAny(t *testing.T) {
	strict := testApp("a", declared(t))
	sender, _ := joinAll(t, strict, "lobby")
	if got := strict.Relay(sender, "lobby", "typing", []byte(`{"thread":"t1"}`), at); got != RefusedNotAllowed {
		t.Fatalf("strict: got %q", got)
	}
	loose := declared(t)
	loose.AllowAny = true
	open := testApp("b", loose)
	sender, otherSink := joinAll(t, open, "lobby")
	if got := open.Relay(sender, "lobby", "typing", []byte(`{"any":"shape"}`), at); got != "" || len(otherSink.frames) != 1 {
		t.Fatalf("allow_any: got %q", got)
	}
}

func TestChannelEventRateAndSchemaArePerResource(t *testing.T) {
	rooms := &config.Event{Schema: []byte(`{"type":"object","required":["room"]}`), MaxBytes: 64, PerSecond: 1, Burst: 1}
	if err := rooms.Compile(); err != nil {
		t.Fatal(err)
	}
	events := declared(t)
	events.Channels["rooms"] = &config.ChannelEvents{Events: map[string]*config.Event{"typing": rooms}}
	a := testApp("a", events)
	sender, _ := joinAll(t, a, "conversations/1", "rooms/1")
	if got := a.Relay(sender, "rooms/1", "typing", []byte(`{"thread":"t1"}`), at); got != RefusedPayload {
		t.Fatalf("the conversations schema checked a room: %q", got)
	}
	// The room's burst of 1 is spent; conversations keep their own burst of 2.
	if got := a.Relay(sender, "rooms/1", "typing", []byte(`{"room":"r1"}`), at); got != RefusedRate {
		t.Fatalf("rooms rate: %q", got)
	}
	for range 2 {
		if got := a.Relay(sender, "conversations/1", "typing", []byte(`{"thread":"t1"}`), at); got != "" {
			t.Fatalf("conversations rate: %q", got)
		}
	}
}

func TestDirectEventsUseTheAppsEventsNotTheChannels(t *testing.T) {
	events := declared(t)
	events.Events = map[string]*config.Event{"ring": typingEvent(t)}
	a := testApp("a", events)
	sender, _ := connect(t, a, "u1", "s1", "conversations/1")
	other, otherSink := connect(t, a, "u2", "s2", "conversations/1")
	joinRelay(t, a, sender)
	joinRelay(t, a, other)
	join(t, a, sender, "conversations/1")
	payload := []byte(`{"thread":"t1"}`)
	if got := a.Direct(sender, "u2", "typing", payload, at); got != RefusedNotAllowed {
		t.Fatalf("a channel event went direct: %q", got)
	}
	if got := a.Relay(sender, "conversations/1", "ring", payload, at); got != RefusedNotAllowed {
		t.Fatalf("a direct event went on a channel: %q", got)
	}
	if got := a.Direct(sender, "u2", "ring", payload, at); got != "" || len(otherSink.frames) != 1 {
		t.Fatalf("direct: %q", got)
	}
}

func TestMaxEventBytesCoversChannelEvents(t *testing.T) {
	if got := testApp("a", declared(t)).MaxEventBytes(); got != 64 {
		t.Fatalf("got %d", got)
	}
}
