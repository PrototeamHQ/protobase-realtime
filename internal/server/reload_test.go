package server

import (
	"context"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
)

const key = "presence-key-0123456789abcdefghij"

func app(id string, presence *config.Presence) config.App {
	return config.App{ID: id, Presence: presence,
		Limits: config.Limits{Connections: 10, ChannelsPerTicket: 5, RequestBytes: 1024}}
}

func TestReloadStartsAndStopsPresenceReporters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &config.Presence{URL: "http://app.invalid/presence", Key: key, GraceSeconds: 5}
	r := New([]config.App{app("a", p)}, time.Now, errreport.Off())
	r.Start(ctx)
	a := r.reporting["a"]
	r.Reload([]config.App{app("b", p)})
	if r.Reporter("a") != nil || a.stop != nil {
		t.Fatal("a removed app's reporter still runs")
	}
	if r.Reporter("b") == nil || r.reporting["b"].stop == nil {
		t.Fatal("a new app's reporter does not run")
	}
	r.Reload([]config.App{app("b", nil)})
	if r.Reporter("b") != nil {
		t.Fatal("presence turned off still reports")
	}
}

func TestReloadKeepsTheTrackerAndReplacesAReporterWhoseURLChanged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := New([]config.App{app("a", &config.Presence{URL: "http://old.invalid/", Key: key})}, time.Now,
		errreport.Off())
	r.Start(ctx)
	before := r.Reporter("a")
	r.Reload([]config.App{app("a", &config.Presence{URL: "http://old.invalid/", Key: key, GraceSeconds: 9})})
	if r.Reporter("a") != before {
		t.Fatal("a changed grace replaced the reporter")
	}
	r.Reload([]config.App{app("a", &config.Presence{URL: "http://new.invalid/", Key: key})})
	after := r.Reporter("a")
	if after == before || after.URL != "http://new.invalid/" || after.Tracker != before.Tracker {
		t.Fatalf("got %+v", after)
	}
	if !after.Started.Equal(before.Started) {
		t.Fatal("a new reporter reports another start, so the app would take everyone as offline")
	}
}

func TestReloadBeforeStartRunsNoReporter(t *testing.T) {
	r := New([]config.App{app("a", nil)}, time.Now, errreport.Off())
	r.Reload([]config.App{app("a", &config.Presence{URL: "http://app.invalid/", Key: key})})
	if r.Reporter("a") == nil || r.reporting["a"].stop != nil {
		t.Fatal("a reporter ran before Start")
	}
}
