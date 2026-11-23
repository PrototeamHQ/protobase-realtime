package tests

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHangupReloadsTheAppsFileWithoutDroppingSockets(t *testing.T) {
	proc := startProcess(t, appA)
	member := connect(t, proc.srv, appA, person{"u1", "s1", []string{"room"}})
	member.join("room")

	proc.reload(appA, appB)
	waitServed(t, proc.srv, appB)
	command(t, proc.srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{}}`)
	if got := member.read(); got != `[null,null,"room","new",{}]` {
		t.Fatalf("after adding an app, a's member got %q", got)
	}
	added := connect(t, proc.srv, appB, person{"u1", "s1", []string{"room"}})
	added.join("room")
	command(t, proc.srv, appB, "/publish", `{"id":"1","channel":"room","event":"new","payload":{}}`)
	if got := added.read(); got != `[null,null,"room","new",{}]` {
		t.Fatalf("the added app's member got %q", got)
	}

	proc.reload(appB)
	if code := member.closeStatus(); code != websocket.StatusNormalClosure {
		t.Fatalf("a removed app's socket closed with %v", code)
	}
	if got := added.read(); got != "" {
		t.Fatalf("the remaining app's member got %q", got)
	}
}

func TestHangupWithABrokenAppsFileKeepsServing(t *testing.T) {
	proc := startProcess(t, appA)
	member := connect(t, proc.srv, appA, person{"u1", "s1", []string{"room"}})
	member.join("room")
	proc.write(`{}`)
	proc.hangup()
	waitFor(t, func() bool { return strings.Contains(proc.stderr.String(), "apps file refused") })
	command(t, proc.srv, appA, "/publish", `{"id":"1","channel":"room","event":"new","payload":{}}`)
	if got := member.read(); got != `[null,null,"room","new",{}]` {
		t.Fatalf("got %q", got)
	}
}

// process is the relay binary running on its own port, as an operator runs it.
type process struct {
	t      *testing.T
	cmd    *exec.Cmd
	file   string
	stderr *lockedBuffer
	// srv carries only the relay's URL, for the helpers that take a test server.
	srv *httptest.Server
}

func startProcess(t *testing.T, apps ...testApp) *process {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "protobase-realtime")
	if out, err := exec.Command("go", "build", "-o", binary, "../cmd/protobase-realtime").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	addr := freeAddr(t)
	p := &process{t: t, file: filepath.Join(dir, "apps.json"), stderr: &lockedBuffer{},
		srv: &httptest.Server{URL: "http://" + addr}}
	p.write(appsFile(apps...))
	p.cmd = exec.Command(binary)
	p.cmd.Env = append(os.Environ(), "PROTOBASE_REALTIME_APPS_FILE="+p.file, "PROTOBASE_REALTIME_ADDR="+addr)
	p.cmd.Stderr = p.stderr
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	})
	waitFor(t, func() bool {
		res, err := http.Get(p.srv.URL + "/socket/websocket")
		if err != nil {
			return false
		}
		res.Body.Close()
		return true
	})
	return p
}

func (p *process) write(contents string) {
	p.t.Helper()
	if err := os.WriteFile(p.file, []byte(contents), 0o600); err != nil {
		p.t.Fatal(err)
	}
}

func (p *process) hangup() {
	p.t.Helper()
	if err := p.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		p.t.Fatal(err)
	}
}

// lockedBuffer holds what the relay writes to stderr, which the test reads while the relay runs.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (p *process) reload(apps ...testApp) {
	p.t.Helper()
	p.write(appsFile(apps...))
	p.hangup()
}

func appsFile(apps ...testApp) string {
	var entries []string
	for _, a := range apps {
		entries = append(entries, `{"id":"`+a.id+`","api_keys":["`+a.apiKey+`"],"ticket_keys":["`+a.ticketKey+`"],`+
			`"limits":{"connections":50,"channels_per_ticket":10,"request_bytes":4096}}`)
	}
	return `{"apps":[` + strings.Join(entries, ",") + `]}`
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// waitServed waits until the relay takes an app's commands, which it does once the reload swapped the app in.
func waitServed(t *testing.T, srv *httptest.Server, app testApp) {
	t.Helper()
	waitFor(t, func() bool {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/apps/"+app.id+"/people/rules",
			strings.NewReader(`{"user":"nobody"}`))
		req.Header.Set("Authorization", "Bearer "+app.apiKey)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		res.Body.Close()
		return res.StatusCode == http.StatusOK
	})
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
