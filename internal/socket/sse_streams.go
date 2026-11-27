package socket

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"sync"

	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// stream is one open server-sent event connection. Its frames come in by POST, so mu keeps them in order and
// keeps them from reaching the session once the stream is gone.
type stream struct {
	id, secret string
	mu         sync.Mutex
	session    *session
	gone       bool
}

// handle passes frames to the session in order, and reports false when the stream is gone.
func (st *stream) handle(frames []phoenix.Frame) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.gone {
		return false
	}
	for _, f := range frames {
		st.session.handle(f)
	}
	return true
}

// streams holds the open streams by id. An entry lives as long as its GET request, which the app's connection limit
// bounds, and is removed when it ends.
type streams struct {
	mu   sync.Mutex
	byID map[string]*stream
}

func (s *streams) open(sess *session) *stream {
	st := &stream{id: token(), secret: token(), session: sess}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = map[string]*stream{}
	}
	s.byID[st.id] = st
	return st
}

// find returns the stream with the id when the secret is its own, comparing secrets in constant time.
func (s *streams) find(id, secret string) *stream {
	st := s.lookup(id)
	if st == nil || subtle.ConstantTimeCompare([]byte(st.secret), []byte(secret)) != 1 {
		return nil
	}
	return st
}

func (s *streams) lookup(id string) *stream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byID[id]
}

// forget removes the stream and waits for frames being handled, so none reaches the session after it.
func (s *streams) forget(st *stream) {
	s.mu.Lock()
	delete(s.byID, st.id)
	s.mu.Unlock()
	st.mu.Lock()
	defer st.mu.Unlock()
	st.gone = true
}

// token is 128 random bits, URL-safe.
func token() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}
