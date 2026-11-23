package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PrototeamHQ/protobase-realtime/internal/errreport/errreporttest"
)

func TestACommandsPanicIsReportedWithItsAppAlone(t *testing.T) {
	in, errs := errreporttest.Serve(t)
	mux := http.NewServeMux()
	mux.Handle("POST "+PublishPath, recovering{errs: errs, command: http.HandlerFunc(func(http.ResponseWriter,
		*http.Request) {
		panic("publish broke")
	})})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/apps/chat/publish", strings.NewReader(`{"secret":"p4yl0ad"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+keyA)
	if res, err := http.DefaultClient.Do(req); err == nil {
		res.Body.Close()
		t.Fatal("the server answered a request whose command panicked")
	}

	got := in.Received()
	if !strings.Contains(got, "publish broke") || !strings.Contains(got, `"app":"chat"`) {
		t.Fatalf("the panic was not reported with its app: %q", got)
	}
	for _, secret := range []string{keyA, "p4yl0ad"} {
		if strings.Contains(got, secret) {
			t.Errorf("the envelope holds %s", secret)
		}
	}
}
