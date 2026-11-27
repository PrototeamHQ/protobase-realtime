// Package changestest replays the changes service's side of a relay session recorded from the real protobase-changes,
// for tests of the relay's connection. The sessions are in tests/relay-sessions.
package changestest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Key is the relay key the sessions were recorded with, 32 bytes of 8 as base64url; the replayed service takes only
// it.
const Key = "CAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAg"

// App is the app every recorded session is for.
const App = "shop"

// Frame is one message of a session, as it went: from the relay or from the service.
type Frame struct {
	From    string          `json:"from"`
	Message json.RawMessage `json:"message"`
}

type Session struct {
	Frames []Frame `json:"frames"`
}

// Register is a recorded register's content, for a test to seal a live token that registers the same.
type Register struct {
	ID        string          `json:"id"`
	Sid       string          `json:"sid"`
	Anonymous bool            `json:"anonymous"`
	Subs      json.RawMessage `json:"subs"`
	Fields    json.RawMessage `json:"fields"`
	Snapshot  string          `json:"snapshot"`
	Cursor    string          `json:"cursor"`
}

// Load reads a recorded session by name, such as "gap".
func Load(t *testing.T, name string) Session {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "tests", "relay-sessions", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Register returns the register the frame at i holds.
func (s Session) Register(t *testing.T, i int) Register {
	t.Helper()
	var r Register
	if err := json.Unmarshal(s.Frames[i].Message, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// Entry returns the entry for a recorded registration id in the transaction the frame at i holds.
func (s Session) Entry(t *testing.T, i int, id string) json.RawMessage {
	t.Helper()
	var event struct {
		Data struct {
			Registrations []json.RawMessage `json:"registrations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(s.Frames[i].Message, &event); err != nil {
		t.Fatal(err)
	}
	for _, raw := range event.Data.Registrations {
		var e struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		if e.ID == id {
			return raw
		}
	}
	t.Fatalf("frame %d holds no entry for %s", i, id)
	return nil
}
