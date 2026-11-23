package hub

import (
	"encoding/json"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// Publish is one message from the app to a channel's joined sockets, narrowed to Users when it lists any and to
// sockets viewing About when it is set.
type Publish struct {
	Channel string
	Users   []string
	Event   string
	Payload json.RawMessage
	About   string
}

// Publish encodes the frame once and queues the same bytes on each recipient. It reports how many sockets it reached.
func (a *App) Publish(p Publish) int {
	frame := phoenix.Encode(phoenix.Frame{Topic: p.Channel, Event: p.Event, Payload: p.Payload})
	users := set[string]{}
	for _, user := range p.Users {
		users[user] = struct{}{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sent := 0
	for c := range a.byChannel[p.Channel] {
		if len(users) > 0 && !has(users, c.User) {
			continue
		}
		if !c.joined[p.Channel].sees(p.About) {
			continue
		}
		c.sink.Send(frame)
		sent++
	}
	return sent
}

func has(s set[string], key string) bool {
	_, ok := s[key]
	return ok
}

// CloseSession closes every socket of a sign-in session and refuses the session's tickets for their lifetime. It
// reports how many sockets it closed.
func (a *App) CloseSession(session string, now time.Time) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed.First(session, now)
	for c := range a.bySession[session] {
		c.sink.Close("session closed")
	}
	return len(a.bySession[session])
}
