package api

import (
	"net/http"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/seen"
)

// Paths of the app commands; {app} is the app id.
const (
	PublishPath           = "/apps/{app}/publish"
	AddToChannelPath      = "/apps/{app}/channels/add"
	RemoveFromChannelPath = "/apps/{app}/channels/remove"
	CloseSessionPath      = "/apps/{app}/sessions/close"
	RulesPath             = "/apps/{app}/people/rules"
)

// Mount adds the app commands to a mux, reporting their panics to errs.
func Mount(mux *http.ServeMux, h *hub.Hub, now func() time.Time, errs *errreport.Reporter) {
	handle := func(path string, command http.Handler) {
		mux.Handle("POST "+path, recovering{command: command, errs: errs})
	}
	handle(PublishPath, &publish{hub: h, ids: seen.New(PublishIDWindow), now: now})
	handle(AddToChannelPath, &members{hub: h, change: (*hub.App).AddToChannel})
	handle(RemoveFromChannelPath, &members{hub: h, change: (*hub.App).RemoveFromChannel})
	handle(CloseSessionPath, &closeSession{hub: h, now: now})
	handle(RulesPath, &rules{hub: h})
}

// recovering reports a command's panic with the app it was for.
type recovering struct {
	command http.Handler
	errs    *errreport.Reporter
}

func (c recovering) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer c.errs.Recover(r.PathValue("app"))
	c.command.ServeHTTP(w, r)
}
