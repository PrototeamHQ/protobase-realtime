// Command protobase-realtime serves the realtime relay. PROTOBASE_REALTIME_APPS_FILE names the apps file, which it
// reads again on SIGHUP; PROTOBASE_REALTIME_ADDR the listen address; PROTOBASE_REALTIME_SENTRY_DSN, when set, the error
// tracker errors are reported to. With -version it prints its version and exits.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/server"
)

const defaultAddr = ":4000"

// flushTimeout bounds the wait for reported errors to be sent as the relay stops.
const flushTimeout = 5 * time.Second

// version is set at release builds with -ldflags "-X main.version=<version>".
var version = "dev"

func main() {
	printVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *printVersion {
		fmt.Println(version)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "protobase-realtime:", err)
		os.Exit(1)
	}
}

func run() error {
	errs, err := errreport.New(os.Getenv("PROTOBASE_REALTIME_SENTRY_DSN"), version)
	if err != nil {
		return err
	}
	defer errs.Flush(flushTimeout)
	path := os.Getenv("PROTOBASE_REALTIME_APPS_FILE")
	if path == "" {
		return errors.New("PROTOBASE_REALTIME_APPS_FILE must name the apps file")
	}
	apps, err := load(path, errs)
	if err != nil {
		return err
	}
	addr := os.Getenv("PROTOBASE_REALTIME_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	r := server.New(apps, time.Now, errs)
	server := &http.Server{Addr: addr, Handler: r.Handler, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	hangup := make(chan os.Signal, 1)
	signal.Notify(hangup, syscall.SIGHUP)
	defer signal.Stop(hangup)
	r.Start(ctx)
	served := make(chan error, 1)
	go func() { served <- server.ListenAndServe() }()
	if err := serve(ctx, served, hangup, r, path, errs); err != nil {
		return err
	}
	// Sockets are hijacked connections that Shutdown does not track, so the hub closes them.
	r.Hub.CloseAll()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// serve waits until the server fails or ctx ends, reloading the apps file on each hangup.
func serve(ctx context.Context, served <-chan error, hangup <-chan os.Signal, r *server.Server, path string,
	errs *errreport.Reporter) error {
	for {
		select {
		case err := <-served:
			return err
		case <-ctx.Done():
			return nil
		case <-hangup:
			reload(r, path, errs)
		}
	}
}
