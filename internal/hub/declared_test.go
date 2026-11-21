package hub

import (
	"strings"
	"testing"
	"time"
)

func TestDeclaredEventsAreCheckedAndRefusalsReachNobody(t *testing.T) {
	a := testApp("a", declared(t))
	sender, _ := connect(t, a, "u1", "s1", "room")
	other, otherSink := connect(t, a, "u2", "s2", "room")
	join(t, a, sender, "room")
	join(t, a, other, "room")
	cases := []struct {
		event, payload, want string
	}{
		{"typing", `{"thread":"t1","extra":1}`, RefusedPayload},
		{"typing", `{"thread":7}`, RefusedPayload},
		{"typing", `{"thread":"` + strings.Repeat("a", 64) + `"}`, RefusedTooLarge},
		{"offer", `{}`, RefusedNotAllowed},
	}
	for _, tc := range cases {
		if got := a.Relay(sender, "room", tc.event, []byte(tc.payload), at); got != tc.want {
			t.Errorf("%s %s: got %q, want %q", tc.event, tc.payload, got, tc.want)
		}
	}
	if len(otherSink.frames) != 0 {
		t.Fatalf("a refused event arrived: %v", otherSink.frames)
	}
	// The refused sends spent the burst, so the valid one waits for the rate to refill.
	later := at.Add(10 * time.Second)
	if got := a.Relay(sender, "room", "typing", []byte(`{"thread":"t1"}`), later); got != "" || len(otherSink.frames) != 1 {
		t.Fatalf("a valid event: %q, %d frames", got, len(otherSink.frames))
	}
}

func TestDeclaredEventRateIsPerPersonAndRefillsOverTime(t *testing.T) {
	a := testApp("a", declared(t))
	sender, _ := connect(t, a, "u1", "s1", "room")
	other, otherSink := connect(t, a, "u2", "s2", "room")
	join(t, a, sender, "room")
	join(t, a, other, "room")
	payload := []byte(`{"thread":"t1"}`)
	got := []string{}
	for _, when := range []time.Time{at, at, at, at.Add(time.Second)} {
		got = append(got, a.Relay(sender, "room", "typing", payload, when))
	}
	if strings.Join(got, ",") != ",,"+RefusedRate+"," {
		t.Fatalf("got %q", got)
	}
	if len(otherSink.frames) != 3 {
		t.Fatalf("delivered %d", len(otherSink.frames))
	}
	if a.Relay(other, "room", "typing", payload, at) != "" {
		t.Fatal("one person's sends limited another")
	}
}

func TestBlockedSendersChannelEventsNeverReachTheBlocker(t *testing.T) {
	a := testApp("a", allowAny)
	sender, _ := connect(t, a, "u1", "s1", "room")
	blocker, blockerSink := connectWithRules(t, a, "u2", blockU1, "room")
	bystander, bystanderSink := connect(t, a, "u3", "s3", "room")
	for _, c := range []*Conn{sender, blocker, bystander} {
		join(t, a, c, "room")
	}
	if got := a.Relay(sender, "room", "typing", []byte(`{}`), at); got != "" {
		t.Fatalf("refused: %s", got)
	}
	if len(blockerSink.frames) != 0 || len(bystanderSink.frames) != 1 {
		t.Fatal("wrong recipients")
	}
}
