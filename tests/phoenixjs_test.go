package tests

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The stock Phoenix JavaScript client, from tests/phoenixjs (npm ci there first), against the relay.
func TestPhoenixClientJoinsListedChannelsOnly(t *testing.T) {
	srv := start(t)
	endpoint := "ws" + strings.TrimPrefix(srv.URL, "http") + "/socket"
	cmd := exec.Command("node", "phoenixjs/client.mjs", endpoint, issue(t, appA, person{"u1", "s1", []string{"room"}}))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	type step struct {
		Step   string          `json:"step"`
		Detail json.RawMessage `json:"detail"`
	}
	steps := make(chan step)
	go func() {
		lines := bufio.NewScanner(stdout)
		for lines.Scan() {
			var s step
			if json.Unmarshal(lines.Bytes(), &s) == nil {
				steps <- s
			}
		}
		close(steps)
	}()
	next := func() step {
		select {
		case s, ok := <-steps:
			if !ok {
				t.Fatal("the client exited early")
			}
			return s
		case <-time.After(10 * time.Second):
			t.Fatal("the client stalled")
			return step{}
		}
	}

	if s := next(); s.Step != "unlisted" || string(s.Detail) != `"unauthorized"` {
		t.Fatalf("unlisted join: %+v", s)
	}
	if s := next(); s.Step != "ready" {
		t.Fatalf("got %+v", s)
	}
	command(t, srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{"text":"hello"}}`)
	if s := next(); s.Step != "received" || string(s.Detail) != `{"text":"hello"}` {
		t.Fatalf("got %+v", s)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
