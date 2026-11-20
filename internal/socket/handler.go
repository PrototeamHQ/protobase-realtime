// Package socket serves the WebSocket endpoint clients connect to with a ticket.
package socket

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/PrototeamHQ/protobase-realtime/internal/hub"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
	"github.com/PrototeamHQ/protobase-realtime/internal/ticket"
)

// Path is where Phoenix clients connect: their endpoint "/socket" with the transport's "/websocket" appended.
const Path = "/socket/websocket"

// Protocol is the only Phoenix serializer version served.
const Protocol = "2.0.0"

// readLimit bounds one incoming frame: a member's message with its framing.
const readLimit = hub.ClientMessageBytes + 1024

type Handler struct {
	Hub     *hub.Hub
	Tickets *ticket.Verifier
	Now     func() time.Time
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("vsn") != Protocol {
		http.Error(w, "vsn must be "+Protocol, http.StatusBadRequest)
		return
	}
	t, err := h.Tickets.Verify(r.URL.Query().Get("ticket"))
	if err != nil {
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return
	}
	app, ok := h.Hub.App(t.App)
	if !ok {
		panic("socket: a verified ticket names an unknown app")
	}
	writeCtx, abort := context.WithCancel(context.WithoutCancel(r.Context()))
	defer abort()
	out := newSink(abort)
	conn, err := app.Register(t, out, h.Now())
	if errors.Is(err, hub.ErrFull) {
		http.Error(w, "connection limit reached", http.StatusServiceUnavailable)
		return
	}
	if errors.Is(err, hub.ErrSessionClosed) {
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return
	}
	if errors.Is(err, hub.ErrTooManyChannels) {
		http.Error(w, "too many channels", http.StatusBadRequest)
		return
	}
	defer app.Unregister(conn)
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: app.Config.Origins})
	if err != nil {
		return
	}
	ws.SetReadLimit(readLimit)
	written := make(chan struct{})
	go func() {
		out.write(writeCtx, ws)
		close(written)
	}()
	h.read(r.Context(), ws, &session{app: app, conn: conn, out: out, now: h.Now})
	<-written
}

// read handles frames until the socket closes or sends something that is not a Phoenix V2 text frame.
func (h *Handler) read(ctx context.Context, ws *websocket.Conn, s *session) {
	defer s.out.end(websocket.StatusNormalClosure, "")
	for {
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
