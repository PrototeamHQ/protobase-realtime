package hub

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var at = time.UnixMilli(1_800_000_000_123)

func TestRelayStampsTheVerifiedSender(t *testing.T) {
	a := testApp("a", true)
	sender, senderSink := connect(t, a, "u1", "s1", "room")
	sibling, siblingSink := connect(t, a, "u1", "s1", "room")
	other, otherSink := connect(t, a, "u2", "s2", "room")
	for _, c := range []*Conn{sender, sibling, other} {
		join(t, a, c, "room")
	}
	payload := `{"from":{"user":"admin"},"channel":"secret","at":1,"text":"hi"}`
	if code := a.Relay(sender, "room", "typing", []byte(payload), at); code != "" {
		t.Fatalf("refused: %s", code)
	}
	if len(senderSink.frames) != 0 || len(siblingSink.frames) != 1 || len(otherSink.frames) != 1 {
		t.Fatal("wrong recipients")
	}
	var frame []json.RawMessage
	_ = json.Unmarshal([]byte(otherSink.frames[0]), &frame)
	var env envelope
	_ = json.Unmarshal(frame[4], &env)
	if string(frame[3]) != `"client_message"` || env.From.User != "u1" || env.From.Connection != sender.ID ||
		env.Channel != "room" || env.At != at.UnixMilli() || env.Event != "typing" || string(env.Payload) != payload {
		t.Fatalf("got %s", otherSink.frames[0])
	}
	if strings.Contains(otherSink.frames[0], "s1") {
		t.Fatal("the sign-in session leaked")
	}
}

func TestRelayRefuses(t *testing.T) {
	on := testApp("a", true)
	off := testApp("b", false)
	member, _ := connect(t, on, "u1", "s1", "room")
	join(t, on, member, "room")
	anon, _ := connect(t, on, "", "anon", "room")
	join(t, on, anon, "room")
	offMember, _ := connect(t, off, "u1", "s1", "room")
	join(t, off, offMember, "room")
	big := []byte(`{"x":"` + strings.Repeat("a", ClientMessageBytes) + `"}`)
	cases := []struct {
		app     *App
		c       *Conn
		channel string
		event   string
		payload []byte
		want    string
	}{
		{on, anon, "room", "typing", []byte(`{}`), RefusedReceiveOnly},
		{off, offMember, "room", "typing", []byte(`{}`), RefusedDisabled},
		{on, member, "other", "typing", []byte(`{}`), RefusedNotJoined},
		{on, member, "room", "phx_join", []byte(`{}`), RefusedEvent},
		{on, member, "room", "client_message", []byte(`{}`), RefusedEvent},
		{on, member, "room", "typing", big, RefusedTooLarge},
	}
	for _, tc := range cases {
		if got := tc.app.Relay(tc.c, tc.channel, tc.event, tc.payload, at); got != tc.want {
			t.Errorf("%s/%s: got %q, want %q", tc.channel, tc.event, got, tc.want)
		}
	}
}

func TestRelayRateIsPerPersonAcrossSockets(t *testing.T) {
	a := testApp("a", true)
	first, _ := connect(t, a, "u1", "s1", "room")
	second, _ := connect(t, a, "u1", "s2", "room")
	join(t, a, first, "room")
	join(t, a, second, "room")
	sent := 0
	for i := 0; i < 2*clientBurst; i++ {
		c := first
		if i%2 == 1 {
			c = second
		}
		if a.Relay(c, "room", "typing", []byte(`{}`), at) == "" {
			sent++
		}
	}
	if sent != clientBurst {
		t.Fatalf("sent %d, want %d", sent, clientBurst)
	}
}
