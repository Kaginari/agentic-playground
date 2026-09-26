package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CLI runs `config show|explain|check|path|patch` and returns the exit code. Flags before the
// subcommand or after it: --dist, --root, --yaml (show), and the config flags ParseFlags reads.
// `check` answers on the wire; the others are human-facing.
func CLI(args []string, stdout, stderr io.Writer) int {
	return cli(args, stdout, stderr, Options{})
}

func cli(args []string, stdout, stderr io.Writer, base Options) int {
	o := base
	var sub string
	asYAML := false
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--dist" && i+1 < len(args):
			i++
			o.Dist = args[i]
		case strings.HasPrefix(a, "--dist="):
			o.Dist = a[7:]
		case a == "--root" && i+1 < len(args):
			i++
			o.Root = args[i]
		case strings.HasPrefix(a, "--root="):
			o.Root = a[7:]
		case a == "--yaml":
			asYAML = true
		case sub == "" && !strings.HasPrefix(a, "-"):
			sub = a
		default:
			rest = append(rest, a)
		}
	}
	if sub == "" {
		sub = "show"
	}
	flags, leftover, err := ParseFlags(rest)
	if err != nil {
		fmt.Fprintf(stderr, "@S FAIL\n@? %v\n", err)
		return 2
	}
	o.Flags = append(o.Flags, flags...)
	if sub != "patch" && len(leftover) > 0 {
		fmt.Fprintf(stderr, "@S FAIL\n@? unknown argument %q\n", leftover[0])
		return 2
	}
	c, err := LoadWith(o)
	switch sub {
	case "check":
		if err != nil {
			fmt.Fprintf(stdout, "@S FAIL\n@? %v\n@E 0\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "@S PASS %s · %s\n", c.Dist.Name, c.Root)
		for _, l := range c.Paths() {
			fmt.Fprintf(stdout, "@F layer %s\n", l)
		}
		for _, f := range c.Off() {
			fmt.Fprintln(stdout, f.String())
		}
		for _, l := range c.Loosenings() {
			fmt.Fprintf(stdout, "@F loosening %s\n", l)
		}
		for _, h := range c.Holes {
			fmt.Fprintf(stdout, "@? %s\n", h)
		}
		fmt.Fprintln(stdout, "@E 0")
		return 0
	case "path":
		if err != nil {
			fmt.Fprintf(stderr, "config: %v\n", err)
			return 1
		}
		for _, l := range c.Paths() {
			fmt.Fprintln(stdout, l)
		}
		return 0
	case "show":
		if err != nil {
			fmt.Fprintf(stderr, "config: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, c.Show(asYAML))
		return 0
	case "explain":
		if err != nil {
			fmt.Fprintf(stderr, "config: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, c.Explain())
		return 0
	case "patch":
		// config patch [--apply] [file] key=value | key= (delete) ...
		if err != nil {
			fmt.Fprintf(stderr, "config: %v\n", err)
			return 1
		}
		apply := false
		file := ""
		var ops []Op
		for _, a := range leftover {
			switch {
			case a == "--apply":
				apply = true
			case strings.Contains(a, "="):
				k, v, _ := strings.Cut(a, "=")
				ops = append(ops, Op{Path: k, Value: v, Delete: v == ""})
			default:
				file = a
			}
		}
		if file == "" {
			file = c.ProjectFile()
		}
		if apply {
			if err := c.ApplyPatch(file, ops); err != nil {
				fmt.Fprintf(stderr, "config: %v\n", err)
				return 1
			}
			fmt.Fprintf(stdout, "wrote %s\n", file)
			return 0
		}
		diff, err := c.Patch(file, ops)
		if err != nil {
			fmt.Fprintf(stderr, "config: %v\n", err)
			return 1
		}
		if diff == "" {
			fmt.Fprintln(stdout, "no change")
			return 0
		}
		fmt.Fprint(stdout, diff)
		return 0
	}
	fmt.Fprintf(stderr, "@S FAIL\n@? unknown subcommand %q — show | explain | check | path | patch\n", sub)
	return 2
}

// Selftest builds a workspace in a temp dir with a fake env and checks the loader end to end:
// layers and precedence, origins, refusals, rule resolution, permission decisions, the CLI.
// It returns the number of checks passed, or the first failure.
func Selftest() (int, error) {
	return SelftestIn(io.Discard)
}

// SelftestIn is Selftest with a writer for the check log.
func SelftestIn(out io.Writer) (n int, err error) {
	dir, err := os.MkdirTemp("", "agent-one-config-selftest-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "workspace")
	home := filepath.Join(dir, "home")
	must := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(s), 0o644)
	}
	must(filepath.Join(home, ".config", "agent-one", "config.yaml"), "policy:\n  wire: {cap: 1024}\nproviders:\n  ollama: {enabled: true}\n")
	must(filepath.Join(root, ".agent-one", "config.yaml"), strings.TrimSpace(`
model: anthropic/claude-opus-5
policy:
  wire: {cap: 4096}
permissions:
  rules:
    - {match: "bash:git push*", action: allow}
    - {match: "bash:git push --force*", action: deny}
    - {match: "edit:**/*.lock", action: deny}
rules:
  - {id: no-force, text: "Never force-push.", scope: all}
  - {file: rules/zone.md, scope: "rank:zone"}
  - {check: "true", scope: all}
tools:
  custom:
    ls:
      class: read
      override: true
      params: {path: {type: string, required: true}}
      run: [ls, -la, "{{path}}"]
`)+"\n")
	must(filepath.Join(root, ".agent-one", "rules", "zone.md"), "Zone workers write only in their zone.\n")
	must(filepath.Join(root, ".agent-one", "config.local.json"), `{"compaction": {"trigger": {"tokens": 150000}}}`)
	env := map[string]string{"HOME": home, "AGENT_ONE_LOG_LEVEL": "DEBUG"}
	getenv := func(k string) string { return env[k] }
	check := func(name string, ok bool) error {
		if !ok {
			return fmt.Errorf("selftest: %s", name)
		}
		n++
		fmt.Fprintf(out, "ok %d %s\n", n, name)
		return nil
	}
	c, err := LoadWith(Options{Dist: "agent-one", Root: root, Home: home, Env: getenv, Flags: []Override{{Path: "policy.budget.steps", Value: "7", Flag: "--max-steps"}}})
	if err != nil {
		return n, fmt.Errorf("selftest: load: %v", err)
	}
	checks := []struct {
		name string
		ok   bool
	}{
		{"project beats global for policy.wire.cap", c.Policy.Wire.Cap == 4096},
		{"origin is file:line", c.Where("policy.wire.cap") == filepath.Join(root, ".agent-one", "config.yaml")+":3"},
		{"global layer applied", c.Providers["ollama"].Enabled},
		{"local json layer applied", c.Compaction.Trigger.Tokens == 150000},
		{"env single key", c.LogLevel == "DEBUG" && c.Where("logLevel") == "env:AGENT_ONE_LOG_LEVEL"},
		{"flag layer", c.Policy.Budget.Steps == 7 && c.Where("policy.budget.steps") == "flag:--max-steps"},
		{"default origin", c.Where("policy.budget.minutes") == "default"},
		{"model alias to models.default", c.Models.Default.Model == "anthropic/claude-opus-5"},
		{"rule file resolved relative to its config", strings.HasPrefix(c.Rules[1].Text, "Zone workers write")},
		{"rule scope by rank", len(c.RulesFor("zone", "")) == 3 && len(c.RulesFor("domain", "")) == 2},
		{"prompt rules after principles", strings.Contains(c.PromptRules("domain", ""), "[no-force] Never force-push.")},
		{"policy facts", len(c.PolicyFacts("zone", "")) == 2 && c.PolicyFacts("zone", "")[0].Kind == "policy"},
		{"checks", len(c.Checks("coord", "")) == 1 && c.Checks("coord", "")[0].Command == "true"},
		{"custom tool argv injection stays one element", func() bool {
			argv, err := c.Tools.Custom["ls"].Argv(map[string]string{"path": "; rm -rf /"})
			return err == nil && len(argv) == 3 && argv[2] == "; rm -rf /"
		}()},
		{"no off-list by default", len(c.Off()) == 0},
		{"loosening listed", len(c.Loosenings()) == 1},
		{"ranks from the policy", len(c.Ranks()) == 7 && len(c.Deviations()) == 0},
	}
	for _, ch := range checks {
		if err := check(ch.name, ch.ok); err != nil {
			return n, err
		}
	}
	decisions := []struct {
		tool, input, class string
		want               Action
	}{
		{"bash", "git push origin main", "outward", Allow},
		{"bash", "git push --force origin main", "outward", Deny},
		{"bash", "git status", "read", Allow},
		{"bash", "curl https://x", "outward", Ask},
		{"edit", "go.lock", "write", Deny},
		{"edit", "vendor/a/b.lock", "write", Deny},
		{"edit", "main.go", "write", Allow},
		{"webfetch", "https://example.com", "outward", Ask},
	}
	for _, d := range decisions {
		got, _ := c.Decide(d.tool, d.input, d.class)
		if err := check(fmt.Sprintf("decide %s %q → %s", d.tool, d.input, d.want), got == d.want); err != nil {
			return n, err
		}
	}
	// Refusals.
	refused := func(name, content string) error {
		o := Options{Dist: "agent-one", Root: root, Home: home, Env: getenv, Overlay: map[string][]byte{filepath.Join(root, ".agent-one", "config.yaml"): []byte(content)}}
		_, err := LoadWith(o)
		return check("refused: "+name, err != nil)
	}
	for _, r := range []struct{ name, content string }{
		{"wildcard allow on bash", "permissions:\n  rules: [{match: \"bash:*\", action: allow}]\n"},
		{"literal key", "providers:\n  anthropic: {apiKey: sk-ant-123}\n"},
		{"missing rule file", "rules: [{file: nope.md}]\n"},
		{"drain that never fires", "policy: {budget: {contextTokens: 100000, stressTokens: 90000}}\n"},
		{"anchor in yaml", "a: &x 1\nb: *x\n"},
		{"wrong type", "policy: {wire: {cap: \"big\"}}\n"},
		{"custom tool without class", "tools:\n  custom:\n    x: {run: [ls]}\n"},
		{"placeholder not in params", "tools:\n  custom:\n    x: {class: read, run: [ls, \"{{p}}\"]}\n"},
		{"rank cycle", "ranks:\n  a: {reportsTo: b}\n  b: {reportsTo: a}\n"},
	} {
		if err := refused(r.name, r.content); err != nil {
			return n, err
		}
	}
	// Gate off only from files.
	envOff := map[string]string{"HOME": home, "AGENT_ONE_CONFIG_CONTENT": "policy: {humanGate: {enabled: false}}"}
	_, err = LoadWith(Options{Dist: "agent-one", Root: root, Home: home, Env: func(k string) string { return envOff[k] }})
	if err := check("gate off from env refused", err != nil); err != nil {
		return n, err
	}
	c2, err := LoadWith(Options{Dist: "agent-one", Root: root, Home: home, Env: getenv, Overlay: map[string][]byte{filepath.Join(root, ".agent-one", "config.local.json"): []byte(`{"policy": {"humanGate": {"enabled": false}}}`)}})
	if err := check("gate off from a file loads", err == nil); err != nil {
		return n, err
	}
	if err := check("gate off is in the off-list", len(c2.Off()) == 1 && c2.Off()[0].Key == "policy.humanGate.enabled"); err != nil {
		return n, err
	}
	// Patch and reload.
	diff, err := c.Patch(c.ProjectFile(), []Op{{Path: "tools.webfetch.enabled", Value: "false"}})
	if err := check("patch drafts a diff", err == nil && strings.Contains(diff, "+  webfetch:")); err != nil {
		return n, err
	}
	if err := check("patch does not write", !strings.Contains(readAll(c.ProjectFile()), "webfetch")); err != nil {
		return n, err
	}
	if err := c.ApplyPatch(c.ProjectFile(), []Op{{Path: "tools.webfetch.enabled", Value: "false"}}); err != nil {
		return n, fmt.Errorf("selftest: apply: %v", err)
	}
	c3, changes, err := c.Reload()
	if err := check("reload reports the change", err == nil && len(changes) == 1 && changes[0].Key == "tools.webfetch.enabled" && !c3.Tools.Webfetch.Enabled); err != nil {
		return n, err
	}
	// CLI.
	var sb strings.Builder
	code := cli([]string{"check", "--dist", "agent-one", "--root", root}, &sb, &sb, Options{Home: home, Env: getenv})
	if err := check("cli check answers on the wire", code == 0 && strings.HasPrefix(sb.String(), "@S PASS")); err != nil {
		return n, err
	}
	sb.Reset()
	code = cli([]string{"explain", "--dist", "agent-one", "--root", root}, &sb, &sb, Options{Home: home, Env: getenv})
	if err := check("cli explain carries origins", code == 0 && strings.Contains(sb.String(), "# default")); err != nil {
		return n, err
	}
	// agent-one distribution.
	must(filepath.Join(dir, "az", ".agent-one", "config.json"), `{"models": {"roles": {"analyst": "anthropic/claude-haiku-4-5"}}}`)
	az, err := LoadWith(Options{Dist: "agent-one", Root: filepath.Join(dir, "az"), Home: home, Env: func(k string) string {
		return map[string]string{"AGENT_ONE_MODEL": "openai/gpt-5"}[k]
	}})
	if err != nil {
		return n, fmt.Errorf("selftest: agent-one: %v", err)
	}
	if err := check("agent-one dirs and prefix", az.Dist.WorkspaceDir == ".agent-one" && az.Dist.EnvPrefix == "AGENT_ONE_" && az.Models.Default.Model == "openai/gpt-5"); err != nil {
		return n, err
	}
	if err := check("agent-one role alias folded", az.Models.Roles["analyst"].Model == "anthropic/claude-haiku-4-5"); err != nil {
		return n, err
	}
	return n, nil
}

func readAll(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}
