package socket

import (
	"encoding/json"

	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// presenceSay is what a client says about itself on the relay topic.
type presenceSay struct {
	Status string `json:"status"`
}

func (s *session) setPresence(f phoenix.Frame) {
	var say presenceSay
	if json.Unmarshal(f.Payload, &say) != nil || (say.Status != "away" && say.Status != "online") {
		s.out.Send(phoenix.Refuse(f, "invalid_presence"))
		return
	}
	s.app.SetAway(s.conn, say.Status == "away")
	s.out.Send(phoenix.Ok(f))
}

// directEvent wraps a direct event's payload with its recipient.
type directEvent struct {
	To      string          `json:"to"`
	Payload json.RawMessage `json:"payload"`
}

func (s *session) direct(f phoenix.Frame) {
	var d directEvent
	if json.Unmarshal(f.Payload, &d) != nil || d.To == "" || !isObject(d.Payload) {
		s.out.Send(phoenix.Refuse(f, "invalid_direct"))
		return
	}
	if code := s.app.Direct(s.conn, d.To, f.Event, d.Payload, s.now()); code != "" {
		s.out.Send(phoenix.Refuse(f, code))
		return
	}
	s.out.Send(phoenix.Ok(f))
}

func isObject(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && m != nil
}
