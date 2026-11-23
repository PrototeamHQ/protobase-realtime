package hub

import (
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

func TestPublishReachesJoinedMembersOnly(t *testing.T) {
	a := testApp("a", nil)
	member, memberSink := connect(t, a, "u1", "s1", "room")
	_, unjoinedSink := connect(t, a, "u2", "s2", "room")
	outsider, outsiderSink := connect(t, a, "u3", "s3", "other")
	join(t, a, member, "room")
	join(t, a, outsider, "other")
	if n := a.Publish(Publish{Channel: "room", Event: "new", Payload: []byte(`{"id":1}`)}); n != 1 {
		t.Fatalf("reached %d", n)
	}
	if len(memberSink.frames) != 1 || len(unjoinedSink.frames) != 0 || len(outsiderSink.frames) != 0 {
		t.Fatal("wrong recipients")
	}
	if memberSink.frames[0] != `[null,null,"room","new",{"id":1}]` {
		t.Fatalf("got %s", memberSink.frames[0])
	}
}

func TestPublishToNobodyConnectedReachesNobody(t *testing.T) {
	if n := testApp("a", nil).Publish(Publish{Channel: "room", Event: "new"}); n != 0 {
		t.Fatalf("reached %d", n)
	}
}

func TestPublishNarrowsToUsers(t *testing.T) {
	a := testApp("a", nil)
	c1, s1 := connect(t, a, "u1", "s1", "room")
	c2, s2 := connect(t, a, "u2", "s2", "room")
	join(t, a, c1, "room")
	join(t, a, c2, "room")
	a.Publish(Publish{Channel: "room", Users: []string{"u2"}, Event: "new"})
	if len(s1.frames) != 0 || len(s2.frames) != 1 {
		t.Fatal("wrong recipients")
	}
}

func TestInterestFiltersButNeverGrants(t *testing.T) {
	a := testApp("a", nil)
	viewer, viewerSink := connect(t, a, "u1", "s1", "room")
	elsewhere, elsewhereSink := connect(t, a, "u2", "s2", "room")
	silent, silentSink := connect(t, a, "u3", "s3", "room")
	ref := "1"
	_ = a.Join(viewer, "room", &ref, []string{"thread:1"})
	_ = a.Join(elsewhere, "room", &ref, []string{"thread:2"})
	join(t, a, silent, "room")
	a.Publish(Publish{Channel: "room", Event: "new", About: "thread:1"})
	if len(viewerSink.frames) != 1 || len(elsewhereSink.frames) != 0 || len(silentSink.frames) != 1 {
		t.Fatal("interest filtered wrongly")
	}
	a.Publish(Publish{Channel: "room", Event: "new"})
	if len(elsewhereSink.frames) != 1 {
		t.Fatal("a publish about nothing was filtered")
	}
	if err := a.SetInterest(viewer, "other", []string{"x"}); err != ErrNotMember {
		t.Fatal("interest in a channel not joined accepted")
	}
	_ = a.SetInterest(elsewhere, "room", []string{"thread:1"})
	a.Publish(Publish{Channel: "room", Event: "new", About: "thread:1"})
	if len(elsewhereSink.frames) != 2 {
		t.Fatal("changed interest not applied")
	}
}

func TestCloseSessionClosesOnlyThatSession(t *testing.T) {
	a := testApp("a", nil)
	_, phone := connect(t, a, "u1", "s1")
	_, laptop := connect(t, a, "u1", "s1")
	_, other := connect(t, a, "u1", "s2")
	if n := a.CloseSession("s1", now); n != 2 {
		t.Fatalf("closed %d", n)
	}
	if phone.closed == "" || laptop.closed == "" || other.closed != "" {
		t.Fatal("wrong sockets closed")
	}
	if n := a.CloseSession("gone", now); n != 0 {
		t.Fatalf("closed %d", n)
	}
}

func TestAClosedSessionCannotReopenWithinATicketsLifetime(t *testing.T) {
	a := testApp("a", nil)
	a.CloseSession("s1", now)
	if _, err := a.Register(ticket.Ticket{User: "u1", Session: "s1"}, &fakeSink{}, now.Add(time.Minute)); err != ErrSessionClosed {
		t.Fatalf("got %v", err)
	}
	if _, err := a.Register(ticket.Ticket{User: "u1", Session: "s1"}, &fakeSink{}, now.Add(ticket.MaxLifetime)); err != nil {
		t.Fatalf("after the lifetime: %v", err)
	}
}

func TestASocketRegisteringWhileItsSessionClosesIsRefused(t *testing.T) {
	a := testApp("a", nil)
	// Hold the app's mutex so Register waits for it, then close the session in that gap as CloseSession does.
	a.mu.Lock()
	result := make(chan error)
	go func() {
		_, err := a.Register(ticket.Ticket{User: "u1", Session: "s1"}, &fakeSink{}, now)
		result <- err
	}()
	time.Sleep(20 * time.Millisecond)
	a.closed.First("s1", now)
	a.mu.Unlock()
	if err := <-result; err != ErrSessionClosed {
		t.Fatalf("got %v", err)
	}
}
