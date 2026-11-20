package api

import (
	"net/http"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/seen"
)

// Paths of the app commands; {app} is the app id.
const (
	PublishPath           = "/apps/{app}/publish"
	AddToChannelPath      = "/apps/{app}/channels/add"
	RemoveFromChannelPath = "/apps/{app}/channels/remove"
	CloseSessionPath      = "/apps/{app}/sessions/close"
)

// Mount adds the app commands to a mux.
func Mount(mux *http.ServeMux, h *hub.Hub, now func() time.Time) {
	mux.Handle("POST "+PublishPath, &publish{hub: h, ids: seen.New(PublishIDWindow), now: now})
	mux.Handle("POST "+AddToChannelPath, &members{hub: h, change: (*hub.App).AddToChannel})
	mux.Handle("POST "+RemoveFromChannelPath, &members{hub: h, change: (*hub.App).RemoveFromChannel})
	mux.Handle("POST "+CloseSessionPath, &closeSession{hub: h, now: now})
}
