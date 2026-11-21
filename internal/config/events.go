package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// MaxEventBytes caps any declared event's max_bytes, so one frame stays small.
const MaxEventBytes = 65536

// ClientEvents lists the events signed-in clients may send each other. An event not listed is refused, unless
// AllowAny lets every valid event name through with the relay's fixed limits (not recommended).
type ClientEvents struct {
	AllowAny bool              `json:"allow_any"`
	Events   map[string]*Event `json:"events"`
}

// Event is one declared client event: its payload must match Schema (a JSON schema, draft 2020-12 unless it names
// another), stay within MaxBytes, and a person may send it PerSecond times a second, in bursts of Burst, over all
// their sockets.
type Event struct {
	Schema    json.RawMessage `json:"schema"`
	MaxBytes  int             `json:"max_bytes"`
	PerSecond float64         `json:"per_second"`
	Burst     int             `json:"burst"`

	compiled *jsonschema.Schema
}

// Compile checks the event's limits and compiles its schema. A schema can reference only itself: the compiler
// loads nothing from files or the network.
func (e *Event) Compile() error {
	if e.MaxBytes <= 0 || e.MaxBytes > MaxEventBytes {
		return fmt.Errorf("max_bytes must be 1 to %d", MaxEventBytes)
	}
	if e.PerSecond <= 0 || e.Burst <= 0 {
		return fmt.Errorf("per_second and burst must be positive")
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(e.Schema))
	if err != nil {
		return fmt.Errorf("schema is not JSON")
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource("event.json", doc); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	compiled, err := c.Compile("event.json")
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	e.compiled = compiled
	return nil
}

// Accepts reports whether a payload matches the event's schema. The payload is already known to be JSON.
func (e *Event) Accepts(payload []byte) bool {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return false
	}
	return e.compiled.Validate(doc) == nil
}

func validateClientEvents(ce *ClientEvents, validName func(string) bool) error {
	for name, event := range ce.Events {
		if !validName(name) {
			return fmt.Errorf("client_events: event %q must be 1 to 64 bytes and not reserved", name)
		}
		if event == nil {
			return fmt.Errorf("client_events: event %q is empty", name)
		}
		if err := event.Compile(); err != nil {
			return fmt.Errorf("client_events: event %q: %w", name, err)
		}
	}
	return nil
}
