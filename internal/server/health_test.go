package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
)

func TestHealthAnswersGETWithTheStartAndNothingAboutApps(t *testing.T) {
	started := time.UnixMilli(1_800_000_000_123)
	r := New([]config.App{app("chat", nil)}, func() time.Time { return started }, errreport.Off())

	res := httptest.NewRecorder()
	r.Handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("got %d %q", res.Code, res.Header().Get("Content-Type"))
	}
	if got, want := res.Body.String(), `{"ok":true,"started":1800000000123}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}

	res = httptest.NewRecorder()
	r.Handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, HealthPath, nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("a POST got %d", res.Code)
	}
}
