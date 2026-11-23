// Package errreport sends the relay's errors to an error tracker that speaks the Sentry protocol. An event holds the
// error and the app id alone: never a request, its headers or query string, a user, a ticket, a key or a payload.
package errreport

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
)

// Reporter sends errors to the tracker its DSN names. Without a DSN it sends nothing, and still recovers panics.
type Reporter struct {
	client *sentry.Client
}

// Off returns a Reporter that sends nothing.
func Off() *Reporter {
	return &Reporter{}
}

// New returns a Reporter for a DSN; an empty DSN turns reporting off.
func New(dsn, release string) (*Reporter, error) {
	if dsn == "" {
		return Off(), nil
	}
	client, err := sentry.NewClient(sentry.ClientOptions{Dsn: dsn, Release: release, BeforeSend: scrub})
	if err != nil {
		// The parse error may quote the DSN, which holds the tracker's key.
		return nil, errors.New("PROTOBASE_REALTIME_SENTRY_DSN must be a DSN such as https://<key>@<host>/<project>")
	}
	return &Reporter{client: client}, nil
}

// scrub drops whatever an event could carry beyond the error and the app id.
func scrub(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	event.Request = nil
	event.User = sentry.User{}
	event.Breadcrumbs = nil
	return event
}

// Error reports an error of an app; app is empty for an error of the relay as a whole.
func (r *Reporter) Error(app string, err error) {
	if r.client == nil {
		return
	}
	r.client.CaptureException(err, nil, scope(app))
}

// Recover, deferred in an HTTP handler, reports a panic and panics again, so the server ends the request as it
// would have.
func (r *Reporter) Recover(app string) {
	v := recover()
	if v == nil {
		return
	}
	if v != http.ErrAbortHandler {
		r.panicked(app, v)
	}
	panic(v)
}

// Contain, deferred in a goroutine of a connection, reports a panic, writes it to stderr and calls end to close the
// connection, so one connection's panic does not stop the relay.
func (r *Reporter) Contain(app string, end func()) {
	v := recover()
	if v == nil {
		return
	}
	r.panicked(app, v)
	fmt.Fprintf(os.Stderr, "protobase-realtime: panic in a connection of app %q: %v\n%s", app, v, debug.Stack())
	end()
}

func (r *Reporter) panicked(app string, v any) {
	if r.client == nil {
		return
	}
	r.client.Recover(v, &sentry.EventHint{RecoveredException: v}, scope(app))
}

// Flush waits up to timeout for the reported errors to be sent.
func (r *Reporter) Flush(timeout time.Duration) {
	if r.client == nil {
		return
	}
	r.client.Flush(timeout)
}

func scope(app string) *sentry.Scope {
	s := sentry.NewScope()
	if app != "" {
		s.SetTag("app", app)
	}
	return s
}
