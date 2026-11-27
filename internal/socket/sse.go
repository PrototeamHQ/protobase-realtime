package socket

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EventsPath is where clients that cannot open a WebSocket connect: the endpoint "/socket" with the transport's
// "/sse" appended. The relay's frames come as server-sent events, the client's go to SendPath.
const EventsPath = "/socket/sse"

// ConnectionParam names the connection id in SendPattern.
const ConnectionParam = "connection"

// SendPattern routes the frames a client sends on a server-sent event connection.
const SendPattern = EventsPath + "/{" + ConnectionParam + "}"

// SecretHeader carries the connection's secret on SendPath; never the URL, since paths get logged.
const SecretHeader = "X-Connection-Secret"

// SendPath is where a client sends the frames of the connection with the id.
func SendPath(connection string) string { return EventsPath + "/" + url.PathEscape(connection) }

// ServeEvents opens a server-sent event connection: the connection event, then each outgoing frame as an event.
func (h *Handler) ServeEvents(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		http.Error(w, "accept must include text/event-stream", http.StatusNotAcceptable)
		return
	}
	t, app, ok := h.ticketApp(w, r)
	if !ok {
		return
	}
	defer h.Errors.Recover(t.App)
	if !allowOrigin(w, r, app.Config().Origins) {
		return
	}
	rc := http.NewResponseController(w)
	// A write to a client that stopped reading fails at once, which ends the stream. A server that cannot set
	// deadlines leaves it to fail on its own.
	out := newSink(func() { _ = rc.SetWriteDeadline(time.Now()) })
	conn, ok := h.register(w, app, t, out)
	if !ok {
		return
	}
	defer app.Unregister(conn)
	st := h.streams.open(&session{app: app, conn: conn, out: out, now: h.Now})
	defer h.streams.forget(st)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	// nginx would otherwise buffer the stream.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	events := &eventWriter{w: w, flush: rc.Flush}
	if events.json(ConnectionEvent, connectionData{st.id, st.secret}) != nil {
		return
	}
	out.stream(r.Context(), events, keepAlive)
}
