package socket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/changes/changestest"
	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

// liveExpiry is when the tests' live tokens expire; the sessions' clock stands just before it, unless a test moves it.
var liveExpiry = time.Unix(1795193600, 0)

func runClient(t *testing.T, svc *changestest.Service) *changes.Client {
	t.Helper()
	c := changes.New("main", svc.URL, changestest.Key, func(err error) { t.Errorf("stuck: %v", err) })
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

// routedSession is a socket of the recorded app whose live queries register with client; user is empty for a
// visitor. Its clock stands at now.
func routedSession(t *testing.T, client *changes.Client, user, sid string, now time.Time) *session {
	t.Helper()
	app := hub.NewApp(config.App{ID: changestest.App, TicketKeys: []string{liveTicketKey}, Changes: "main",
		Limits: config.Limits{Connections: 10, ChannelsPerTicket: 5, RequestBytes: 1024}}, nil)
	out := newSink(func() {})
	conn, err := app.Register(ticket.Ticket{App: changestest.App, User: user, Anonymous: user == "", Session: sid},
		out, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return &session{app: app, conn: conn, out: out, now: func() time.Time { return now },
		changes: func(string) *changes.Client { return client }, live: newLiveTopics(out)}
}

// tokenOf seals a live token that registers as the recorded register does.
func tokenOf(t *testing.T, r changestest.Register, user string) string {
	t.Helper()
	claims := map[string]any{"iss": changestest.App, "exp": liveExpiry.Unix(), "subs": r.Subs, "fields": r.Fields,
		"snapshot": r.Snapshot}
	if user == "" {
		claims["anonymous"] = true
	} else {
		claims["sub"], claims["sid"] = user, r.Sid
	}
	return sealLive(t, claims)
}

func joinFrame(topic, token, cursor string) string {
	if cursor != "" {
		return fmt.Sprintf(`["1","1","%s","phx_join",{"live":"%s","cursor":"%s"}]`, topic, token, cursor)
	}
	return fmt.Sprintf(`["1","1","%s","phx_join",{"live":"%s"}]`, topic, token)
}

func okReply(topic string) string {
	return `["1","1","` + topic + `","phx_reply",{"response":{},"status":"ok"}]`
}

// nextFrame waits for the socket's next frame.
func nextFrame(t *testing.T, s *session) string {
	t.Helper()
	select {
	case f := <-s.out.queue:
		return string(f)
	case <-time.After(5 * time.Second):
		t.Fatal("the socket got nothing")
		return ""
	}
}

// expectChange checks the socket gets the recorded entry on topic, as the service shaped it.
func expectChange(t *testing.T, s *session, topic string, rec changestest.Session, frame int, id string) {
	t.Helper()
	raw := nextFrame(t, s)
	var parts []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &parts); err != nil || len(parts) != 5 {
		t.Fatalf("got %s", raw)
	}
	if string(parts[0]) != `"1"` || string(parts[1]) != `null` || string(parts[2]) != `"`+topic+`"` ||
		string(parts[3]) != `"change"` {
		t.Fatalf("got %s", raw)
	}
	got, want := decodeJSON(t, parts[4]), decodeJSON(t, rec.Entry(t, frame, id))
	got["id"] = want["id"]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func decodeJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func expectFrame(t *testing.T, s *session, want string) {
	t.Helper()
	if got := nextFrame(t, s); got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func closed(topic, reason string) string {
	return `["1","1","` + topic + `","phx_close",{"reason":"` + reason + `"}]`
}

var beforeExpiry = liveExpiry.Add(-time.Hour)

func TestALiveJoinRegistersAndGetsEachChangeWithItsCursor(t *testing.T) {
	svc := changestest.Start(t, "register-snapshot")
	rec := svc.Session()
	s := routedSession(t, runClient(t, svc), "u1", "s-r1", beforeExpiry)
	if got := exchange(t, s, joinFrame("live:q1", tokenOf(t, rec.Register(t, 0), "u1"), "")); got != okReply("live:q1") {
		t.Fatalf("got %s", got)
	}
	svc.Play(0, 3)
	expectChange(t, s, "live:q1", rec, 1, "r1")
	expectChange(t, s, "live:q1", rec, 2, "r1")
	if got := exchange(t, s, `["1","2","live:q1","phx_leave",{}]`); got != `["1","2","live:q1","phx_reply",{"response":{},"status":"ok"}]` {
		t.Fatalf("got %s", got)
	}
	svc.Play(3, 4)
}

func TestAJoinWithACursorResumesAfterIt(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	s := routedSession(t, runClient(t, svc), "u1", "s-r1", beforeExpiry)
	token := tokenOf(t, rec.Register(t, 0), "u1")
	exchange(t, s, joinFrame("live:q1", token, ""))
	svc.Play(0, 3)
	expectChange(t, s, "live:q1", rec, 1, "r1")
	expectChange(t, s, "live:q1", rec, 2, "r1")
	// A join on a joined topic replaces its registration.
	exchange(t, s, joinFrame("live:q1", token, rec.Register(t, 4).Cursor))
	svc.Play(3, 6)
	expectChange(t, s, "live:q1", rec, 5, "r2")
}

func TestATokensExpiryClosesItsTopic(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	s := routedSession(t, runClient(t, svc), "u1", "s-r1", liveExpiry.Add(-50*time.Millisecond))
	exchange(t, s, joinFrame("live:q1", tokenOf(t, rec.Register(t, 0), "u1"), ""))
	svc.Play(0, 2)
	expectChange(t, s, "live:q1", rec, 1, "r1")
	expectFrame(t, s, closed("live:q1", "expired"))
	svc.Play(3, 4)
}

func TestAGapClosesItsTopic(t *testing.T) {
	svc := changestest.Start(t, "gap")
	s := routedSession(t, runClient(t, svc), "u1", "s-r1", beforeExpiry)
	exchange(t, s, joinFrame("live:q1", tokenOf(t, svc.Session().Register(t, 0), "u1"), ""))
	svc.Play(0, 2)
	expectFrame(t, s, closed("live:q1", "gap"))
	want := `["1","2","live:q1","phx_reply",{"response":{"reason":"unmatched topic"},"status":"error"}]`
	if got := exchange(t, s, `["1","2","live:q1","phx_leave",{}]`); got != want {
		t.Fatalf("got %s", got)
	}
	svc.Quiet()
}

func TestALostServiceErrorsTheTopicAndRegistersItAgain(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	s := routedSession(t, runClient(t, svc), "u1", "s-r1", beforeExpiry)
	exchange(t, s, joinFrame("live:q1", tokenOf(t, rec.Register(t, 0), "u1"), ""))
	svc.Play(0, 2)
	expectChange(t, s, "live:q1", rec, 1, "r1")
	svc.Drop()
	expectFrame(t, s, `["1","1","live:q1","phx_error",{}]`)
	svc.Play(4, 6)
	expectChange(t, s, "live:q1", rec, 5, "r2")
}

func TestASocketThatEndsUnregistersItsLiveQueries(t *testing.T) {
	svc := changestest.Start(t, "register-cursor")
	rec := svc.Session()
	s := routedSession(t, runClient(t, svc), "u1", "s-r1", beforeExpiry)
	exchange(t, s, joinFrame("live:q1", tokenOf(t, rec.Register(t, 0), "u1"), ""))
	svc.Play(0, 2)
	expectChange(t, s, "live:q1", rec, 1, "r1")
	s.live.end()
	svc.Play(3, 4)
	// A join still on its way as the socket ended registers nothing.
	f, err := phoenix.Decode([]byte(joinFrame("live:q2", tokenOf(t, rec.Register(t, 0), "u1"), "")))
	if err != nil {
		t.Fatal(err)
	}
	s.handle(f)
	svc.Quiet()
}

func TestALiveJoinIsRefusedPastTheLimitOrWithALongCursor(t *testing.T) {
	// A client that never connects queues its registrations.
	client := changes.New("main", "ws://127.0.0.1:1/relay", changestest.Key, func(error) {})
	t.Cleanup(client.Close)
	rec := changestest.Load(t, "gap")
	s := routedSession(t, client, "u1", "s-r1", beforeExpiry)
	token := tokenOf(t, rec.Register(t, 0), "u1")
	long := string(bytes.Repeat([]byte("a"), maxCursorBytes+1))
	if got := exchange(t, s, joinFrame("live:q", token, long)); got != `["1","1","live:q","phx_reply",{"response":{"reason":"invalid_cursor"},"status":"error"}]` {
		t.Fatalf("got %s", got)
	}
	for i := range maxLiveTopics {
		topic := fmt.Sprintf("live:q%d", i)
		if got := exchange(t, s, joinFrame(topic, token, "")); got != okReply(topic) {
			t.Fatalf("join %d: %s", i, got)
		}
	}
	if got := exchange(t, s, joinFrame("live:more", token, "")); got != `["1","1","live:more","phx_reply",{"response":{"reason":"too_many_live_queries"},"status":"error"}]` {
		t.Fatalf("got %s", got)
	}
	// A join on a joined topic replaces it, so it is no new live query.
	if got := exchange(t, s, joinFrame("live:q0", token, "")); got != okReply("live:q0") {
		t.Fatalf("got %s", got)
	}
}
