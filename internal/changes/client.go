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
	// queueLength bounds the messages waiting for the service; a service that falls further behind is dropped.
	queueLength = 1024
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
	out chan []byte
	// drop ends the current connection.
	drop context.CancelFunc
	// closed is set once the client stops for good.
	closed bool
}

func New(id, url, key string, stuck func(error)) *Client {
	return &Client{ID: id, URL: url, Key: key, Stuck: stuck, regs: map[regKey]*Registration{}}
}

// Register adds a registration for one of the app's live queries, sent to the service now when it is connected or
// once it is. ok is false once the client stopped.
func (c *Client) Register(app string, start Start, t Target) (*Registration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, false
	}
	r := &Registration{client: c, app: app, id: newRegistrationID(), start: start, target: t}
	c.regs[r.key()] = r
	if c.out != nil {
		c.send(registerMessage(r.app, r.id, r.start))
		r.sent = true
	}
	return r, true
}

// send queues a message for the current connection, under the mutex. A full queue drops the connection.
func (c *Client) send(msg []byte) {
	select {
	case c.out <- msg:
	default:
		c.drop()
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
	out := make(chan []byte, queueLength)
	written := make(chan struct{})
	go func() {
		defer close(written)
		write(connCtx, ws, out, drop)
	}()
	c.attach(out, drop)
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

// attach makes a new connection the current one and registers every registration on it, from its last cursor.
func (c *Client) attach(out chan []byte, drop context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.out, c.drop = out, drop
	for _, r := range c.regs {
		c.send(registerMessage(r.app, r.id, r.start))
		r.sent, r.lost = true, false
	}
}

// detach forgets a dropped connection and returns the targets of the registrations it held.
func (c *Client) detach() []Target {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.out, c.drop = nil, nil
	var held []Target
	for _, r := range c.regs {
		if r.sent {
			r.sent, r.lost = false, true
			held = append(held, r.target)
		}
	}
	return held
}

func write(ctx context.Context, ws *websocket.Conn, out <-chan []byte, drop context.CancelFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-out:
			if ws.Write(ctx, websocket.MessageText, msg) != nil {
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
