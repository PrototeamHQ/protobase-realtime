package socket

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/changes/changestest"
)

// idle is the target of registrations a test only makes to fill the app's bound.
type idle struct{}

func (idle) Change(json.RawMessage) {}
func (idle) End(string)             {}
func (idle) Lost()                  {}

func TestAJoinPastTheAppsWaitingRegistrationsIsRateLimited(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	rec := svc.Session()
	client := runClient(t, svc)
	s := routedSession(t, client, "u1", "s-r1", beforeExpiry)
	token := tokenOf(t, rec.Register(t, 0), "u1")
	exchange(t, s, joinFrame("live:q1", token, ""))
	svc.Play(0, 1)
	svc.Stall()
	// Other sockets of the app fill its bound with registrations of about 24 KiB, which also fill the connection's
	// buffers, while the service reads nothing.
	subs, _ := json.Marshal([]map[string]string{{"resource": string(bytes.Repeat([]byte("a"), 24<<10))}})
	big := changes.Start{Subs: subs, Cursor: "c"}
	for range 8192 {
		if _, err := client.Register(changestest.App, big, idle{}, nil); err != nil {
			break
		}
	}
	want := `["1","1","live:q1","phx_reply",{"response":{"reason":"rate_limited"},"status":"error"}]`
	if got := exchange(t, s, joinFrame("live:q1", token, "")); got != want {
		t.Fatalf("got %s", got)
	}
	// The joined topic keeps its registration and its changes.
	svc.Play(1, 3)
	expectChange(t, s, "live:q1", rec, 1, "r1")
	expectChange(t, s, "live:q1", rec, 2, "r1")
}
