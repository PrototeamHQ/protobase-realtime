package hub

import (
	"time"

	"golang.org/x/time/rate"

	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

// person is what the hub holds for someone signed in while they have a socket open. Guarded by the app's mutex.
type person struct {
	rules rules
	// rates holds one limiter per event the person sent, summed over all their sockets; anyEvent keys the shared
	// limiter of events let through by allow_any.
	rates map[string]*rate.Limiter
}

const anyEvent = ""

// allow takes one send of an event from the person's limiter for it.
func (p *person) allow(l limits, now time.Time) bool {
	if p.rates[l.key] == nil {
		p.rates[l.key] = rate.NewLimiter(rate.Limit(l.perSecond), l.burst)
	}
	return p.rates[l.key].AllowN(now, 1)
}

// rules filter which client events reach the person: never from someone they blocked, and with friendsOnly set,
// direct events only from friends.
type rules struct {
	blocked     set[string]
	friendsOnly bool
	friends     set[string]
}

func newRules(r ticket.Rules) rules {
	return rules{blocked: setOf(r.Blocked), friendsOnly: r.FriendsOnly, friends: setOf(r.Friends)}
}

func setOf(values []string) set[string] {
	s := set[string]{}
	for _, v := range values {
		s[v] = struct{}{}
	}
	return s
}

func (r rules) acceptsChannelEvent(from string) bool { return !has(r.blocked, from) }

func (r rules) acceptsDirectEvent(from string) bool {
	if has(r.blocked, from) {
		return false
	}
	return !r.friendsOnly || has(r.friends, from)
}

// accepts reports whether a client event from someone reaches a socket. Anonymous sessions have no rules.
func (a *App) accepts(c *Conn, from string, direct bool) bool {
	p := a.people[c.User]
	if c.Anonymous || p == nil {
		return true
	}
	if direct {
		return p.rules.acceptsDirectEvent(from)
	}
	return p.rules.acceptsChannelEvent(from)
}

// SetRules replaces a person's rules on their open sockets. Tickets issued later carry the app's own rules. It
// reports how many sockets the person has open.
func (a *App) SetRules(user string, r ticket.Rules) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.people[user]
	if p == nil {
		return 0
	}
	p.rules = newRules(r)
	return len(a.byUser[user])
}
