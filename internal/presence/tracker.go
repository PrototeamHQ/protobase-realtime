// Package presence tells an app when its people come online, go away or go offline, combined over each person's
// sockets. The relay reports facts only: what they mean, and who else hears of them, is the app's to decide.
package presence

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

type Status string

const (
	Online  Status = "online"
	Away    Status = "away"
	Offline Status = "offline"
)

// Change is one person's status, and since when it holds.
type Change struct {
	User   string `json:"user"`
	Status Status `json:"status"`
	At     int64  `json:"at"`
}

// person is someone with a socket open, or whose last change the app has not acknowledged yet.
type person struct {
	sockets int
	status  Status
	since   time.Time
	// goneAt is when the last socket closed; the person goes offline a grace period later.
	goneAt time.Time
	// reported is the status the app acknowledged last. Someone never reported counts as offline.
	reported Status
}

// Tracker holds each person's status. Its size is bounded: people with a socket open count against the app's
// connection limit, and at most as many people again wait to be reported offline; past that, the one who left first
// is dropped unreported.
type Tracker struct {
	mu       sync.Mutex
	grace    time.Duration
	debounce time.Duration
	limit    int
	now      func() time.Time
	people   map[string]*person
}

// NewTracker reports a status once it has held for debounce, and offline once a person had no socket for grace.
func NewTracker(grace, debounce time.Duration, limit int, now func() time.Time) *Tracker {
	return &Tracker{grace: grace, debounce: debounce, limit: limit, now: now, people: map[string]*person{}}
}

// Configure takes a changed grace period and connection limit, from the next change or report on.
func (t *Tracker) Configure(grace time.Duration, limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.grace, t.limit = grace, limit
}

// Change records how many of a person's sockets are online and away.
func (t *Tracker) Change(user string, online, away int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	p := t.people[user]
	if p == nil {
		t.makeRoom()
		p = &person{status: Offline, reported: Offline}
		t.people[user] = p
	}
	p.sockets = online + away
	switch {
	case online > 0:
		p.set(Online, now)
	case away > 0:
		p.set(Away, now)
	default:
		p.goneAt = now
	}
}

func (p *person) set(s Status, now time.Time) {
	if p.status == s {
		return
	}
	p.status, p.since = s, now
}

// makeRoom drops the person who left first when the tracker is full.
func (t *Tracker) makeRoom() {
	if len(t.people) < 2*t.limit {
		return
	}
	var first string
	for user, p := range t.people {
		if p.sockets == 0 && (first == "" || p.goneAt.Before(t.people[first].goneAt)) {
			first = user
		}
	}
	delete(t.people, first)
}

// Due returns up to max changes that held for the debounce and differ from what the app acknowledged, the oldest
// first. It forgets people who are offline and reported so.
func (t *Tracker) Due(max int) []Change {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	var due []Change
	for user, p := range t.people {
		if p.sockets == 0 && p.status != Offline && now.Sub(p.goneAt) >= t.grace {
			p.set(Offline, p.goneAt.Add(t.grace))
		}
		if p.status == p.reported {
			if p.status == Offline {
				delete(t.people, user)
			}
			continue
		}
		if now.Sub(p.since) >= t.debounce {
			due = append(due, Change{User: user, Status: p.status, At: p.since.UnixMilli()})
		}
	}
	slices.SortFunc(due, func(a, b Change) int {
		return cmp.Or(cmp.Compare(a.At, b.At), cmp.Compare(a.User, b.User))
	})
	return due[:min(len(due), max)]
}

// Acknowledge records the changes the app accepted. A person whose status moved on since is reported again.
func (t *Tracker) Acknowledge(changes []Change) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range changes {
		p := t.people[c.User]
		if p == nil {
			continue
		}
		p.reported = c.Status
		if p.sockets == 0 && p.status == Offline && p.reported == Offline {
			delete(t.people, c.User)
		}
	}
}
