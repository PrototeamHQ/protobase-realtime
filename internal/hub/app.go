// Package hub holds one app's connected sockets and routes messages between them. Apps never share a hub, so
// equal channel names in two apps never meet.
package hub

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/seen"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

// Sink is a socket's outgoing side. Send queues a frame without blocking. Close ends the socket for good, as when
// the app closes its session; Restart ends it so the client reconnects, as on shutdown.
type Sink interface {
	Send(frame []byte)
	Close(reason string)
	Restart()
}

var (
	ErrFull            = errors.New("the app has reached its connection limit")
	ErrTooManyChannels = errors.New("the ticket lists more channels than the app's limit")
	ErrSessionClosed   = errors.New("the app closed the ticket's session")
	ErrAppRemoved      = errors.New("the app is no longer served")
)

type set[T comparable] map[T]struct{}

func add[K, T comparable](index map[K]set[T], key K, value T) {
	if index[key] == nil {
		index[key] = set[T]{}
	}
	index[key][value] = struct{}{}
}

func remove[K, T comparable](index map[K]set[T], key K, value T) {
	delete(index[key], value)
	if len(index[key]) == 0 {
		delete(index, key)
	}
}

// App is one app's hub. One mutex guards its indexes and the membership state of its connections.
type App struct {
	// settings are swapped whole when the apps file is reloaded.
	settings atomic.Pointer[settings]

	mu        sync.Mutex
	conns     set[*Conn]
	bySession map[string]set[*Conn]
	byUser    map[string]set[*Conn]
	byChannel map[string]set[*Conn]
	people    map[string]*person
	presence  Presence
	// closed remembers sessions the app closed for a ticket's lifetime, so a ticket issued before the close
	// cannot reopen one.
	closed *seen.Set
	// removed is set once a reload dropped the app, so a socket whose ticket was verified before cannot join it.
	removed bool
}

// Presence hears how many of a person's sockets are online and away after each change. The hub calls it under its
// mutex, so it must not call back into the hub.
type Presence interface {
	Change(user string, online, away int)
}

// NewApp makes an app's hub; presence is nil when the app has no presence reports.
func NewApp(cfg config.App, presence Presence) *App {
	a := &App{
		conns:     set[*Conn]{},
		bySession: map[string]set[*Conn]{},
		byUser:    map[string]set[*Conn]{},
		byChannel: map[string]set[*Conn]{},
		people:    map[string]*person{},
		presence:  presence,
		closed:    seen.New(ticket.MaxLifetime),
	}
	a.settings.Store(newSettings(cfg))
	return a
}

// Register adds a socket opened with a verified ticket.
func (a *App) Register(t ticket.Ticket, sink Sink, now time.Time) (*Conn, error) {
	limits := a.Config().Limits
	if len(t.Channels) > limits.ChannelsPerTicket {
		return nil, ErrTooManyChannels
	}
	// The closed set is read under the mutex CloseSession holds while it marks a session, so a socket either
	// sees the close or is registered before it and gets closed.
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.removed {
		return nil, ErrAppRemoved
	}
	if a.closed.Holds(t.Session, now) {
		return nil, ErrSessionClosed
	}
	if len(a.conns) >= limits.Connections {
		return nil, ErrFull
	}
	c := &Conn{ID: newConnID(), User: t.User, Anonymous: t.Anonymous, Session: t.Session, sink: sink,
		allowed: set[string]{}, joined: map[string]*joined{}}
	for _, channel := range t.Channels {
		c.allowed[channel] = struct{}{}
	}
	a.conns[c] = struct{}{}
	add(a.bySession, c.Session, c)
	if c.Anonymous {
		return c, nil
	}
	add(a.byUser, c.User, c)
	if a.people[c.User] == nil {
		a.people[c.User] = &person{rates: map[rateKey]*rate.Limiter{}}
	}
	// The newest ticket carries the app's newest rules for the person.
	a.people[c.User].rules = newRules(t.Rules)
	a.reportPresence(c.User)
	return c, nil
}

// Unregister drops a closed socket. A person's rules and rate limiters go with their last socket.
func (a *App) Unregister(c *Conn) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.conns[c]; !ok {
		return
	}
	delete(a.conns, c)
	remove(a.bySession, c.Session, c)
	for channel := range c.joined {
		remove(a.byChannel, channel, c)
	}
	// A closed socket holds no membership, so a late call on it joins or reaches nothing.
	c.joined = map[string]*joined{}
	c.allowed = set[string]{}
	if c.Anonymous {
		return
	}
	remove(a.byUser, c.User, c)
	if a.byUser[c.User] == nil {
		delete(a.people, c.User)
	}
	a.reportPresence(c.User)
}

// Connections reports how many sockets are open.
func (a *App) Connections() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.conns)
}

func newConnID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}
