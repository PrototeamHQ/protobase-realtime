package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/errreport/errreporttest"
)

func TestLoadReportsAFileThatFailsToLoad(t *testing.T) {
	in, errs := errreporttest.Serve(t)
	path := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(path, []byte(`{"apps":[{"id":"b","api_keys":["k3y-in-the-file"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path, errs); err == nil {
		t.Fatal("a file that fails to load loaded")
	}
	got := in.Received()
	if !strings.Contains(got, "apps file") || strings.Contains(got, "k3y-in-the-file") {
		t.Fatalf("got %q", got)
	}
}
