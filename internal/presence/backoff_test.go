package presence

import (
	"testing"
	"time"
)

func TestBackoffDoublesUpToTheCeilingAndTellsOncePastIt(t *testing.T) {
	var b backoff
	var waits []time.Duration
	told := 0
	for range 10 {
		wait, tell := b.fail()
		waits = append(waits, wait)
		if tell {
			told++
			if len(waits) != 8 {
				t.Fatalf("told after failure %d, want 8, the first after a wait of a minute", len(waits))
			}
		}
	}
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60, 60}
	for i, w := range want {
		if waits[i] != w*time.Second {
			t.Fatalf("got %v", waits)
		}
	}
	if told != 1 {
		t.Fatalf("told %d times", told)
	}
	b.succeed()
	if wait, tell := b.fail(); wait != Tick || tell {
		t.Fatalf("a success did not reset the backoff: %v %v", wait, tell)
	}
}
