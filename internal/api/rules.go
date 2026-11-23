package api

import (
	"net/http"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

type rulesRequest struct {
	User        string   `json:"user"`
	Blocked     []string `json:"blocked"`
	FriendsOnly bool     `json:"friends_only"`
	Friends     []string `json:"friends"`
}

// rules replaces the rules of a person's open sockets. Tickets issued later carry the app's own rules.
type rules struct {
	hub *hub.Hub
}

func (h *rules) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app, ok := authorize(h.hub, r)
	if !ok {
		refuse(w, http.StatusUnauthorized, "unauthorized", "unknown app or API key")
		return
	}
	var req rulesRequest
	if !decode(w, r, app.Config().Limits.RequestBytes, &req) {
		return
	}
	next := ticket.Rules{Blocked: req.Blocked, FriendsOnly: req.FriendsOnly, Friends: req.Friends}
	if req.User == "" || !next.Valid() {
		refuse(w, http.StatusBadRequest, "invalid_request", "user is required, and blocked and friends hold at most 1000 people each")
		return
	}
	respond(w, http.StatusOK, sockets{Sockets: app.SetRules(req.User, next)})
}
