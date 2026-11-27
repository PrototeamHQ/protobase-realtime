package tests

import (
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes/changestest"
)

func TestALiveQueryGetsEachChangeWithItsCursorUntilItLeaves(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	rec := svc.Session()
	srv := startLive(t, svc)
	c := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	c.joinLive("live:messages", liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:messages"))
	svc.Play(0, 3)
	c.change("live:messages", rec, 1, "r1")
	c.change("live:messages", rec, 2, "r1")
	c.send(`["1","2","live:messages","phx_leave",{}]`)
	if got := c.next(); got != `["1","2","live:messages","phx_reply",{"response":{},"status":"ok"}]` {
		t.Fatalf("got %s", got)
	}
	svc.Play(3, 4)
}

func TestAJoinResumesAfterTheCursorItGives(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	srv := startLive(t, svc)
	c := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	token := liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour())
	c.joinLive("live:a", token, "", joined("live:a"))
	svc.Play(0, 3)
	c.change("live:a", rec, 1, "r1")
	c.change("live:a", rec, 2, "r1")
	c.send(`["1","2","live:a","phx_leave",{}]`)
	c.next()
	svc.Play(3, 4)
	c.joinLive("live:b", token, rec.Register(t, 4).Cursor, joined("live:b"))
	svc.Play(4, 6)
	c.change("live:b", rec, 5, "r2")
}

func TestOnlyTheSocketWhoseRegistrationMatchedGetsTheChange(t *testing.T) {
	svc := changestest.Start(t, "unregister")
	rec := svc.Session()
	srv := startLive(t, svc)
	one := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	two := connect(t, srv, shop, person{user: "u2", session: "s-r2"})
	one.joinLive("live:q", liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	svc.Play(0, 1)
	two.joinLive("live:q", liveToken(t, shop, rec.Register(t, 1), "u2", inAnHour()), "", joined("live:q"))
	svc.Play(1, 3)
	one.change("live:q", rec, 2, "r1")
	two.change("live:q", rec, 2, "r2")
	one.send(`["1","2","live:q","phx_leave",{}]`)
	one.next()
	svc.Play(3, 4)
	// The service refuses the second topic's register: that topic closes, the first stays.
	two.joinLive("live:again", liveToken(t, shop, rec.Register(t, 4), "u2", inAnHour()), "", joined("live:again"))
	svc.Play(4, 7)
	if got := two.next(); got != liveClosed("live:again", "refused") {
		t.Fatalf("got %s", got)
	}
	two.change("live:q", rec, 6, "r2")
	one.quiet()
	two.quiet()
}

func TestARevokedSessionClosesOnlyItsTopic(t *testing.T) {
	svc := changestest.Start(t, "revoked")
	rec := svc.Session()
	srv := startLive(t, svc)
	signedIn := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	visitor := connect(t, srv, shop, person{session: "anon-1"})
	signedIn.joinLive("live:q", liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	svc.Play(0, 1)
	visitor.joinLive("live:q", liveToken(t, shop, rec.Register(t, 1), "", inAnHour()), "", joined("live:q"))
	svc.Play(1, 5)
	signedIn.change("live:q", rec, 2, "r1")
	if got := signedIn.next(); got != liveClosed("live:q", "revoked") {
		t.Fatalf("got %s", got)
	}
	visitor.change("live:q", rec, 2, "visitor")
	visitor.change("live:q", rec, 4, "visitor")
	signedIn.quiet()
}

func TestAGapClosesTheTopic(t *testing.T) {
	svc := changestest.Start(t, "gap")
	srv := startLive(t, svc)
	c := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	c.joinLive("live:q", liveToken(t, shop, svc.Session().Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	svc.Play(0, 2)
	if got := c.next(); got != liveClosed("live:q", "gap") {
		t.Fatalf("got %s", got)
	}
}

func TestTwoAppsShareOneServiceConnectionAndGetOnlyTheirOwnChanges(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	rec := svc.Session()
	srv := startLive(t, svc)
	mine := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	theirs := connect(t, srv, other, person{user: "u1", session: "s-r1"})
	mine.joinLive("live:q", liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	svc.Play(0, 1)
	theirs.joinLive("live:q", liveToken(t, other, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	if got := svc.Next(); got["app"] != other.id || got["type"] != "register" {
		t.Fatalf("got %v", got)
	}
	svc.Play(1, 3)
	mine.change("live:q", rec, 1, "r1")
	mine.change("live:q", rec, 2, "r1")
	theirs.quiet()
	if n := svc.Connections(); n != 0 {
		t.Fatalf("the relay opened %d more connections", n)
	}
}

func TestClosingTheSocketUnregistersItsLiveQueries(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	srv := startLive(t, svc)
	c := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	c.joinLive("live:q", liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	svc.Play(0, 2)
	c.change("live:q", rec, 1, "r1")
	c.ws.Close(websocket.StatusNormalClosure, "")
	svc.Play(3, 4)
}

func TestClosingTheSessionUnregistersItsLiveQueries(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	srv := startLive(t, svc)
	c := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	c.joinLive("live:q", liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	svc.Play(0, 2)
	c.change("live:q", rec, 1, "r1")
	command(t, srv, shop, "/sessions/close", `{"session":"s-r1"}`)
	svc.Play(3, 4)
	if code := c.closeStatus(); code != websocket.StatusNormalClosure {
		t.Fatalf("closed with %d", code)
	}
}

func TestTheTokensExpiryClosesTheTopic(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	srv := startLive(t, svc)
	c := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	exp := time.Unix(time.Now().Unix()+2, 0)
	c.joinLive("live:q", liveToken(t, shop, rec.Register(t, 0), "u1", exp), "", joined("live:q"))
	svc.Play(0, 2)
	c.change("live:q", rec, 1, "r1")
	if got := c.next(); got != liveClosed("live:q", "expired") {
		t.Fatalf("got %s", got)
	}
	if time.Now().Before(exp) {
		t.Fatal("the topic closed before its token expired")
	}
	svc.Play(3, 4)
}

func TestALostServiceErrorsTheTopicAndTheRelayRegistersItAgain(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	srv := startLive(t, svc)
	c := connect(t, srv, shop, person{user: "u1", session: "s-r1"})
	c.joinLive("live:q", liveToken(t, shop, rec.Register(t, 0), "u1", inAnHour()), "", joined("live:q"))
	svc.Play(0, 2)
	c.change("live:q", rec, 1, "r1")
	svc.Drop()
	if got := c.next(); got != `["1","1","live:q","phx_error",{}]` {
		t.Fatalf("got %s", got)
	}
	// On its next connection the relay registers the topic again after its last change.
	svc.Play(4, 6)
	c.change("live:q", rec, 5, "r2")
}

func TestAnAppWithoutAChangesServiceRefusesLiveJoins(t *testing.T) {
	svc := changestest.Start(t, "gap")
	srv := startLive(t, svc)
	c := connect(t, srv, appA, person{user: "u1", session: "s-r1"})
	c.joinLive("live:q", liveToken(t, appA, svc.Session().Register(t, 0), "u1", inAnHour()), "",
		`["1","1","live:q","phx_reply",{"response":{"reason":"changes_unavailable"},"status":"error"}]`)
	svc.Quiet()
}
