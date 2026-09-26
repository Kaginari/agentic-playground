package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
)

// benchTask is one fixed task of `bench`: an ask, the mock's script for it, and the check.
type benchTask struct {
	Name   string
	Ask    string
	Script []map[string]interface{}
	Check  func(w *benchWorkspace, r *loop.Result) string // "" = pass, else why it failed
}

type benchWorkspace struct {
	root string
	a    *App
}

func (w *benchWorkspace) read(p string) string {
	b, _ := os.ReadFile(filepath.Join(w.root, p))
	return string(b)
}

// benchTasks is the fixed set: edit in ownership, pass the gate, dispatch a Subagent with a valid
// report and @U, survive a drain, a custom tool call, an MCP call.
func benchTasks(mcpBin string) []benchTask {
	step := func(name string, in interface{}) map[string]interface{} {
		return map[string]interface{}{"calls": []interface{}{map[string]interface{}{"name": name, "input": in}}}
	}
	text := func(s string) map[string]interface{} { return map[string]interface{}{"text": s} }
	tasks := []benchTask{
		{
			// the zone's own zone worker edits in its ownership and keeps its doc truthful; the gate passes
			Name: "edit in ownership + gate",
			Ask:  "have zone-auth add a comment to src/auth/login.go",
			Script: []map[string]interface{}{
				step("dispatch", map[string]string{"agent": "zone-auth", "ask": "findings: add a comment to login.go", "scope": "src/auth"}),
				text("@S DONE\n@E 12"),
			},
			Check: func(w *benchWorkspace, r *loop.Result) string {
				if r.Status != loop.Done {
					return "status " + r.Status + " " + strings.Join(r.Holes, "; ")
				}
				if !strings.Contains(w.read("src/auth/login.go"), "// bench") {
					return "the edit did not land: " + firstLine(strings.Join(outputsOf(r), "|"))
				}
				if !strings.Contains(w.read(".agent-one/log.md"), "zone-auth — gate pass") {
					return "no gate pass for zone-auth in log.md"
				}
				return ""
			},
		},
		{
			Name: "dispatch a Subagent (report + @U)",
			Ask:  "dispatch zone-auth to look at the login zone",
			Script: []map[string]interface{}{
				step("dispatch", map[string]string{"agent": "zone-auth", "ask": "findings: look at src/auth", "scope": "src/auth"}),
				text("@S DONE\n@E 12"),
			},
			Check: func(w *benchWorkspace, r *loop.Result) string {
				if r.Status != loop.Done || len(r.Steps) != 1 {
					return "status " + r.Status
				}
				out := r.Steps[0].Result.Output
				if !strings.Contains(out, "@S PASS") || !strings.Contains(out, "@U ") {
					return "subagent report: " + firstLine(out)
				}
				return ""
			},
		},
		{
			Name: "survive a drain",
			Ask:  "echo five bulky things",
			Script: func() []map[string]interface{} {
				var s []map[string]interface{}
				for i := 0; i < 5; i++ {
					s = append(s, step("bash", map[string]string{"command": "seq 1 300"}))
				}
				return append(s, text("@S DONE\n@E 12"))
			}(),
			Check: func(w *benchWorkspace, r *loop.Result) string {
				if r.Status != loop.Done || len(r.Steps) != 5 {
					return "status " + r.Status + " steps " + fmt.Sprint(len(r.Steps)) + " " + strings.Join(r.Holes, "; ")
				}
				if w.a.Drainer == nil || w.a.Drainer.Last == nil || w.a.Drainer.Last.Aborted != "" {
					return "no drain happened"
				}
				return ""
			},
		},
		{
			Name: "custom tool call",
			Ask:  "list the root with lsl",
			Script: []map[string]interface{}{
				step("lsl", map[string]string{"path": "."}),
				text("@S DONE\n@E 12"),
			},
			Check: func(w *benchWorkspace, r *loop.Result) string {
				if r.Status != loop.Done || len(r.Steps) != 1 || !strings.Contains(r.Steps[0].Result.Output, "README.md") {
					return "lsl: " + r.Status + " " + firstLine(strings.Join(outputsOf(r), "|"))
				}
				return ""
			},
		},
	}
	if mcpBin != "" {
		tasks = append(tasks, benchTask{
			Name: "MCP call",
			Ask:  "add 2 and 3 through the MCP server",
			Script: []map[string]interface{}{
				step("mcp__t__add", map[string]int{"a": 2, "b": 3}),
				text("@S DONE\n@E 12"),
			},
			Check: func(w *benchWorkspace, r *loop.Result) string {
				if r.Status != loop.Done || len(r.Steps) != 1 || strings.TrimSpace(r.Steps[0].Result.Output) != "5" {
					return "mcp add: " + r.Status + " " + firstLine(strings.Join(outputsOf(r), "|"))
				}
				return ""
			},
		})
	}
	return tasks
}

func outputsOf(r *loop.Result) []string {
	var out []string
	for _, st := range r.Steps {
		out = append(out, st.Result.Output)
	}
	return out
}

// subagentScript is the Subagent's side of the dispatch task and the drain's answer.
var subagentScript = []map[string]interface{}{
	{"when": "drained messages", "text": "@S DRAINED\n@F bench drained\n@U team the bench drained once\n@D goal: echo\n@E 70", "repeat": true},
	{"when": "add a comment", "calls": []interface{}{
		map[string]interface{}{"name": "edit", "input": map[string]string{"path": "src/auth/login.go", "old": "package auth", "new": "package auth // bench"}},
		map[string]interface{}{"name": "write", "input": map[string]string{"path": ".agent-one/zone/auth/README.md", "content": "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- bench comment added\n"}},
	}},
	{"when": "edited src/auth/login.go", "text": "@S PASS\n@F src/auth/login.go:1 comment added\n@U domain login.go is the entry\n@E 70"},
	{"when": "@ASK findings", "text": "@S PASS\n@F src/auth/login.go:1 the entry\n@U domain login is the entry\n@E 60", "repeat": true},
}

// Bench runs the fixed task set on the mock and on every configured, keyed real provider,
// and prints a table. A real provider without its key is skipped, never faked.
func (a *App) Bench(ctx context.Context, io IO) int {
	mcpBin := a.benchMCPServer()
	rows := []string{fmt.Sprintf("%-28s %-30s %-7s %8s %s", "task", "model", "result", "ms", "why")}
	failed := 0
	// the mock: scripted, always
	for _, t := range benchTasks(mcpBin) {
		res, ms, why := a.benchOne(ctx, "mock", t, mcpBin)
		if res != "pass" {
			failed++
		}
		rows = append(rows, fmt.Sprintf("%-28s %-30s %-7s %8d %s", t.Name, "mock/bench", res, ms, why))
	}
	// real providers: the same asks, judged by the same checks, when a key exists
	for _, name := range a.Cfg.ProviderNames() {
		p := a.Cfg.Providers[name]
		if p == nil || !p.Enabled || p.Type == "mock" {
			continue
		}
		if p.APIKeyEnv != "" && a.Opt.Env(p.APIKeyEnv) == "" {
			rows = append(rows, fmt.Sprintf("%-28s %-30s %-7s %8s %s", "(all)", name, "skip", "—", p.APIKeyEnv+" is not set"))
			continue
		}
		model := ""
		if m, _ := a.Cfg.Mount(); m.Provider == name {
			model = m.Ref.Model
		} else if bm, _ := a.Cfg.ResolveModel("", "", "", "bench"); bm.Provider == name {
			model = bm.Ref.Model
		}
		if model == "" {
			rows = append(rows, fmt.Sprintf("%-28s %-30s %-7s %8s %s", "(all)", name, "skip", "—", "no model of this provider is the mount or models.tasks.bench"))
			continue
		}
		for _, t := range benchTasks(mcpBin) {
			res, ms, why := a.benchOne(ctx, model, t, mcpBin)
			if res == "fail" {
				failed++
			}
			rows = append(rows, fmt.Sprintf("%-28s %-30s %-7s %8d %s", t.Name, model, res, ms, why))
		}
	}
	for _, r := range rows {
		fmt.Fprintln(io.Out, r)
	}
	if failed > 0 {
		fmt.Fprintf(io.Out, "@S FAIL %d task(s) failed\n", failed)
		return 1
	}
	fmt.Fprintln(io.Out, "@S PASS")
	return 0
}

// benchMCPServer builds the test MCP server when a Go toolchain is at hand; "" otherwise.
func (a *App) benchMCPServer() string {
	src := ""
	for _, cand := range []string{filepath.Join(a.Root, "app", "testdata", "mcpserver")} {
		if _, err := os.Stat(filepath.Join(cand, "main.go")); err == nil {
			src = cand
			break
		}
	}
	if src == "" {
		return ""
	}
	bin := filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "tmp", "bench-mcpserver")
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	if out, err := runGo(src, bin); err != nil {
		a.hole("bench: cannot build the MCP test server: " + firstLine(out))
		return ""
	}
	return bin
}

// benchOne runs one task in a throwaway workspace on one model.
func (a *App) benchOne(ctx context.Context, model string, t benchTask, mcpBin string) (result string, ms int64, why string) {
	tmp := filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "tmp")
	_ = os.MkdirAll(tmp, 0o755)
	T, err := os.MkdirTemp(tmp, "bench-")
	if err != nil {
		return "fail", 0, err.Error()
	}
	defer os.RemoveAll(T)
	root := filepath.Join(T, "workspace")
	write := func(p, s string) {
		full := filepath.Join(root, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(s), 0o644)
	}
	write(".agent-one/AGENT-ONE.md", PolicyText())
	write(".agent-one/log.md", "# Chronicle\n\n---\n")
	write(".agent-one/domain/security/README.md", "# domain-security\n\n- **Rank:** Domain owner\n- **Owns:** `src/`\n")
	write(".agent-one/zone/auth/README.md", "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n")
	write("src/auth/login.go", "package auth\n")
	write("README.md", "bench\n")
	sb, _ := json.Marshal(t.Script)
	write(".agent-one/tmp/session.json", string(sb))
	cb, _ := json.Marshal(subagentScript)
	write(".agent-one/tmp/subagent.json", string(cb))
	mount := "mock/bench"
	providers := "providers:\n  mock: {enabled: true, script: .agent-one/tmp/session.json}\n  subagent: {type: mock, script: .agent-one/tmp/subagent.json}\n"
	if model != "mock" {
		mount = model
		p, _, _ := splitModel(model)
		providers = "providers:\n  subagent: {type: mock, script: .agent-one/tmp/subagent.json}\n"
		if e := a.Cfg.Providers[p]; e != nil {
			eb, _ := json.Marshal(e)
			providers += "  " + p + ": " + string(eb) + "\n"
		}
	}
	mcp := ""
	if mcpBin != "" {
		mcp = "mcp:\n  servers:\n    t: {command: [" + mcpBin + "], inward: true, sandbox: none}\n"
	}
	write(".agent-one/config.yaml", "models:\n  default: "+mount+"\n  roles: {analyst: subagent/c}\n  tasks: {drain: subagent/c}\n"+providers+
		"tools:\n  bash: {sandbox: none}\n  custom:\n    lsl: {description: list, class: read, params: {path: {type: string, required: true}}, run: [ls, \"-1\", \"{{path}}\"]}\n"+
		"policy: {budget: {stressTokens: 2600}}\ncompaction: {keepRecentTurns: 1}\nui: {board: {autostart: false}}\n"+mcp)
	var out, errb strings.Builder
	app, err := New(Options{Dist: "agent-one", Root: root, Cwd: root, Home: a.Opt.Home, Env: a.Opt.Env, In: strings.NewReader(""), Out: &out, Err: &errb, Quiet: true, IsTTY: func() bool { return false }, NoBoard: true})
	if err != nil {
		return "fail", 0, err.Error()
	}
	defer app.Close()
	e, err := app.Engine()
	if err != nil {
		return "fail", 0, err.Error()
	}
	t0 := time.Now()
	r, err := e.Run(ctx, t.Ask)
	ms = time.Since(t0).Milliseconds()
	if err != nil {
		return "fail", ms, err.Error()
	}
	if why := t.Check(&benchWorkspace{root: root, a: app}, r); why != "" {
		return "fail", ms, why
	}
	return "pass", ms, ""
}

func splitModel(ref string) (string, string, bool) {
	p, m, ok := strings.Cut(ref, "/")
	return p, m, ok
}

var _ = provider.StopEnd
