package seen

import (
	"testing"
	"time"
)

func TestFirstAcceptsAKeyOncePerWindow(t *testing.T) {
	s := New(time.Minute)
	t0 := time.Unix(1000, 0)
	if !s.First("a", t0) {
		t.Fatal("first use refused")
	}
	if s.First("a", t0.Add(59*time.Second)) {
		t.Fatal("repeat inside the window accepted")
	}
	if !s.First("a", t0.Add(time.Minute)) {
		t.Fatal("repeat after the window refused")
	}
}

func TestHoldsDoesNotRecord(t *testing.T) {
	s := New(time.Minute)
	t0 := time.Unix(1000, 0)
	if s.Holds("a", t0) {
		t.Fatal("held an unknown key")
	}
	s.First("a", t0)
	if !s.Holds("a", t0) || s.Holds("a", t0.Add(time.Minute)) {
		t.Fatal("wrong verdict")
	}
}

func TestPruneDropsExpiredKeys(t *testing.T) {
	s := New(time.Second)
	t0 := time.Unix(1000, 0)
	for _, k := range []string{"a", "b", "c"} {
		s.First(k, t0)
	}
	s.First("d", t0.Add(2*time.Second))
	if s.Len() != 1 {
		t.Fatalf("held %d keys, want 1", s.Len())
	}
}
