package socket

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/coder/websocket"
)

// Event names of the stream. Phoenix frames are unnamed events, which EventSource delivers as "message".
const (
	// ConnectionEvent is the first event: the connection's id and the secret its frames are sent with.
	ConnectionEvent = "connection"
	// CloseEvent is the last event of a final close, after which a client does not reconnect.
	CloseEvent = "close"
)

// keepAlive is how often a stream gets a comment line, so proxies do not close it while it is idle.
const keepAlive = 25 * time.Second

type connectionData struct {
	Connection string `json:"connection"`
	Secret     string `json:"secret"`
}

type closeData struct {
	Code   websocket.StatusCode `json:"code"`
	Reason string               `json:"reason"`
}

// eventWriter writes server-sent events, flushing each one so it is not held in a buffer.
type eventWriter struct {
	w     io.Writer
	flush func() error
}

// event writes one event, unnamed when name is empty, with each line of data on a data line of its own.
func (e *eventWriter) event(name string, data []byte) error {
	var b bytes.Buffer
	if name != "" {
		b.WriteString("event: " + name + "\n")
	}
	for line := range bytes.Lines(data) {
		b.WriteString("data: ")
		b.Write(bytes.TrimSuffix(line, []byte("\n")))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return e.write(b.Bytes())
}

func (e *eventWriter) comment() error { return e.write([]byte(":\n\n")) }

func (e *eventWriter) json(name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		panic("socket: encode event: " + err.Error())
	}
	return e.event(name, raw)
}

func (e *eventWriter) write(b []byte) error {
	if _, err := e.w.Write(b); err != nil {
		return err
	}
	return e.flush()
}

// stream writes the sink's frames as events until the sink ends or ctx does, as when the client goes. A final
// close is told with a close event; any other ending only ends the stream, after which a client reconnects.
func (s *sink) stream(ctx context.Context, events *eventWriter, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case frame := <-s.queue:
			if events.event("", frame) != nil {
				s.end(websocket.StatusInternalError, "write failed")
				return
			}
		case <-tick.C:
			if events.comment() != nil {
				s.end(websocket.StatusInternalError, "write failed")
				return
			}
		case <-s.done:
			if s.code == websocket.StatusNormalClosure {
				_ = events.json(CloseEvent, closeData{s.code, s.why}) // the stream ends either way
			}
			return
		case <-ctx.Done():
			s.end(websocket.StatusNormalClosure, "")
			return
		}
	}
}
