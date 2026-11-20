package api

import (
	"net/http"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
)

type membersRequest struct {
	Channel  string   `json:"channel"`
	Users    []string `json:"users"`
	Sessions []string `json:"sessions"`
}

// members adds people to a channel or removes them, on the sockets they have open now. Tickets issued later carry
// the app's own channel list.
type members struct {
	hub    *hub.Hub
	change func(app *hub.App, channel string, t hub.Target) int
}

func (h *members) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app, ok := authorize(h.hub, r)
	if !ok {
		refuse(w, http.StatusUnauthorized, "unauthorized", "unknown app or API key")
		return
	}
	var req membersRequest
	if !decode(w, r, app.Config.Limits.RequestBytes, &req) {
		return
	}
	if !validChannel(req.Channel) {
		refuse(w, http.StatusBadRequest, "invalid_request", "channel must be 1 to 256 bytes")
		return
	}
	n := h.change(app, req.Channel, hub.Target{Users: req.Users, Sessions: req.Sessions})
	respond(w, http.StatusOK, sockets{Sockets: n})
}
