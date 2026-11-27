package changes

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
)

// Target is the socket topic a registration's frames go to. The client calls it from its one reader, outside its
// lock, so a target may call back into the client.
type Target interface {
	// Change hands on one entry of a transaction, as the service shaped it, its cursor included.
	Change(entry json.RawMessage)
	// End says the service ended the registration, with Gap, Revoked or Refused. The client has dropped it.
	End(reason string)
	// Lost says the connection to the service dropped. The client keeps the registration and registers it again
	// from its last cursor once it is back.
	Lost()
}

// Registration is one live query registered with the service. Its fields are guarded by the client's mutex.
type Registration struct {
	client *Client
	app    string
	id     string
	start  Start
	target Target
	// sent is set while the current connection holds the registration.
	sent bool
	// lost is set once its target was told of a dropped connection, until a connection holds it again.
	lost bool
}

type regKey struct{ app, id string }

func (r *Registration) key() regKey { return regKey{r.app, r.id} }

// advance moves the start past a delivered entry, so a registration made again resumes after it.
func (r *Registration) advance(cursor string) {
	r.start.Snapshot, r.start.Cursor = nil, cursor
}

// Unregister ends the registration; the service hears of it when it holds it. A second call does nothing.
func (r *Registration) Unregister() {
	c := r.client
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.regs[r.key()] != r {
		return
	}
	delete(c.regs, r.key())
	if r.sent {
		c.send(unregisterMessage(r.app, r.id))
	}
}

func newRegistrationID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}
