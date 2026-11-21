package presence

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReporterRetriesUntilTheAppAccepts(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	tr := newTracker(c, 10)
	var bodies []string
	fail := true
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer the-key" {
			t.Errorf("got %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer app.Close()
	r := NewReporter(tr, app.URL, "the-key", time.UnixMilli(1000))
	tr.Change("u1", 1, 0)
	c.pass(Debounce)
	if r.Flush(context.Background()) == nil {
		t.Fatal("a refused report counted as sent")
	}
	fail = false
	if err := r.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := `{"started":1000,"changes":[{"user":"u1","status":"online","at":1800000000000}]}`
	if len(bodies) != 2 || bodies[0] != want || bodies[1] != want {
		t.Fatalf("got %q", bodies)
	}
}
