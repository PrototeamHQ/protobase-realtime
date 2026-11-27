package changes

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes/changestest"
)

// target records what a registration's topic is told.
type target struct {
	t      *testing.T
	events chan string
}

func newTarget(t *testing.T) *target { return &target{t: t, events: make(chan string, 16)} }

func (g *target) Change(entry json.RawMessage) { g.events <- "change " + string(entry) }
func (g *target) End(reason string)            { g.events <- "end " + reason }
func (g *target) Lost()                        { g.events <- "lost" }

func (g *target) next() string {
	g.t.Helper()
	select {
	case e := <-g.events:
		return e
	case <-time.After(5 * time.Second):
		g.t.Fatal("the target was told nothing")
		return ""
	}
}

func (g *target) quiet() {
	g.t.Helper()
	select {
	case e := <-g.events:
		g.t.Fatalf("the target was told %s", e)
	case <-time.After(100 * time.Millisecond):
	}
}

// change checks the target gets the recorded entry for the recorded id, as the service shaped it, its cursor included.
func (g *target) change(s changestest.Session, frame int, id string) {
	g.t.Helper()
	got, ok := strings.CutPrefix(g.next(), "change ")
	if !ok {
		g.t.Fatalf("got %s", got)
	}
	gotEntry, want := decodeEntry(g.t, []byte(got)), decodeEntry(g.t, s.Entry(g.t, frame, id))
	gotEntry["id"] = want["id"]
	if !reflect.DeepEqual(gotEntry, want) {
		g.t.Fatalf("got %v\nwant %v", gotEntry, want)
	}
}

func decodeEntry(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func run(t *testing.T, svc *changestest.Service) *Client {
	t.Helper()
	c := New("main", svc.URL, changestest.Key, func(err error) { t.Errorf("stuck: %v", err) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return c
}

// startOf is the start a recorded register asks for.
func startOf(r changestest.Register) Start {
	s := Start{Subs: r.Subs, Fields: r.Fields, Session: r.Sid, Anonymous: r.Anonymous, Cursor: r.Cursor}
	if r.Snapshot != "" {
		s.Snapshot, _ = json.Marshal(r.Snapshot)
	}
	return s
}

func mustRegister(t *testing.T, c *Client, app string, start Start, g *target) *Registration {
	t.Helper()
	r, err := c.Register(app, start, g, nil)
	if err != nil {
		t.Fatal("the client refused a registration")
	}
	return r
}

func TestRoutesEachEntryWithItsCursor(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	s := svc.Session()
	c := run(t, svc)
	g := newTarget(t)
	r := mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), g)
	svc.Play(0, 3)
	g.change(s, 1, "r1")
	g.change(s, 2, "r1")
	r.Unregister()
	svc.Play(3, 4)
	r.Unregister()
	svc.Quiet()
}

func TestResumesFromTheCursorAJoinGives(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	s := svc.Session()
	c := run(t, svc)
	first := newTarget(t)
	r := mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), first)
	svc.Play(0, 3)
	first.change(s, 1, "r1")
	first.change(s, 2, "r1")
	r.Unregister()
	svc.Play(3, 4)
	resumed := newTarget(t)
	r = mustRegister(t, c, changestest.App, startOf(s.Register(t, 4)), resumed)
	svc.Play(4, 6)
	resumed.change(s, 5, "r2")
	r.Unregister()
	svc.Play(6, 7)
	first.quiet()
}

func TestAGapEndsTheRegistration(t *testing.T) {
	svc := changestest.Start(t, "gap")
	c := run(t, svc)
	g := newTarget(t)
	r := mustRegister(t, c, changestest.App, startOf(svc.Session().Register(t, 0)), g)
	svc.Play(0, 2)
	if got := g.next(); got != "end gap" {
		t.Fatalf("got %s", got)
	}
	// The service ended it already, so leaving sends nothing.
	r.Unregister()
	svc.Quiet()
}

func TestARevokedSessionEndsOnlyItsRegistration(t *testing.T) {
	svc := changestest.Start(t, "revoked")
	s := svc.Session()
	c := run(t, svc)
	person, visitor := newTarget(t), newTarget(t)
	mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), person)
	svc.Play(0, 1)
	mustRegister(t, c, changestest.App, startOf(s.Register(t, 1)), visitor)
	svc.Play(1, 5)
	person.change(s, 2, "r1")
	if got := person.next(); got != "end revoked" {
		t.Fatalf("got %s", got)
	}
	visitor.change(s, 2, "visitor")
	visitor.change(s, 4, "visitor")
	person.quiet()
}

func TestARefusalEndsOnlyItsRegistration(t *testing.T) {
	svc := changestest.Start(t, "unregister")
	s := svc.Session()
	c := run(t, svc)
	one, two := newTarget(t), newTarget(t)
	r1 := mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), one)
	svc.Play(0, 1)
	mustRegister(t, c, changestest.App, startOf(s.Register(t, 1)), two)
	svc.Play(1, 3)
	one.change(s, 2, "r1")
	two.change(s, 2, "r2")
	r1.Unregister()
	svc.Play(3, 4)
	refused := newTarget(t)
	mustRegister(t, c, changestest.App, startOf(s.Register(t, 4)), refused)
	svc.Play(4, 7)
	if got := refused.next(); got != "end refused" {
		t.Fatalf("got %s", got)
	}
	two.change(s, 6, "r2")
	one.quiet()
	refused.quiet()
}

func TestADroppedConnectionRegistersAgainFromTheLastCursor(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	s := svc.Session()
	c := run(t, svc)
	g := newTarget(t)
	r := mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), g)
	svc.Play(0, 2)
	g.change(s, 1, "r1")
	svc.Drop()
	if got := g.next(); got != "lost" {
		t.Fatalf("got %s", got)
	}
	// The recording resumes after the first change, as the relay does on its next connection.
	svc.Play(4, 6)
	g.change(s, 5, "r2")
	r.Unregister()
	svc.Play(6, 7)
}

func TestAppsShareOneConnectionAndGetOnlyTheirOwnChanges(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	s := svc.Session()
	c := run(t, svc)
	mine, other := newTarget(t), newTarget(t)
	mustRegister(t, c, changestest.App, startOf(s.Register(t, 0)), mine)
	svc.Play(0, 1)
	mustRegister(t, c, "other", startOf(s.Register(t, 0)), other)
	if got := svc.Next(); got["app"] != "other" || got["type"] != "register" {
		t.Fatalf("got %v", got)
	}
	svc.Play(1, 3)
	mine.change(s, 1, "r1")
	mine.change(s, 2, "r1")
	other.quiet()
	if n := svc.Connections(); n != 0 {
		t.Fatalf("the relay opened %d more connections", n)
	}
}

func TestAStoppedClientTellsItsRegistrationsAndTakesNoMore(t *testing.T) {
	c := New("main", "ws://127.0.0.1:1/relay", changestest.Key, func(error) {})
	g := newTarget(t)
	mustRegister(t, c, changestest.App, Start{}, g)
	c.Close()
	if got := g.next(); got != "lost" {
		t.Fatalf("got %s", got)
	}
	if _, err := c.Register(changestest.App, Start{}, g, nil); err != ErrStopped {
		t.Fatal("a stopped client took a registration")
	}
}

func TestARefusedKeyIsToldWithoutTheURL(t *testing.T) {
	svc := changestest.Start(t, "gap")
	c := New("main", svc.URL, "CAgI", func(error) {})
	_, err := c.connect(context.Background())
	if err == nil || err.Error() != "changes service main: the service answered 401" {
		t.Fatalf("got %v", err)
	}
}

func TestAReconnectRegistersMoreThanTheQueueHoldsWithoutDropping(t *testing.T) {
	c := New("main", "ws://127.0.0.1:1/relay", changestest.Key, func(error) {})
	g := newTarget(t)
	for range queueLength + 1 {
		mustRegister(t, c, changestest.App, Start{}, g)
	}
	dropped := false
	replay := c.attach(make(chan queued, queueLength), func() { dropped = true })
	if dropped || len(replay) != queueLength+1 {
		t.Fatalf("dropped %v, replayed %d", dropped, len(replay))
	}
}
