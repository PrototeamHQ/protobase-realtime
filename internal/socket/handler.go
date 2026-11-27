// Package socket serves the endpoints clients connect to with a ticket: a WebSocket, or server-sent events.
package socket

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

// Path is where Phoenix clients connect: their endpoint "/socket" with the transport's "/websocket" appended.
const Path = "/socket/websocket"

// Protocol is the only Phoenix serializer version served.
const Protocol = "2.0.0"

// frameOverhead is what one incoming frame may hold beyond its payload: the framing, and a direct event's recipient.
const frameOverhead = 2048

// interestBytes is the payload of a full interest list.
const interestBytes = hub.MaxInterestKeys * (hub.MaxInterestKeyLen + 3)

// readLimit bounds one incoming frame: the largest event the app lets a client send, or a full interest list.
func readLimit(app *hub.App) int64 {
	return int64(max(app.MaxEventBytes(), interestBytes) + frameOverhead)
}

type Handler struct {
	Hub     *hub.Hub
	Tickets *ticket.Verifier
	Now     func() time.Time
	Errors  *errreport.Reporter
	// Changes finds a changes service's client by id, nil when there is none.
	Changes func(service string) *changes.Client
	// streams are the open server-sent event connections, by connection id.
	streams streams
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t, app, ok := h.ticketApp(w, r)
	if !ok {
		return
	}
	defer h.Errors.Recover(t.App)
	writeCtx, abort := context.WithCancel(context.WithoutCancel(r.Context()))
	defer abort()
	out := newSink(abort)
	conn, ok := h.register(w, app, t, out)
	if !ok {
		return
	}
	defer app.Unregister(conn)
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: app.Config().Origins})
	if err != nil {
		return
	}
	written := make(chan struct{})
	go func() {
		defer close(written)
		defer h.Errors.Contain(t.App, func() { _ = ws.CloseNow() })
		out.write(writeCtx, ws)
	}()
	s := &session{app: app, conn: conn, out: out, now: h.Now, changes: h.Changes, live: newLiveTopics(out)}
	h.read(r.Context(), ws, s)
	// The socket's live queries end with it, however it closed.
	s.live.end()
	<-written
}

// ticketApp checks the protocol version and the ticket, and finds its app. It has answered when ok is false.
func (h *Handler) ticketApp(w http.ResponseWriter, r *http.Request) (ticket.Ticket, *hub.App, bool) {
	if r.URL.Query().Get("vsn") != Protocol {
		http.Error(w, "vsn must be "+Protocol, http.StatusBadRequest)
		return ticket.Ticket{}, nil, false
	}
	t, err := h.Tickets.Verify(r.URL.Query().Get("ticket"))
	if err != nil {
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return ticket.Ticket{}, nil, false
	}
	// A reload may have removed the app since its ticket was verified.
	app, ok := h.Hub.App(t.App)
	if !ok {
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return ticket.Ticket{}, nil, false
	}
	return t, app, true
}

// register adds the connection to its app. It has answered when ok is false.
func (h *Handler) register(w http.ResponseWriter, app *hub.App, t ticket.Ticket, out *sink) (*hub.Conn, bool) {
	conn, err := app.Register(t, out, h.Now())
	if errors.Is(err, hub.ErrFull) {
		http.Error(w, "connection limit reached", http.StatusServiceUnavailable)
		return nil, false
	}
	if errors.Is(err, hub.ErrSessionClosed) || errors.Is(err, hub.ErrAppRemoved) {
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return nil, false
	}
	if errors.Is(err, hub.ErrTooManyChannels) {
		http.Error(w, "too many channels", http.StatusBadRequest)
		return nil, false
	}
	return conn, true
}

// read handles frames until the socket closes or sends something that is not a Phoenix V2 text frame.
func (h *Handler) read(ctx context.Context, ws *websocket.Conn, s *session) {
	defer s.out.end(websocket.StatusNormalClosure, "")
	for {
		// The limit is set before each frame, so a reload that changes the app's client events applies to it.
		ws.SetReadLimit(readLimit(s.app))
		typ, raw, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			s.out.end(websocket.StatusUnsupportedData, "text frames only")
			return
		}
		f, err := phoenix.Decode(raw)
		if err != nil {
			s.out.end(websocket.StatusInvalidFramePayloadData, "malformed frame")
			return
		}
		s.handle(f)
	}
}
