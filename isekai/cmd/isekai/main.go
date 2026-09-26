// Command isekai runs an agent session on a world: the law as a harness (canon/binary.md).
// It is one of two distributions of the same engine (isekai/app); agent-one is the other.
package main

import (
	"os"

	"github.com/Kaginari/agentic-playground/isekai/app"
)

// Set by GoReleaser: -ldflags "-X main.version=… -X main.commit=… -X main.date=…".
var version, commit, date = "dev", "none", "unknown"

func main() {
	os.Exit(app.Main("isekai", os.Args[1:], app.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Env: os.Getenv}, app.Version{Version: version, Commit: commit, Date: date}))
}
