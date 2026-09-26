// A stdio MCP server for the client's tests; built by the test itself.
package main

import (
	"os"

	"github.com/Kaginari/agent-one/mcp/internal/testserver"
)

func main() {
	testserver.New().ServeStdio(os.Stdin, os.Stdout)
}
