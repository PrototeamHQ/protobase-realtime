package socket

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func newEventWriter() (*eventWriter, *bytes.Buffer) {
	var b bytes.Buffer
	return &eventWriter{w: &b, flush: func() error { return nil }, deadline: func(time.Time) error { return nil }}, &b
}

func TestEventWriterBoundsEachWriteAndThenClearsTheDeadline(t *testing.T) {
	var deadlines []time.Time
	events, _ := newEventWriter()
	events.deadline = func(d time.Time) error {
		deadlines = append(deadlines, d)
		return nil
	}
	before := time.Now()
	_ = events.comment()
	if len(deadlines) != 2 || deadlines[0].Before(before.Add(writeTimeout)) || !deadlines[1].IsZero() {
		t.Fatalf("got deadlines %v", deadlines)
	}
}

func TestEventWriterPutsEachLineOnADataLine(t *testing.T) {
	events, b := newEventWriter()
	_ = events.event("", []byte(`[null,null,"room","x",{}]`))
	_ = events.event("note", []byte("one\ntwo"))
	_ = events.comment()
	want := "data: [null,null,\"room\",\"x\",{}]\n\nevent: note\ndata: one\ndata: two\n\n:\n\n"
	if b.String() != want {
		t.Fatalf("got %q", b.String())
	}
}

func TestStreamEndsAFinalCloseWithACloseEvent(t *testing.T) {
	events, b := newEventWriter()
	s := newSink(func() {})
	s.Send([]byte(`[null,null,"room","x",{}]`))
	go s.Close("session closed")
	s.stream(context.Background(), events, time.Hour)
	want := "data: [null,null,\"room\",\"x\",{}]\n\n"
	// The close may come before the queued frame is written; the close event is last either way.
	if got := b.String(); got != want+"event: close\ndata: {\"code\":1000,\"reason\":\"session closed\"}\n\n" &&
		got != "event: close\ndata: {\"code\":1000,\"reason\":\"session closed\"}\n\n" {
		t.Fatalf("got %q", got)
	}
}

func TestStreamEndsARestartWithoutACloseEvent(t *testing.T) {
	events, b := newEventWriter()
	s := newSink(func() {})
	s.Restart()
	s.stream(context.Background(), events, time.Hour)
	if b.Len() != 0 {
		t.Fatalf("got %q", b.String())
	}
}

func TestStreamKeepsAnIdleStreamAlive(t *testing.T) {
	events, b := newEventWriter()
	s := newSink(func() {})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	s.stream(ctx, events, 10*time.Millisecond)
	if !bytes.HasPrefix(b.Bytes(), []byte(":\n\n")) {
		t.Fatalf("got %q", b.String())
	}
	select {
	case <-s.done:
	default:
		t.Fatal("a stream whose client went did not end its sink")
	}
}
