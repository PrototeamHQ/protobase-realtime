package hub

import "github.com/PrototeamHQ/protobase-realtime/internal/config"

// settings are an app's configuration and what the hub derives from it, swapped whole on reload so a reader never
// sees half of an old and half of a new configuration.
type settings struct {
	config        config.App
	maxEventBytes int
}

func newSettings(cfg config.App) *settings {
	return &settings{config: cfg, maxEventBytes: maxEventBytes(cfg.ClientEvents)}
}

// Config is the app's current configuration. Read it once per decision: a reload may swap it between two calls.
func (a *App) Config() *config.App {
	return &a.settings.Load().config
}

// MaxEventBytes is the largest payload the app lets a client send, so the socket can bound what it reads.
func (a *App) MaxEventBytes() int {
	return a.settings.Load().maxEventBytes
}
