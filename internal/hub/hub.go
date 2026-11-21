package hub

import "github.com/PrototeamHQ/protobase-realtime/internal/config"

// Hub holds every app's hub, keyed by app id.
type Hub struct {
	apps map[string]*App
}

// New makes every app's hub. presence holds the presence of the apps that report it, by app id.
func New(apps []config.App, presence map[string]Presence) *Hub {
	h := &Hub{apps: map[string]*App{}}
	for _, cfg := range apps {
		h.apps[cfg.ID] = NewApp(cfg, presence[cfg.ID])
	}
	return h
}

func (h *Hub) App(id string) (*App, bool) {
	a, ok := h.apps[id]
	return a, ok
}

// TicketKeys returns an app's ticket keys, for ticket verification.
func (h *Hub) TicketKeys(id string) ([]string, bool) {
	a, ok := h.apps[id]
	if !ok {
		return nil, false
	}
	return a.Config.TicketKeys, true
}

// CloseAll closes every socket of every app, on shutdown.
func (h *Hub) CloseAll() {
	for _, a := range h.apps {
		a.mu.Lock()
		for c := range a.conns {
			c.sink.Restart()
		}
		a.mu.Unlock()
	}
}
