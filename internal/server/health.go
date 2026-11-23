package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// HealthPath answers 200 while the relay serves, for a container health check or an uptime monitor. It needs no
// auth and tells nothing about the apps.
const HealthPath = "/health"

type healthBody struct {
	OK bool `json:"ok"`
	// Started is when the relay started, in Unix milliseconds, so a monitor sees a restart.
	Started int64 `json:"started"`
}

func health(started time.Time) http.Handler {
	body, err := json.Marshal(healthBody{OK: true, Started: started.UnixMilli()})
	if err != nil {
		panic("server: health: " + err.Error())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body) // the client is gone when this fails
	})
}
