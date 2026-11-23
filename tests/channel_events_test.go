package tests

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/server"
)

// startDeclared serves app a from an apps file that lets clients send typing on conversations channels only.
func startDeclared(t *testing.T) *httptest.Server {
	t.Helper()
	apps, err := config.Parse([]byte(`{"apps":[{"id":"` + appA.id + `","api_keys":["` + appA.apiKey + `"],` +
		`"ticket_keys":["` + appA.ticketKey + `"],"client_events":{"channels":{"conversations":{"events":{"typing":` +
		`{"schema":{"type":"object","required":["thread"]},"max_bytes":64,"per_second":5,"burst":5}}}}},` +
		`"limits":{"connections":50,"channels_per_ticket":10,"request_bytes":4096}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	r := server.New(apps, time.Now, errreport.Off())
	srv := httptest.NewServer(r.Handler)
	t.Cleanup(func() {
		r.Hub.CloseAll()
		srv.Close()
	})
	return srv
}

func TestDeclaredEventPassesOnlyOnItsChannelResource(t *testing.T) {
	srv := startDeclared(t)
	channels := []string{"conversations/1", "rooms/1"}
	sender := connect(t, srv, appA, person{"u1", "s1", channels})
	other := connect(t, srv, appA, person{"u2", "s2", channels})
	for _, channel := range channels {
		sender.join(channel)
		other.join(channel)
	}
	sender.send(`["1","2","rooms/1","typing",{"thread":"t1"}]`)
	if got := sender.read(); !strings.Contains(got, `"reason":"event_not_allowed"`) {
		t.Fatalf("got %s", got)
	}
	sender.send(`["1","3","conversations/1","typing",{"thread":"t1"}]`)
	sender.read()
	if got := other.read(); !strings.HasPrefix(got, `[null,null,"conversations/1","client_event",`) {
		t.Fatalf("other got %s", got)
	}
	if got := other.read(); got != "" {
		t.Fatalf("other got the refused event %s", got)
	}
}
