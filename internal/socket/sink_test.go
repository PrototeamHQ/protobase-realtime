package socket

import (
	"testing"

	"github.com/coder/websocket"
)

func TestSinkEndsASocketThatFallsBehind(t *testing.T) {
	aborted := false
	s := newSink(func() { aborted = true })
	for range queueLength + 1 {
		s.Send([]byte("x"))
	}
	if !aborted || s.code != websocket.StatusTryAgainLater {
		t.Fatalf("aborted %v, code %v", aborted, s.code)
	}
}

func TestSinkKeepsTheFirstEnding(t *testing.T) {
	s := newSink(func() {})
	s.Close("session closed")
	s.Restart()
	if s.code != websocket.StatusNormalClosure || s.why != "session closed" {
		t.Fatalf("got %v %q", s.code, s.why)
	}
}
