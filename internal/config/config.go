// Package config reads the apps the relay serves from a JSON file.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// Limits caps what one app may use of the relay.
type Limits struct {
	Connections       int `json:"connections"`
	ChannelsPerTicket int `json:"channels_per_ticket"`
	RequestBytes      int `json:"request_bytes"`
}

// App is one tenant. Any key in a list is valid, so a key rotates without downtime.
type App struct {
	ID             string   `json:"id"`
	APIKeys        []string `json:"api_keys"`
	TicketKeys     []string `json:"ticket_keys"`
	Origins        []string `json:"origins"`
	ClientMessages bool     `json:"client_messages"`
	Limits         Limits   `json:"limits"`
}

type File struct {
	Apps []App `json:"apps"`
}

const minKeyLength = 32

var appID = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

func Load(path string) ([]App, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) ([]App, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var file File
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("apps file: %w", err)
	}
	if len(file.Apps) == 0 {
		return nil, fmt.Errorf("apps file: lists no apps")
	}
	ids := map[string]bool{}
	for _, app := range file.Apps {
		if err := validate(app); err != nil {
			return nil, fmt.Errorf("apps file: app %q: %w", app.ID, err)
		}
		if ids[app.ID] {
			return nil, fmt.Errorf("apps file: app %q is listed twice", app.ID)
		}
		ids[app.ID] = true
	}
	return file.Apps, nil
}

func validate(app App) error {
	if !appID.MatchString(app.ID) {
		return fmt.Errorf("id must be 1 to 64 of a-z, 0-9, _ and -")
	}
	if err := validateKeys("api_keys", app.APIKeys); err != nil {
		return err
	}
	if err := validateKeys("ticket_keys", app.TicketKeys); err != nil {
		return err
	}
	if app.Limits.Connections <= 0 || app.Limits.ChannelsPerTicket <= 0 || app.Limits.RequestBytes <= 0 {
		return fmt.Errorf("limits.connections, limits.channels_per_ticket and limits.request_bytes must be positive")
	}
	return nil
}

func validateKeys(name string, keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("%s lists no key", name)
	}
	for _, key := range keys {
		if len(key) < minKeyLength {
			return fmt.Errorf("%s holds a key shorter than %d characters", name, minKeyLength)
		}
	}
	return nil
}
