package guard

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed corpus.txt
var corpus string

// Case is one line of the corpus: a command that must be blocked or allowed.
type Case struct {
	Block bool
	Cmd   string
}

// Cases is the embedded corpus.
func Cases() []Case {
	var out []Case
	sc := bufio.NewScanner(strings.NewReader(corpus))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, cmd, _ := strings.Cut(line, " ")
		out = append(out, Case{Block: kind == "block", Cmd: strings.ReplaceAll(cmd, `\n`, "\n")})
	}
	return out
}

// Test runs the corpus against g; the wrong cases, one line each.
func (g *Guard) Test() (passed int, wrong []string) {
	for _, c := range Cases() {
		r := g.Match(c.Cmd)
		switch {
		case c.Block && r == nil:
			wrong = append(wrong, "not blocked: "+c.Cmd)
		case !c.Block && r != nil:
			wrong = append(wrong, "blocked ("+r.Source+"): "+c.Cmd)
		default:
			passed++
		}
	}
	return passed, wrong
}

// Hook answers an agent's pre-tool hook: the hook JSON on stdin (.tool_input.command for Claude
// Code and Codex, .toolInput.command for Grok, .command for Cursor). Blocked: exit 2 and the reason
// on stderr, or with cursor a deny JSON on stdout. Allowed: exit 0 (cursor: an allow JSON).
func (g *Guard) Hook(in io.Reader, out, errw io.Writer, cursor bool) int {
	var p struct {
		ToolInput  struct{ Command string } `json:"tool_input"`
		ToolInput2 struct{ Command string } `json:"toolInput"`
		Command    string                   `json:"command"`
	}
	b, _ := io.ReadAll(in)
	_ = json.Unmarshal(b, &p)
	cmd := p.ToolInput.Command
	if cmd == "" {
		cmd = p.ToolInput2.Command
	}
	if cmd == "" {
		cmd = p.Command
	}
	r := g.Match(cmd)
	if r == nil {
		if cursor {
			fmt.Fprintln(out, `{"permission":"allow"}`)
		}
		return 0
	}
	if cursor {
		msg, _ := json.Marshal(map[string]string{"permission": "deny", "user_message": "The command guard blocked a dangerous command.", "agent_message": Refusal(r)})
		fmt.Fprintln(out, string(msg))
		return 0
	}
	fmt.Fprintln(errw, Refusal(r))
	return 2
}

// ClaudeSettings merges the guard's PreToolUse hook into a Claude Code settings.json, keeping
// everything else; changed false when the hook is already there.
func ClaudeSettings(settings []byte, command string) (out []byte, changed bool, err error) {
	doc := map[string]any{}
	if len(strings.TrimSpace(string(settings))) > 0 {
		if err := json.Unmarshal(settings, &doc); err != nil {
			return nil, false, fmt.Errorf("settings.json does not parse: %w", err)
		}
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)
	for _, e := range pre {
		m, _ := e.(map[string]any)
		hs, _ := m["hooks"].([]any)
		for _, h := range hs {
			if hm, _ := h.(map[string]any); hm != nil && hm["command"] == command {
				return settings, false, nil
			}
		}
	}
	pre = append(pre, map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": command}}})
	hooks["PreToolUse"] = pre
	doc["hooks"] = hooks
	out, err = json.MarshalIndent(doc, "", "  ")
	return append(out, '\n'), true, err
}

// OpenCodePlugin is an OpenCode plugin that asks the binary's hook before every bash call.
func OpenCodePlugin(binary string) string {
	return `// The global dangerous-command guard (written by ` + filepath.Base(binary) + ` guard install).
import { spawnSync } from "node:child_process"

export const CommandGuard = async () => ({
  "tool.execute.before": async (input, output) => {
    if (input.tool !== "bash") return
    const r = spawnSync(` + fmt.Sprintf("%q", binary) + `, ["guard", "hook"], { input: JSON.stringify({ tool_input: { command: output.args?.command ?? "" } }) })
    if (r.status === 2) throw new Error(String(r.stderr).trim())
  },
})
`
}

// Exists reports whether a path exists.
func Exists(p string) bool { _, err := os.Stat(p); return err == nil }
