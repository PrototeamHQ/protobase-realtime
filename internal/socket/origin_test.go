package socket

import (
	"net/http/httptest"
	"testing"
)

func TestOriginAllowedMatchesTheWebSocketUpgrade(t *testing.T) {
	patterns := []string{"*.allowed.example", "https://exact.example"}
	cases := map[string]bool{
		"":                          true,
		"http://example.com":        true, // the relay's own host
		"https://a.allowed.example": true,
		"https://exact.example":     true,
		"http://exact.example":      false,
		"https://other.example":     false,
	}
	for origin, want := range cases {
		r := httptest.NewRequest("GET", "http://example.com/socket/sse", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if got := originAllowed(r, patterns); got != want {
			t.Errorf("%q: got %v", origin, got)
		}
	}
}
