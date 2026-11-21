package main

import (
	"fmt"
	"os"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/errreport"
	"github.com/PrototeamHQ/protobase-realtime/internal/relay"
)

// load reads the apps file, at startup and on each reload, and reports a file that fails to load.
func load(path string, errs *errreport.Reporter) ([]config.App, error) {
	apps, err := config.Load(path)
	if err != nil {
		errs.Error("", err)
	}
	return apps, err
}

// reload reads the apps file again and swaps it in. A file that fails to load is refused whole: the relay keeps
// serving the apps it had.
func reload(r *relay.Relay, path string, errs *errreport.Reporter) {
	apps, err := load(path, errs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay: apps file refused, still serving the previous apps:", err)
		return
	}
	r.Reload(apps)
}
