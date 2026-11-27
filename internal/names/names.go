// Package names holds the event and topic names the protocol and the relay reserve, so neither apps nor clients may
// use them for their own.
package names

import "strings"

const (
	// ClientEvent is the one event a client's event is delivered under, so it never passes for an app publish.
	ClientEvent = "client_event"
	// Interest is the event a client sends to say what it views in a joined channel.
	Interest = "interest"
	// Presence is the event a client sends on the relay topic to say whether it is away.
	Presence = "presence"
	// RelayTopic is the relay's own topic: a signed-in client joins it to send and receive direct events and to
	// report presence.
	RelayTopic = "relay"
	// SystemTopic is the Phoenix protocol's own topic, which carries heartbeats.
	SystemTopic = "phoenix"
	// LivePrefix starts the topics of live queries: a client joins one per query with the live token of its read.
	LivePrefix = "live:"

	maxEventLen = 64
)

// ReservedEvent reports whether an event name belongs to the protocol or the relay.
func ReservedEvent(event string) bool {
	switch event {
	case "heartbeat", ClientEvent, Interest, Presence:
		return true
	}
	return strings.HasPrefix(event, "phx_")
}

// ValidEvent accepts a non-reserved event name of 1 to 64 bytes.
func ValidEvent(event string) bool {
	return event != "" && len(event) <= maxEventLen && !ReservedEvent(event)
}

// ReservedTopic reports whether a topic belongs to the protocol or the relay, so it is never an app's channel.
func ReservedTopic(topic string) bool {
	return topic == RelayTopic || topic == SystemTopic || LiveTopic(topic)
}

// LiveTopic reports whether a topic is a live query's.
func LiveTopic(topic string) bool {
	return strings.HasPrefix(topic, LivePrefix)
}
