package server

import "github.com/PrototeamHQ/protobase-realtime/internal/config"

// Reload swaps in a newly loaded apps file. Apps still listed keep their sockets and their presence state; presence
// that changed takes its new grace, limit, URL and key; a removed app's sockets close and its reporter stops. A changes
// service whose URL or key changed, or that no app names any longer, is disconnected, and its live topics error.
func (r *Server) Reload(apps []config.App) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := map[string]*reporting{}
	for _, app := range apps {
		if app.Presence == nil {
			continue
		}
		p := r.reporting[app.ID]
		if p == nil {
			p = newReporting(app, r.now, r.started, r.errs)
		} else {
			p.reconfigure(app)
		}
		next[app.ID] = p
	}
	for id, p := range r.reporting {
		if next[id] == nil {
			p.halt()
		}
	}
	r.reporting = next
	nextLinks := links(apps, r.links, r.errs)
	for id, l := range r.links {
		if nextLinks[id] != l {
			l.halt()
		}
	}
	r.links = nextLinks
	r.Hub.Reload(apps, r.trackers())
	if r.ctx == nil {
		return
	}
	for _, p := range r.reporting {
		p.run(r.ctx)
	}
	for _, l := range r.links {
		l.run(r.ctx)
	}
}
