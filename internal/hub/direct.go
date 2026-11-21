package hub

import (
	"encoding/json"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/names"
)

// Direct sends a client event to one person's sockets that joined the relay topic, the sender's other sockets
// included when they send to themselves. The sender must share a channel with one of the person's open sockets,
// else it is refused as unreachable, which is also the answer when the person has no socket open. A person who
// blocked the sender, or takes direct events from friends only, gets nothing, and the sender is not told.
func (a *App) Direct(c *Conn, to, event string, payload json.RawMessage, now time.Time) string {
	l, code := a.admit(c, event, payload)
	if code != "" {
		return code
	}
	if code := a.spend(c, names.RelayTopic, l, now); code != "" {
		return code
	}
	if !l.accepts(payload) {
		return RefusedPayload
	}
	frame := encodeEvent(names.RelayTopic, envelope{From: sender{c.User, c.ID}, To: to, At: now.UnixMilli(),
		Event: event, Payload: payload})
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.sharesChannel(c, to) {
		return RefusedUnreachable
	}
	for other := range a.byUser[to] {
		if _, ok := other.joined[names.RelayTopic]; !ok || other == c {
			continue
		}
		if a.accepts(other, c.User, true) {
			other.sink.Send(frame)
		}
	}
	return ""
}

// sharesChannel reports whether a socket's channel list meets the channel list of any of a person's open sockets.
func (a *App) sharesChannel(c *Conn, user string) bool {
	for other := range a.byUser[user] {
		for channel := range c.allowed {
			if has(other.allowed, channel) {
				return true
			}
		}
	}
	return false
}
