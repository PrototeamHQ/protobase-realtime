package server

import (
	"context"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
)

func withChanges(id string, s *config.ChangesService) config.App {
	a := app(id, nil)
	a.Changes, a.ChangesService = s.ID, s
	return a
}

func TestAppsNamingOneServiceShareItsClientAcrossReloads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &config.ChangesService{ID: "main", URL: "ws://changes.invalid/relay", Key: "k1"}
	r := New([]config.App{withChanges("a", s), withChanges("b", s), app("c", nil)}, time.Now, errreport.Off())
	r.Start(ctx)
	before := r.changesClient("main")
	if before == nil || len(r.links) != 1 || r.links["main"].stop == nil {
		t.Fatalf("got %v", r.links)
	}
	r.Reload([]config.App{withChanges("a", s), app("b", nil)})
	if r.changesClient("main") != before {
		t.Fatal("an unchanged service got a new client")
	}
	r.Reload([]config.App{withChanges("a", &config.ChangesService{ID: "main", URL: s.URL, Key: "k2"})})
	after := r.changesClient("main")
	if after == before || after == nil || r.links["main"].stop == nil {
		t.Fatal("a changed key kept the client")
	}
	if _, err := before.Register("a", changes.Start{}, nil, nil); err != changes.ErrStopped {
		t.Fatal("the replaced client still takes registrations")
	}
	r.Reload([]config.App{app("a", nil)})
	if r.changesClient("main") != nil {
		t.Fatal("a service no app names is still connected")
	}
}
