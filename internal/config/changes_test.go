package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const relayKey = "CAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAg"

func keyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "relay_key")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func changesJSON(id, url, keyFile string) string {
	return `{"id":"` + id + `","url":"` + url + `","key_file":"` + keyFile + `"}`
}

func TestParsePointsAnAppAtItsChangesService(t *testing.T) {
	file := keyFile(t, relayKey+"\n")
	apps, err := Parse([]byte(`{"changes":[` + changesJSON("main", "wss://changes.internal/relay", file) + `],` +
		`"apps":[` + appJSON("chat", `,"changes":"main"`) + `,` + appJSON("shop", "") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	s := apps[0].ChangesService
	if s == nil || s.ID != "main" || s.URL != "wss://changes.internal/relay" || s.Key != relayKey {
		t.Fatalf("got %+v", s)
	}
	if apps[1].ChangesService != nil {
		t.Fatalf("an app without changes got %+v", apps[1].ChangesService)
	}
}

func TestParseRefusesChangesServices(t *testing.T) {
	good := keyFile(t, relayKey)
	cases := map[string]string{
		"unknown service": `{"apps":[` + appJSON("chat", `,"changes":"main"`) + `]}`,
		"bad id":          `{"changes":[` + changesJSON("Main", "wss://c/relay", good) + `],"apps":[]}`,
		"listed twice": `{"changes":[` + changesJSON("main", "wss://c/relay", good) + `,` +
			changesJSON("main", "wss://d/relay", good) + `],"apps":[]}`,
		"http url":    `{"changes":[` + changesJSON("main", "https://c/relay", good) + `],"apps":[]}`,
		"no host":     `{"changes":[` + changesJSON("main", "wss:///relay", good) + `],"apps":[]}`,
		"no key file": `{"changes":[` + changesJSON("main", "wss://c/relay", filepath.Join(t.TempDir(), "none")) + `],"apps":[]}`,
		"short key":   `{"changes":[` + changesJSON("main", "wss://c/relay", keyFile(t, "CAgI")) + `],"apps":[]}`,
		"padded key":  `{"changes":[` + changesJSON("main", "wss://c/relay", keyFile(t, relayKey+"=")) + `],"apps":[]}`,
		"unknown field": `{"changes":[{"id":"main","url":"wss://c/relay","key_file":"` + good + `","colour":"red"}],` +
			`"apps":[]}`,
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil || !strings.HasPrefix(err.Error(), "apps file") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
