package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/tool"
)

// A deny (or ask) on `bash:git push*` holds against the shapes the same act hides in: env,
// sudo, xargs, a quoted `sh -c`, a `cd … &&` prefix, a subshell, git's global options. An allow
// still needs the whole command — a rule matched on an inner form only tightens.
func TestRulesHoldOnCommandForms(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "w")
	os.MkdirAll(filepath.Join(root, ".isekai"), 0o755)
	os.WriteFile(filepath.Join(root, ".isekai", "config.yaml"), []byte("permissions:\n  rules:\n    - {match: \"bash:git push*\", action: deny}\n    - {match: \"bash:make test\", action: allow}\n    - {match: \"bash:rm -rf *\", action: ask}\n"), 0o644)
	cfg, err := config.LoadWith(config.Options{Dist: "isekai", Root: root, Home: filepath.Join(dir, "h"), Env: func(string) string { return "" }})
	if err != nil {
		t.Fatal(err)
	}
	env := func() tool.Env { return tool.Env{Root: root} }
	decide := decideHook(cfg, env)
	ask := func(cmd string) loop.Decision {
		in, _ := json.Marshal(map[string]string{"command": cmd})
		st := &loop.StepRecord{Tool: "bash", Input: in}
		return decide(nil, st, env().ClassifyCommand(cmd))
	}
	for _, cmd := range []string{"git push", "git -C . push", "env git push", "sh -c 'git push'", "cd sub && git push", "xargs git push", "sudo git push origin main", "(git push)", "true; git push"} {
		if d := ask(cmd); d.Action != "deny" {
			t.Errorf("%q: want deny, got %+v", cmd, d)
		}
	}
	if d := ask("make test"); d.Action != "allow" {
		t.Errorf("whole-command allow: %+v", d)
	}
	if d := ask("env make test"); d.Action != "" {
		t.Errorf("an allow never widens to an inner form: %+v", d)
	}
	if d := ask("sh -c 'rm -rf build'"); d.Action != "ask" {
		t.Errorf("ask holds on an inner form: %+v", d)
	}
}
