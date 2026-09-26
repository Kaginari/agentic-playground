package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/provider"
)

func timeZero() time.Time { return time.Time{} }

// relTo renders p relative to root when it lies inside it.
func relTo(root, p string) (string, error) {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(r, "..") {
		return p, nil
	}
	return filepath.ToSlash(r), nil
}

type provider_Message = provider.Message
type provider_ToolResult = provider.ToolResult

func jsonMarshal(v interface{}) ([]byte, error) { return json.Marshal(v) }

// parseDist resolves a distribution name without a config load.
func parseDist(dist string) (config.Dist, error) {
	if dist == "" {
		dist = config.Name
	}
	return config.ParseDist(dist, "", nil)
}

// runGo builds a Go main package into bin with the toolchain on PATH or beside this binary's
// runtime; it returns the tool's output on failure.
func runGo(src, bin string) (string, error) {
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		p, err := exec.LookPath("go")
		if err != nil {
			return "", err
		}
		goBin = p
	}
	cmd := exec.Command(goBin, "build", "-o", bin, ".")
	cmd.Dir = src
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// startBoard and cmdBoard are the board's seat (rung 5); boardLine is its status line.
