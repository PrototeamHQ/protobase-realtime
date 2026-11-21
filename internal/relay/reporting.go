package relay

import (
	"context"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/presence"
)

// reporting is one app's presence: the tracker the hub tells, and the reporter that posts it to the app while it
// runs. Guarded by the relay's mutex.
type reporting struct {
	tracker  *presence.Tracker
	reporter *presence.Reporter
	// stop and done are set while the reporter runs.
	stop context.CancelFunc
	done chan struct{}
}

func newReporting(app config.App, now func() time.Time, started time.Time) *reporting {
	t := presence.NewTracker(grace(app.Presence), presence.Debounce, app.Limits.Connections, now)
	return &reporting{tracker: t, reporter: presence.NewReporter(t, app.Presence.URL, app.Presence.Key, started)}
}

func grace(p *config.Presence) time.Duration {
	return time.Duration(p.GraceSeconds) * time.Second
}

// reconfigure keeps the tracker, so no status is lost, and replaces the reporter when the app's URL or key changed.
func (p *reporting) reconfigure(app config.App) {
	p.tracker.Configure(grace(app.Presence), app.Limits.Connections)
	if p.reporter.URL == app.Presence.URL && p.reporter.Key == app.Presence.Key {
		return
	}
	p.halt()
	p.reporter = presence.NewReporter(p.tracker, app.Presence.URL, app.Presence.Key, p.reporter.Started)
}

// run starts the reporter unless it runs already.
func (p *reporting) run(ctx context.Context) {
	if p.stop != nil {
		return
	}
	ctx, p.stop = context.WithCancel(ctx)
	p.done = make(chan struct{})
	reporter, done := p.reporter, p.done
	go func() {
		reporter.Run(ctx)
		close(done)
	}()
}

// halt stops the reporter and waits until it has, so two reporters never post one app's presence at once.
func (p *reporting) halt() {
	if p.stop == nil {
		return
	}
	p.stop()
	<-p.done
	p.stop, p.done = nil, nil
}
