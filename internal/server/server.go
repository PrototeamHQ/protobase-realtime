// Package server wires the socket endpoint and the app commands into one HTTP handler.
package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/api"
	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/presence"
	"github.com/PrototeamHQ/protobase-realtime/internal/socket"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

type Server struct {
	Hub     *hub.Hub
	Handler http.Handler

	now     func() time.Time
	started time.Time
	errs    *errreport.Reporter
	// mu serializes Start and Reload.
	mu sync.Mutex
	// ctx is what Start was given; reporters run until it ends. Nil before Start.
	ctx context.Context
	// reporting holds the presence of the apps that turned it on, by app id.
	reporting map[string]*reporting
	// links holds the connections to the changes services apps name, by service id.
	links map[string]*link
}

// New serves apps, reporting errors to errs.
func New(apps []config.App, now func() time.Time, errs *errreport.Reporter) *Server {
	r := &Server{now: now, started: now(), errs: errs, reporting: map[string]*reporting{}}
	for _, app := range apps {
		if app.Presence != nil {
			r.reporting[app.ID] = newReporting(app, now, r.started, errs)
		}
	}
	r.links = links(apps, nil, errs)
	r.Hub = hub.New(apps, r.trackers())
	mux := http.NewServeMux()
	mux.Handle("GET "+HealthPath, health(r.started))
	sockets := &socket.Handler{Hub: r.Hub, Tickets: ticket.NewVerifier(r.Hub.TicketKeys, now), Now: now, Errors: errs,
		Changes: r.changesClient}
	mux.Handle("GET "+socket.Path, sockets)
	mux.HandleFunc("GET "+socket.EventsPath, sockets.ServeEvents)
	mux.HandleFunc("POST "+socket.SendPattern, sockets.ServeSend)
	mux.HandleFunc("OPTIONS "+socket.SendPattern, sockets.ServeSendPreflight)
	api.Mount(mux, r.Hub, now, errs)
	r.Handler = mux
	return r
}

// Start reports presence and connects to the changes services in the background until ctx ends.
func (r *Server) Start(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = ctx
	for _, p := range r.reporting {
		p.run(ctx)
	}
	for _, l := range r.links {
		l.run(ctx)
	}
}

// Reporter returns the presence reporter of an app, or nil when the app reports no presence.
func (r *Server) Reporter(app string) *presence.Reporter {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.reporting[app]
	if p == nil {
		return nil
	}
	return p.reporter
}

func (r *Server) trackers() map[string]hub.Presence {
	trackers := map[string]hub.Presence{}
	for id, p := range r.reporting {
		trackers[id] = p.tracker
	}
	return trackers
}
