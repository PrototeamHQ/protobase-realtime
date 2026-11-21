// Package relay wires the socket endpoint and the app commands into one HTTP handler.
package relay

import (
	"context"
	"net/http"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/api"
	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/presence"
	"github.com/PrototeamHQ/protobase-realtime/internal/socket"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

type Relay struct {
	Hub     *hub.Hub
	Handler http.Handler
	// Reporters post presence for the apps that turned it on; Start starts them.
	Reporters []*presence.Reporter
}

func New(apps []config.App, now func() time.Time) *Relay {
	trackers := map[string]hub.Presence{}
	var reporters []*presence.Reporter
	started := now()
	for _, app := range apps {
		if app.Presence == nil {
			continue
		}
		t := presence.NewTracker(time.Duration(app.Presence.GraceSeconds)*time.Second, presence.Debounce,
			app.Limits.Connections, now)
		trackers[app.ID] = t
		reporters = append(reporters, presence.NewReporter(t, app.Presence.URL, app.Presence.Key, started))
	}
	h := hub.New(apps, trackers)
	mux := http.NewServeMux()
	mux.Handle("GET "+socket.Path, &socket.Handler{Hub: h, Tickets: ticket.NewVerifier(h.TicketKeys, now), Now: now})
	api.Mount(mux, h, now)
	return &Relay{Hub: h, Handler: mux, Reporters: reporters}
}

// Start reports presence in the background until ctx ends.
func (r *Relay) Start(ctx context.Context) {
	for _, reporter := range r.Reporters {
		go reporter.Run(ctx)
	}
}
