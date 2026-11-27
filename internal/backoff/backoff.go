// Package backoff paces retries after failures: presence reports and the connections to changes services wait alike.
package backoff

import "time"

const (
	// Min is the wait after the first failure.
	Min = time.Second
	// Max caps the wait after failures.
	Max = time.Minute
)

// Backoff is the wait after failures: it doubles with each failure, from Min up to Max.
type Backoff struct {
	wait time.Duration
	// stuck is set once a try failed after a wait at the ceiling, until one succeeds.
	stuck bool
}

// Fail returns the wait before the next try, and whether this failure is the first one after a wait at the ceiling
// since the last success, the one worth telling about.
func (b *Backoff) Fail() (time.Duration, bool) {
	first := b.wait == Max && !b.stuck
	if first {
		b.stuck = true
	}
	b.wait = min(max(2*b.wait, Min), Max)
	return b.wait, first
}

func (b *Backoff) Succeed() {
	b.wait, b.stuck = 0, false
}
