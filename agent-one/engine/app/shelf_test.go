package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/sandbox"
	"github.com/Kaginari/agent-one/tool"
)

var (
	buildOnce sync.Once
	mcpBin    string
	buildErr  error
)

func goBin() string {
	g := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(g); err == nil {
		return g
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return "go"
}

// mcpServer builds testdata/mcpserver once per test binary.
func mcpServer(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "agent-one-mcpserver")
		if err != nil {
			buildErr = err
			return
		}
		mcpBin = filepath.Join(dir, "mcpserver")
		cmd := exec.Command(goBin(), "build", "-o", mcpBin, "./testdata/mcpserver")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = err
			t.Log(string(out))
		}
	})
	if buildErr != nil {
		t.Skip("cannot build the MCP test server: " + buildErr.Error())
	}
	return mcpBin
}

func TestMain(m *testing.M) {
	code := m.Run()
	if mcpBin != "" {
		os.RemoveAll(filepath.Dir(mcpBin))
	}
	os.Exit(code)
}

// step is a scripted tool call for the loop.
func step(name string, input interface{}) *scriptStep {
	raw, _ := json.Marshal(input)
	return &scriptStep{Calls: []scriptCall{{Name: name, Input: raw}}}
}

// drive runs the scripted calls through a real loop on the shelf and returns the result.
func drive(t *testing.T, reg *tool.Registry, root string, hooks loop.Hooks, steps ...*scriptStep) *loop.Result {
	t.Helper()
	g := gate.New()
	g.IsTTY = func() bool { return false }
	return driveGate(t, reg, root, hooks, g, steps...)
}

func driveGate(t *testing.T, reg *tool.Registry, root string, hooks loop.Hooks, g *gate.Gate, steps ...*scriptStep) *loop.Result {
	t.Helper()
	steps = append(steps, &scriptStep{Text: "@S DONE\n@U team nothing new\n@E 30"})
	p := &scriptProvider{name: "mock/m", steps: steps, usage: provider.Usage{Input: 1000, Output: 50}}
	e := &loop.Engine{Provider: p, Tools: reg, Gate: g, Root: root, As: "orchestrator", Journal: "-", Hooks: hooks}
	r, err := e.Run(context.Background(), "do the scripted things")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func outputs(r *loop.Result) []string {
	var out []string
	for _, st := range r.Steps {
		out = append(out, st.Result.Output)
	}
	return out
}

// TestShelfJunction is rung 2's junction: the real config (rung 1) builds the real tool shelf
// and a scripted provider drives a real loop through it.
func TestShelfJunction(t *testing.T) {
	bin := mcpServer(t)
	outside := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("ANTHROPIC_API_KEY", "sk-secret-never-crosses")
	cfg, root := loadCfg(t, `
models: {default: mock/m}
providers: {mock: {enabled: true}}
tools:
  webfetch: {enabled: false}
  grep: {description: search text (overridden)}
  bash: {sandbox: bwrap, timeout: 30s}
  custom:
    lsl:
      description: long listing of one directory
      class: read
      params: {path: {type: string, required: true}}
      run: [ls, "-1", "{{path}}"]
permissions:
  rules:
    - {match: "bash:git push*", action: allow}
    - {match: "edit:**/*.lock", action: deny}
hooks:
  preTool:
    - {match: "write", command: "echo 'no writes today' >&2; exit 2"}
mcp:
  servers:
    t:
      command: [.agent-one/tmp/mcpserver]
      inward: true
      env: {MCP_PROBE: probe-ok}
      tools: {echo: {class: read}}
`, map[string]string{"ANTHROPIC_API_KEY": "sk-secret-never-crosses"})
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "go.lock"), []byte("v1\n"), 0o644)
	// the server binary must live inside the workspace: the sandbox hides /tmp (a private tmpfs)
	if data, err := os.ReadFile(bin); err == nil {
		_ = os.MkdirAll(filepath.Join(root, ".agent-one", "tmp"), 0o755)
		_ = os.WriteFile(filepath.Join(root, ".agent-one", "tmp", "mcpserver"), data, 0o755)
	}
	sb := newSandbox(cfg, root)
	shelf := NewShelf(cfg, root, sb, nil, os.Getenv)
	set := ConnectMCP(context.Background(), cfg, root, sb, os.Getenv)
	defer set.Close()
	if len(set.Holes()) != 0 || set.Servers["t"] == nil || set.Servers["t"].Client == nil {
		t.Fatalf("mcp: %v %+v", set.Holes(), set.Servers["t"])
	}
	shelf.Extra = append(shelf.Extra, func(string) []*tool.Tool { return set.Tools() })
	shelf.Resource = set.ReadResource
	shelf.Inward = set.IsInward

	// the shelf as config says: order, the disabled tool, the override, the custom tool, mcp
	reg, closer := shelf.Build("orchestrator")
	defer closer()
	names := strings.Join(reg.Names(), ",")
	for _, want := range []string{"bash", "read", "ls", "glob", "grep", "write", "edit", "str_replace_based_edit_tool", "multiedit", "patch", "git", "websearch", "ask", "lsl", "mcp__t__echo", "mcp__t__add", "mcp__t__wipe"} {
		if !strings.Contains(","+names+",", ","+want+",") {
			t.Errorf("shelf lacks %s: %s", want, names)
		}
	}
	if strings.Contains(names, "webfetch") {
		t.Error("webfetch is off")
	}
	if g, _ := reg.Get("grep"); g.Description != "search text (overridden)" {
		t.Errorf("override: %q", g.Description)
	}
	if why := shelf.Disabled()["webfetch"]; !strings.Contains(why, "tools.webfetch.enabled in") || !strings.Contains(why, "config.yaml:") {
		t.Errorf("disabled origin: %q", why)
	}
	if b, _ := reg.Get("bash"); b.Declare["anthropic"] == nil {
		t.Error("bash keeps its anthropic declaration under profile max")
	}
	echo, _ := reg.Get("mcp__t__echo")
	wipe, _ := reg.Get("mcp__t__wipe")
	if echo.Class != tool.Read || wipe.Class != tool.Destructive {
		t.Errorf("mcp classes: echo %s wipe %s", echo.Class, wipe.Class)
	}
	if len(set.Prompts) != 1 || set.Prompts[0].Name != "greet" {
		t.Errorf("prompts: %+v", set.Prompts)
	}

	env := func() tool.Env { return tool.Env{Root: root, WorkspaceDir: ".agent-one"} }
	missing := &MissingPolicy{Cfg: cfg, Disabled: shelf.Disabled}
	hooksRunner := &ShellHooks{Cfg: cfg, Root: root, Session: "s1"}
	hooks := loop.Hooks{Decide: decideHook(cfg, env), Missing: missing.Hook(), PreTool: hooksRunner.PreTool, PostTool: hooksRunner.PostTool}

	// 1. the custom tool, the persistent shell, secret scrubbing, mcp tool and resource
	r := drive(t, reg, root, hooks,
		step("lsl", map[string]string{"path": "."}),
		step("bash", map[string]string{"command": "export MARK=kept; echo hello > out.txt && cat out.txt"}),
		step("bash", map[string]string{"command": "echo \"k=$ANTHROPIC_API_KEY mark=$MARK\""}),
		step("mcp__t__echo", map[string]string{"text": "hi"}),
		step("mcp__t__add", map[string]int{"a": 2, "b": 3}),
		step("read", map[string]string{"path": "mcp://t/note://hello"}),
	)
	out := outputs(r)
	if r.Status != loop.Done || len(out) != 6 {
		t.Fatalf("run 1: %s %v %v", r.Status, r.Holes, out)
	}
	if !strings.Contains(out[0], "README.md") {
		t.Errorf("custom lsl: %q", out[0])
	}
	if !strings.Contains(out[1], "hello") {
		t.Errorf("bash: %q", out[1])
	}
	if !strings.Contains(out[2], "k= mark=kept") {
		t.Errorf("secret scrubbed and export kept across calls: %q", out[2])
	}
	if !strings.Contains(out[3], "echo: hi (env probe-ok)") || out[4] != "5" || !strings.Contains(out[5], "hello from the resource") {
		t.Errorf("mcp: %q %q %q", out[3], out[4], out[5])
	}

	// 2. the sandbox: a write outside the workspace does not land; the network is unreachable
	if sb.Mode() != sandbox.Bwrap {
		t.Logf("sandbox none (%s) — egress control checks skipped", sb.Why())
	} else {
		// outward acts pre-approved on purpose: what must stop them here is the sandbox itself
		approved := gate.New()
		approved.IsTTY = func() bool { return false }
		approved.Approve[tool.Outward] = true
		r = driveGate(t, reg, root, hooks, approved,
			step("bash", map[string]string{"command": "echo x > " + outside + "/probe; echo wrote $?"}),
			step("bash", map[string]string{"command": "exec 3<>/dev/tcp/127.0.0.1/" + port + " && echo connected"}),
		)
		out = outputs(r)
		if len(out) != 2 {
			t.Fatalf("sandbox run: %s %v %v", r.Status, r.Holes, out)
		}
		if _, err := os.Stat(filepath.Join(outside, "probe")); err == nil {
			t.Errorf("a write outside the workspace landed on the host: %q", out[0])
		}
		if !strings.Contains(out[1], "exit") || strings.Contains(out[1], "connected") {
			t.Errorf("the network was reachable without the gate: %q", out[1])
		}
	}

	// 3. a disabled tool: one error naming why and the alternatives; twice stops the turn
	r = drive(t, reg, root, hooks,
		step("webfetch", map[string]string{"url": "https://example.com"}),
		step("webfetch", map[string]string{"url": "https://example.com"}),
		step("bash", map[string]string{"command": "echo not reached"}),
	)
	out = outputs(r)
	if r.Status != loop.Escalate || len(r.Steps) != 2 {
		var sts []string
		for _, st := range r.Steps {
			sts = append(sts, st.Tool+":"+st.Status)
		}
		t.Fatalf("doom loop: %s steps %v holes %v out %q", r.Status, sts, r.Holes, out)
	}
	if !strings.Contains(out[0], `tool "webfetch" is disabled by tools.webfetch.enabled in`) || !strings.Contains(out[0], "closest enabled:") {
		t.Errorf("first missing call: %q", out[0])
	}
	if !strings.Contains(out[1], "doom-loop guard") || !strings.Contains(strings.Join(r.Holes, "|"), "doom-loop guard") {
		t.Errorf("second missing call: %q %v", out[1], r.Holes)
	}
	if !strings.Contains(out[1], "named twice (Provisioning): proposed config patch") || !strings.Contains(out[1], "+    enabled: true") {
		t.Errorf("second naming proposes the patch: %q", out[1])
	}
	if strings.Contains(out[0], "disabled by disabled by") {
		t.Errorf("double prefix: %q", out[0])
	}

	// 4. permissions before the gate: allow silences it, deny refuses; a hook blocks
	r = drive(t, reg, root, hooks,
		step("bash", map[string]string{"command": "git push origin main"}),
		step("edit", map[string]string{"path": "go.lock", "old": "v1", "new": "v2"}),
		step("write", map[string]string{"path": "note.txt", "content": "x"}),
	)
	out = outputs(r)
	if len(r.Steps) != 3 {
		t.Fatalf("run 4: %s %v", r.Status, out)
	}
	if g := r.Steps[0].Gate; g.By != "rule" || g.Decision != gate.Approved || !strings.Contains(g.Why, "bash:git push*") {
		t.Errorf("allow rule: %+v", g)
	}
	if r.Steps[1].Status != "refused" || !strings.Contains(out[1], "denied by permission rule") || !strings.Contains(out[1], "edit:**/*.lock") {
		t.Errorf("deny rule: %s %q", r.Steps[1].Status, out[1])
	}
	if r.Steps[2].Status != "refused" || !strings.Contains(out[2], "no writes today") {
		t.Errorf("hook block: %s %q", r.Steps[2].Status, out[2])
	}
	if _, err := os.Stat(filepath.Join(root, "note.txt")); err == nil {
		t.Error("the blocked write landed")
	}

	// 5. a destructive mcp tool asks the gate; no TTY → denied, the turn ends
	r = drive(t, reg, root, hooks, step("mcp__t__wipe", map[string]string{}))
	if r.Status != loop.Denied || r.Steps[0].Gate.By != "no-tty" {
		t.Errorf("mcp destructive: %s %+v", r.Status, r.Steps[0].Gate)
	}

	// 6. a fresh build gives a fresh shell (per-agent session), and the closer ends it
	reg2, closer2 := shelf.Build("zone-x")
	r = drive(t, reg2, root, hooks, step("bash", map[string]string{"command": "echo mark=$MARK"}))
	if out := outputs(r); !strings.Contains(out[0], "mark=") || strings.Contains(out[0], "mark=kept") {
		t.Errorf("per-agent shell: %q", out)
	}
	closer2()
	if len(shelf.Holes()) != 0 {
		t.Errorf("shelf holes: %v", shelf.Holes())
	}
}

func TestMinimalProfileAndOpenAIShape(t *testing.T) {
	cfg, root := loadCfg(t, `
models: {default: mock/m}
providers: {mock: {enabled: true}}
tools:
  profile: minimal
  bash: {sandbox: none}
`, nil)
	shelf := NewShelf(cfg, root, newSandbox(cfg, root), nil, os.Getenv)
	reg, closer := shelf.Build("orchestrator")
	defer closer()
	names := strings.Join(reg.Names(), ",")
	for _, off := range []string{"ls", "glob", "grep", "git", "webfetch", "str_replace_based_edit_tool", "multiedit"} {
		if strings.Contains(","+names+",", ","+off+",") {
			t.Errorf("minimal offers %s: %s", off, names)
		}
	}
	b, ok := reg.Get("bash")
	if !ok || !strings.Contains(string(b.Schema), `"command":{"type":"string"}`) {
		t.Fatalf("minimal must offer bash with a command string: %v %s", ok, names)
	}
	if why := shelf.Disabled()["grep"]; !strings.Contains(why, "tools.profile minimal") {
		t.Errorf("profile reason: %q", why)
	}
	cfg2, root2 := loadCfg(t, "models: {default: mock/m}\nproviders: {mock: {enabled: true}}\ntools: {profile: openai, bash: {sandbox: none}}\n", nil)
	reg2, closer2 := NewShelf(cfg2, root2, nil, nil, os.Getenv).Build("orchestrator")
	defer closer2()
	if b, _ := reg2.Get("bash"); b.Declare != nil {
		t.Error("profile openai strips the anthropic declarations")
	}
}

func TestDiscoverAndHooks(t *testing.T) {
	cfg, root := loadCfg(t, "models: {default: mock/m}\nproviders: {mock: {enabled: true}}\nhooks:\n  sessionStart: [{command: \"echo '@S OK' ; echo '@F hook ran'\"}]\n  userPrompt: [{command: \"grep -q forbidden && { echo nope >&2; exit 2; }; exit 0\"}]\n", nil)
	home := filepath.Join(filepath.Dir(root), "home")
	write := func(p, s string) {
		full := filepath.Join(root, p)
		if strings.HasPrefix(p, "~/") {
			full = filepath.Join(home, p[2:])
		}
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(s), 0o644)
	}
	write(".claude/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: test first\n---\nagent\n")
	write(".agent-one/skills/wire/SKILL.md", "---\ndescription: speak the wire\n---\nwire agent\n")
	write(".claude/commands/review.md", "---\ndescription: review\n---\nReview $ARGUMENTS now\n")
	write(".agent-one/commands/gate/check.md", "Check the gate for $1\n")
	write("~/.claude/skills/global-skill/SKILL.md", "---\ndescription: global\n---\n")
	write(".claude/agents/reviewer.md", "---\nname: reviewer\ndescription: reviews\ntools: read, grep\n---\nYou review.\n")
	d := Discover(cfg, root, home)
	names := func(xs []string) string { return strings.Join(xs, ",") }
	var sk, cm, ag []string
	for _, s := range d.Skills {
		sk = append(sk, s.Name+":"+s.Source)
	}
	for _, c := range d.Commands {
		cm = append(cm, c.Name)
	}
	for _, a := range d.Agents {
		ag = append(ag, a.Name+":"+a.Mode)
	}
	if names(sk) != "global-skill:claude-global,tdd:claude,wire:native" {
		t.Errorf("skills: %s", names(sk))
	}
	if names(cm) != "gate:check,review" {
		t.Errorf("commands: %s", names(cm))
	}
	if names(ag) != "reviewer:subagent" {
		t.Errorf("agents: %s", names(ag))
	}
	if c, ok := d.Command("gate:check"); !ok || c.Expand("auth zone") != "Check the gate for auth\n" {
		t.Errorf("expand: %+v", c)
	}
	if s, ok := d.Skill("wire"); !ok {
		t.Error("native skill")
	} else if agent, _ := s.Load(); strings.TrimSpace(agent) != "wire agent" {
		t.Errorf("skill agent: %q", agent)
	}
	var journaled []map[string]interface{}
	h := &ShellHooks{Cfg: cfg, Root: root, Session: "s", Journal: func(ev string, f map[string]interface{}) { journaled = append(journaled, f) }}
	h.SessionStart(context.Background())
	if len(journaled) != 1 || journaled[0]["exit"] != 0 || len(journaled[0]["findings"].([]string)) != 1 {
		t.Errorf("session hook wire: %+v", journaled)
	}
	if why := h.UserPrompt(context.Background(), "this is forbidden"); why != "nope" {
		t.Errorf("userPrompt block: %q", why)
	}
	if why := h.UserPrompt(context.Background(), "fine"); why != "" {
		t.Errorf("userPrompt pass: %q", why)
	}
	_ = time.Second
}
