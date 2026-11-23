package hub

import (
	"errors"

	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

const (
	MaxInterestKeys   = 32
	MaxInterestKeyLen = 128
)

var ErrNotMember = errors.New("not a member of the channel")

// Join joins a socket to a channel its person belongs to. Joining again replaces the join reference.
func (a *App) Join(c *Conn, channel string, ref *string, viewing []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := c.allowed[channel]; !ok {
		return ErrNotMember
	}
	c.joined[channel] = &joined{ref: ref, interest: interestSet(viewing)}
	add(a.byChannel, channel, c)
	return nil
}

// Leave leaves a channel; leaving one not joined does nothing.
func (a *App) Leave(c *Conn, channel string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(c.joined, channel)
	remove(a.byChannel, channel, c)
}

// IsJoined reports whether the socket has joined the channel.
func (a *App) IsJoined(c *Conn, channel string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := c.joined[channel]
	return ok
}

// SetInterest replaces what a socket views in a joined channel. Nil viewing declares nothing and filters nothing.
func (a *App) SetInterest(c *Conn, channel string, viewing []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	j, ok := c.joined[channel]
	if !ok {
		return ErrNotMember
	}
	j.interest = interestSet(viewing)
	return nil
}

func interestSet(viewing []string) set[string] {
	if viewing == nil {
		return nil
	}
	s := set[string]{}
	for _, key := range viewing {
		s[key] = struct{}{}
	}
	return s
}

// ValidInterest bounds what a client may declare.
func ValidInterest(viewing []string) bool {
	if len(viewing) > MaxInterestKeys {
		return false
	}
	for _, key := range viewing {
		if key == "" || len(key) > MaxInterestKeyLen {
			return false
		}
	}
	return true
}

// Target picks people's sockets for a membership change: by person, or by session for anonymous sessions too.
type Target struct {
	Users    []string
	Sessions []string
}

func (a *App) targets(t Target) set[*Conn] {
	out := set[*Conn]{}
	for _, user := range t.Users {
		for c := range a.byUser[user] {
			out[c] = struct{}{}
		}
	}
	for _, session := range t.Sessions {
		for c := range a.bySession[session] {
			out[c] = struct{}{}
		}
	}
	return out
}

// AddToChannel lets the targets' open sockets join a channel. It reports how many sockets it reached.
func (a *App) AddToChannel(channel string, t Target) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	conns := a.targets(t)
	for c := range conns {
		c.allowed[channel] = struct{}{}
	}
	return len(conns)
}

// RemoveFromChannel takes a channel from the targets' open sockets and closes it on those that joined it.
func (a *App) RemoveFromChannel(channel string, t Target) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	conns := a.targets(t)
	for c := range conns {
		delete(c.allowed, channel)
		j, ok := c.joined[channel]
		if !ok {
			continue
		}
		delete(c.joined, channel)
		remove(a.byChannel, channel, c)
		c.sink.Send(phoenix.Encode(phoenix.Frame{JoinRef: j.ref, Ref: j.ref, Topic: channel, Event: phoenix.Close}))
	}
	return len(conns)
}
