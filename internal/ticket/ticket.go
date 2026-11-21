// Package ticket verifies the short-lived, single-use tickets apps issue for a socket connection.
//
// A ticket is a JWT signed with HS256 by one of the app's ticket keys. Claims: iss (the app id), sub (the person,
// absent for an anonymous session), anonymous (true for an anonymous session), sid (the sign-in session), jti (a
// random id, used once), exp (at most MaxLifetime ahead), channels (the channels the person belongs to), and the
// person's delivery rules: blocked (people whose events never reach them), friends_only and friends (when
// friends_only is set, only friends' direct events reach them).
package ticket

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/PrototeamHQ/protobase-realtime/internal/seen"
)

// MaxLifetime caps how far ahead a ticket's exp may lie, and is how long a used ticket id is remembered.
const MaxLifetime = 120 * time.Second

var ErrInvalid = errors.New("invalid ticket")

type claims struct {
	jwt.RegisteredClaims
	Session     string   `json:"sid"`
	Channels    []string `json:"channels"`
	Anonymous   bool     `json:"anonymous"`
	Blocked     []string `json:"blocked"`
	FriendsOnly bool     `json:"friends_only"`
	Friends     []string `json:"friends"`
}

// Ticket is what a verified ticket grants. User is empty for an anonymous session.
type Ticket struct {
	App       string
	User      string
	Anonymous bool
	Session   string
	Channels  []string
	Rules     Rules
}

// Rules filter which client events reach a person.
type Rules struct {
	Blocked     []string
	FriendsOnly bool
	Friends     []string
}

// MaxRuleList caps the people one rules list may name.
const MaxRuleList = 1000

// Valid bounds the lists.
func (r Rules) Valid() bool {
	return len(r.Blocked) <= MaxRuleList && len(r.Friends) <= MaxRuleList
}

// Keys returns the ticket keys of an app, or false for an unknown app.
type Keys func(app string) ([]string, bool)

type Verifier struct {
	keys Keys
	used *seen.Set
	now  func() time.Time
}

func NewVerifier(keys Keys, now func() time.Time) *Verifier {
	return &Verifier{keys: keys, used: seen.New(MaxLifetime), now: now}
}

func (v *Verifier) Verify(raw string) (Ticket, error) {
	var c claims
	_, err := jwt.ParseWithClaims(raw, &c, v.keyFor,
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return Ticket{}, ErrInvalid
	}
	if !v.wellFormed(c) {
		return Ticket{}, ErrInvalid
	}
	if !v.used.First(c.Issuer+"\x00"+c.ID, v.now()) {
		return Ticket{}, ErrInvalid
	}
	return Ticket{App: c.Issuer, User: c.Subject, Anonymous: c.Anonymous, Session: c.Session, Channels: c.Channels,
		Rules: Rules{Blocked: c.Blocked, FriendsOnly: c.FriendsOnly, Friends: c.Friends}}, nil
}

func (v *Verifier) keyFor(token *jwt.Token) (any, error) {
	iss, err := token.Claims.GetIssuer()
	if err != nil {
		return nil, err
	}
	keys, ok := v.keys(iss)
	if !ok {
		return nil, ErrInvalid
	}
	set := jwt.VerificationKeySet{}
	for _, key := range keys {
		set.Keys = append(set.Keys, []byte(key))
	}
	return set, nil
}

// wellFormed requires exactly one of a person or the anonymous flag, a session, a ticket id, rules only for a person
// and within bounds, and a short lifetime.
func (v *Verifier) wellFormed(c claims) bool {
	if c.Anonymous == (c.Subject != "") {
		return false
	}
	if c.Session == "" || c.ID == "" {
		return false
	}
	if c.Anonymous && (len(c.Blocked) > 0 || c.FriendsOnly || len(c.Friends) > 0) {
		return false
	}
	if !(Rules{Blocked: c.Blocked, Friends: c.Friends}).Valid() {
		return false
	}
	return c.ExpiresAt.Sub(v.now()) <= MaxLifetime
}
