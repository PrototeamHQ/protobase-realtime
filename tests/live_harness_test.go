package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes/changestest"
	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/live"
	"github.com/PrototeamHQ/protobase-realtime/internal/server"
)

var (
	// shop is the app the relay sessions were recorded for; other shares its changes service.
	shop  = testApp{changestest.App, "shop-api-key-0123456789abcdefghij", "shop-ticket-key-0123456789abcdefg"}
	other = testApp{"other", "other-api-key-0123456789abcdefghi", "other-ticket-key-0123456789abcdef"}
)

// startLive serves shop and other, whose live queries register with the replayed service, and a, which names no
// changes service.
func startLive(t *testing.T, svc *changestest.Service) *httptest.Server {
	t.Helper()
	service := &config.ChangesService{ID: "main", URL: svc.URL, Key: changestest.Key}
	var apps []config.App
	for _, a := range []testApp{shop, other, appA} {
		app := config.App{ID: a.id, APIKeys: []string{a.apiKey}, TicketKeys: []string{a.ticketKey},
			Limits: config.Limits{Connections: 50, ChannelsPerTicket: 10, RequestBytes: 4096}}
		if a != appA {
			app.Changes, app.ChangesService = service.ID, service
		}
		apps = append(apps, app)
	}
	r := server.New(apps, time.Now, errreport.Off())
	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	srv := httptest.NewServer(r.Handler)
	t.Cleanup(func() {
		r.Hub.CloseAll()
		srv.Close()
		cancel()
	})
	return srv
}

// liveToken seals a token for app that registers as the recorded register does, for the person or, when user is
// empty, a visitor.
func liveToken(t *testing.T, app testApp, r changestest.Register, user string, exp time.Time) string {
	t.Helper()
	claims := map[string]any{"iss": app.id, "exp": exp.Unix(), "subs": r.Subs, "fields": r.Fields,
		"snapshot": r.Snapshot}
	if user == "" {
		claims["anonymous"] = true
	} else {
		claims["sub"], claims["sid"] = user, r.Sid
	}
	encrypter, err := jose.NewEncrypter(jose.A256GCM,
		jose.Recipient{Algorithm: jose.DIRECT, Key: live.Key(app.ticketKey)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Encrypted(encrypter).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func inAnHour() time.Time { return time.Now().Add(time.Hour) }

// joinLive joins a live topic and checks the reply.
func (c *client) joinLive(topic, token, cursor, want string) {
	c.t.Helper()
	payload := map[string]string{"live": token}
	if cursor != "" {
		payload["cursor"] = cursor
	}
	body, _ := json.Marshal(payload)
	c.send(fmt.Sprintf(`["1","1","%s","phx_join",%s]`, topic, body))
	if got := c.next(); got != want {
		c.t.Fatalf("join %s: got %s\nwant %s", topic, got, want)
	}
}

func joined(topic string) string {
	return `["1","1","` + topic + `","phx_reply",{"response":{},"status":"ok"}]`
}

func liveClosed(topic, reason string) string {
	return `["1","1","` + topic + `","phx_close",{"reason":"` + reason + `"}]`
}

// next waits for the socket's next frame.
func (c *client) next() string {
	c.t.Helper()
	select {
	case frame := <-c.frames:
		return frame
	case <-time.After(5 * time.Second):
		c.t.Fatal("the socket got nothing")
		return ""
	}
}

// change checks the socket gets the recorded entry on topic, as the service shaped it, its cursor included.
func (c *client) change(topic string, rec changestest.Session, frame int, id string) {
	c.t.Helper()
	raw := c.next()
	var parts []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &parts); err != nil || len(parts) != 5 {
		c.t.Fatalf("got %s", raw)
	}
	head := string(parts[0]) + string(parts[1]) + string(parts[2]) + string(parts[3])
	if head != `"1"null"`+topic+`""change"` {
		c.t.Fatalf("got %s", raw)
	}
	got, want := decodeObject(c.t, parts[4]), decodeObject(c.t, rec.Entry(c.t, frame, id))
	got["id"] = want["id"]
	if !reflect.DeepEqual(got, want) {
		c.t.Fatalf("got %v\nwant %v", got, want)
	}
}

func decodeObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

// quiet checks the socket gets nothing for a short while.
func (c *client) quiet() {
	c.t.Helper()
	if got := c.read(); got != "" {
		c.t.Fatalf("got %s", got)
	}
}
