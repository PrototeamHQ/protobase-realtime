package presence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fixed pacing of reports.
const (
	// Debounce is how long a status must hold before it is reported, so a quick flap reaches the app as nothing.
	Debounce = 2 * time.Second
	// Tick is how often due changes are sent.
	Tick = time.Second
	// BatchSize caps the changes in one report.
	BatchSize = 500
	// maxBackoff caps the wait after failed reports.
	maxBackoff = time.Minute
	timeout    = 10 * time.Second
)

// report is the body posted to the app. Started is when the relay started: once it differs from the last report's,
// the relay restarted, and the people it has not reported since are offline.
type report struct {
	Started int64    `json:"started"`
	Changes []Change `json:"changes"`
}

// Reporter posts a tracker's changes to the app until the app answers 2xx, so each change arrives at least once.
type Reporter struct {
	Tracker *Tracker
	URL     string
	Key     string
	Client  *http.Client
	Started time.Time
}

func NewReporter(t *Tracker, url, key string, started time.Time) *Reporter {
	return &Reporter{Tracker: t, URL: url, Key: key, Client: &http.Client{Timeout: timeout}, Started: started}
}

// Run sends due changes every tick until ctx ends. After a failed report it waits twice as long as before, up to
// a minute, before it tries again.
func (r *Reporter) Run(ctx context.Context) {
	ticker := time.NewTicker(Tick)
	defer ticker.Stop()
	backoff := time.Duration(0)
	var next time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if now.Before(next) {
				continue
			}
			if r.Flush(ctx) == nil {
				backoff = 0
				continue
			}
			backoff = min(max(2*backoff, Tick), maxBackoff)
			next = now.Add(backoff)
		}
	}
}

// Flush sends every due change, a batch at a time, and stops at the first batch the app does not accept.
func (r *Reporter) Flush(ctx context.Context) error {
	for {
		changes := r.Tracker.Due(BatchSize)
		if len(changes) == 0 {
			return nil
		}
		if err := r.post(ctx, changes); err != nil {
			return err
		}
		r.Tracker.Acknowledge(changes)
	}
}

func (r *Reporter) post(ctx context.Context, changes []Change) error {
	body, err := json.Marshal(report{Started: r.Started.UnixMilli(), Changes: changes})
	if err != nil {
		panic("presence: report: " + err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// Reading a short answer to the end lets the connection be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("presence: the app answered %d", res.StatusCode)
	}
	return nil
}
