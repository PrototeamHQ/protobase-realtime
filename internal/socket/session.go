package socket

import (
	"encoding/json"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// session handles one socket's incoming frames.
type session struct {
	app  *hub.App
	conn *hub.Conn
	out  *sink
	now  func() time.Time
}

type interest struct {
	Viewing []string `json:"viewing"`
}

func (s *session) handle(f phoenix.Frame) {
	switch {
	case f.Topic == phoenix.SystemTopic && f.Event == phoenix.Heartbeat:
		s.out.Send(phoenix.Ok(f))
	case f.Event == phoenix.Join:
		s.join(f)
	case !s.app.IsJoined(s.conn, f.Topic):
		s.out.Send(phoenix.Refuse(f, "unmatched topic"))
	case f.Event == phoenix.Leave:
		s.app.Leave(s.conn, f.Topic)
		s.out.Send(phoenix.Ok(f))
	case f.Event == hub.InterestEvent:
		s.setInterest(f)
	default:
		s.relay(f)
	}
}

func (s *session) join(f phoenix.Frame) {
	viewing, ok := readInterest(f.Payload)
	if !ok {
		s.out.Send(phoenix.Refuse(f, "invalid_interest"))
		return
	}
	if s.app.Join(s.conn, f.Topic, f.JoinRef, viewing) != nil {
		s.out.Send(phoenix.Refuse(f, "unauthorized"))
		return
	}
	s.out.Send(phoenix.Ok(f))
}

func (s *session) setInterest(f phoenix.Frame) {
	viewing, ok := readInterest(f.Payload)
	if !ok {
		s.out.Send(phoenix.Refuse(f, "invalid_interest"))
		return
	}
	if s.app.SetInterest(s.conn, f.Topic, viewing) != nil {
		s.out.Send(phoenix.Refuse(f, "unmatched topic"))
		return
	}
	s.out.Send(phoenix.Ok(f))
}

func (s *session) relay(f phoenix.Frame) {
	if code := s.app.Relay(s.conn, f.Topic, f.Event, f.Payload, s.now()); code != "" {
		s.out.Send(phoenix.Refuse(f, code))
		return
	}
	s.out.Send(phoenix.Ok(f))
}

// readInterest reads an optional viewing list. A payload without one declares nothing.
func readInterest(payload json.RawMessage) ([]string, bool) {
	var in interest
	if json.Unmarshal(payload, &in) != nil {
		return nil, false
	}
	return in.Viewing, hub.ValidInterest(in.Viewing)
}
