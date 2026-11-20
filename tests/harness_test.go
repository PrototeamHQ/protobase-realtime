// Integration tests: the relay served over HTTP, driven by real WebSocket clients and app commands.
package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/golang-jwt/jwt/v5"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/relay"
)

type testApp struct {
	id, apiKey, ticketKey string
}

var (
	appA = testApp{"a", "app-a-api-key-0123456789abcdefgh", "app-a-ticket-key-0123456789abcde"}
	appB = testApp{"b", "app-b-api-key-0123456789abcdefgh", "app-b-ticket-key-0123456789abcde"}
)

func start(t *testing.T) *httptest.Server {
	t.Helper()
	var apps []config.App
	for _, a := range []testApp{appA, appB} {
		apps = append(apps, config.App{ID: a.id, APIKeys: []string{a.apiKey}, TicketKeys: []string{a.ticketKey},
			Origins: []string{"allowed.example"}, ClientMessages: true,
			Limits: config.Limits{Connections: 50, ChannelsPerTicket: 10, RequestBytes: 4096}})
	}
	r := relay.New(apps, time.Now)
	srv := httptest.NewServer(r.Handler)
	t.Cleanup(func() {
		r.Hub.CloseAll()
		srv.Close()
	})
	return srv
}

// person is who a ticket names; an empty user is an anonymous session.
type person struct {
	user, session string
	channels      []string
}

func issue(t *testing.T, app testApp, p person) string {
	t.Helper()
	jti := make([]byte, 8)
	_, _ = rand.Read(jti)
	claims := jwt.MapClaims{"iss": app.id, "sid": p.session, "jti": hex.EncodeToString(jti),
		"exp": time.Now().Add(time.Minute).Unix(), "channels": p.channels}
	if p.user == "" {
		claims["anonymous"] = true
	} else {
		claims["sub"] = p.user
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(app.ticketKey))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func socketURL(srv *httptest.Server, ticket string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/socket/websocket?vsn=2.0.0&ticket=" + url.QueryEscape(ticket)
}

// client reads frames in the background, since a cancelled read closes a coder/websocket connection.
type client struct {
	t      *testing.T
	ws     *websocket.Conn
	frames chan string
	closed chan error
}

func dial(t *testing.T, srv *httptest.Server, ticket string) *client {
	t.Helper()
	ws, _, err := websocket.Dial(context.Background(), socketURL(srv, ticket), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	c := &client{t, ws, make(chan string, 64), make(chan error, 1)}
	go func() {
		for {
			_, raw, err := ws.Read(context.Background())
			if err != nil {
				c.closed <- err
				return
			}
			c.frames <- string(raw)
		}
	}()
	return c
}

func connect(t *testing.T, srv *httptest.Server, app testApp, p person) *client {
	t.Helper()
	return dial(t, srv, issue(t, app, p))
}

func (c *client) send(frame string) {
	c.t.Helper()
	if err := c.ws.Write(context.Background(), websocket.MessageText, []byte(frame)); err != nil {
		c.t.Fatal(err)
	}
}

// read returns the next frame, or "" when none arrives within a short wait.
func (c *client) read() string {
	c.t.Helper()
	select {
	case frame := <-c.frames:
		return frame
	case <-time.After(300 * time.Millisecond):
		return ""
	}
}

// closeStatus waits for the relay to close the socket and returns its close code.
func (c *client) closeStatus() websocket.StatusCode {
	c.t.Helper()
	select {
	case err := <-c.closed:
		return websocket.CloseStatus(err)
	case <-time.After(5 * time.Second):
		c.t.Fatal("socket stayed open")
		return 0
	}
}

func (c *client) join(topic string) {
	c.t.Helper()
	c.send(`["1","1","` + topic + `","phx_join",{}]`)
	if got := c.read(); !strings.Contains(got, `"status":"ok"`) {
		c.t.Fatalf("join %s: %s", topic, got)
	}
}

func command(t *testing.T, srv *httptest.Server, app testApp, path, body string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/apps/"+app.id+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+app.apiKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d %s", path, res.StatusCode, out)
	}
	return strings.TrimSpace(string(out))
}
