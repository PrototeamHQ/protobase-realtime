package changes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/PrototeamHQ/protobase-realtime/internal/backoff"
)

const (
	// appRegisters bounds the registers of one app waiting for the service, as the service holds no more for an app
	// before it stops reading; an app past it is refused rather than filling the queue every app shares.
	appRegisters = 1024
	// queueLength bounds the messages waiting for the service; a service that falls further behind is dropped. It holds
	// many apps at their bound, so only a service that stops reading fills it.
	queueLength = 16 * appRegisters
	// readLimit bounds one message of the service: one transaction's changes for every registration they concern.
	readLimit = 32 << 20
	// dialTimeout bounds the handshake.
	dialTimeout = 10 * time.Second
)

// Client is the one connection to a changes service, for every app that names it. Registrations outlive a dropped
// connection and are registered again once it is back.
type Client struct {
	// ID names the service in errors.
	ID  string
	URL string
	Key string
	// Stuck is told of a connection that still fails after the longest wait, once until one succeeds again.
	Stuck func(error)

	mu   sync.Mutex
	regs map[regKey]*Registration
	// out is the current connection's queue, nil while there is none.
	out chan queued
	// waiting counts each app's registers in out.
	waiting map[string]int
	// drop ends the current connection.
	drop context.CancelFunc
	// closed is set once the client stops for good.
	closed bool
}

func New(id, url, key string, stuck func(error)) *Client {
	return &Client{ID: id, URL: url, Key: key, Stuck: stuck, regs: map[regKey]*Registration{}}
}

// Errors of Register.
var (
	// ErrStopped refuses a registration once the client stopped.
	ErrStopped = errors.New("changes: the client stopped")
	// ErrBusy refuses a registration while the app has appRegisters registers waiting for the service.
	ErrBusy = errors.New("changes: the app has too many registrations waiting")
)

// Register adds a registration for one of the app's live queries, sent to the service now when it is connected or
// once it is. It unregisters replaces first when it is not nil, and only once the registration is accepted, so a
// refused one leaves it as it was.
func (c *Client) Register(app string, start Start, t Target, replaces *Registration) (*Registration, error) {
	if replaces == nil || replaces.client == c {
		return c.register(app, start, t, replaces)
	}
	// The app names another service since: the old registration ends on its own client, once this one is accepted,
	// so neither client's mutex is held while the other's is taken.
	r, err := c.register(app, start, t, nil)
	if err == nil {
		replaces.Unregister()
	}
	return r, err
}

// register adds a registration and unregisters replaces, a registration of c, in one step under c's mutex.
func (c *Client) register(app string, start Start, t Target, replaces *Registration) (*Registration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrStopped
	}
	if c.out != nil && c.waiting[app] >= appRegisters {
		return nil, ErrBusy
	}
	if replaces != nil {
		replaces.unregisterLocked()
	}
	r := &Registration{client: c, app: app, id: newRegistrationID(), start: start, target: t}
	c.regs[r.key()] = r
	if c.out != nil {
		c.send(queued{app: app, register: true, msg: registerMessage(r.app, r.id, r.start)})
		r.sent = true
	}
	return r, nil
}

// queued is a message waiting for the service; a register counts against its app's bound until it is taken.
type queued struct {
	app      string
	register bool
	msg      []byte
}

// send queues a message for the current connection, under the mutex. A full queue drops the connection.
func (c *Client) send(q queued) {
	select {
	case c.out <- q:
		if q.register {
			c.waiting[q.app]++
		}
	default:
		c.drop()
	}
}

// taken counts a message off the queue.
func (c *Client) taken(q queued) {
	if !q.register {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waiting[q.app]--
	if c.waiting[q.app] == 0 {
		delete(c.waiting, q.app)
	}
}

// Run connects and serves the connection until ctx ends, connecting again after each drop. A failed connection waits
// as presence reports do: twice as long as the last, from a second up to a minute. Once ctx ends, the client stops:
// every registration is told it lost the connection and is dropped, so its client joins again elsewhere.
func (c *Client) Run(ctx context.Context) {
	defer c.Close()
	var b backoff.Backoff
	for {
		healthy, err := c.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		if healthy {
			b.Succeed()
		}
		wait, stuck := b.Fail()
		if stuck {
			c.Stuck(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Close stops a client that does not run, or whose Run's context ended, for good. Every registration is dropped, and
// told it lost the connection unless it was told already.
func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	var lost []Target
	for _, r := range c.regs {
		if !r.lost {
			lost = append(lost, r.target)
		}
	}
	clear(c.regs)
	c.mu.Unlock()
	for _, t := range lost {
		t.Lost()
	}
}

// connect serves one connection until it drops. healthy is set when it dropped after the handshake for a reason
// other than a message the relay cannot read.
func (c *Client) connect(ctx context.Context) (healthy bool, err error) {
	ws, err := c.dial(ctx)
	if err != nil {
		return false, err
	}
	defer ws.CloseNow()
	ws.SetReadLimit(readLimit)
	connCtx, drop := context.WithCancel(ctx)
	defer drop()
	out := make(chan queued, queueLength)
	replay := c.attach(out, drop)
	written := make(chan struct{})
	go func() {
		defer close(written)
		c.write(connCtx, ws, replay, out, drop)
	}()
	err = c.read(connCtx, ws)
	drop()
	<-written
	for _, t := range c.detach() {
		t.Lost()
	}
	if errors.Is(err, errMessage) {
		ws.Close(websocket.StatusPolicyViolation, "invalid message")
		return false, fmt.Errorf("changes service %s: %w", c.ID, err)
	}
	return true, fmt.Errorf("changes service %s: the connection dropped", c.ID)
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	header := http.Header{"Authorization": {"Bearer " + c.Key}}
	ws, res, err := websocket.Dial(ctx, c.URL, &websocket.DialOptions{HTTPHeader: header,
		Subprotocols: []string{Subprotocol}})
	// The URL stays out of the error, as it goes to the error tracker.
	if res != nil && res.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("changes service %s: the service answered %d", c.ID, res.StatusCode)
	}
	if err != nil {
		return nil, fmt.Errorf("changes service %s: the connection failed", c.ID)
	}
	return ws, nil
}

// attach makes a new connection the current one and returns the register of every registration, from its last
// cursor. They are written before anything queued, so the queue's bound applies only to new traffic and an unregister
// never goes before its register.
func (c *Client) attach(out chan queued, drop context.CancelFunc) [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.out, c.drop, c.waiting = out, drop, map[string]int{}
	replay := make([][]byte, 0, len(c.regs))
	for _, r := range c.regs {
		replay = append(replay, registerMessage(r.app, r.id, r.start))
		r.sent, r.lost = true, false
	}
	return replay
}

// detach forgets a dropped connection and returns the targets of the registrations it held.
func (c *Client) detach() []Target {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.out, c.drop, c.waiting = nil, nil, nil
	var held []Target
	for _, r := range c.regs {
		if r.sent {
			r.sent, r.lost = false, true
			held = append(held, r.target)
		}
	}
	return held
}

// write writes replay, then the queue, until ctx ends or a write fails.
func (c *Client) write(ctx context.Context, ws *websocket.Conn, replay [][]byte, out <-chan queued,
	drop context.CancelFunc) {
	for _, msg := range replay {
		if ws.Write(ctx, websocket.MessageText, msg) != nil {
			drop()
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case q := <-out:
			c.taken(q)
			if ws.Write(ctx, websocket.MessageText, q.msg) != nil {
				drop()
				return
			}
		}
	}
}

// read routes the service's messages until the connection drops or sends one that is not the service's.
func (c *Client) read(ctx context.Context, ws *websocket.Conn) error {
	for {
		typ, raw, err := ws.Read(ctx)
		if err != nil {
			return err
		}
		m, err := parse(raw)
		if typ != websocket.MessageText || err != nil {
			return errMessage
		}
		if m.Type == transactionType {
			if err := c.deliver(m); err != nil {
				return err
			}
			continue
		}
		c.end(m.App, m.ID, m.Type)
	}
}

// deliver hands each entry of a transaction to its registration's target, and moves the registration past it.
// An entry for a registration the relay dropped meanwhile goes nowhere.
func (c *Client) deliver(m incoming) error {
	for _, raw := range m.Data.Registrations {
		var e entry
		if json.Unmarshal(raw, &e) != nil || e.ID == "" || e.Cursor == "" {
			return errMessage
		}
		c.mu.Lock()
		r := c.regs[regKey{m.App, e.ID}]
		if r != nil {
			r.advance(e.Cursor)
		}
		c.mu.Unlock()
		if r != nil {
			r.target.Change(raw)
		}
	}
	return nil
}

// end drops a registration the service ended and tells its target why.
func (c *Client) end(app, id, reason string) {
	c.mu.Lock()
	r := c.regs[regKey{app, id}]
	if r != nil {
		delete(c.regs, r.key())
	}
	c.mu.Unlock()
	if r != nil {
		r.target.End(reason)
	}
}
