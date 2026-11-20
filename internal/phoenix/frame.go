// Package phoenix reads and writes the Phoenix channels V2 JSON framing: [join_ref, ref, topic, event, payload].
package phoenix

import (
	"encoding/json"
	"errors"
)

// Events of the protocol itself.
const (
	Join      = "phx_join"
	Leave     = "phx_leave"
	Reply     = "phx_reply"
	Close     = "phx_close"
	Error     = "phx_error"
	Heartbeat = "heartbeat"
	// SystemTopic carries heartbeats.
	SystemTopic = "phoenix"
)

// Frame is one message. JoinRef and Ref are nil when the frame carries none.
type Frame struct {
	JoinRef *string
	Ref     *string
	Topic   string
	Event   string
	Payload json.RawMessage
}

var ErrMalformed = errors.New("malformed frame")

func Decode(raw []byte) (Frame, error) {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) != 5 {
		return Frame{}, ErrMalformed
	}
	var f Frame
	if json.Unmarshal(parts[0], &f.JoinRef) != nil || json.Unmarshal(parts[1], &f.Ref) != nil {
		return Frame{}, ErrMalformed
	}
	if json.Unmarshal(parts[2], &f.Topic) != nil || json.Unmarshal(parts[3], &f.Event) != nil {
		return Frame{}, ErrMalformed
	}
	if f.Topic == "" || f.Event == "" || !isObject(parts[4]) {
		return Frame{}, ErrMalformed
	}
	f.Payload = parts[4]
	return f, nil
}

func isObject(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && m != nil
}

// Encode writes a frame. A nil Payload is sent as {}.
func Encode(f Frame) []byte {
	payload := f.Payload
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	raw, err := json.Marshal([]any{f.JoinRef, f.Ref, f.Topic, f.Event, payload})
	if err != nil {
		panic("phoenix: encode: " + err.Error())
	}
	return raw
}

// ReplyTo answers a frame with a status and a response object.
func ReplyTo(f Frame, status string, response any) []byte {
	body, err := json.Marshal(map[string]any{"status": status, "response": response})
	if err != nil {
		panic("phoenix: reply: " + err.Error())
	}
	return Encode(Frame{JoinRef: f.JoinRef, Ref: f.Ref, Topic: f.Topic, Event: Reply, Payload: body})
}

// Ok answers a frame with an empty ok reply.
func Ok(f Frame) []byte { return ReplyTo(f, "ok", struct{}{}) }

// Refuse answers a frame with an error reply naming the reason.
func Refuse(f Frame, reason string) []byte {
	return ReplyTo(f, "error", map[string]string{"reason": reason})
}
