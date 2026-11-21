package hub

import (
	"encoding/json"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/names"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// Fixed limits on the events an app lets through with allow_any, summed over all of a person's sockets.
const (
	AnyEventBytes = 4096
	anyPerSecond  = 10
	anyBurst      = 20
)

// Refusal codes for a client event, sent as the reply's reason.
const (
	RefusedReceiveOnly = "receive_only"
	RefusedNotAllowed  = "event_not_allowed"
	RefusedEvent       = "invalid_event"
	RefusedTooLarge    = "too_large"
	RefusedPayload     = "invalid_payload"
	RefusedNotJoined   = "not_joined"
	RefusedUnreachable = "unreachable"
	RefusedRate        = "rate_limited"
)

// rateKey names the limiter a client event spends from: the channel resource it was declared for ("" for a direct
// event) and its name. The zero key is the shared limiter of events let through by allow_any.
type rateKey struct {
	resource string
	event    string
}

// limits are what a client event is held to. event is nil for one let through by allow_any.
type limits struct {
	key       rateKey
	perSecond float64
	burst     int
	event     *config.Event
}

// accepts checks the payload against the declared event's schema.
func (l limits) accepts(payload json.RawMessage) bool {
	return l.event == nil || l.event.Accepts(payload)
}

// admitChannel admits an event on a channel against the events declared for the channel's resource.
func (a *App) admitChannel(c *Conn, channel, event string, payload json.RawMessage) (limits, string) {
	var declared *config.Event
	if a.Config.ClientEvents != nil {
		declared = a.Config.ClientEvents.OnChannel(channel, event)
	}
	return a.admit(c, rateKey{resource: config.Resource(channel), event: event}, declared, payload)
}

// admitDirect admits a direct event against the app's direct events.
func (a *App) admitDirect(c *Conn, event string, payload json.RawMessage) (limits, string) {
	var declared *config.Event
	if a.Config.ClientEvents != nil {
		declared = a.Config.ClientEvents.Direct(event)
	}
	return a.admit(c, rateKey{event: event}, declared, payload)
}

// admit checks what a client event's sender, name and payload size alone decide: it returns the limits the event is
// held to, or a refusal code. declared is the event's declaration where it is sent, or nil. The schema is checked
// only once the send is taken from the rate, so a flood of events costs no more than reading them.
func (a *App) admit(c *Conn, key rateKey, declared *config.Event, payload json.RawMessage) (limits, string) {
	if c.Anonymous {
		return limits{}, RefusedReceiveOnly
	}
	if !names.ValidEvent(key.event) {
		return limits{}, RefusedEvent
	}
	if declared == nil {
		return a.admitAny(payload)
	}
	if len(payload) > declared.MaxBytes {
		return limits{}, RefusedTooLarge
	}
	return limits{key: key, perSecond: declared.PerSecond, burst: declared.Burst, event: declared}, ""
}

func (a *App) admitAny(payload json.RawMessage) (limits, string) {
	if a.Config.ClientEvents == nil || !a.Config.ClientEvents.AllowAny {
		return limits{}, RefusedNotAllowed
	}
	if len(payload) > AnyEventBytes {
		return limits{}, RefusedTooLarge
	}
	return limits{perSecond: anyPerSecond, burst: anyBurst}, ""
}

// MaxEventBytes is the largest payload the app lets a client send, so the socket can bound what it reads.
func (a *App) MaxEventBytes() int {
	largest := 0
	if a.Config.ClientEvents == nil {
		return largest
	}
	if a.Config.ClientEvents.AllowAny {
		largest = AnyEventBytes
	}
	a.Config.ClientEvents.Declared(func(e *config.Event) {
		largest = max(largest, e.MaxBytes)
	})
	return largest
}

type sender struct {
	User       string `json:"user"`
	Connection string `json:"connection"`
}

// envelope stamps a client event: the relay sets from, channel (for a channel event), to (for a direct event) and
// at; the client's payload is carried as is.
type envelope struct {
	From    sender          `json:"from"`
	Channel string          `json:"channel,omitempty"`
	To      string          `json:"to,omitempty"`
	At      int64           `json:"at"`
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

func encodeEvent(topic string, env envelope) []byte {
	body, err := json.Marshal(env)
	if err != nil {
		panic("hub: envelope: " + err.Error())
	}
	return phoenix.Encode(phoenix.Frame{Topic: topic, Event: names.ClientEvent, Payload: body})
}

// spend checks that the socket joined the topic and takes one send from the person's rate. Refused events count
// against the rate too.
func (a *App) spend(c *Conn, topic string, l limits, now time.Time) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := c.joined[topic]; !ok {
		return RefusedNotJoined
	}
	if !a.people[c.User].allow(l, now) {
		return RefusedRate
	}
	return ""
}

// Relay delivers a channel event to the channel's other joined sockets, including the sender's other sockets, but
// not to people who blocked the sender. It returns a refusal code, or "" once delivered.
func (a *App) Relay(c *Conn, channel, event string, payload json.RawMessage, now time.Time) string {
	l, code := a.admitChannel(c, channel, event, payload)
	if code != "" {
		return code
	}
	if code := a.spend(c, channel, l, now); code != "" {
		return code
	}
	if !l.accepts(payload) {
		return RefusedPayload
	}
	frame := encodeEvent(channel, envelope{From: sender{c.User, c.ID}, Channel: channel, At: now.UnixMilli(),
		Event: event, Payload: payload})
	a.mu.Lock()
	defer a.mu.Unlock()
	for other := range a.byChannel[channel] {
		if other != c && a.accepts(other, c.User, false) {
			other.sink.Send(frame)
		}
	}
	return ""
}
