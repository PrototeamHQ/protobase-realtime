package config

import (
	"strconv"
	"strings"
	"testing"
)

const key = "0123456789abcdef0123456789abcdef"

func appJSON(id, extra string) string {
	return `{"id":"` + id + `","api_keys":["` + key + `"],"ticket_keys":["` + key + `"]` + extra +
		`,"limits":{"connections":10,"channels_per_ticket":5,"request_bytes":1024}}`
}

func TestParseReadsApps(t *testing.T) {
	apps, err := Parse([]byte(`{"apps":[` + appJSON("chat", `,"client_events":{"allow_any":true},"origins":["a.example"]`) + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].ID != "chat" || !apps[0].ClientEvents.AllowAny || apps[0].Limits.Connections != 10 {
		t.Fatalf("got %+v", apps)
	}
}

func TestParseRefuses(t *testing.T) {
	cases := map[string]string{
		"no apps":            `{"apps":[]}`,
		"unknown field":      `{"apps":[` + appJSON("chat", `,"colour":"red"`) + `]}`,
		"duplicate":          `{"apps":[` + appJSON("chat", "") + `,` + appJSON("chat", "") + `]}`,
		"bad id":             `{"apps":[` + appJSON("Chat App", "") + `]}`,
		"short key":          `{"apps":[{"id":"x","api_keys":["short"],"ticket_keys":["` + key + `"],"limits":{"connections":1,"channels_per_ticket":1,"request_bytes":1}}]}`,
		"no limits":          `{"apps":[{"id":"x","api_keys":["` + key + `"],"ticket_keys":["` + key + `"]}]}`,
		"bad schema":         `{"apps":[` + appJSON("chat", `,"client_events":{"events":{"typing":`+event(`{"type":7}`, 64)+`}}`) + `]}`,
		"remote schema":      `{"apps":[` + appJSON("chat", `,"client_events":{"events":{"typing":`+event(`{"$ref":"https://schemas.example/x.json"}`, 64)+`}}`) + `]}`,
		"huge event":         `{"apps":[` + appJSON("chat", `,"client_events":{"events":{"typing":`+event(`{}`, MaxEventBytes+1)+`}}`) + `]}`,
		"no rate":            `{"apps":[` + appJSON("chat", `,"client_events":{"events":{"typing":{"schema":{},"max_bytes":64}}}`) + `]}`,
		"reserved name":      `{"apps":[` + appJSON("chat", `,"client_events":{"events":{"phx_join":`+event(`{}`, 64)+`}}`) + `]}`,
		"channel bad event":  `{"apps":[` + appJSON("chat", `,"client_events":{"channels":{"conversations":{"events":{"typing":`+event(`{"type":7}`, 64)+`}}}}`) + `]}`,
		"channel unknown":    `{"apps":[` + appJSON("chat", `,"client_events":{"channels":{"conversations":{"typing":`+event(`{}`, 64)+`}}}`) + `]}`,
		"channel slash":      `{"apps":[` + appJSON("chat", `,"client_events":{"channels":{"a/b":{"events":{}}}}`) + `]}`,
		"channel empty name": `{"apps":[` + appJSON("chat", `,"client_events":{"channels":{"":{"events":{}}}}`) + `]}`,
		"channel null":       `{"apps":[` + appJSON("chat", `,"client_events":{"channels":{"conversations":null}}`) + `]}`,
		"presence url":       `{"apps":[` + appJSON("chat", `,"presence":{"url":"ftp://app","key":"`+key+`","grace_seconds":30}`) + `]}`,
		"presence key":       `{"apps":[` + appJSON("chat", `,"presence":{"url":"https://app/p","key":"short","grace_seconds":30}`) + `]}`,
		"grace":              `{"apps":[` + appJSON("chat", `,"presence":{"url":"https://app/p","key":"`+key+`","grace_seconds":-1}`) + `]}`,
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil || !strings.HasPrefix(err.Error(), "apps file") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func event(schema string, maxBytes int) string {
	return `{"schema":` + schema + `,"max_bytes":` + strconv.Itoa(maxBytes) + `,"per_second":2,"burst":4}`
}

func TestParseCompilesDeclaredEvents(t *testing.T) {
	schema := `{"type":"object","required":["thread"]}`
	apps, err := Parse([]byte(`{"apps":[` + appJSON("chat", `,"client_events":{"events":{"typing":`+event(schema, 64)+`}}`+
		`,"presence":{"url":"https://chat.example/presence","key":"`+key+`","grace_seconds":30}`) + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	typing := apps[0].ClientEvents.Events["typing"]
	if !typing.Accepts([]byte(`{"thread":"t1"}`)) || typing.Accepts([]byte(`{}`)) {
		t.Fatal("the schema was not applied")
	}
	if apps[0].Presence.GraceSeconds != 30 {
		t.Fatalf("got %+v", apps[0].Presence)
	}
}

func TestChannelEventsAreDeclaredPerResource(t *testing.T) {
	apps, err := Parse([]byte(`{"apps":[` + appJSON("chat", `,"client_events":{"events":{"ring":`+event(`{}`, 64)+`},`+
		`"channels":{"conversations":{"events":{"typing":`+event(`{"type":"object"}`, 128)+`}}}}`) + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	ce := apps[0].ClientEvents
	typing := ce.OnChannel("conversations/1", "typing")
	if typing == nil || typing.MaxBytes != 128 || !typing.Accepts([]byte(`{}`)) || typing.Accepts([]byte(`7`)) {
		t.Fatalf("conversations typing: %+v", typing)
	}
	refused := [][2]string{{"rooms/1", "typing"}, {"conversations", "typing"}, {"conversations/1", "ring"}}
	for _, c := range refused {
		if ce.OnChannel(c[0], c[1]) != nil {
			t.Errorf("%s on %s was declared", c[1], c[0])
		}
	}
	if ce.Direct("ring") == nil || ce.Direct("typing") != nil {
		t.Fatal("direct events mixed with channel events")
	}
}

func TestResourceIsTheChannelNameBeforeTheSlash(t *testing.T) {
	cases := map[string]string{"conversations/7": "conversations", "a/b/c": "a", "lobby": "", "/x": ""}
	for channel, want := range cases {
		if got := Resource(channel); got != want {
			t.Errorf("%s: got %q, want %q", channel, got, want)
		}
	}
}
