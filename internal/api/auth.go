// Package api serves the commands an app sends the relay with one of its API keys.
package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
)

// authorize returns the app named in the path when the request carries one of its API keys. An unknown app and a
// wrong key get the same answer, so neither reveals which apps exist.
func authorize(h *hub.Hub, r *http.Request) (*hub.App, bool) {
	app, ok := h.App(r.PathValue("app"))
	if !ok {
		return nil, false
	}
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return nil, false
	}
	for _, valid := range app.Config.APIKeys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(valid)) == 1 {
			return app, true
		}
	}
	return nil, false
}
