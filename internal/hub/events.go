package hub

import "strings"

// ClientMessageEvent is the one event a member's message is delivered under, so it never passes for an app publish.
const ClientMessageEvent = "client_message"

// InterestEvent is the event a client sends to say what it views in a joined channel.
const InterestEvent = "interest"

const maxEventLen = 64

// Reserved reports whether an event name belongs to the protocol or the relay, so neither apps nor clients may use it.
func Reserved(event string) bool {
	switch event {
	case "heartbeat", ClientMessageEvent, InterestEvent:
		return true
	}
	return strings.HasPrefix(event, "phx_")
}

// ValidEvent accepts a non-reserved event name of 1 to 64 bytes.
func ValidEvent(event string) bool {
	return event != "" && len(event) <= maxEventLen && !Reserved(event)
}
