package hub

import "github.com/PrototeamHQ/protobase-realtime/internal/config"

// Reload swaps in a new apps table in one step. An app still listed keeps its hub, and so its sockets, channels and
// people, and takes its new configuration from the next decision on; a new app gets a hub; a removed app's sockets
// close for good. presence holds the presence of the apps that report it, by app id.
func (h *Hub) Reload(apps []config.App, presence map[string]Presence) {
	h.mu.Lock()
	next := map[string]*App{}
	for _, cfg := range apps {
		a, ok := h.apps[cfg.ID]
		if !ok {
			next[cfg.ID] = NewApp(cfg, presence[cfg.ID])
			continue
		}
		a.reconfigure(cfg, presence[cfg.ID])
		next[cfg.ID] = a
	}
	var removed []*App
	for id, a := range h.apps {
		if next[id] == nil {
			removed = append(removed, a)
		}
	}
	h.apps = next
	h.mu.Unlock()
	for _, a := range removed {
		a.remove()
	}
}

// reconfigure takes a changed configuration. Open sockets stay: each already proved its ticket, and a lower
// connection limit holds for new sockets only. Presence that is new to the app hears of everyone online now.
func (a *App) reconfigure(cfg config.App, presence Presence) {
	a.settings.Store(newSettings(cfg))
	a.mu.Lock()
	defer a.mu.Unlock()
	if presence == a.presence {
		return
	}
	a.presence = presence
	for user := range a.byUser {
		a.reportPresence(user)
	}
}

// remove closes every socket of an app the apps file no longer lists, with a normal closure so clients do not
// reconnect to it, and refuses sockets still on their way in.
func (a *App) remove() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.removed = true
	for c := range a.conns {
		c.sink.Close("app removed")
	}
}
