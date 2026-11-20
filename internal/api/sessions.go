package api

import (
	"net/http"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
)

type closeSessionRequest struct {
	Session string `json:"session"`
}

type closeSession struct {
	hub *hub.Hub
	now func() time.Time
}

func (h *closeSession) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app, ok := authorize(h.hub, r)
	if !ok {
		refuse(w, http.StatusUnauthorized, "unauthorized", "unknown app or API key")
		return
	}
	var req closeSessionRequest
	if !decode(w, r, app.Config.Limits.RequestBytes, &req) {
		return
	}
	if req.Session == "" {
		refuse(w, http.StatusBadRequest, "invalid_request", "session is required")
		return
	}
	respond(w, http.StatusOK, sockets{Sockets: app.CloseSession(req.Session, h.now())})
}
