package main

import (
	"fmt"
	"os"

	"github.com/PrototeamHQ/protobase-realtime/internal/config"
	"github.com/PrototeamHQ/protobase-realtime/internal/relay"
)

// reload reads the apps file again and swaps it in. A file that fails to load is refused whole: the relay keeps
// serving the apps it had.
func reload(r *relay.Relay, path string) {
	apps, err := config.Load(path)
	if err != nil {
		reportReloadError(err)
		return
	}
	r.Reload(apps)
}

// reportReloadError is where a refused apps file is reported.
func reportReloadError(err error) {
	fmt.Fprintln(os.Stderr, "relay: apps file refused, still serving the previous apps:", err)
}
