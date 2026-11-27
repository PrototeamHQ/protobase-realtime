// Package live opens the live tokens an app returns with a read, for a socket that subscribes to what it read.
//
// A live token is a JWE (alg dir, enc A256GCM) sealed with a key derived from one of the app's ticket keys, so the
// client can neither read nor change its conditions. Claims: iss (the app id), sub and sid (the person and their
// sign-in session) or anonymous (true for a visitor, bound to no session), exp, and the read's subs, fields and
// snapshot, which the relay passes on to the changes service unread.
package live

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// keyInfo is what the live token key is derived for, so the same ticket key signs tickets and seals live tokens
// apart.
const keyInfo = "protobase live token"

// ErrInvalid covers every token that does not open or is not for the socket, so a client learns nothing about which
// check failed.
var ErrInvalid = errors.New("invalid live token")

// Socket is who holds the socket a token is offered on. User is empty for an anonymous session.
type Socket struct {
	App       string
	User      string
	Anonymous bool
	Session   string
}

type claims struct {
	Session   string          `json:"sid"`
	Anonymous bool            `json:"anonymous"`
	Subs      json.RawMessage `json:"subs"`
	Fields    json.RawMessage `json:"fields"`
	Snapshot  json.RawMessage `json:"snapshot"`
}

// Read is what an opened token says of its read: the subscriptions, fields and snapshot as the token holds them, the
// session that revokes it, or that it is a visitor's, and when it expires.
type Read struct {
	Subs      json.RawMessage
	Fields    json.RawMessage
	Snapshot  json.RawMessage
	Session   string
	Anonymous bool
	Expiry    time.Time
}

// Key derives the 256-bit key live tokens are sealed with from a ticket key: HKDF-SHA-256 without salt.
func Key(ticketKey string) []byte {
	key, err := hkdf.Key(sha256.New, []byte(ticketKey), nil, keyInfo, 32)
	if err != nil {
		panic("live: derive key: " + err.Error())
	}
	return key
}

// Open decrypts a token under a key derived from any of the ticket keys and checks it is for the socket and valid at
// now: issued by the socket's app, for its person and session, or for a visitor on an anonymous socket.
func Open(raw string, ticketKeys []string, s Socket, now time.Time) (Read, error) {
	jwe, err := jwt.ParseEncrypted(raw, []jose.KeyAlgorithm{jose.DIRECT}, []jose.ContentEncryption{jose.A256GCM})
	if err != nil {
		return Read{}, ErrInvalid
	}
	var std jwt.Claims
	var own claims
	if !decrypt(jwe, ticketKeys, &std, &own) {
		return Read{}, ErrInvalid
	}
	// RFC 7519 wants now before exp, which the library's check lets equal.
	if std.Expiry == nil || !now.Before(std.Expiry.Time()) ||
		std.ValidateWithLeeway(jwt.Expected{Issuer: s.App, Time: now}, 0) != nil {
		return Read{}, ErrInvalid
	}
	if !forSocket(std.Subject, own, s) {
		return Read{}, ErrInvalid
	}
	return Read{Subs: own.Subs, Fields: own.Fields, Snapshot: own.Snapshot, Session: own.Session,
		Anonymous: own.Anonymous, Expiry: std.Expiry.Time()}, nil
}

func decrypt(jwe *jwt.JSONWebToken, ticketKeys []string, out ...any) bool {
	for _, ticketKey := range ticketKeys {
		if jwe.Claims(Key(ticketKey), out...) == nil {
			return true
		}
	}
	return false
}

// forSocket binds a person's token to their sign-in session, and lets a visitor's token, which names no session,
// only onto an anonymous socket.
func forSocket(subject string, c claims, s Socket) bool {
	if c.Anonymous {
		return subject == "" && c.Session == "" && s.Anonymous
	}
	return subject != "" && c.Session != "" && !s.Anonymous && subject == s.User && c.Session == s.Session
}
