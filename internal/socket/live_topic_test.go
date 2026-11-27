package socket

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/live"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

const liveTicketKey = "ticket-key-0123456789abcdefghijklmn"

func newLiveSession(t *testing.T, user, sid string) *session {
	t.Helper()
	app := hub.NewApp(config.App{ID: "a", TicketKeys: []string{liveTicketKey},
		Limits: config.Limits{Connections: 10, ChannelsPerTicket: 5, RequestBytes: 1024}}, nil)
	out := newSink(func() {})
	conn, err := app.Register(ticket.Ticket{App: "a", User: user, Anonymous: user == "", Session: sid},
		out, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &session{app: app, conn: conn, out: out, now: func() time.Time { return time.Unix(100, 0) },
		live: newLiveTopics(out)}
}

func sealLive(t *testing.T, claims map[string]any) string {
	t.Helper()
	encrypter, err := jose.NewEncrypter(jose.A256GCM,
		jose.Recipient{Algorithm: jose.DIRECT, Key: live.Key(liveTicketKey)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Encrypted(encrypter).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func liveReply(reason string) string {
	return `["1","1","live:q1","phx_reply",{"response":{"reason":"` + reason + `"},"status":"error"}]`
}

func TestLiveJoinChecksTheTokenAgainstTheSocket(t *testing.T) {
	s := newLiveSession(t, "u1", "s1")
	mine := sealLive(t, map[string]any{"iss": "a", "sub": "u1", "sid": "s1", "exp": 200, "resource": "messages"})
	otherSession := sealLive(t, map[string]any{"iss": "a", "sub": "u1", "sid": "s2", "exp": 200})
	expired := sealLive(t, map[string]any{"iss": "a", "sub": "u1", "sid": "s1", "exp": 100})
	steps := []struct{ payload, want string }{
		{`{}`, liveReply("invalid_live_token")},
		{`{"live":7}`, liveReply("invalid_live_token")},
		{`{"live":"` + otherSession + `"}`, liveReply("invalid_live_token")},
		{`{"live":"` + expired + `"}`, liveReply("invalid_live_token")},
		{`{"live":"` + mine + `"}`, liveReply("changes_unavailable")},
	}
	for _, step := range steps {
		send := fmt.Sprintf(`["1","1","live:q1","phx_join",%s]`, step.payload)
		if got := exchange(t, s, send); got != step.want {
			t.Errorf("%s:\n got %s\nwant %s", step.payload, got, step.want)
		}
	}
}

func TestLiveJoinTakesAVisitorsTokenOnAnAnonymousSocket(t *testing.T) {
	s := newLiveSession(t, "", "anon-1")
	token := sealLive(t, map[string]any{"iss": "a", "anonymous": true, "exp": 200})
	if got := exchange(t, s, `["1","1","live:q1","phx_join",{"live":"`+token+`"}]`); got != liveReply("changes_unavailable") {
		t.Fatalf("got %s", got)
	}
}

func TestLiveTopicIsNotJoinedAfterARefusal(t *testing.T) {
	s := newLiveSession(t, "u1", "s1")
	exchange(t, s, `["1","1","live:q1","phx_join",{}]`)
	want := `["1","2","live:q1","phx_reply",{"response":{"reason":"unmatched topic"},"status":"error"}]`
	if got := exchange(t, s, `["1","2","live:q1","phx_leave",{}]`); got != want {
		t.Fatalf("got %s", got)
	}
}
