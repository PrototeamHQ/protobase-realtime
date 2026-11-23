package presence

import (
	"fmt"
	"testing"
	"time"
)

const grace = 30 * time.Second

type clock struct{ t time.Time }

func (c *clock) now() time.Time               { return c.t }
func (c *clock) pass(d time.Duration)         { c.t = c.t.Add(d) }
func newTracker(c *clock, limit int) *Tracker { return NewTracker(grace, Debounce, limit, c.now) }

func statuses(changes []Change) string {
	out := ""
	for _, c := range changes {
		out += fmt.Sprintf("%s:%s ", c.User, c.Status)
	}
	return out
}

// settle reports everything due and acknowledges it, as a working app does.
func settle(t *Tracker) string {
	due := t.Due(BatchSize)
	t.Acknowledge(due)
	return statuses(due)
}

func TestOnlineIsReportedOnceItHeldForTheDebounce(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 10)
	tr.Change("u1", 1, 0)
	if got := settle(tr); got != "" {
		t.Fatalf("reported before the debounce: %s", got)
	}
	c.pass(Debounce)
	if got := settle(tr); got != "u1:online " {
		t.Fatalf("got %q", got)
	}
	if got := settle(tr); got != "" {
		t.Fatalf("reported twice: %s", got)
	}
}

func TestAPhoneFlappingWithinTheGraceIsNeverReportedOffline(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 10)
	tr.Change("u1", 1, 0)
	c.pass(Debounce)
	settle(tr)
	for range 5 {
		tr.Change("u1", 0, 0)
		c.pass(grace - time.Second)
		if got := settle(tr); got != "" {
			t.Fatalf("got %q", got)
		}
		tr.Change("u1", 1, 0)
	}
	tr.Change("u1", 0, 0)
	c.pass(grace)
	if got := settle(tr); got != "" {
		t.Fatalf("offline reported before the debounce: %q", got)
	}
	c.pass(Debounce)
	due := tr.Due(BatchSize)
	if statuses(due) != "u1:offline " || due[0].At != c.t.Add(-Debounce).UnixMilli() {
		t.Fatalf("got %+v", due)
	}
	tr.Acknowledge(due)
	if len(tr.people) != 0 {
		t.Fatal("an offline person was kept")
	}
}

func TestOneDeviceOnlineAndOneAwayIsOnline(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 10)
	tr.Change("u1", 1, 1)
	c.pass(Debounce)
	if got := settle(tr); got != "u1:online " {
		t.Fatalf("got %q", got)
	}
	tr.Change("u1", 0, 2)
	c.pass(Debounce)
	if got := settle(tr); got != "u1:away " {
		t.Fatalf("got %q", got)
	}
}

func TestAQuickAwayAndBackIsNotReported(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 10)
	tr.Change("u1", 1, 0)
	c.pass(Debounce)
	settle(tr)
	tr.Change("u1", 0, 1)
	c.pass(Debounce / 2)
	tr.Change("u1", 1, 0)
	c.pass(Debounce)
	if got := settle(tr); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestUnacknowledgedChangesAreReportedAgainInBatches(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 10)
	for i := range 3 {
		tr.Change(fmt.Sprintf("u%d", i), 1, 0)
		c.pass(time.Millisecond)
	}
	c.pass(Debounce)
	if got := statuses(tr.Due(2)); got != "u0:online u1:online " {
		t.Fatalf("got %q", got)
	}
	if got := settle(tr); got != "u0:online u1:online u2:online " {
		t.Fatalf("a failed report was lost: %q", got)
	}
}

func TestAChangeDuringAReportIsReportedNext(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 10)
	tr.Change("u1", 1, 0)
	c.pass(Debounce)
	sending := tr.Due(BatchSize)
	tr.Change("u1", 0, 1)
	tr.Acknowledge(sending)
	c.pass(Debounce)
	if got := settle(tr); got != "u1:away " {
		t.Fatalf("got %q", got)
	}
}

func TestTheTrackerDropsWhoLeftFirstWhenFull(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 1)
	for _, user := range []string{"u1", "u2", "u3"} {
		tr.Change(user, 1, 0)
		c.pass(time.Second)
		tr.Change(user, 0, 0)
	}
	if len(tr.people) != 2 || tr.people["u1"] != nil {
		t.Fatalf("kept %v", tr.people)
	}
}
