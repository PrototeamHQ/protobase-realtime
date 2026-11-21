package config

import (
	"fmt"
	"net/url"
)

// MaxGraceSeconds caps how long a person who closed their last socket still counts as present.
const MaxGraceSeconds = 3600

// Presence turns on presence reports: the relay posts who came online, went away or went offline to URL, with
// Key as the bearer token. A person whose last socket closed counts as offline only after GraceSeconds.
type Presence struct {
	URL          string `json:"url"`
	Key          string `json:"key"`
	GraceSeconds int    `json:"grace_seconds"`
}

func validatePresence(p *Presence) error {
	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("presence.url must be an http or https URL")
	}
	if len(p.Key) < minKeyLength {
		return fmt.Errorf("presence.key must be at least %d characters", minKeyLength)
	}
	if p.GraceSeconds < 0 || p.GraceSeconds > MaxGraceSeconds {
		return fmt.Errorf("presence.grace_seconds must be 0 to %d", MaxGraceSeconds)
	}
	return nil
}
