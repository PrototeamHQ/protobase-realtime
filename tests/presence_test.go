package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/presence"
)

const presenceKey = "app-a-presence-key-0123456789abc"

// appPresence records the presence reports the relay posts to the app.
type appPresence struct {
	mu      sync.Mutex
	changes []presence.Change
}

func (a *appPresence) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+presenceKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var body struct{ Changes []presence.Change }
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.changes = append(a.changes, body.Changes...)
}

// take returns the statuses reported since the last take.
func (a *appPresence) take() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, c := range a.changes {
		out = append(out, c.User+":"+string(c.Status))
	}
	a.changes = nil
	return out
}

func TestPresenceReachesTheAppCombinedOverDevices(t *testing.T) {
	app := &appPresence{}
	appSrv := httptest.NewServer(app)
	t.Cleanup(appSrv.Close)
	var skew atomic.Int64
	now := func() time.Time { return time.Now().Add(time.Duration(skew.Load())) }
	srv, r := startWith(t, &config.Presence{URL: appSrv.URL, Key: presenceKey, GraceSeconds: 5}, now)
	// report skips the debounce and sends what is due, as the reporter does each tick.
	report := func() []string {
		t.Helper()
		skew.Add(int64(presence.Debounce))
		if err := r.Reporter(appA.id).Flush(context.Background()); err != nil {
			t.Fatal(err)
		}
		return app.take()
	}

	phone := connect(t, srv, appA, person{"u1", "s1", nil})
	laptop := connect(t, srv, appA, person{"u1", "s2", nil})
	phone.join("relay")
	if got := report(); len(got) != 1 || got[0] != "u1:online" {
		t.Fatalf("got %v", got)
	}
	phone.send(`["1","2","relay","presence",{"status":"away"}]`)
	phone.read()
	if got := report(); len(got) != 0 {
		t.Fatalf("one device away reported %v", got)
	}
	laptop.ws.CloseNow()
	waitConnections(t, r.Hub, 1)
	if got := report(); len(got) != 1 || got[0] != "u1:away" {
		t.Fatalf("got %v", got)
	}
	phone.ws.CloseNow()
	waitConnections(t, r.Hub, 0)
	reconnected := connect(t, srv, appA, person{"u1", "s1", nil})
	if got := report(); len(got) != 1 || got[0] != "u1:online" {
		t.Fatalf("a reconnect within the grace got %v", got)
	}
	reconnected.ws.CloseNow()
	waitConnections(t, r.Hub, 0)
	skew.Add(int64(5 * time.Second))
	if got := report(); len(got) != 1 || got[0] != "u1:offline" {
		t.Fatalf("got %v", got)
	}
}

// waitConnections waits until the relay has dropped closed sockets, which it does once it reads their close.
func waitConnections(t *testing.T, h *hub.Hub, want int) {
	t.Helper()
	app, _ := h.App(appA.id)
	deadline := time.Now().Add(5 * time.Second)
	for app.Connections() != want {
		if time.Now().After(deadline) {
			t.Fatalf("%d sockets open, want %d", app.Connections(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
