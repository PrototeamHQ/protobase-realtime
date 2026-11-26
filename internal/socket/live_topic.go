package socket

import (
	"encoding/json"

	"github.com/PrototeamHQ/protobase-realtime/internal/live"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// Refusal reasons of a live query's join.
const (
	refusedLiveToken = "invalid_live_token"
	// refusedNoChanges answers a valid token while the relay routes no changes, so no join sits delivering nothing.
	refusedNoChanges = "changes_unavailable"
)

// liveJoin is a live query's join payload: the token its read returned.
type liveJoin struct {
	Live string `json:"live"`
}

func (s *session) joinLive(f phoenix.Frame) {
	var join liveJoin
	if json.Unmarshal(f.Payload, &join) != nil || join.Live == "" {
		s.out.Send(phoenix.Refuse(f, refusedLiveToken))
		return
	}
	socket := live.Socket{App: s.app.Config().ID, User: s.conn.User, Anonymous: s.conn.Anonymous,
		Session: s.conn.Session}
	if live.Open(join.Live, s.app.Config().TicketKeys, socket, s.now()) != nil {
		s.out.Send(phoenix.Refuse(f, refusedLiveToken))
		return
	}
	s.out.Send(phoenix.Refuse(f, refusedNoChanges))
}
