// Command tt administers a go-torrent-trader site from the terminal.
package main

import (
	"os"

	"github.com/williamokano/go-torrent-trader/cli/internal/command"
)

// version is overridden at build time with -ldflags "-X main.version=v1.2.3".
// The release workflow supplies the tag; a plain `go build` reports "dev".
var version = "dev"

// run returns the process exit code, following the same pattern as the backend's
// main so the wiring stays testable.
func run() int {
	return command.Execute(version, os.Stdout, os.Stderr, os.Args[1:])
}

func main() {
	os.Exit(run())
}
