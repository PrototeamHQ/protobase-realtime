package socket

import (
	"encoding/json"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/names"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// session handles one socket's incoming frames.
type session struct {
	app  *hub.App
	conn *hub.Conn
	out  *sink
	now  func() time.Time
	// changes finds a changes service's client by id, nil when there is none.
	changes func(service string) *changes.Client
	live    *liveTopics
}

type interest struct {
	Viewing []string `json:"viewing"`
}

func (s *session) handle(f phoenix.Frame) {
	switch {
	case f.Topic == phoenix.SystemTopic && f.Event == phoenix.Heartbeat:
		s.out.Send(phoenix.Ok(f))
	case f.Topic == names.RelayTopic && f.Event == phoenix.Join:
		s.joinRelay(f)
	case names.LiveTopic(f.Topic) && f.Event == phoenix.Join:
		s.joinLive(f)
	case names.LiveTopic(f.Topic) && f.Event == phoenix.Leave:
		s.leaveLive(f)
	case f.Event == phoenix.Join:
		s.join(f)
	case !s.app.IsJoined(s.conn, f.Topic):
		s.out.Send(phoenix.Refuse(f, "unmatched topic"))
	case f.Event == phoenix.Leave:
		s.app.Leave(s.conn, f.Topic)
		s.out.Send(phoenix.Ok(f))
	case f.Topic == names.RelayTopic && f.Event == names.Presence:
		s.setPresence(f)
	case f.Topic == names.RelayTopic:
		s.direct(f)
	case f.Event == names.Interest:
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

func (s *session) joinRelay(f phoenix.Frame) {
	if s.app.JoinRelay(s.conn, f.JoinRef) != nil {
		s.out.Send(phoenix.Refuse(f, hub.RefusedReceiveOnly))
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
