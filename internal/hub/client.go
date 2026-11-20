package hub

import (
	"encoding/json"
	"time"

	"golang.org/x/time/rate"

	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// Fixed limits on members' messages, summed over all of a person's sockets.
const (
	ClientMessageBytes = 4096
	clientRatePerSec   = 10
	clientBurst        = 20
)

// Refusal codes for a member's message, sent as the reply's reason.
const (
	RefusedReceiveOnly = "receive_only"
	RefusedDisabled    = "client_messages_disabled"
	RefusedNotJoined   = "not_joined"
	RefusedEvent       = "invalid_event"
	RefusedTooLarge    = "too_large"
	RefusedRate        = "rate_limited"
)

func newUserRate() *rate.Limiter { return rate.NewLimiter(clientRatePerSec, clientBurst) }

type sender struct {
	User       string `json:"user"`
	Connection string `json:"connection"`
}

// envelope stamps a member's message: the relay sets from, channel and at; the client's payload is carried as is.
type envelope struct {
	From    sender          `json:"from"`
	Channel string          `json:"channel"`
	At      int64           `json:"at"`
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

// Relay delivers a member's message to the channel's other joined sockets, including the sender's other sockets.
// It returns a refusal code, or "" once delivered.
func (a *App) Relay(c *Conn, channel, event string, payload json.RawMessage, now time.Time) string {
	if c.Anonymous {
		return RefusedReceiveOnly
	}
	if !a.Config.ClientMessages {
		return RefusedDisabled
	}
	if !ValidEvent(event) {
		return RefusedEvent
	}
	if len(payload) > ClientMessageBytes {
		return RefusedTooLarge
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := c.joined[channel]; !ok {
		return RefusedNotJoined
	}
	if !a.rates[c.User].AllowN(now, 1) {
		return RefusedRate
	}
	body, err := json.Marshal(envelope{From: sender{c.User, c.ID}, Channel: channel, At: now.UnixMilli(), Event: event,
		Payload: payload})
	if err != nil {
		panic("hub: envelope: " + err.Error())
	}
	frame := phoenix.Encode(phoenix.Frame{Topic: channel, Event: ClientMessageEvent, Payload: body})
	for other := range a.byChannel[channel] {
		if other != c {
			other.sink.Send(frame)
		}
	}
	return ""
}
