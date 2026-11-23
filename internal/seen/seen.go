// Package seen remembers keys for a fixed window, so a ticket or a publish id is accepted once.
package seen

import (
	"sync"
	"time"
)

type entry struct {
	key string
	at  time.Time
}

// Set holds each key for its window after first use. Entries are kept in insertion order, so pruning takes them
// from the front on each call; no timers run. Its size is bounded by the keys inserted in one window.
type Set struct {
	mu      sync.Mutex
	window  time.Duration
	order   []entry
	members map[string]struct{}
}

func New(window time.Duration) *Set {
	return &Set{window: window, members: map[string]struct{}{}}
}

// First records key and reports whether it was not already held in the window.
func (s *Set) First(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	if _, ok := s.members[key]; ok {
		return false
	}
	s.members[key] = struct{}{}
	s.order = append(s.order, entry{key, now})
	return true
}

// Holds reports whether key was recorded within the window, without recording it.
func (s *Set) Holds(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	_, ok := s.members[key]
	return ok
}

func (s *Set) prune(now time.Time) {
	cut := 0
	for cut < len(s.order) && now.Sub(s.order[cut].at) >= s.window {
		delete(s.members, s.order[cut].key)
		cut++
	}
	if cut == 0 {
		return
	}
	s.order = append(s.order[:0:0], s.order[cut:]...)
}

// Len reports how many keys are held, for tests.
func (s *Set) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.members)
}
