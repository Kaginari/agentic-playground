// Command agent-one runs an agent session on a workspace: the same engine as agent-one
// (agent-one/app) under the agent-one vocabulary — .agent-one/, AGENT-ONE.md, AGENT_ONE_ env.
package main

import (
	"os"

	"github.com/Kaginari/agent-one/app"
)

// Set by GoReleaser: -ldflags "-X main.version=… -X main.commit=… -X main.date=…".
var version, commit, date = "dev", "none", "unknown"

func main() {
	os.Exit(app.Main("agent-one", os.Args[1:], app.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Env: os.Getenv}, app.Version{Version: version, Commit: commit, Date: date}))
}
