package socket

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/coder/websocket"

	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// ServeSend takes one frame, or a JSON array of frames, from a server-sent event connection's client and hands
// them to its session in order. A wrong secret or an unknown connection gets 404.
func (h *Handler) ServeSend(w http.ResponseWriter, r *http.Request) {
	st := h.streams.find(r.PathValue(ConnectionParam), r.Header.Get(SecretHeader))
	if st == nil {
		http.NotFound(w, r)
		return
	}
	app := st.session.app
	defer h.Errors.Recover(app.Config().ID)
	if !allowOrigin(w, r, app.Config().Origins) {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, readLimit(app)))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	frames, err := decodeFrames(raw)
	if err != nil {
		// As on a WebSocket, a malformed frame ends the connection.
		st.session.out.end(websocket.StatusInvalidFramePayloadData, "malformed frame")
		http.Error(w, "malformed frame", http.StatusBadRequest)
		return
	}
	if !st.handle(frames) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ServeSendPreflight lets a browser on an allowed origin send frames with the secret header.
func (h *Handler) ServeSendPreflight(w http.ResponseWriter, r *http.Request) {
	st := h.streams.lookup(r.PathValue(ConnectionParam))
	if st == nil {
		http.NotFound(w, r)
		return
	}
	if !allowOrigin(w, r, st.session.app.Config().Origins) {
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", http.MethodPost)
	w.Header().Set("Access-Control-Allow-Headers", SecretHeader+", Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

// decodeFrames reads one frame or an array of frames. A frame starts with its join ref, a string or null, never an
// array, which tells the two apart.
func decodeFrames(raw []byte) ([]phoenix.Frame, error) {
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || len(parts) == 0 {
		return nil, phoenix.ErrMalformed
	}
	if parts[0][0] != '[' {
		f, err := phoenix.Decode(raw)
		return []phoenix.Frame{f}, err
	}
	frames := make([]phoenix.Frame, 0, len(parts))
	for _, part := range parts {
		f, err := phoenix.Decode(part)
		if err != nil {
			return nil, err
		}
		frames = append(frames, f)
	}
	return frames, nil
}
