package socket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/changes"
	"github.com/PrototeamHQ/protobase-realtime/internal/phoenix"
)

// maxLiveTopics bounds the live queries one socket holds, as each is a registration with the changes service.
const maxLiveTopics = 100

// Change is the event a live query's matched change arrives under.
const Change = "change"

// closedExpired is why a live topic closes when its token expires; the client joins again with a fresh token and
// its last cursor.
const closedExpired = "expired"

// liveTopics are a socket's live queries, each registered with the app's changes service until the topic leaves,
// closes or the socket ends.
type liveTopics struct {
	out *sink

	mu      sync.Mutex
	byTopic map[string]*liveTopic
	// ended is set once the socket closed, so a join still on its way registers nothing.
	ended bool
}

func newLiveTopics(out *sink) *liveTopics {
	return &liveTopics{out: out, byTopic: map[string]*liveTopic{}}
}

// liveTopic is one joined live query: the target of its registration. Its registration and timer are guarded by the
// topics' mutex.
type liveTopic struct {
	topics  *liveTopics
	topic   string
	joinRef *string
	reg     *changes.Registration
	expiry  *time.Timer
}

// join answers a valid join and registers it. A join on a topic already joined replaces its registration.
func (l *liveTopics) join(f phoenix.Frame, app string, client *changes.Client, start changes.Start,
	expiresIn time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ended {
		return
	}
	old := l.byTopic[f.Topic]
	if old == nil && len(l.byTopic) >= maxLiveTopics {
		l.out.Send(phoenix.Refuse(f, refusedTooMany))
		return
	}
	if old != nil {
		old.stop()
		delete(l.byTopic, f.Topic)
	}
	t := &liveTopic{topics: l, topic: f.Topic, joinRef: f.JoinRef}
	// The reply goes first, so no change arrives on a topic the client has not joined yet.
	l.out.Send(phoenix.Ok(f))
	reg, ok := client.Register(app, start, t)
	if !ok {
		// A reload stopped the service's client since the join looked it up.
		l.out.Send(t.closeFrame(refusedNoChanges))
		return
	}
	t.reg = reg
	t.expiry = time.AfterFunc(expiresIn, t.expire)
	l.byTopic[f.Topic] = t
}

// leave unregisters a joined live topic; ok is false when the topic is not joined.
func (l *liveTopics) leave(topic string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.byTopic[topic]
	if t == nil {
		return false
	}
	t.stop()
	delete(l.byTopic, topic)
	return true
}

// end unregisters every live topic of a socket that closed.
func (l *liveTopics) end() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ended = true
	for _, t := range l.byTopic {
		t.stop()
	}
	clear(l.byTopic)
}

// stop, under the topics' mutex, unregisters the topic and stops its expiry.
func (t *liveTopic) stop() {
	t.reg.Unregister()
	t.expiry.Stop()
}

// close, under the topics' mutex, drops a topic that is still the joined one and tells the client why it closed.
// stop says whether the registration is still to end.
func (t *liveTopic) close(reason string, stop bool) {
	l := t.topics
	if l.byTopic[t.topic] != t {
		return
	}
	delete(l.byTopic, t.topic)
	if stop {
		t.stop()
	} else {
		t.expiry.Stop()
	}
	l.out.Send(t.closeFrame(reason))
}

func (t *liveTopic) expire() {
	t.topics.mu.Lock()
	defer t.topics.mu.Unlock()
	t.close(closedExpired, true)
}

func (t *liveTopic) closeFrame(reason string) []byte {
	payload, _ := json.Marshal(map[string]string{"reason": reason}) // a map of strings always encodes
	return phoenix.Encode(phoenix.Frame{JoinRef: t.joinRef, Ref: t.joinRef, Topic: t.topic, Event: phoenix.Close,
		Payload: payload})
}

// Change sends a matched change, as the service shaped it, its cursor included.
func (t *liveTopic) Change(entry json.RawMessage) {
	t.topics.out.Send(phoenix.Encode(phoenix.Frame{JoinRef: t.joinRef, Topic: t.topic, Event: Change,
		Payload: entry}))
}

// End closes the topic the service ended, with its reason.
func (t *liveTopic) End(reason string) {
	t.topics.mu.Lock()
	defer t.topics.mu.Unlock()
	t.close(reason, false)
}

// Lost tells the client the topic errored, so a Phoenix client joins it again. The registration stays, and is
// registered again from its last cursor once the service is back.
func (t *liveTopic) Lost() {
	t.topics.out.Send(phoenix.Encode(phoenix.Frame{JoinRef: t.joinRef, Ref: t.joinRef, Topic: t.topic,
		Event: phoenix.Error}))
}
