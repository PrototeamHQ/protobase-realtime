package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/relay"
)

func TestReloadRefusesAFileThatFailsToLoad(t *testing.T) {
	apps := []config.App{{ID: "a", Limits: config.Limits{Connections: 1, ChannelsPerTicket: 1, RequestBytes: 1}}}
	r := relay.New(apps, time.Now, errreport.Off())
	path := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(path, []byte(`{"apps":[{"id":"b"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reload(r, path, errreport.Off())
	if _, ok := r.Hub.App("a"); !ok {
		t.Fatal("a refused file dropped the apps served")
	}
	if _, ok := r.Hub.App("b"); ok {
		t.Fatal("a refused file was applied")
	}
}

func TestReloadTakesAFileThatListsNoApps(t *testing.T) {
	apps := []config.App{{ID: "a", Limits: config.Limits{Connections: 1, ChannelsPerTicket: 1, RequestBytes: 1}}}
	r := relay.New(apps, time.Now, errreport.Off())
	path := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(path, []byte(`{"apps":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reload(r, path, errreport.Off())
	if _, ok := r.Hub.App("a"); ok {
		t.Fatal("an app the file no longer lists is still served")
	}
}
