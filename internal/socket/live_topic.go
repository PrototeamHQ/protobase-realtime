package socket

import (
	"encoding/json"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/live"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// Refusal reasons of a live query's join.
const (
	refusedLiveToken = "invalid_live_token"
	// refusedNoChanges answers a valid token while the app names no changes service, so no join sits delivering
	// nothing.
	refusedNoChanges = "changes_unavailable"
	refusedCursor    = "invalid_cursor"
	refusedTooMany   = "too_many_live_queries"
)

// maxCursorBytes bounds the cursor a join resumes after; the service's cursors are far shorter.
const maxCursorBytes = 512

// liveJoin is a live query's join payload: the token its read returned, and the cursor of the last change the client
// got, to resume after it.
type liveJoin struct {
	Live   string `json:"live"`
	Cursor string `json:"cursor"`
}

func (s *session) joinLive(f phoenix.Frame) {
	var join liveJoin
	if json.Unmarshal(f.Payload, &join) != nil || join.Live == "" {
		s.out.Send(phoenix.Refuse(f, refusedLiveToken))
		return
	}
	if len(join.Cursor) > maxCursorBytes {
		s.out.Send(phoenix.Refuse(f, refusedCursor))
		return
	}
	cfg := s.app.Config()
	socket := live.Socket{App: cfg.ID, User: s.conn.User, Anonymous: s.conn.Anonymous, Session: s.conn.Session}
	read, err := live.Open(join.Live, cfg.TicketKeys, socket, s.now())
	if err != nil {
		s.out.Send(phoenix.Refuse(f, refusedLiveToken))
		return
	}
	var client *changes.Client
	if cfg.Changes != "" && s.changes != nil {
		client = s.changes(cfg.Changes)
	}
	if client == nil {
		s.out.Send(phoenix.Refuse(f, refusedNoChanges))
		return
	}
	s.live.join(f, cfg.ID, client, startOf(read, join.Cursor), read.Expiry.Sub(s.now()))
}

// startOf is where a registration starts: after the join's cursor when it gives one, else from the read's snapshot.
func startOf(read live.Read, cursor string) changes.Start {
	start := changes.Start{Subs: read.Subs, Fields: read.Fields, Session: read.Session, Anonymous: read.Anonymous}
	if cursor != "" {
		start.Cursor = cursor
		return start
	}
	start.Snapshot = read.Snapshot
	return start
}

func (s *session) leaveLive(f phoenix.Frame) {
	if !s.live.leave(f.Topic) {
		s.out.Send(phoenix.Refuse(f, "unmatched topic"))
		return
	}
	s.out.Send(phoenix.Ok(f))
}
