package changestest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// wait bounds how long the replayed service waits for the relay.
const wait = 5 * time.Second

// Service serves a session's service side on a relay connection. The relay mints its own registration ids, so the
// service maps each recorded id to the id the relay sent in its place: a register adds the relay's id, and an
// unregister, gap, revoked or refused ends the newest one, as the real service ends that registration.
type Service struct {
	t       *testing.T
	session Session
	// URL is where the relay connects.
	URL   string
	conns chan *conn
	conn  *conn
	ids   map[string][]string
	// stalled stops the service reading, until the test ends.
	stalled atomic.Bool
	// done is closed as the test ends, so a stalled read gives up.
	done chan struct{}
}

type conn struct {
	ws       *websocket.Conn
	messages chan []byte
}

// Start serves a recorded session, by name, until the test ends.
func Start(t *testing.T, name string) *Service {
	t.Helper()
	s := &Service{t: t, session: Load(t, name), conns: make(chan *conn, 8), ids: map[string][]string{},
		done: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(s.done) })
	s.URL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/relay"
	return s
}

// Session is what the service replays.
func (s *Service) Session() Session { return s.session }

func (s *Service) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/relay" || r.Header.Get("Authorization") != "Bearer "+Key {
		http.Error(w, "invalid relay key", http.StatusUnauthorized)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"protobase-relay.json"}})
	if err != nil {
		return
	}
	c := &conn{ws: ws, messages: make(chan []byte, 64)}
	s.conns <- c
	// A cancelled read closes a coder/websocket connection, so the messages are read here, until it closes.
	defer close(c.messages)
	for {
		if s.stalled.Load() {
			<-s.done
			return
		}
		_, raw, err := ws.Read(context.Background())
		if err != nil {
			return
		}
		select {
		case c.messages <- raw:
		case <-s.done:
			return
		}
	}
}

// Stall makes the service stop reading the relay's connection while it still writes, as the real service does while
// one app's registrations wait to be applied. It reads at most one more message.
func (s *Service) Stall() { s.stalled.Store(true) }

// current is the relay's newest connection, waited for when there is none.
func (s *Service) current() *conn {
	s.t.Helper()
	if s.conn != nil {
		return s.conn
	}
	select {
	case s.conn = <-s.conns:
		return s.conn
	case <-time.After(wait):
		s.t.Fatal("the relay did not connect")
		return nil
	}
}

// Play replays the frames from index from up to to: it checks each of the relay's against what the relay sends,
// and sends each of the service's.
func (s *Service) Play(from, to int) {
	s.t.Helper()
	for i := from; i < to; i++ {
		f := s.session.Frames[i]
		if f.From == "relay" {
			s.expect(i, f.Message)
			continue
		}
		s.send(f.Message)
	}
}

// Next returns the relay's next message, for one the session does not record.
func (s *Service) Next() map[string]any {
	s.t.Helper()
	c := s.current()
	select {
	case raw, ok := <-c.messages:
		if !ok {
			s.t.Fatal("the relay's connection closed")
		}
		return decode(s.t, raw)
	case <-time.After(wait):
		s.t.Fatal("the relay sent nothing")
		return nil
	}
}

// Quiet checks the relay sends nothing more for a short while.
func (s *Service) Quiet() {
	s.t.Helper()
	select {
	case raw, ok := <-s.current().messages:
		if ok {
			s.t.Fatalf("the relay sent %s", raw)
		}
	case <-time.After(300 * time.Millisecond):
	}
}

// Drop closes the relay's connection, as a service that goes away does; the relay's next one replays on.
func (s *Service) Drop() {
	s.t.Helper()
	c := s.current()
	c.ws.Close(websocket.StatusGoingAway, "restarting")
	s.conn = nil
}

// Connections reports how many connections the relay opened that the service has not taken up yet, besides the
// current one.
func (s *Service) Connections() int { return len(s.conns) }

func (s *Service) expect(i int, want json.RawMessage) {
	s.t.Helper()
	got := s.Next()
	recorded := decode(s.t, want)
	id, _ := recorded["id"].(string)
	actual, _ := got["id"].(string)
	switch recorded["type"] {
	case "register":
		s.ids[id] = append(s.ids[id], actual)
	case "unregister":
		if s.newest(id) != actual {
			s.t.Fatalf("frame %d: the relay unregistered %s, not the registration recorded as %s", i, actual, id)
		}
		s.end(id)
	}
	got["id"] = id
	if !reflect.DeepEqual(got, recorded) {
		s.t.Fatalf("frame %d: the relay sent\n%v\nrecorded\n%v", i, got, recorded)
	}
}

// send writes a service message with the relay's ids in place of the recorded ones.
func (s *Service) send(raw json.RawMessage) {
	s.t.Helper()
	m := decode(s.t, raw)
	switch m["type"] {
	case "gap", "revoked", "refused":
		id := m["id"].(string)
		m["id"] = s.newest(id)
		s.end(id)
	default:
		for _, e := range m["data"].(map[string]any)["registrations"].([]any) {
			entry := e.(map[string]any)
			entry["id"] = s.newest(entry["id"].(string))
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		s.t.Fatal(err)
	}
	if err := s.current().ws.Write(context.Background(), websocket.MessageText, out); err != nil {
		s.t.Fatal(err)
	}
}

func (s *Service) newest(id string) string {
	s.t.Helper()
	ids := s.ids[id]
	if len(ids) == 0 {
		s.t.Fatalf("no registration recorded as %s is open", id)
	}
	return ids[len(ids)-1]
}

func (s *Service) end(id string) {
	s.ids[id] = s.ids[id][:len(s.ids[id])-1]
}

// decode reads a message keeping its numbers as they were written.
func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}
