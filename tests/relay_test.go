package tests

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

func TestTwoAppsWithEqualChannelNamesNeverMeet(t *testing.T) {
	srv := start(t)
	a := connect(t, srv, appA, person{"u1", "s1", []string{"room"}})
	b := connect(t, srv, appB, person{"u1", "s1", []string{"room"}})
	a.join("room")
	b.join("room")
	if got := command(t, srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{"n":1}}`); got != `{"sockets":1}` {
		t.Fatalf("got %s", got)
	}
	if got := a.read(); got != `[null,null,"room","new",{"n":1}]` {
		t.Fatalf("a got %s", got)
	}
	b.send(`["1","2","room","typing",{}]`)
	b.read()
	if got := a.read(); got != "" {
		t.Fatalf("a got b's message %s", got)
	}
}

func TestPublishReachesConnectedMembersOnly(t *testing.T) {
	srv := start(t)
	member := connect(t, srv, appA, person{"u1", "s1", []string{"room"}})
	outsider := connect(t, srv, appA, person{"u2", "s2", []string{"other"}})
	member.join("room")
	outsider.join("other")
	command(t, srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{}}`)
	if member.read() == "" || outsider.read() != "" {
		t.Fatal("wrong recipients")
	}
	if got := command(t, srv, appA, "/publish", `{"id":"2","channel":"room","users":["gone"],"event":"new","payload":{}}`); got != `{"sockets":0}` {
		t.Fatalf("got %s", got)
	}
}

func TestClosingASessionLeavesTheOthersOpen(t *testing.T) {
	srv := start(t)
	phone := connect(t, srv, appA, person{"u1", "s1", nil})
	laptop := connect(t, srv, appA, person{"u1", "s1", nil})
	other := connect(t, srv, appA, person{"u1", "s2", []string{"room"}})
	if got := command(t, srv, appA, "/sessions/close", `{"session":"s1"}`); got != `{"sockets":2}` {
		t.Fatalf("got %s", got)
	}
	for _, c := range []*client{phone, laptop} {
		if code := c.closeStatus(); code != websocket.StatusNormalClosure {
			t.Fatalf("got %v", code)
		}
	}
	other.join("room")
	if status := handshake(t, socketURL(srv, issue(t, appA, person{"u1", "s1", nil})), nil); status != http.StatusUnauthorized {
		t.Fatalf("a ticket for the closed session connected: %d", status)
	}
	if got := command(t, srv, appA, "/sessions/close", `{"session":"s1"}`); got != `{"sockets":0}` {
		t.Fatalf("repeat got %s", got)
	}
}

func TestAClaimedSenderIsReplacedByTheVerifiedOne(t *testing.T) {
	srv := start(t)
	sender := connect(t, srv, appA, person{"u1", "s1", []string{"room"}})
	receiver := connect(t, srv, appA, person{"u2", "s2", []string{"room"}})
	sender.join("room")
	receiver.join("room")
	sender.send(`["1","2","room","typing",{"from":{"user":"u2"},"channel":"other"}]`)
	if got := sender.read(); !strings.Contains(got, `"status":"ok"`) {
		t.Fatalf("sender got %s", got)
	}
	got := receiver.read()
	if !strings.HasPrefix(got, `[null,null,"room","client_event",{"from":{"user":"u1","connection":"`) ||
		!strings.Contains(got, `"channel":"room","at":`) ||
		!strings.HasSuffix(got, `"event":"typing","payload":{"from":{"user":"u2"},"channel":"other"}}]`) {
		t.Fatalf("receiver got %s", got)
	}
}

func TestAnAnonymousSendIsRefused(t *testing.T) {
	srv := start(t)
	anon := connect(t, srv, appA, person{"", "visitor-1", []string{"room"}})
	member := connect(t, srv, appA, person{"u1", "s1", []string{"room"}})
	anon.join("room")
	member.join("room")
	anon.send(`["1","2","room","typing",{}]`)
	if got := anon.read(); !strings.Contains(got, `"reason":"receive_only"`) {
		t.Fatalf("got %s", got)
	}
	if got := member.read(); got != "" {
		t.Fatalf("member got %s", got)
	}
	command(t, srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{}}`)
	if anon.read() == "" {
		t.Fatal("anonymous session did not receive")
	}
}

func TestRemovedMemberIsClosedOutOfTheChannel(t *testing.T) {
	srv := start(t)
	c := connect(t, srv, appA, person{"u1", "s1", []string{"room"}})
	c.join("room")
	command(t, srv, appA, "/channels/remove", `{"channel":"room","users":["u1"]}`)
	if got := c.read(); got != `["1","1","room","phx_close",{}]` {
		t.Fatalf("got %s", got)
	}
	command(t, srv, appA, "/channels/add", `{"channel":"news","users":["u1"]}`)
	c.join("news")
}

func TestInterestFiltersDelivery(t *testing.T) {
	srv := start(t)
	viewer := connect(t, srv, appA, person{"u1", "s1", []string{"room"}})
	viewer.send(`["1","1","room","phx_join",{"viewing":["thread:2"]}]`)
	viewer.read()
	command(t, srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{},"about":"thread:1"}`)
	if got := viewer.read(); got != "" {
		t.Fatalf("got %s", got)
	}
	viewer.send(`["1","2","room","interest",{"viewing":["thread:1"]}]`)
	viewer.read()
	command(t, srv, appA, "/publish", `{"id":"2","channel":"room","event":"new","payload":{},"about":"thread:1"}`)
	if viewer.read() == "" {
		t.Fatal("viewer missed a message about what it views")
	}
}

func TestHandshakeRefusals(t *testing.T) {
	srv := start(t)
	ticket := issue(t, appA, person{"u1", "s1", nil})
	dial(t, srv, ticket)
	if status := handshake(t, socketURL(srv, ticket), nil); status != http.StatusUnauthorized {
		t.Fatalf("reused ticket: %d", status)
	}
	wrongVsn := strings.Replace(socketURL(srv, issue(t, appA, person{"u1", "s1", nil})), "vsn=2.0.0", "vsn=1.0.0", 1)
	if status := handshake(t, wrongVsn, nil); status != http.StatusBadRequest {
		t.Fatalf("vsn 1: %d", status)
	}
	forged := issue(t, testApp{appA.id, "", appB.ticketKey}, person{"u1", "s1", nil})
	if status := handshake(t, socketURL(srv, forged), nil); status != http.StatusUnauthorized {
		t.Fatalf("ticket signed with another app's key: %d", status)
	}
	origin := http.Header{"Origin": {"https://evil.example"}}
	if status := handshake(t, socketURL(srv, issue(t, appA, person{"u1", "s1", nil})), origin); status != http.StatusForbidden {
		t.Fatalf("foreign origin: %d", status)
	}
	allowed := http.Header{"Origin": {"https://allowed.example"}}
	if status := handshake(t, socketURL(srv, issue(t, appA, person{"u1", "s1", nil})), allowed); status != http.StatusSwitchingProtocols {
		t.Fatalf("allowed origin: %d", status)
	}
}

func handshake(t *testing.T, u string, header http.Header) int {
	t.Helper()
	ws, res, err := websocket.Dial(context.Background(), u, &websocket.DialOptions{HTTPHeader: header})
	if err == nil {
		ws.CloseNow()
		return http.StatusSwitchingProtocols
	}
	if res == nil {
		t.Fatal(errors.Join(errors.New("no response"), err))
	}
	return res.StatusCode
}
