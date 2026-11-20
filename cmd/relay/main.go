// Command relay serves the realtime relay. RELAY_APPS_FILE names the apps file; RELAY_ADDR the listen address.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/relay"
)

const defaultAddr = ":4000"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "relay:", err)
		os.Exit(1)
	}
}

func run() error {
	path := os.Getenv("RELAY_APPS_FILE")
	if path == "" {
		return errors.New("RELAY_APPS_FILE must name the apps file")
	}
	apps, err := config.Load(path)
	if err != nil {
		return err
	}
	addr := os.Getenv("RELAY_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	r := relay.New(apps, time.Now)
	server := &http.Server{Addr: addr, Handler: r.Handler, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	served := make(chan error, 1)
	go func() { served <- server.ListenAndServe() }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	// Sockets are hijacked connections that Shutdown does not track, so the hub closes them.
	r.Hub.CloseAll()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
