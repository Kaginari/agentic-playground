package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// workspace builds a temp workspace and home; files maps workspace-relative or "~/"-relative paths.
type workspace struct {
	t    *testing.T
	root string
	home string
	env  map[string]string
}

func newWorkspace(t *testing.T, files map[string]string) *workspace {
	t.Helper()
	dir := t.TempDir()
	w := &workspace{t: t, root: filepath.Join(dir, "workspace"), home: filepath.Join(dir, "home"), env: map[string]string{}}
	_ = os.MkdirAll(filepath.Join(w.root, ".agent-one"), 0o755)
	for p, s := range files {
		w.write(p, s)
	}
	return w
}

func (w *workspace) path(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(w.home, p[2:])
	}
	return filepath.Join(w.root, p)
}

func (w *workspace) write(p, s string) {
	full := w.path(p)
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *workspace) opts() Options {
	env := w.env
	return Options{Dist: "agent-one", Root: w.root, Home: w.home, Env: func(k string) string { return env[k] }}
}

func (w *workspace) load(flags ...Override) *Config {
	w.t.Helper()
	o := w.opts()
	o.Flags = flags
	c, err := LoadWith(o)
	if err != nil {
		w.t.Fatalf("load: %v", err)
	}
	return c
}

func (w *workspace) loadErr() error {
	_, err := LoadWith(w.opts())
	return err
}

func TestDefaultsAlone(t *testing.T) {
	w := newWorkspace(t, nil)
	c := w.load()
	if c.Models.Default.Model != "anthropic/claude-opus-5" || c.Policy.Wire.Cap != 2048 || !c.Policy.HumanGate.Enabled {
		t.Fatalf("defaults: %+v", c.Policy)
	}
	if c.Where("policy.wire.cap") != "default" {
		t.Errorf("origin: %s", c.Where("policy.wire.cap"))
	}
	if c.Policy.Log.Path != ".agent-one/log.md" || c.Memory.Shared.Machine.Path != filepath.Join(w.home, ".agent-one/shared/notes.jsonl") {
		t.Errorf("substitution: %s %s", c.Policy.Log.Path, c.Memory.Shared.Machine.Path)
	}
	if len(c.Off()) != 0 || len(c.Loosenings()) != 0 || len(c.Holes) != 0 {
		t.Errorf("clean defaults: off %v loose %v holes %v", c.Off(), c.Loosenings(), c.Holes)
	}
	if len(c.Ranks()) != 7 || c.RankSet != "extend" {
		t.Errorf("ranks: %d %s", len(c.Ranks()), c.RankSet)
	}
	if got := c.EnabledTools(); len(got) != len(Builtins) {
		t.Errorf("profile max enables every builtin: %v", got)
	}
}

func TestDefaultsOnly(t *testing.T) {
	c, err := Defaults("agent-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Layers) != 1 || c.Policy.Log.Path != ".agent-one/log.md" || c.Where("models.default") != "default" {
		t.Errorf("defaults: %+v %s", c.Layers, c.Policy.Log.Path)
	}
}

func TestPrecedence(t *testing.T) {
	w := newWorkspace(t, map[string]string{
		"~/.config/agent-one/config.yaml": "policy:\n  wire: {cap: 100, raw: true}\nlogLevel: INFO\nmode: plan\n",
		".agent-one/config.yaml":          "policy:\n  wire: {cap: 200}\nlogLevel: ERROR\n",
		".agent-one/config.local.yml":     "policy:\n  wire: {cap: 300}\n",
		".agent-one/../env-file.json":     `{"policy": {"wire": {"cap": 400}}, "sessions": {"keepDays": 3}}`,
	})
	w.env["AGENT_ONE_CONFIG"] = w.path("env-file.json")
	w.env["AGENT_ONE_CONFIG_CONTENT"] = "sessions: {keepDays: 4}"
	w.env["AGENT_ONE_LOG_LEVEL"] = "DEBUG"
	c := w.load(Override{Path: "mode", Value: "build", Flag: "--mode"})
	cases := map[string]struct{ val, origin string }{
		"policy.wire.cap":   {"400", w.path("env-file.json") + ":1"},
		"policy.wire.raw":   {"true", w.path("~/.config/agent-one/config.yaml") + ":2"},
		"sessions.keepDays": {"4", "env:AGENT_ONE_CONFIG_CONTENT:1"},
		"logLevel":          {"DEBUG", "env:AGENT_ONE_LOG_LEVEL"},
		"mode":              {"build", "flag:--mode"},
	}
	for k, want := range cases {
		if got := c.Where(k); got != want.origin {
			t.Errorf("%s origin: %s, want %s", k, got, want.origin)
		}
	}
	if c.Policy.Wire.Cap != 400 || !c.Policy.Wire.Raw || c.Sessions.KeepDays != 4 || c.LogLevel != "DEBUG" || c.Mode != "build" {
		t.Errorf("values: %+v %+v %s %s", c.Policy.Wire, c.Sessions, c.LogLevel, c.Mode)
	}
	for i, want := range []string{"default", "global", "project", "local", "env-file", "env", "flag"} {
		if c.Layers[i].Name != want || !c.Layers[i].Present {
			t.Errorf("layer %d: %+v", i, c.Layers[i])
		}
	}
	// DISABLE_PROJECT_CONFIG skips layers 3 and 4.
	w.env["AGENT_ONE_DISABLE_PROJECT_CONFIG"] = "1"
	delete(w.env, "AGENT_ONE_CONFIG")
	delete(w.env, "AGENT_ONE_CONFIG_CONTENT")
	c = w.load()
	if c.Policy.Wire.Cap != 100 || c.Layers[2].Present || c.Layers[3].Present {
		t.Errorf("disable project: cap %d layers %+v", c.Policy.Wire.Cap, c.Layers[2:4])
	}
	if len(c.Holes) == 0 || !strings.Contains(c.Holes[0], "DISABLE_PROJECT_CONFIG") {
		t.Errorf("skip is a hole: %v", c.Holes)
	}
}

func TestListsConcatenateAndDedupe(t *testing.T) {
	w := newWorkspace(t, map[string]string{
		"~/.config/agent-one/config.yaml": "discovery:\n  instructions: {files: [AGENTS.md, NOTES.md]}\npermissions:\n  rules: [{match: \"edit:**/*.lock\", action: deny}]\n",
		".agent-one/config.yaml":          "discovery:\n  instructions: {files: [NOTES.md, TEAM.md]}\npermissions:\n  rules: [{match: \"read:**/.env*\", action: ask}]\n",
	})
	c := w.load()
	if got := strings.Join(c.Discovery.Instructions.Files, ","); got != "AGENTS.md,CLAUDE.md,NOTES.md,TEAM.md" {
		t.Errorf("files: %s", got)
	}
	if len(c.Permissions.Rules) != 2 || c.Permissions.Rules[0].Tool != "edit" || c.Permissions.Rules[1].Tool != "read" {
		t.Errorf("rules: %+v", c.Permissions.Rules)
	}
	if c.Permissions.Rules[1].Origin.Layer != "project" {
		t.Errorf("rule origin: %+v", c.Permissions.Rules[1].Origin)
	}
}

func TestTwoFormatsOneLayer(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "logLevel: INFO\n", ".agent-one/config.json": `{"logLevel": "WARN"}`})
	err := w.loadErr()
	if err == nil || !strings.Contains(err.Error(), "one config file per layer") {
		t.Fatalf("want a two-formats error, got %v", err)
	}
}

func TestExampleJSONYAMLEquivalent(t *testing.T) {
	yamlSrc, err := os.ReadFile("testdata/example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	jsonSrc, err := os.ReadFile("testdata/example.json")
	if err != nil {
		t.Fatal(err)
	}
	load := func(name, src string) *Config {
		w := newWorkspace(t, map[string]string{".agent-one/" + name: src, ".agent-one/rules/zone-zone.md": "Zone workers write only inside their zone.\n"})
		w.env["VLLM_API_KEY"] = "set"
		return w.load()
	}
	cy := load("config.yaml", string(yamlSrc))
	cj := load("config.json", string(jsonSrc))
	if !Equal(cy, cj) {
		var diff []string
		for _, ch := range Delta(cy, cj) {
			diff = append(diff, ch.String())
		}
		t.Fatalf("yaml and json examples differ:\n%s", strings.Join(diff, "\n"))
	}
	for _, c := range []*Config{cy, cj} {
		if len(c.Holes) != 0 {
			t.Errorf("example has holes: %v", c.Holes)
		}
		if c.Providers["vllm"].Type != "openai" || c.Providers["vllm"].Timeout.D().Minutes() != 5 || !c.Providers["vllm"].ContextWindow.Auto {
			t.Errorf("vllm: %+v", c.Providers["vllm"])
		}
		m, _ := c.ResolveModel("", "", "", "bench")
		if m.Provider != "vllm" || m.ID != "meta-llama/Llama-3.1-70B-Instruct" || m.Entry == nil {
			t.Errorf("bench model: %+v", m)
		}
		if c.Tools.Webfetch.Enabled || c.Tools.Bash.Timeout.D() != 3*time.Minute || len(c.Tools.Custom) != 2 {
			t.Errorf("tools: %+v", c.Tools.Bash)
		}
		if c.MCP.Servers["files"].Type != "stdio" || !c.MCP.Servers["files"].Inward || c.MCP.Servers["docs"].HeadersEnv["Authorization"] != "DOCS_MCP_TOKEN" {
			t.Errorf("mcp: %+v", c.MCP.Servers)
		}
		if c.MCPClass("files", "read_file") != "read" || c.MCPClass("files", "other") != "write" || c.MCPClass("docs", "search") != "outward" {
			t.Errorf("mcp classes: %s %s %s", c.MCPClass("files", "read_file"), c.MCPClass("files", "other"), c.MCPClass("docs", "search"))
		}
		if r, ok := c.Rank("scribe"); !ok || r.ReportsTo != "domain" || !r.Authors || r.Role != "drafter" {
			t.Errorf("scribe: %+v", r)
		}
		if r, _ := c.Rank("domain"); r.Model.Model != "anthropic/claude-opus-5" {
			t.Errorf("domain model slot: %+v", r.Model)
		}
		if dev := c.Deviations(); len(dev) != 2 {
			t.Errorf("deviations: %v", dev)
		}
		if got := c.Rules[1].Text; !strings.HasPrefix(got, "Zone workers write") {
			t.Errorf("rule file: %q", got)
		}
		if off := c.OffLines(); len(off) != 0 {
			t.Errorf("off-list: %v", off)
		}
		if lo := c.Loosenings(); len(lo) != 1 || !strings.Contains(lo[0], "allow bash:git push*") {
			t.Errorf("loosenings: %v", lo)
		}
		if p, ok := c.PriceFor("anthropic/claude-opus-5"); !ok || p.Cost(1000000, 100000, 0, 0) != 7.5 {
			t.Errorf("price: %v %v", p, ok)
		}
		if _, ok := c.PriceFor("vllm/meta-llama/Llama-3.1-70B-Instruct"); ok {
			t.Error("unpriced model priced")
		}
		if c.Budgets.Session.USD != 20 || c.Budgets.Subagent.Tokens != 400000 || !c.UI.StatusLine {
			t.Errorf("budgets/ui: %+v %+v", c.Budgets, c.UI)
		}
	}
}

func TestSyntaxErrorsCarryFileLine(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy:\n  wire: {cap: 1\n"})
	err := w.loadErr()
	if err == nil || !strings.Contains(err.Error(), filepath.Join(w.root, ".agent-one/config.yaml")+":2") {
		t.Fatalf("want file:line, got %v", err)
	}
	w = newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy:\n  wire:\n    cap: many\n"})
	err = w.loadErr()
	if err == nil || !strings.Contains(err.Error(), "config.yaml:3: policy.wire.cap: expected an integer") {
		t.Fatalf("want a typed error, got %v", err)
	}
	w = newWorkspace(t, map[string]string{".agent-one/config.yaml": "- a\n"})
	if err = w.loadErr(); err == nil || !strings.Contains(err.Error(), "must be a map") {
		t.Fatalf("want a map error, got %v", err)
	}
}

func TestUnknownKeyIsAHole(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy:\n  wire: {cap: 1, future: 2}\nnewSection: {x: 1}\n"})
	c := w.load()
	if len(c.Holes) != 2 || !strings.Contains(c.Holes[1], "unknown key policy.wire.future — "+w.path(".agent-one/config.yaml")+":2") {
		t.Fatalf("holes: %v", c.Holes)
	}
}

func TestEnvProviderKeyLocation(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "providers:\n  vllm: {type: openai, baseURL: http://gpu:8000/v1}\n"})
	w.env["AGENT_ONE_PROVIDER_VLLM_API_KEY"] = "sk-secret"
	w.env["AGENT_ONE_PROVIDER_GROQ_API_KEY"] = "sk-other"
	w.env["AGENT_ONE_MODEL"] = "anthropic/claude-opus-4-5"
	c := w.load()
	if len(c.Holes) != 1 || !strings.Contains(c.Holes[0], "env:AGENT_ONE_PROVIDER_GROQ_API_KEY names no provider") {
		t.Errorf("unknown provider from env is a hole: %v", c.Holes)
	}
	if c.Models.Default.Model != "anthropic/claude-opus-4-5" || c.Where("models.default") != "env:AGENT_ONE_MODEL" {
		t.Errorf("model from env: %s %s", c.Models.Default.Model, c.Where("models.default"))
	}
	// only the location lands; the value never enters the config
	if c.Providers["vllm"] == nil || c.Providers["vllm"].APIKeyEnv != "AGENT_ONE_PROVIDER_VLLM_API_KEY" {
		t.Fatalf("apiKeyEnv from env: %+v", c.Providers["vllm"])
	}
	if strings.Contains(c.Explain(), "sk-secret") || strings.Contains(c.Show(false), "sk-secret") {
		t.Error("the key crossed into the config")
	}
	if _, err := LoadWith(w.opts()); err != nil {
		t.Fatal(err)
	}
}

func TestParseFlags(t *testing.T) {
	ov, rest, err := ParseFlags([]string{"--model", "openai/gpt-5", "--no-mcp", "--no-policy.principles", "--approve", "outward", "--dry-run", "--max-steps=9", "--set", "policy.wire.cap=1", "other", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, o := range ov {
		got[o.Path] = o.Value
	}
	want := map[string]string{"models.default": "openai/gpt-5", "mcp.enabled": "false", "policy.principles.enabled": "false", "policy.humanGate.approve": "[outward]", "policy.humanGate.dryRun": "true", "policy.budget.steps": "9", "policy.wire.cap": "1"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q want %q", k, got[k], v)
		}
	}
	if strings.Join(rest, " ") != "other --json" {
		t.Errorf("rest: %v", rest)
	}
	for _, bad := range [][]string{{"--no-human-gate"}, {"--no-policy.human-gate"}, {"--set", "policy.humanGate.enabled=false"}} {
		if _, _, err := ParseFlags(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	w := newWorkspace(t, nil)
	c := w.load(ov...)
	if c.Mode != "build" || !c.Policy.HumanGate.DryRun || c.Policy.HumanGate.Approve[0] != "outward" || c.MCP.Enabled || c.Policy.Principles.Enabled {
		t.Errorf("flags applied: %+v", c.Policy.HumanGate)
	}
	if off := c.OffLines(); len(off) != 2 || !strings.Contains(off[1], "policy.principles.enabled — flag:--no-policy.principles") {
		t.Errorf("off: %v", off)
	}
}

func TestAgentOne(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "ws")
	_ = os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".agent-one", "config.yaml"), []byte("models:\n  roles: {analyst: anthropic/claude-haiku-4-5, judge: anthropic/claude-opus-5, drafter: anthropic/claude-opus-4-5}\n  ranks: {zone: anthropic/claude-haiku-4-5}\nrules: [{text: x, scope: rank:zone}]\n"), 0o644)
	env := map[string]string{"AGENT_ONE_LOG_LEVEL": "INFO", "XDG_CONFIG_HOME": filepath.Join(dir, "xdg")}
	c, err := LoadWith(Options{Root: root, Home: filepath.Join(dir, "home"), Env: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatal(err)
	}
	if c.Dist.Name != "agent-one" || c.Dist.WorkspaceDir != ".agent-one" || c.Dist.EnvPrefix != "AGENT_ONE_" || c.Dist.PolicyFile != "AGENT-ONE.md" {
		t.Errorf("dist: %+v", c.Dist)
	}
	if c.Dist.GlobalDir != filepath.Join(dir, "xdg", "agent-one") || c.Layers[1].Path != filepath.Join(dir, "xdg", "agent-one", "config.yaml") {
		t.Errorf("xdg: %s %s", c.Dist.GlobalDir, c.Layers[1].Path)
	}
	if c.LogLevel != "INFO" || c.Policy.Log.Path != ".agent-one/log.md" {
		t.Errorf("lexicon: %s %s", c.LogLevel, c.Policy.Log.Path)
	}
	if c.Models.Roles["analyst"].Model != "anthropic/claude-haiku-4-5" || c.Models.Roles["drafter"].Model != "anthropic/claude-opus-4-5" {
		t.Errorf("role aliases: %+v", c.Models.Roles)
	}
	if m, _ := c.ResolveModel("", "zone", "", ""); m.Slot != "models.ranks.zone" {
		t.Errorf("rank alias on resolve: %+v", m)
	}
	if len(c.RulesFor("zone", "")) != 1 || len(c.RulesFor("zone", "")) != 1 || len(c.RulesFor("domain", "")) != 0 {
		t.Errorf("scope aliases")
	}
	if _, err := ParseDist("nope", "", nil); err == nil {
		t.Error("unknown dist accepted")
	}
}

func TestCLI(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "ontology: {enabled: false}\n"})
	run := func(args ...string) (int, string) {
		var out strings.Builder
		code := cli(args, &out, &out, Options{Home: w.home, Env: func(k string) string { return w.env[k] }})
		return code, out.String()
	}
	code, out := run("check", "--dist", "agent-one", "--root", w.root)
	if code != 0 || !strings.HasPrefix(out, "@S PASS agent-one") || !strings.Contains(out, "@? off ontology.enabled — "+w.path(".agent-one/config.yaml")+":1") || !strings.HasSuffix(out, "@E 0\n") {
		t.Errorf("check:\n%s", out)
	}
	code, out = run("path", "--root", w.root)
	if code != 0 || !strings.Contains(out, "project  present") || !strings.Contains(out, "global   absent") {
		t.Errorf("path:\n%s", out)
	}
	code, out = run("show", "--root", w.root, "--yaml")
	if code != 0 || !strings.Contains(out, "ontology:\n  enabled: false") {
		t.Errorf("show yaml:\n%s", out)
	}
	code, out = run("explain", "--root", w.root)
	if code != 0 || !strings.Contains(out, "ontology.enabled: false") || !strings.Contains(out, "# orchestrator [rankSet: extend]") {
		t.Errorf("explain:\n%s", out)
	}
	code, out = run("check", "--root", w.root, "--no-human-gate")
	if code != 2 || !strings.Contains(out, "@? --no-human-gate does not exist") {
		t.Errorf("no-human-gate: %d %s", code, out)
	}
	w.write(".agent-one/config.yaml", "policy: {wire: {cap: x}}\n")
	code, out = run("check", "--root", w.root)
	if code != 1 || !strings.HasPrefix(out, "@S FAIL") {
		t.Errorf("check fail: %d %s", code, out)
	}
	code, out = run("bogus", "--root", w.root)
	if code != 2 {
		t.Errorf("bogus: %d %s", code, out)
	}
}

func TestSelftest(t *testing.T) {
	n, err := Selftest()
	if err != nil {
		t.Fatalf("after %d checks: %v", n, err)
	}
	if n < 40 {
		t.Errorf("only %d checks", n)
	}
}

func TestTimeoutSpellings(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "tools:\n  git: {timeoutMs: 1500}\n  bash: {timeout: 45s}\nhooks: {timeoutMs: 2000}\n"})
	c, err := LoadWith(w.opts())
	if err != nil {
		t.Fatal(err)
	}
	if c.Tools.Git.Timeout.D() != 1500*time.Millisecond || c.Tools.Bash.Timeout.D() != 45*time.Second || c.Hooks.Timeout.D() != 2*time.Second {
		t.Fatalf("got git %v bash %v hooks %v", c.Tools.Git.Timeout, c.Tools.Bash.Timeout, c.Hooks.Timeout)
	}
	w2 := newWorkspace(t, map[string]string{".agent-one/config.yaml": "tools:\n  bash: {timeoutMs: 1000, timeout: 2s}\n"})
	if _, err := LoadWith(w2.opts()); err == nil || !strings.Contains(err.Error(), "both timeoutMs and timeout") {
		t.Fatalf("want both-spellings error, got %v", err)
	}
}

// A section may live in its own file beside config.yaml: guards.yaml (a map, or just the list of
// patterns), models.yaml, rules.yaml… It wins within its layer and keeps its file for explain.
func TestSectionFiles(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	workspace := filepath.Join(root, ".agent-one")
	os.MkdirAll(workspace, 0o755)
	os.WriteFile(filepath.Join(workspace, "config.yaml"), []byte("models: {default: anthropic/a}\nguard: {files: [x.txt]}\n"), 0o644)
	os.WriteFile(filepath.Join(workspace, "models.yaml"), []byte("default: openai/b\n"), 0o644)
	os.WriteFile(filepath.Join(workspace, "guards.yaml"), []byte("- '(^|[[:space:]])terraform[[:space:]]+destroy'\n- 'kubectl[[:space:]]+delete[[:space:]]+ns'\n"), 0o644)
	os.WriteFile(filepath.Join(workspace, "notes.yaml"), []byte("anything: at all\n"), 0o644)
	gdir := filepath.Join(home, ".config", "agent-one")
	os.MkdirAll(gdir, 0o755)
	os.WriteFile(filepath.Join(gdir, "providers.yaml"), []byte("openrouter: {enabled: true}\n"), 0o644)
	c, err := LoadWith(Options{Dist: "agent-one", Root: root, Home: home, Env: func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Models.Default.Model != "openai/b" || !strings.Contains(c.Where("models.default"), "models.yaml") {
		t.Fatalf("models.yaml wins in its layer: %q from %s", c.Models.Default.Model, c.Where("models.default"))
	}
	if len(c.Guard.Patterns) != 2 || len(c.Guard.Files) != 1 || !c.Guard.Enabled {
		t.Fatalf("guards.yaml (a list) adds patterns, config.yaml's guard stays: %+v", c.Guard)
	}
	if p := c.Providers["openrouter"]; p == nil || !p.Enabled {
		t.Fatal("a global section file (providers.yaml) loads too")
	}
	os.WriteFile(filepath.Join(workspace, "guard.yaml"), []byte("enabled: false\n"), 0o644)
	if _, err := LoadWith(Options{Dist: "agent-one", Root: root, Home: home, Env: func(string) string { return "" }}); err == nil || !strings.Contains(err.Error(), "one file per section") {
		t.Fatalf("guard.yaml beside guards.yaml is ambiguous: %v", err)
	}
}
