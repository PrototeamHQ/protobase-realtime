// Package errreporttest stands in for an error tracker's ingest endpoint in tests.
package errreporttest

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
)

// Ingest records the envelopes it receives, each prefixed with the path it was posted to.
type Ingest struct {
	reporter  *errreport.Reporter
	mu        sync.Mutex
	envelopes []string
}

// Serve starts an ingest endpoint for the test and returns it with a Reporter that sends to it.
func Serve(t *testing.T) (*Ingest, *errreport.Reporter) {
	in := &Ingest{}
	server := httptest.NewServer(in)
	t.Cleanup(server.Close)
	r, err := errreport.New("http://public@"+strings.TrimPrefix(server.URL, "http://")+"/7", "test")
	if err != nil {
		t.Fatal(err)
	}
	in.reporter = r
	return in, r
}

func (in *Ingest) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		z, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "bad gzip", http.StatusBadRequest)
			return
		}
		body = z
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	in.mu.Lock()
	in.envelopes = append(in.envelopes, r.URL.Path+"\n"+string(raw))
	in.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// Received sends what was reported and returns every envelope received, joined.
func (in *Ingest) Received() string {
	in.reporter.Flush(5 * time.Second)
	in.mu.Lock()
	defer in.mu.Unlock()
	return strings.Join(in.envelopes, "\n")
}
