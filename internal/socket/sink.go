package socket

import (
	"context"
	"sync"

	"github.com/coder/websocket"
)

// queueLength bounds the frames waiting for one socket; a socket that falls further behind is closed.
const queueLength = 256

// sink is a socket's outgoing side: frames are queued and written by one writer goroutine.
type sink struct {
	queue chan []byte
	done  chan struct{}
	once  sync.Once
	code  websocket.StatusCode
	why   string
	// abort cancels a write in progress, for a socket that stopped reading.
	abort context.CancelFunc
}

func newSink(abort context.CancelFunc) *sink {
	return &sink{queue: make(chan []byte, queueLength), done: make(chan struct{}), abort: abort}
}

func (s *sink) Send(frame []byte) {
	select {
	case s.queue <- frame:
	default:
		s.end(websocket.StatusTryAgainLater, "too slow")
		s.abort()
	}
}

// Close ends the socket with a normal closure, which Phoenix clients take as final and do not reconnect after.
func (s *sink) Close(reason string) { s.end(websocket.StatusNormalClosure, reason) }

// Restart ends the socket with going away, after which Phoenix clients reconnect.
func (s *sink) Restart() { s.end(websocket.StatusGoingAway, "relay restarting") }

func (s *sink) end(code websocket.StatusCode, reason string) {
	s.once.Do(func() {
		s.code, s.why = code, reason
		close(s.done)
	})
}

// write sends queued frames until the socket ends, then closes it with the code and reason it ended with.
func (s *sink) write(ctx context.Context, ws *websocket.Conn) {
	for {
		select {
		case frame := <-s.queue:
			if ws.Write(ctx, websocket.MessageText, frame) != nil {
				s.end(websocket.StatusInternalError, "write failed")
				ws.CloseNow()
				return
			}
		case <-s.done:
			ws.Close(s.code, s.why)
			return
		}
	}
}
