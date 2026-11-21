package hub

import (
	"encoding/json"
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

var blockU1 = ticket.Rules{Blocked: []string{"u1"}}

func TestDirectEventArrivesWithoutJoiningTheSharedChannel(t *testing.T) {
	a := testApp("a", allowAny)
	sender, _ := connect(t, a, "u1", "s1", "room", "lobby")
	phone, phoneSink := connect(t, a, "u2", "s2", "room")
	laptop, laptopSink := connect(t, a, "u2", "s3", "room")
	joinRelay(t, a, sender)
	joinRelay(t, a, phone)
	joinRelay(t, a, laptop)
	if got := a.Direct(sender, "u2", "ring", []byte(`{"call":1}`), at); got != "" {
		t.Fatalf("refused: %s", got)
	}
	if len(phoneSink.frames) != 1 || len(laptopSink.frames) != 1 {
		t.Fatal("not every device rang")
	}
	var frame []json.RawMessage
	_ = json.Unmarshal([]byte(phoneSink.frames[0]), &frame)
	var env envelope
	_ = json.Unmarshal(frame[4], &env)
	if string(frame[2]) != `"relay"` || env.From.User != "u1" || env.To != "u2" || env.Channel != "" ||
		env.Event != "ring" || string(env.Payload) != `{"call":1}` {
		t.Fatalf("got %s", phoneSink.frames[0])
	}
}

func TestDirectEventNeedsASharedChannel(t *testing.T) {
	a := testApp("a", allowAny)
	sender, _ := connect(t, a, "u1", "s1", "room")
	stranger, strangerSink := connect(t, a, "u2", "s2", "other")
	joinRelay(t, a, sender)
	joinRelay(t, a, stranger)
	for _, to := range []string{"u2", "nobody"} {
		if got := a.Direct(sender, to, "ring", []byte(`{}`), at); got != RefusedUnreachable {
			t.Errorf("to %s: got %q", to, got)
		}
	}
	if len(strangerSink.frames) != 0 {
		t.Fatal("a stranger was reached")
	}
}

func TestDirectEventRulesDropSilently(t *testing.T) {
	a := testApp("a", allowAny)
	sender, _ := connect(t, a, "u1", "s1", "room")
	blocker, blockerSink := connectWithRules(t, a, "u2", blockU1, "room")
	picky, pickySink := connectWithRules(t, a, "u3", ticket.Rules{FriendsOnly: true, Friends: []string{"u9"}}, "room")
	friendly, friendlySink := connectWithRules(t, a, "u4", ticket.Rules{FriendsOnly: true, Friends: []string{"u1"}}, "room")
	for _, c := range []*Conn{sender, blocker, picky, friendly} {
		joinRelay(t, a, c)
	}
	for _, to := range []string{"u2", "u3", "u4"} {
		if got := a.Direct(sender, to, "ring", []byte(`{}`), at); got != "" {
			t.Fatalf("to %s: the sender learned of a rule: %q", to, got)
		}
	}
	if len(blockerSink.frames) != 0 || len(pickySink.frames) != 0 || len(friendlySink.frames) != 1 {
		t.Fatal("wrong recipients")
	}
}

func TestSetRulesReplacesTheRulesOfOpenSockets(t *testing.T) {
	a := testApp("a", allowAny)
	sender, _ := connect(t, a, "u1", "s1", "room")
	recipient, sink := connect(t, a, "u2", "s2", "room")
	joinRelay(t, a, sender)
	joinRelay(t, a, recipient)
	if n := a.SetRules("u2", blockU1); n != 1 {
		t.Fatalf("reached %d", n)
	}
	a.Direct(sender, "u2", "ring", []byte(`{}`), at)
	a.SetRules("u2", ticket.Rules{})
	a.Direct(sender, "u2", "ring", []byte(`{}`), at)
	if len(sink.frames) != 1 {
		t.Fatalf("got %d", len(sink.frames))
	}
	if a.SetRules("nobody", blockU1) != 0 {
		t.Fatal("rules kept for someone with no socket")
	}
}

func TestAnonymousSessionsHaveNoRelayTopic(t *testing.T) {
	a := testApp("a", allowAny)
	anon, _ := connect(t, a, "", "anon", "room")
	if a.JoinRelay(anon, nil) != ErrReceiveOnly {
		t.Fatal("an anonymous session joined the relay topic")
	}
	if got := a.Direct(anon, "u1", "ring", []byte(`{}`), at); got != RefusedReceiveOnly {
		t.Fatalf("got %q", got)
	}
}
