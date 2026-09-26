package config

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCustomToolArgvInjection(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
tools:
  custom:
    lsl:
      class: read
      params: {path: {type: string, required: true}}
      run: [ls, -la, "{{path}}"]
    kget:
      class: read
      params:
        kind: {type: string, required: true, enum: [pods, svc]}
        ns: {type: string, default: default}
      run: [kubectl, get, "{{kind}}", "-n={{ns}}", -o, wide]
    sh:
      class: write
      shell: "echo \"$P_msg\" >> notes.txt"
      params: {msg: {type: string}}
      profiles: [minimal]
`})
	c := w.load()
	argv, err := c.Tools.Custom["lsl"].Argv(map[string]string{"path": "; rm -rf / && echo $(whoami)"})
	if err != nil || !reflect.DeepEqual(argv, []string{"ls", "-la", "; rm -rf / && echo $(whoami)"}) {
		t.Fatalf("argv: %q %v", argv, err)
	}
	argv, err = c.Tools.Custom["kget"].Argv(map[string]string{"kind": "pods"})
	if err != nil || !reflect.DeepEqual(argv, []string{"kubectl", "get", "pods", "-n=default", "-o", "wide"}) {
		t.Fatalf("kget: %q %v", argv, err)
	}
	if _, err := c.Tools.Custom["kget"].Argv(map[string]string{"kind": "secrets"}); err == nil || !strings.Contains(err.Error(), "not one of") {
		t.Errorf("enum: %v", err)
	}
	if _, err := c.Tools.Custom["lsl"].Argv(nil); err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("required: %v", err)
	}
	if _, err := c.Tools.Custom["lsl"].Argv(map[string]string{"path": ".", "x": "1"}); err == nil {
		t.Error("unknown param accepted")
	}
	env, err := c.Tools.Custom["sh"].ShellEnv(map[string]string{"msg": "a; rm -rf /"})
	if err != nil || !reflect.DeepEqual(env, []string{"P_msg=a; rm -rf /"}) {
		t.Errorf("shell env: %q %v", env, err)
	}
	if s := c.Tools.Custom["kget"].Schema(); !strings.Contains(s, `"kind":{"type":"string","enum":["pods","svc"]}`) || !strings.Contains(s, `"required":["kind"]`) {
		t.Errorf("schema: %s", s)
	}
	if ok, why := c.ToolEnabled("sh"); ok || !strings.Contains(why, "not offered") {
		// profile is max: custom profiles restrict only under a named profile
		t.Logf("sh under max: %v %s", ok, why)
	}
	if ok, _ := c.ToolEnabled("sh"); !ok {
		t.Error("max profile offers every custom tool")
	}
	w.write(".agent-one/config.local.yaml", "tools: {profile: openai}\n")
	c = w.load()
	if ok, why := c.ToolEnabled("sh"); ok || !strings.Contains(why, "tools.profile openai") {
		t.Errorf("profile filter: %v %s", ok, why)
	}
	w.write(".agent-one/config.local.yaml", "tools: {profile: minimal, webfetch: {enabled: false}}\n")
	c = w.load()
	if ok, why := c.ToolEnabled("glob"); ok || !strings.Contains(why, "minimal") {
		t.Errorf("minimal drops glob: %v %s", ok, why)
	}
	if ok, why := c.ToolEnabled("webfetch"); ok || !strings.Contains(why, "disabled by tools.webfetch.enabled in "+w.path(".agent-one/config.local.yaml")+":1") {
		t.Errorf("disabled reason: %v %s", ok, why)
	}
	if off := c.ToolsOff(); len(off) == 0 || len(c.Off()) != 0 {
		t.Errorf("tools off is not a policy finding: %v %v", off, c.Off())
	}
	if ok, why := c.ToolEnabled("nope"); ok || why != "no such tool" {
		t.Error("unknown tool")
	}
}

func TestToolRefusals(t *testing.T) {
	cases := []struct{ name, yaml, want string }{
		{"collision", "tools:\n  custom:\n    grep: {class: read, run: [rg]}\n", "collides with the builtin"},
		{"placeholder", "tools:\n  custom:\n    x: {class: read, run: [ls, \"{{p}}\"]}\n", "not in params"},
		{"no class", "tools:\n  custom:\n    x: {run: [ls]}\n", "class is required"},
		{"bad class", "tools:\n  custom:\n    x: {class: huge, run: [ls]}\n", "not read, write"},
		{"run and shell", "tools:\n  custom:\n    x: {class: read, run: [ls], shell: ls}\n", "one or the other"},
		{"neither", "tools:\n  custom:\n    x: {class: read}\n", "needs run"},
		{"shell placeholder", "tools:\n  custom:\n    x: {class: read, shell: \"ls {{p}}\", params: {p: {}}}\n", "$P_<name>"},
		{"loosen builtin", "tools: {webfetch: {class: read}}\n", "only tightens"},
		{"bad profile", "tools: {profile: huge}\n", "not max"},
		{"bad sandbox", "tools: {bash: {sandbox: docker}}\n", "not bwrap or none"},
		{"custom profile max", "tools:\n  custom:\n    x: {class: read, run: [ls], profiles: [max]}\n", "not anthropic, openai or minimal"},
	}
	for _, cs := range cases {
		w := newWorkspace(t, map[string]string{".agent-one/config.yaml": cs.yaml})
		err := w.loadErr()
		if err == nil || !strings.Contains(err.Error(), cs.want) {
			t.Errorf("%s: got %v, want %q", cs.name, err, cs.want)
		}
	}
	// override replaces a builtin; class may tighten
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "tools:\n  custom:\n    grep: {class: read, run: [rg, \"{{q}}\"], params: {q: {}}, override: true}\n  git: {class: write}\n"})
	c := w.load()
	if c.ToolClass("git") != "write" || c.ToolClass("grep") != "read" || c.ToolClass("bash") != "" {
		t.Errorf("classes: %s %s %q", c.ToolClass("git"), c.ToolClass("grep"), c.ToolClass("bash"))
	}
}

func TestProviders(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
models: {default: vllm/meta-llama/Llama-3.1-70B-Instruct}
providers:
  vllm:
    type: openai
    baseURL: http://gpu:8000/v1
    apiKeyEnv: VLLM_API_KEY
    headers: {X-Team: platform}
    timeout: 90
    tls: {insecureSkipVerify: true}
    toolCalls: text
    contextWindow: 32768
  local: {baseUrl: http://127.0.0.1:1234/v1, kind: openai}
  mock: {enabled: true}
`})
	w.env["VLLM_API_KEY"] = "k"
	c := w.load()
	v := c.Providers["vllm"]
	if v.Timeout.D().Seconds() != 90 || !v.TLS.InsecureSkipVerify || v.ToolCalls != "text" || v.ContextWindow.N != 32768 || v.ContextWindow.Auto {
		t.Errorf("vllm: %+v", v)
	}
	if !v.Enabled || c.Providers["local"].Type != "openai" || c.Providers["local"].BaseURL != "http://127.0.0.1:1234/v1" {
		t.Errorf("aliases/implicit enabled: %+v", c.Providers["local"])
	}
	key, err := v.ProviderKey(func(k string) string { return w.env[k] })
	if err != nil || key != "k" {
		t.Errorf("key: %q %v", key, err)
	}
	if _, err := v.ProviderKey(func(string) string { return "" }); err == nil || strings.Contains(err.Error(), "k") == false {
		t.Errorf("missing key names the location only: %v", err)
	}
	m, o := c.Mount()
	if m.Provider != "vllm" || m.ID != "meta-llama/Llama-3.1-70B-Instruct" || o.Layer != "project" {
		t.Errorf("mount: %+v %+v", m, o)
	}
	if len(c.Holes) != 0 { // one model for every role: the lineage holds without tiers
		t.Errorf("holes: %v", c.Holes)
	}
	refusals := []struct{ name, yaml, want string }{
		{"literal key", "providers:\n  anthropic: {apiKey: sk-ant-1}\n", "credential never lives in a config file"},
		{"literal token", "providers:\n  x: {type: openai, baseURL: u, token: abc}\n", "credential"},
		{"auth header", "providers:\n  x: {type: openai, baseURL: u, headers: {Authorization: \"Bearer x\"}}\n", "carries a credential"},
		{"secret in apiKeyEnv", "providers:\n  x: {type: openai, baseURL: u, apiKeyEnv: sk-abc-123}\n", "must name an environment variable"},
		{"no type", "providers:\n  weird: {baseURL: u}\n", "type is required"},
		{"bad type", "providers:\n  weird: {type: grpc, baseURL: u}\n", "not anthropic, openai or mock"},
		{"no baseURL", "providers:\n  weird: {type: openai}\n", "baseURL is required"},
		{"bad toolCalls", "providers:\n  anthropic: {toolCalls: magic}\n", "not native or text"},
		{"bad model ref", "models: {default: claude}\n", "not <provider>/<model-id>"},
		{"unknown provider", "models: {default: nope/x}\n", "no provider"},
		{"disabled provider", "models: {default: ollama/x}\n", "is disabled"},
		{"bad role", "models: {roles: {wizard: anthropic/x}}\n", "roles are"},
		{"bad task", "models: {tasks: {sing: anthropic/x}}\n", "tasks are"},
		{"bad contextWindow", "providers:\n  anthropic: {contextWindow: big}\n", "\"auto\" or a number"},
		{"bad duration", "providers:\n  anthropic: {timeout: soon}\n", "not a duration"},
	}
	for _, cs := range refusals {
		w := newWorkspace(t, map[string]string{".agent-one/config.yaml": cs.yaml})
		err := w.loadErr()
		if err == nil || !strings.Contains(err.Error(), cs.want) {
			t.Errorf("%s: got %v, want %q", cs.name, err, cs.want)
		}
	}
}

func TestModelsResolution(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
models:
  default: anthropic/claude-sonnet-5
  roles: {analyst: anthropic/claude-haiku-4-5, judge: anthropic/claude-opus-5, drafter: {model: anthropic/claude-fable-5-1, effort: high}}
  ranks: {domain: anthropic/claude-opus-5}
  members: {zone-changelog: anthropic/claude-haiku-4-5}
  tasks: {log: anthropic/claude-haiku-4-5}
providers:
  anthropic:
    models:
      claude-haiku-4-5: {tier: 1}
      claude-sonnet-5: {tier: 2}
      claude-opus-5: {tier: 3}
      claude-fable-5-1: {tier: 3}
`})
	c := w.load()
	if len(c.Holes) != 0 {
		t.Fatalf("holes: %v", c.Holes)
	}
	cases := []struct {
		member, rank, role, task string
		want, slot               string
	}{
		{"zone-changelog", "zone", "drafter", "log", "anthropic/claude-haiku-4-5", "models.members.zone-changelog"},
		{"zone-auth", "zone", "drafter", "log", "anthropic/claude-fable-5-1", "models.roles.drafter"},
		{"", "domain", "", "log", "anthropic/claude-opus-5", "models.ranks.domain"},
		{"", "zone", "", "log", "anthropic/claude-haiku-4-5", "models.tasks.log"},
		{"", "zone", "", "", "anthropic/claude-sonnet-5", "models.default"},
		{"", "", "judge", "", "anthropic/claude-opus-5", "models.roles.judge"},
		{"", "domain", "", "", "anthropic/claude-opus-5", "models.ranks.domain"},
	}
	for _, cs := range cases {
		m, o := c.ResolveModel(cs.member, cs.rank, cs.role, cs.task)
		if m.Ref.Model != cs.want || m.Slot != cs.slot || o.Layer != "project" && cs.slot != "models.default" {
			t.Errorf("Resolve(%q,%q,%q,%q) = %s from %s (%s), want %s from %s", cs.member, cs.rank, cs.role, cs.task, m.Ref.Model, m.Slot, o, cs.want, cs.slot)
		}
	}
	if m, _ := c.ResolveModel("", "", "drafter", ""); m.Ref.Effort != "high" || m.Entry == nil {
		t.Errorf("drafter ref: %+v", m)
	}
	if tbl := c.ModelTable(); len(tbl) < 1+3+7+4+1 {
		t.Errorf("table: %d lines", len(tbl))
	}
	// lineage broken: judge below analyst
	w.write(".agent-one/config.local.yaml", "models: {roles: {judge: anthropic/claude-haiku-4-5, analyst: anthropic/claude-opus-5}}\n")
	if err := w.loadErr(); err == nil || !strings.Contains(err.Error(), "lineage") {
		t.Errorf("lineage: %v", err)
	}
	// missing tier is a hole, not an error
	w.write(".agent-one/config.local.yaml", "models: {roles: {drafter: anthropic/claude-new}}\n")
	c = w.load()
	if len(c.Holes) != 1 || !strings.Contains(c.Holes[0], "declares no tier") {
		t.Errorf("tier hole: %v", c.Holes)
	}
	// ranks.<r>.model is the same slot as models.ranks.<r>
	w.write(".agent-one/config.local.yaml", "ranks: {domain: {model: anthropic/claude-sonnet-5}}\n")
	if err := w.loadErr(); err == nil || !strings.Contains(err.Error(), "one slot") {
		t.Errorf("slot conflict: %v", err)
	}
	w.write(".agent-one/config.local.yaml", "ranks: {zone: {model: anthropic/claude-haiku-4-5}}\n")
	c = w.load()
	if m, _ := c.ResolveModel("", "zone", "", ""); m.Slot != "ranks.zone.model" || m.Ref.Model != "anthropic/claude-haiku-4-5" {
		t.Errorf("rank model slot: %+v", m)
	}
}

func TestRanks(t *testing.T) {
	// replace mode: exactly the listed ranks (+ orchestrator root)
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
rankSet: replace
ranks:
  lead: {reportsTo: orchestrator, holdsGate: true, agent: persistent}
  reviewer: {reportsTo: lead, holdsGate: true}
  builder: {reportsTo: reviewer, authors: true, role: analyst, tools: [read, write, edit, bash]}
  scribe: {reportsTo: lead, authors: true}
  senior-builder: {promotesFrom: builder, job: an promoted builder}
`})
	c := w.load()
	rs := c.Ranks()
	if len(rs) != 5 {
		t.Fatalf("replace: %d ranks", len(rs))
	}
	names := map[string]Rank{}
	for _, r := range rs {
		names[r.Name] = r
	}
	for _, n := range []string{"lead", "reviewer", "builder", "scribe", "senior-builder"} {
		if _, ok := names[n]; !ok {
			t.Errorf("missing %s", n)
		}
	}
	if _, ok := names["coord"]; ok {
		t.Error("policy table loaded under replace")
	}
	sb := names["senior-builder"]
	if sb.ReportsTo != "reviewer" || !sb.Authors || sb.Role != "analyst" || len(sb.Tools) != 4 || sb.Origins["tools"] != "promotes:builder" {
		t.Errorf("promoted inherits: %+v", sb)
	}
	if names["lead"].Agent != "persistent" || names["builder"].Agent != "ephemeral" || names["builder"].Prefix != "builder-" {
		t.Errorf("defaults: %+v", names["builder"])
	}
	tree := c.RankTree()
	if !strings.Contains(tree, "orchestrator [rankSet: replace]") || !strings.Contains(tree, "    reviewer (gate, ephemeral)") || !strings.Contains(tree, "      builder (authors, ephemeral, role analyst)") {
		t.Errorf("tree:\n%s", tree)
	}
	if dev := c.Deviations(); len(dev) != 6 {
		t.Errorf("deviations: %v", dev)
	}
	// extend: override a field, keep the rest, origins per field
	w = newWorkspace(t, map[string]string{".agent-one/config.yaml": "ranks:\n  zone: {role: drafter}\n  principal-domain-owner: {job: x}\n"})
	c = w.load()
	if r, _ := c.Rank("zone"); r.Role != "drafter" || r.Origins["role"] != w.path(".agent-one/config.yaml")+":2" || r.Origins["authors"] != "policy" || !r.Authors {
		t.Errorf("extend: %+v", r)
	}
	if r, _ := c.Rank("principal-domain-owner"); !r.HoldsGate || !r.Sideways || r.ReportsTo != "coord" || r.Origins["holdsGate"] != "promotes:domain" {
		t.Errorf("promoted builtin: %+v", r)
	}
	if dev := c.Deviations(); len(dev) != 2 || !strings.HasPrefix(dev[0], "rank zone.role:") || !strings.HasPrefix(dev[1], "rank principal-domain-owner.job:") {
		t.Errorf("deviations: %v", dev)
	}
	refusals := []struct{ name, yaml, want string }{
		{"cycle", "ranks:\n  a: {reportsTo: b}\n  b: {reportsTo: a}\n", "cycle"},
		{"unknown parent", "ranks:\n  a: {reportsTo: nobody}\n", "no rank"},
		{"no parent", "rankSet: replace\nranks:\n  a: {job: x}\n", "escalation parent"},
		{"author without gate", "rankSet: replace\nranks:\n  a: {reportsTo: orchestrator, authors: true}\n", "no rank above it holds a gate"},
		{"promoted drops a base field", "ranks:\n  principal-domain-owner: {holdsGate: false}\n", "keeps everything"},
		{"promotes from promoted", "ranks:\n  x: {promotesFrom: principal-domain-owner}\n", "itself promoted"},
		{"root configured", "ranks:\n  orchestrator: {job: x}\n", "not configured"},
		{"bad agent", "ranks:\n  coord: {agent: ghost}\n", "not ephemeral or persistent"},
		{"bad rankSet", "rankSet: merge\n", "not extend or replace"},
		{"detach zone", "ranks:\n  zone: {reportsTo: orchestrator}\n", "no rank above it holds a gate"},
	}
	for _, cs := range refusals {
		w := newWorkspace(t, map[string]string{".agent-one/config.yaml": cs.yaml})
		err := w.loadErr()
		if err == nil || !strings.Contains(err.Error(), cs.want) {
			t.Errorf("%s: got %v, want %q", cs.name, err, cs.want)
		}
	}
}

func TestMCP(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
mcp:
  servers:
    files: {command: "npx", args: [-y, "@x/fs"], inward: true, tools: {read_file: {class: read}}}
    docs: {url: https://mcp.example.com, headersEnv: {Authorization: DOCS_TOKEN}, tools: {search: {class: destructive}}}
    off: {enabled: false}
`})
	c := w.load()
	if c.MCP.Servers["files"].Type != "stdio" || c.MCP.Servers["docs"].Type != "http" || !c.MCP.Servers["files"].Enabled || c.MCP.Servers["off"].Enabled {
		t.Errorf("servers: %+v", c.MCP.Servers)
	}
	if c.MCPClass("docs", "search") != "destructive" || c.MCPClass("docs", "x") != "outward" || c.MCPClass("files", "read_file") != "read" || c.MCPClass("nope", "x") != "outward" {
		t.Error("mcp classes")
	}
	refusals := []struct{ name, yaml, want string }{
		{"oauth later", "mcp:\n  servers:\n    r: {url: https://x, oauth: {clientId: a}}\n", "later"},
		{"auth header inline", "mcp:\n  servers:\n    r: {url: https://x, headers: {Authorization: \"Bearer t\"}}\n", "carries a credential"},
		{"headersEnv not a var", "mcp:\n  servers:\n    r: {url: https://x, headersEnv: {Authorization: \"Bearer t\"}}\n", "must name an environment variable"},
		{"secret in env", "mcp:\n  servers:\n    r: {command: [x], env: {TOKEN: sk-123}}\n", "carries a credential"},
		{"class below outward on outward server", "mcp:\n  servers:\n    r: {url: https://x, tools: {t: {class: read}}}\n", "below outward"},
		{"no transport", "mcp:\n  servers:\n    r: {inward: true}\n", "not stdio or http"},
		{"stdio without command", "mcp:\n  servers:\n    r: {type: stdio}\n", "needs command"},
		{"http without url", "mcp:\n  servers:\n    r: {type: http}\n", "needs url"},
	}
	for _, cs := range refusals {
		w := newWorkspace(t, map[string]string{".agent-one/config.yaml": cs.yaml})
		err := w.loadErr()
		if err == nil || !strings.Contains(err.Error(), cs.want) {
			t.Errorf("%s: got %v, want %q", cs.name, err, cs.want)
		}
	}
}

func TestRules(t *testing.T) {
	w := newWorkspace(t, map[string]string{
		".agent-one/config.yaml":          "rules:\n  - {id: a, text: A, scope: all}\n  - {file: r/b.md, scope: \"rank:domain\"}\n  - {text: C, scope: \"member:zone-auth\", check: \"exit 3\", timeout: 2}\n  - {check: \"echo hi; true\"}\n",
		".agent-one/r/b.md":               "B from file\n",
		"~/.config/agent-one/config.yaml": "rules:\n  - {file: g.md}\n",
		"~/.config/agent-one/g.md":        "G global\n",
	})
	c := w.load()
	if len(c.Rules) != 5 || c.Rules[0].ID != "rule1" || c.Rules[0].Text != "G global\n" || c.Rules[2].Text != "B from file\n" || c.Rules[2].ID != "rule3" {
		t.Fatalf("rules: %+v", c.Rules)
	}
	if c.Rules[2].Resolved != w.path(".agent-one/r/b.md") || c.Rules[0].Resolved != w.path("~/.config/agent-one/g.md") {
		t.Errorf("resolved relative to the declaring file: %s %s", c.Rules[2].Resolved, c.Rules[0].Resolved)
	}
	if got := c.PromptRules("domain", "domain-sec"); got != "## Rules\n- [rule1] G global\n- [a] A\n- [rule3] B from file\n" {
		t.Errorf("prompt: %q", got)
	}
	if got := c.PromptRules("zone", "zone-auth"); !strings.Contains(got, "- [rule4] C") || strings.Contains(got, "B from") {
		t.Errorf("prompt zone: %q", got)
	}
	facts := c.PolicyFacts("zone", "zone-auth")
	if len(facts) != 3 || facts[2].Kind != "policy" || facts[2].Scope != "member:zone-auth" || facts[2].Origin.Line != 4 {
		t.Errorf("facts: %+v", facts)
	}
	checks := c.Checks("zone", "zone-auth")
	if len(checks) != 2 || checks[0].Timeout.Seconds() != 2 || checks[1].Timeout.Seconds() != 60 {
		t.Fatalf("checks: %+v", checks)
	}
	res, ok := RunChecks(context.Background(), w.root, checks)
	if ok || res[0].Exit != 3 || res[0].Passed() || !res[1].Passed() || !strings.Contains(res[1].Output, "hi") {
		t.Errorf("run: %+v", res)
	}
	if _, ok := RunChecks(context.Background(), w.root, c.Checks("coord", "")); !ok {
		t.Error("coord checks should pass")
	}
	refusals := []struct{ name, yaml, want string }{
		{"missing file", "rules: [{file: nope.md}]\n", "no such file"},
		{"text and file", "rules: [{text: x, file: r/b.md}]\n", "one or the other"},
		{"empty", "rules: [{scope: all}]\n", "needs text, file or check"},
		{"bad scope", "rules: [{text: x, scope: everyone}]\n", "not all, rank:<rank> or member:<name>"},
		{"dup id", "rules: [{id: a, text: x}, {id: a, text: y}]\n", "used twice"},
	}
	for _, cs := range refusals {
		w := newWorkspace(t, map[string]string{".agent-one/config.yaml": cs.yaml, ".agent-one/r/b.md": "b"})
		err := w.loadErr()
		if err == nil || !strings.Contains(err.Error(), cs.want) {
			t.Errorf("%s: got %v, want %q", cs.name, err, cs.want)
		}
	}
}

func TestOffListAndBudgets(t *testing.T) {
	w := newWorkspace(t, map[string]string{
		"~/.config/agent-one/config.yaml": "memory: {short: {enabled: false}}\n",
		".agent-one/config.yaml":          "ontology: {enabled: false}\ncompaction: {passes: {unsaid: {enabled: false}}}\nmcp: {import: {opencode: {enabled: false}}}\ntools: {grep: {enabled: false}}\ninstruments: {status: {showOff: false}}\nbudgets: {subagent: {usd: 1.5}}\n",
	})
	c := w.load(Override{Path: "hooks.enabled", Value: "false", Flag: "--no-hooks"})
	want := []string{
		"@? off compaction.passes.unsaid.enabled — " + w.path(".agent-one/config.yaml") + ":2",
		"@? off hooks.enabled — flag:--no-hooks",
		"@? off instruments.status.showOff — " + w.path(".agent-one/config.yaml") + ":5",
		"@? off mcp.import.opencode.enabled — " + w.path(".agent-one/config.yaml") + ":3",
		"@? off memory.short.enabled — " + w.path("~/.config/agent-one/config.yaml") + ":1",
		"@? off ontology.enabled — " + w.path(".agent-one/config.yaml") + ":1",
	}
	if got := c.OffLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("off:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	st := strings.Join(c.StatusLines(), "\n")
	if !strings.Contains(st, "tools off: grep — disabled by tools.grep.enabled") || !strings.Contains(st, "ranks: orchestrator [rankSet: extend]") {
		t.Errorf("status:\n%s", st)
	}
	if b := c.BudgetLines(); b[0] != "budget session: off" || b[1] != "budget subagent: 0 tokens · $1.50" {
		t.Errorf("budgets: %v", b)
	}
}
