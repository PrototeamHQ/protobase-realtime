package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// MaxEventBytes caps any declared event's max_bytes, so one frame stays small.
const MaxEventBytes = 65536

// ClientEvents lists the events signed-in clients may send each other: Events on the relay topic, as direct events,
// and Channels by the resource a channel belongs to. An event not listed is refused, unless AllowAny lets every
// valid event name through with the relay's fixed limits (not recommended).
type ClientEvents struct {
	AllowAny bool                      `json:"allow_any"`
	Events   map[string]*Event         `json:"events"`
	Channels map[string]*ChannelEvents `json:"channels"`
}

// ChannelEvents lists the events clients may send on the channels of one resource.
type ChannelEvents struct {
	Events map[string]*Event `json:"events"`
}

// Resource is the part of a channel's name before the first "/", as in "conversations/7", or "" when the name has
// no "/".
func Resource(channel string) string {
	resource, _, found := strings.Cut(channel, "/")
	if !found {
		return ""
	}
	return resource
}

// Direct returns the declared direct event of that name, or nil.
func (ce *ClientEvents) Direct(event string) *Event {
	return ce.Events[event]
}

// OnChannel returns the event of that name declared for the channel's resource, or nil. A channel without "/"
// belongs to no resource, so nothing is declared for it.
func (ce *ClientEvents) OnChannel(channel, event string) *Event {
	declared := ce.Channels[Resource(channel)]
	if declared == nil {
		return nil
	}
	return declared.Events[event]
}

// Declared calls fn for every declared event, direct and per channel resource.
func (ce *ClientEvents) Declared(fn func(*Event)) {
	for _, e := range ce.Events {
		fn(e)
	}
	for _, channel := range ce.Channels {
		for _, e := range channel.Events {
			fn(e)
		}
	}
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
	if err := validateEvents("client_events", ce.Events, validName); err != nil {
		return err
	}
	for resource, channel := range ce.Channels {
		if resource == "" || strings.Contains(resource, "/") {
			return fmt.Errorf("client_events: channel resource %q must be non-empty and hold no /", resource)
		}
		if channel == nil {
			return fmt.Errorf("client_events: channel resource %q is empty", resource)
		}
		if err := validateEvents("client_events: channel resource "+strconv.Quote(resource), channel.Events, validName); err != nil {
			return err
		}
	}
	return nil
}

func validateEvents(where string, events map[string]*Event, validName func(string) bool) error {
	for name, event := range events {
		if !validName(name) {
			return fmt.Errorf("%s: event %q must be 1 to 64 bytes and not reserved", where, name)
		}
		if event == nil {
			return fmt.Errorf("%s: event %q is empty", where, name)
		}
		if err := event.Compile(); err != nil {
			return fmt.Errorf("%s: event %q: %w", where, name, err)
		}
	}
	return nil
}
