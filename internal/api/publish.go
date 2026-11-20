package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/seen"
)

// PublishIDWindow is how long a publish id is remembered; a repeat inside it is answered without sending again.
const PublishIDWindow = 10 * time.Minute

const (
	maxIDLen      = 128
	maxChannelLen = 256
)

type publishRequest struct {
	ID      string          `json:"id"`
	Channel string          `json:"channel"`
	Users   []string        `json:"users"`
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	About   string          `json:"about"`
}

func (p publishRequest) invalid() string {
	switch {
	case p.ID == "" || len(p.ID) > maxIDLen:
		return "id must be 1 to 128 bytes"
	case !validChannel(p.Channel):
		return "channel must be 1 to 256 bytes"
	case !hub.ValidEvent(p.Event):
		return "event must be 1 to 64 bytes and not reserved"
	case !isObject(p.Payload):
		return "payload must be a JSON object"
	case len(p.About) > hub.MaxInterestKeyLen:
		return "about must be at most 128 bytes"
	}
	return ""
}

func validChannel(channel string) bool { return channel != "" && len(channel) <= maxChannelLen }

func isObject(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && m != nil
}

type publish struct {
	hub *hub.Hub
	ids *seen.Set
	now func() time.Time
}

func (h *publish) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app, ok := authorize(h.hub, r)
	if !ok {
		refuse(w, http.StatusUnauthorized, "unauthorized", "unknown app or API key")
		return
	}
	var req publishRequest
	if !decode(w, r, app.Config.Limits.RequestBytes, &req) {
		return
	}
	if why := req.invalid(); why != "" {
		refuse(w, http.StatusBadRequest, "invalid_request", why)
		return
	}
	if !h.ids.First(app.Config.ID+"\x00"+req.ID, h.now()) {
		respond(w, http.StatusOK, sockets{Duplicate: true})
		return
	}
	n := app.Publish(hub.Publish{Channel: req.Channel, Users: req.Users, Event: req.Event, Payload: req.Payload,
		About: req.About})
	respond(w, http.StatusOK, sockets{Sockets: n})
}
