// Package changes holds the relay's connection to a changes service: it registers the live queries of every app that
// names the service and routes each matched change to the socket topic whose registration it names.
package changes

import (
	"encoding/json"
	"errors"
)

// Subprotocol is the WebSocket subprotocol of the connection: one JSON message per text message.
const Subprotocol = "protobase-relay.json"

// The types of the messages on the connection.
const (
	registerType    = "register"
	unregisterType  = "unregister"
	transactionType = "protobase.changes.transaction"
	// The service ends a registration with one of these, its type naming why.
	Gap     = "gap"
	Revoked = "revoked"
	Refused = "refused"
)

// Start is what a registration asks for: the live token's subs and fields, as the token holds them, the session
// that revokes it or that it is a visitor's, and where it starts: the read's snapshot, or a cursor to resume after.
// Exactly one of Snapshot and Cursor is set.
type Start struct {
	Subs      json.RawMessage
	Fields    json.RawMessage
	Session   string
	Anonymous bool
	Snapshot  json.RawMessage
	Cursor    string
}

// register is a registration's message. Sid is sent empty for a visitor, beside anonymous.
type register struct {
	Type      string          `json:"type"`
	App       string          `json:"app"`
	ID        string          `json:"id"`
	Sid       string          `json:"sid"`
	Anonymous bool            `json:"anonymous,omitempty"`
	Subs      json.RawMessage `json:"subs"`
	Fields    json.RawMessage `json:"fields,omitempty"`
	Snapshot  json.RawMessage `json:"snapshot,omitempty"`
	Cursor    string          `json:"cursor,omitempty"`
}

type unregister struct {
	Type string `json:"type"`
	App  string `json:"app"`
	ID   string `json:"id"`
}

func registerMessage(app, id string, s Start) []byte {
	return marshal(register{Type: registerType, App: app, ID: id, Sid: s.Session, Anonymous: s.Anonymous,
		Subs: s.Subs, Fields: s.Fields, Snapshot: s.Snapshot, Cursor: s.Cursor})
}

func unregisterMessage(app, id string) []byte {
	return marshal(unregister{Type: unregisterType, App: app, ID: id})
}

func marshal(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic("changes: encode: " + err.Error())
	}
	return raw
}

// incoming is any message of the service: a transaction's event, or the end of one registration.
type incoming struct {
	Type string `json:"type"`
	App  string `json:"app"`
	ID   string `json:"id"`
	Data struct {
		Registrations []json.RawMessage `json:"registrations"`
	} `json:"data"`
}

// entry is what one registration gets of a transaction, read for its id and cursor; the page gets it whole.
type entry struct {
	ID     string `json:"id"`
	Cursor string `json:"cursor"`
}

var errMessage = errors.New("changes: the service sent a message that is not one of its own")

func parse(raw []byte) (incoming, error) {
	var m incoming
	if json.Unmarshal(raw, &m) != nil || m.App == "" {
		return incoming{}, errMessage
	}
	switch m.Type {
	case transactionType:
		return m, nil
	case Gap, Revoked, Refused:
		if m.ID == "" {
			return incoming{}, errMessage
		}
		return m, nil
	}
	return incoming{}, errMessage
}
