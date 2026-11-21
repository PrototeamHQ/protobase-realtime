package presence

import "time"

// backoff is the wait after failed reports: it doubles with each failure, from a tick up to maxBackoff.
type backoff struct {
	wait time.Duration
	// stuck is set once a report failed after a wait at the ceiling, until a report succeeds.
	stuck bool
}

// fail returns the wait before the next try, and whether this failure is the first one after a wait at the ceiling
// since the last success, the one worth telling about.
func (b *backoff) fail() (time.Duration, bool) {
	first := b.wait == maxBackoff && !b.stuck
	if first {
		b.stuck = true
	}
	b.wait = min(max(2*b.wait, Tick), maxBackoff)
	return b.wait, first
}

func (b *backoff) succeed() {
	b.wait, b.stuck = 0, false
}
