package tests

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestADirectEventRingsEveryDeviceOfAMemberAndNeverABlocker(t *testing.T) {
	srv := start(t)
	caller := connect(t, srv, appA, person{"u1", "s1", []string{"room"}})
	phone := connect(t, srv, appA, person{"u2", "s2", []string{"room"}})
	laptop := connect(t, srv, appA, person{"u2", "s3", []string{"room"}})
	blocker := dial(t, srv, issueWith(t, appA, person{"u3", "s4", []string{"room"}}, jwt.MapClaims{"blocked": []string{"u1"}}))
	stranger := connect(t, srv, appA, person{"u4", "s5", []string{"other"}})
	for _, c := range []*client{caller, phone, laptop, blocker, stranger} {
		c.join("relay")
	}
	caller.send(`["1","2","relay","ring",{"to":"u2","payload":{"call":"c1"}}]`)
	if got := caller.read(); !strings.Contains(got, `"status":"ok"`) {
		t.Fatalf("caller got %s", got)
	}
	for _, device := range []*client{phone, laptop} {
		got := device.read()
		if !strings.HasPrefix(got, `[null,null,"relay","client_event",{"from":{"user":"u1","connection":"`) ||
			!strings.Contains(got, `"to":"u2","at":`) || !strings.HasSuffix(got, `,"event":"ring","payload":{"call":"c1"}}]`) {
			t.Fatalf("device got %s", got)
		}
	}
	caller.send(`["1","3","relay","ring",{"to":"u3","payload":{}}]`)
	if got := caller.read(); !strings.Contains(got, `"status":"ok"`) {
		t.Fatalf("the caller learned of the block: %s", got)
	}
	if got := blocker.read(); got != "" {
		t.Fatalf("the blocker got %s", got)
	}
	caller.send(`["1","4","relay","ring",{"to":"u4","payload":{}}]`)
	if got := caller.read(); !strings.Contains(got, `"reason":"unreachable"`) {
		t.Fatalf("a stranger was reachable: %s", got)
	}
	if got := stranger.read(); got != "" {
		t.Fatalf("the stranger got %s", got)
	}
}
