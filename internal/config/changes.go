package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
)

// relayKeySize is the length of a changes service's relay key, which the service takes as base64url without
// padding.
const relayKeySize = 32

// ChangesService is a changes service the relay registers live queries with, over one connection for every app that
// names it. Key is read from KeyFile each time the apps file loads, so a new key takes effect on reload.
type ChangesService struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	KeyFile string `json:"key_file"`
	Key     string `json:"-"`
}

// resolveChanges validates the services and points each app that names one at it.
func resolveChanges(services []ChangesService, apps []App) error {
	byID := map[string]*ChangesService{}
	for i := range services {
		s := &services[i]
		if err := validateChanges(s); err != nil {
			return fmt.Errorf("apps file: changes service %q: %w", s.ID, err)
		}
		if byID[s.ID] != nil {
			return fmt.Errorf("apps file: changes service %q is listed twice", s.ID)
		}
		byID[s.ID] = s
	}
	for i := range apps {
		app := &apps[i]
		if app.Changes == "" {
			continue
		}
		app.ChangesService = byID[app.Changes]
		if app.ChangesService == nil {
			return fmt.Errorf("apps file: app %q: changes names no listed service", app.ID)
		}
	}
	return nil
}

func validateChanges(s *ChangesService) error {
	if !appID.MatchString(s.ID) {
		return fmt.Errorf("id must be 1 to 64 of a-z, 0-9, _ and -")
	}
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "wss" && u.Scheme != "ws") || u.Host == "" {
		return fmt.Errorf("url must be a ws or wss URL")
	}
	raw, err := os.ReadFile(s.KeyFile)
	if err != nil {
		return fmt.Errorf("key_file: %w", err)
	}
	key := string(bytes.TrimSpace(raw))
	decoded, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(decoded) != relayKeySize {
		return fmt.Errorf("key_file must hold one %d-byte key, base64url without padding", relayKeySize)
	}
	s.Key = key
	return nil
}
