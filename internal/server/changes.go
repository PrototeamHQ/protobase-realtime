package server

import (
	"context"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
)

// link is the relay's one connection to a changes service, for every app that names it. Guarded by the relay's
// mutex.
type link struct {
	service config.ChangesService
	client  *changes.Client
	// stop and done are set while the client runs.
	stop context.CancelFunc
	done chan struct{}
}

// links makes a link for each changes service an app names, reusing a current one whose URL and key are unchanged.
func links(apps []config.App, current map[string]*link, errs *errreport.Reporter) map[string]*link {
	next := map[string]*link{}
	for _, app := range apps {
		s := app.ChangesService
		if s == nil || next[s.ID] != nil {
			continue
		}
		if l := current[s.ID]; l != nil && l.service.URL == s.URL && l.service.Key == s.Key {
			next[s.ID] = l
			continue
		}
		stuck := func(err error) { errs.Error("", err) }
		next[s.ID] = &link{service: *s, client: changes.New(s.ID, s.URL, s.Key, stuck)}
	}
	return next
}

// run starts the client unless it runs already.
func (l *link) run(ctx context.Context) {
	if l.stop != nil {
		return
	}
	ctx, l.stop = context.WithCancel(ctx)
	l.done = make(chan struct{})
	client, done := l.client, l.done
	go func() {
		client.Run(ctx)
		close(done)
	}()
}

// halt stops the client for good: its live topics error, and their clients join again on the link that replaced it.
func (l *link) halt() {
	if l.stop == nil {
		l.client.Close()
		return
	}
	l.stop()
	<-l.done
	l.stop, l.done = nil, nil
}

// changesClient returns the client of a changes service, or nil when no app names it.
func (r *Server) changesClient(service string) *changes.Client {
	r.mu.Lock()
	defer r.mu.Unlock()
	l := r.links[service]
	if l == nil {
		return nil
	}
	return l.client
}
