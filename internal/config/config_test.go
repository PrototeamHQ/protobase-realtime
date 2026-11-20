package config

import (
	"strings"
	"testing"
)

const key = "0123456789abcdef0123456789abcdef"

func appJSON(id, extra string) string {
	return `{"id":"` + id + `","api_keys":["` + key + `"],"ticket_keys":["` + key + `"]` + extra +
		`,"limits":{"connections":10,"channels_per_ticket":5,"request_bytes":1024}}`
}

func TestParseReadsApps(t *testing.T) {
	apps, err := Parse([]byte(`{"apps":[` + appJSON("chat", `,"client_messages":true,"origins":["a.example"]`) + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].ID != "chat" || !apps[0].ClientMessages || apps[0].Limits.Connections != 10 {
		t.Fatalf("got %+v", apps)
	}
}

func TestParseRefuses(t *testing.T) {
	cases := map[string]string{
		"no apps":       `{"apps":[]}`,
		"unknown field": `{"apps":[` + appJSON("chat", `,"colour":"red"`) + `]}`,
		"duplicate":     `{"apps":[` + appJSON("chat", "") + `,` + appJSON("chat", "") + `]}`,
		"bad id":        `{"apps":[` + appJSON("Chat App", "") + `]}`,
		"short key":     `{"apps":[{"id":"x","api_keys":["short"],"ticket_keys":["` + key + `"],"limits":{"connections":1,"channels_per_ticket":1,"request_bytes":1}}]}`,
		"no limits":     `{"apps":[{"id":"x","api_keys":["` + key + `"],"ticket_keys":["` + key + `"]}]}`,
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil || !strings.HasPrefix(err.Error(), "apps file") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
