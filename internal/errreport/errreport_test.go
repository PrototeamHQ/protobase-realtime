package errreport_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport/errreporttest"
)

const (
	ticket  = "ticket-eyJhbGciOiJIUzI1NiJ9-secret"
	apiKey  = "api-key-0123456789abcdefghijklmnop"
	payload = `{"thread":"payload-text-42"}`
)

func TestAPanicInAHandlerSendsTheErrorAndTheAppIdAlone(t *testing.T) {
	in, r := errreporttest.Serve(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer r.Recover("chat")
		_, _ = io.ReadAll(req.Body)
		panic(errors.New("command broke"))
	})
	req := httptest.NewRequest(http.MethodPost, "/apps/chat/publish?ticket="+ticket, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	func() {
		defer func() { _ = recover() }()
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	r.Error("chat", errors.New("presence: the app answered 503"))

	got := in.Received()
	if !strings.Contains(got, "/api/7/envelope/") {
		t.Fatalf("no envelope reached the ingest endpoint: %q", got)
	}
	for _, want := range []string{"command broke", "presence: the app answered 503", `"app":"chat"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the envelope lacks %s: %s", want, got)
		}
	}
	for _, secret := range []string{ticket, apiKey, "payload-text-42", "/apps/chat/publish", `"user"`, `"request"`} {
		if strings.Contains(got, secret) {
			t.Errorf("the envelope holds %s: %s", secret, got)
		}
	}
}

func TestRecoverReportsAPanicAndPanicsAgain(t *testing.T) {
	in, r := errreporttest.Serve(t)
	defer func() {
		if recover() == nil {
			t.Fatal("Recover swallowed the panic")
		}
		got := in.Received()
		if !strings.Contains(got, "socket broke") || !strings.Contains(got, `"app":"chat"`) {
			t.Fatalf("the panic was not reported: %q", got)
		}
	}()
	func() {
		defer r.Recover("chat")
		panic("socket broke")
	}()
}

func TestRecoverLeavesAnAbortedRequestUnreported(t *testing.T) {
	in, r := errreporttest.Serve(t)
	func() {
		defer func() { _ = recover() }()
		defer r.Recover("chat")
		panic(http.ErrAbortHandler)
	}()
	if got := in.Received(); got != "" {
		t.Fatalf("an aborted request was reported: %q", got)
	}
}

func TestContainReportsAPanicAndEndsTheConnection(t *testing.T) {
	in, r := errreporttest.Serve(t)
	ended := false
	func() {
		defer r.Contain("chat", func() { ended = true })
		panic("writer broke")
	}()
	if !ended {
		t.Fatal("the connection was not ended")
	}
	if !strings.Contains(in.Received(), "writer broke") {
		t.Fatal("the panic was not reported")
	}
}

func TestOffSendsNothingAndStillRecoversPanics(t *testing.T) {
	r, err := errreport.New("", "test")
	if err != nil {
		t.Fatal(err)
	}
	r.Error("chat", errors.New("ignored"))
	ended := false
	func() {
		defer r.Contain("chat", func() { ended = true })
		panic("writer broke")
	}()
	if !ended {
		t.Fatal("the connection was not ended")
	}
}

func TestAnInvalidDSNNamesTheVariableAndNotTheDSN(t *testing.T) {
	_, err := errreport.New("not a dsn with a-secret-key", "test")
	if err == nil || !strings.Contains(err.Error(), "RELAY_SENTRY_DSN") || strings.Contains(err.Error(), "a-secret-key") {
		t.Fatalf("got %v", err)
	}
}
