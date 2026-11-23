package hub

import (
	"errors"

	"github.com/PrototeamHQ/protobase-realtime/internal/names"
)

var ErrReceiveOnly = errors.New("an anonymous session has no relay topic")

// JoinRelay joins a signed-in person's socket to the relay topic, where direct events arrive and presence is said.
func (a *App) JoinRelay(c *Conn, ref *string) error {
	if c.Anonymous {
		return ErrReceiveOnly
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.conns[c]; !ok {
		return ErrNotMember
	}
	// The relay topic is kept apart from byChannel, so nothing the app publishes reaches it.
	c.joined[names.RelayTopic] = &joined{ref: ref}
	return nil
}

// SetAway marks a socket away, as a client says before it is suspended, or back online.
func (a *App) SetAway(c *Conn, away bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.conns[c]; !ok {
		return
	}
	c.away = away
	a.reportPresence(c.User)
}

// reportPresence tells the app's presence how many of a person's sockets are online and away. Called under the
// mutex.
func (a *App) reportPresence(user string) {
	if a.presence == nil {
		return
	}
	online, away := 0, 0
	for c := range a.byUser[user] {
		if c.away {
			away++
			continue
		}
		online++
	}
	a.presence.Change(user, online, away)
}
