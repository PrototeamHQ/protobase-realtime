package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/socket"
)

const emptyKey = "0123456789abcdef0123456789abcdef"

func TestAServerWithNoAppsAnswersHealthAndRefusesAppCommandsAndSockets(t *testing.T) {
	r := New(nil, time.Now, errreport.Off())

	res := httptest.NewRecorder()
	r.Handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	if res.Code != http.StatusOK {
		t.Fatalf("health got %d", res.Code)
	}

	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/apps/chat/publish", nil)
	req.Header.Set("Authorization", "Bearer "+emptyKey)
	r.Handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("publish got %d", res.Code)
	}

	claims := jwt.MapClaims{"iss": "chat", "sub": "u1", "sid": "s1", "jti": "j1",
		"exp": time.Now().Add(time.Minute).Unix(), "channels": []string{"room"}}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(emptyKey))
	if err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	r.Handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet,
		socket.Path+"?vsn="+socket.Protocol+"&ticket="+signed, nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("socket got %d", res.Code)
	}
}
