package socket

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

// originAllowed checks a browser's origin as the WebSocket upgrade does: a request without one, the relay's own
// host, and the app's origin patterns, hosts or scheme://host, pass.
func originAllowed(r *http.Request, patterns []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(r.Host, u.Host) {
		return true
	}
	for _, pattern := range patterns {
		target := u.Host
		if strings.Contains(pattern, "://") {
			target = u.Scheme + "://" + u.Host
		}
		// A malformed pattern matches nothing.
		if ok, _ := path.Match(strings.ToLower(pattern), strings.ToLower(target)); ok {
			return true
		}
	}
	return false
}

// allowOrigin lets a browser on an allowed origin read the response, or answers 403. It has answered when it
// returns false.
func allowOrigin(w http.ResponseWriter, r *http.Request, patterns []string) bool {
	if !originAllowed(r, patterns) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return false
	}
	w.Header().Add("Vary", "Origin")
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	return true
}
