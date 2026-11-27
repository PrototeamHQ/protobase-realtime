package changes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes/changestest"
)

// bigStart is a registration of about 24 KiB, below what the replayed service reads in one message, so a few
// thousand fill the connection's buffers once the service stops reading and the rest wait in the relay's queue.
func bigStart() Start {
	subs, _ := json.Marshal([]map[string]string{{"resource": string(bytes.Repeat([]byte("a"), 24<<10))}})
	return Start{Subs: subs, Cursor: "c"}
}

// flood registers app's queries until the client refuses one, at most limit of them.
func flood(t *testing.T, c *Client, app string, start Start, limit int) ([]*Registration, error) {
	t.Helper()
	var regs []*Registration
	for range limit {
		r, err := c.Register(app, start, newTarget(t), nil)
		if err != nil {
			return regs, err
		}
		regs = append(regs, r)
	}
	return regs, nil
}

func TestAFloodingAppIsRefusedWhileAnotherKeepsItsChanges(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	s := svc.Session()
	c := run(t, svc)
	mine := newTarget(t)
	mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), mine)
	svc.Play(0, 1)
	svc.Stall()
	regs, err := flood(t, c, "flood", bigStart(), appRegisters+4096)
	if err != ErrBusy || len(regs) < appRegisters {
		t.Fatalf("registered %d, then %v", len(regs), err)
	}
	// Another app still registers, as its register waits in the queue rather than past the flooding app's bound.
	if _, err := c.Register("other", startOf(s.Register(t, 0)), newTarget(t), nil); err != nil {
		t.Fatal(err)
	}
	svc.Play(1, 3)
	mine.change(s, 1, "r1")
	mine.change(s, 2, "r1")
	// Unregisters go out past the bound, and drop nothing.
	for _, r := range regs {
		r.Unregister()
	}
	mine.quiet()
	if n := svc.Connections(); n != 0 {
		t.Fatalf("the relay opened %d more connections", n)
	}
}

func TestAServiceThatStopsReadingStillDropsTheConnection(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	s := svc.Session()
	c := run(t, svc)
	mine := newTarget(t)
	mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), mine)
	svc.Play(0, 1)
	svc.Stall()
	// Large registers of a few apps fill the connection's buffers, then small ones of many apps fill the queue.
	for i := range 4 {
		if _, err := flood(t, c, fmt.Sprintf("big%d", i), bigStart(), appRegisters); err != nil && err != ErrBusy {
			t.Fatal(err)
		}
	}
	for i := range 2 * queueLength / appRegisters {
		if _, err := flood(t, c, fmt.Sprintf("app%d", i), Start{Cursor: "c"}, appRegisters); err != nil &&
			err != ErrBusy {
			t.Fatal(err)
		}
	}
	if got := mine.next(); got != "lost" {
		t.Fatalf("got %s", got)
	}
}

func TestAnAppAtItsBoundStillUnregisters(t *testing.T) {
	c := New("main", "ws://127.0.0.1:1/relay", changestest.Key, func(error) {})
	dropped := false
	c.attach(make(chan queued, queueLength), func() { dropped = true })
	regs, err := flood(t, c, changestest.App, Start{}, appRegisters+1)
	if err != ErrBusy || len(regs) != appRegisters {
		t.Fatalf("registered %d, then %v", len(regs), err)
	}
	if _, err := c.Register("other", Start{}, newTarget(t), nil); err != nil {
		t.Fatal(err)
	}
	for _, r := range regs {
		r.Unregister()
	}
	if dropped || len(c.out) != appRegisters+1+appRegisters {
		t.Fatalf("dropped %v, queued %d", dropped, len(c.out))
	}
}

func TestAReplacementOnAnotherServiceEndsTheOldOneOnlyOnceAccepted(t *testing.T) {
	old := New("old", "ws://127.0.0.1:1/relay", changestest.Key, func(error) {})
	old.attach(make(chan queued, queueLength), func() {})
	r := mustRegister(t, old, changestest.App, Start{}, newTarget(t))
	full := New("full", "ws://127.0.0.1:1/relay", changestest.Key, func(error) {})
	full.attach(make(chan queued, queueLength), func() {})
	if _, err := flood(t, full, changestest.App, Start{}, appRegisters); err != nil {
		t.Fatal(err)
	}
	if _, err := full.Register(changestest.App, Start{}, newTarget(t), r); err != ErrBusy {
		t.Fatalf("got %v", err)
	}
	if old.regs[r.key()] != r || len(old.out) != 1 {
		t.Fatal("a refused replacement ended the old registration")
	}
	accepting := New("accepting", "ws://127.0.0.1:1/relay", changestest.Key, func(error) {})
	if _, err := accepting.Register(changestest.App, Start{}, newTarget(t), r); err != nil {
		t.Fatal(err)
	}
	if old.regs[r.key()] != nil || len(old.out) != 2 {
		t.Fatal("an accepted replacement kept the old registration")
	}
}
