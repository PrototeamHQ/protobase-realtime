// Package relay wires the socket endpoint and the app commands into one HTTP handler.
package relay

import (
	"net/http"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/api"
	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/socket"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

type Relay struct {
	Hub     *hub.Hub
	Handler http.Handler
}

func New(apps []config.App, now func() time.Time) *Relay {
	h := hub.New(apps)
	mux := http.NewServeMux()
	mux.Handle("GET "+socket.Path, &socket.Handler{Hub: h, Tickets: ticket.NewVerifier(h.TicketKeys, now), Now: now})
	api.Mount(mux, h, now)
	return &Relay{Hub: h, Handler: mux}
}
