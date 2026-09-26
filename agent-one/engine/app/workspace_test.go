package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/workspace"
)

const testPolicy = `# The policy

Every member reads this file before working.

## Core principles

1. **Docs-as-code** — When anything changes, its doc changes in the same change — or it dies.
2. **Egress control** — Ownership inward; nothing leaves outward without the human agreeing.

## Policies

1. Operator's word is policy.
`

// testWorkspace writes a workspace (files are root-relative; "~/" is the fake home) with a config and
// mock scripts, and opens the App on it with a fake environment and no TTY.
type testWorkspace struct {
	t    *testing.T
	root string
	home string
	dist string
}

func newTestWorkspace(t *testing.T, dist string, files map[string]string) *testWorkspace {
	t.Helper()
	dir := t.TempDir()
	w := &testWorkspace{t: t, root: filepath.Join(dir, "workspace"), home: filepath.Join(dir, "home"), dist: dist}
	_ = os.MkdirAll(w.home, 0o755)
	for p, s := range files {
		w.write(p, s)
	}
	return w
}

func (w *testWorkspace) write(p, s string) {
	full := filepath.Join(w.root, p)
	if strings.HasPrefix(p, "~/") {
		full = filepath.Join(w.home, p[2:])
	}
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *testWorkspace) read(p string) string {
	b, _ := os.ReadFile(filepath.Join(w.root, p))
	return string(b)
}

func (w *testWorkspace) script(name string, steps ...map[string]interface{}) string {
	b, _ := json.MarshalIndent(steps, "", " ")
	w.write(name, string(b))
	return name
}

func (w *testWorkspace) open() *App {
	w.t.Helper()
	a, err := New(Options{Dist: w.dist, Root: w.root, Cwd: w.root, Home: w.home, Env: func(k string) string {
		if k == "HOME" {
			return w.home
		}
		return ""
	}, In: strings.NewReader(""), Out: &strings.Builder{}, Err: &strings.Builder{}, Quiet: true, IsTTY: func() bool { return false }, NoBoard: true})
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(a.Close)
	return a
}

func call(name string, input interface{}) map[string]interface{} {
	return map[string]interface{}{"calls": []interface{}{map[string]interface{}{"name": name, "input": input}}}
}

func when(s string, step map[string]interface{}) map[string]interface{} {
	step["when"] = s
	return step
}

func text(s string) map[string]interface{} { return map[string]interface{}{"text": s} }

func agentOneMembers() map[string]string {
	return map[string]string{
		".agent-one/AGENT-ONE.md":              testPolicy,
		".agent-one/log.md":                    "# Chronicle\n\nAppend-only.\n\n---\n",
		".agent-one/coord/core/README.md":      "# coord-core\n\n- **Rank:** Coordinator\n- **Owns:** `src/`\n- **Purpose:** the shared skill\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Rank:** Domain owner\n- **Owns:** `src/auth/`, `src/api/`\n- **Reports to:** coord-core\n",
		".agent-one/zone/auth/README.md":       "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n\n## Invariants\n- tokens expire after one hour\n",
		".agent-one/zone/api/README.md":        "# zone-api\n\n- **Rank:** Zone worker\n- **Owns:** `src/api/`\n- **Reports to:** domain-security\n",
		"src/auth/login.go":                    "package auth\n",
		"src/api/server.go":                    "package api\n",
		"README.md":                            "hello\n",
	}
}

// TestDispatchJunction is rung 3's junction: the real shelf (rung 2) under the real workspace —
// a subagent dispatched with role routing to a distinct model, ownership enforced, the gate's
// verdict in log.md, the unsaid landed.
func TestDispatchJunction(t *testing.T) {
	files := agentOneMembers()
	w := newTestWorkspace(t, "agent-one", files)
	w.script(".agent-one/tmp/session.json",
		when("do the work", call("dispatch", map[string]string{"agent": "zone-auth", "ask": "findings on the login zone: land the token file", "scope": "src/auth"})),
		text("@S DONE\n@F the subagent reported\n@E 34"),
	)
	w.script(".agent-one/tmp/analyst.json",
		when("@ASK findings", map[string]interface{}{"calls": []interface{}{
			map[string]interface{}{"name": "write", "input": map[string]string{"path": "src/auth/token.go", "content": "package auth // ttl 15m\n"}},
			map[string]interface{}{"name": "write", "input": map[string]string{"path": ".agent-one/zone/auth/README.md", "content": files[".agent-one/zone/auth/README.md"] + "- token ttl is 15m\n"}},
		}}),
		call("write", map[string]string{"path": "src/api/rogue.go", "content": "package api\n"}),
		text("@S PASS\n@F src/auth/token.go:1 token ttl is 15m\n@U domain the token TTL is 15m\n@E 80"),
	)
	w.write(".agent-one/config.yaml", `
models:
  default: m1/mount
  roles: {analyst: m2/analyst}
providers:
  m1: {type: mock, script: .agent-one/tmp/session.json}
  m2: {type: mock, script: .agent-one/tmp/analyst.json}
tools: {bash: {sandbox: none}}
ui: {board: {autostart: false}}
`)
	a := w.open()
	if a.Lex.Vocab != "agent-one" || len(a.Workspace.Members) != 4 {
		t.Fatalf("workspace: %s %d members", a.Lex.Vocab, len(a.Workspace.Members))
	}
	e, err := a.Engine()
	if err != nil {
		t.Fatal(err)
	}
	if names := strings.Join(e.Tools.Names(), ","); !strings.Contains(names, "dispatch") || !strings.Contains(names, "recall") || !strings.Contains(names, "policy") || !strings.Contains(names, "workingNotes") {
		t.Fatalf("orchestrator's shelf: %s", names)
	}
	r, err := e.Run(context.Background(), "do the work")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != loop.Done || len(r.Steps) != 1 {
		t.Fatalf("session: %s %v", r.Status, r.Holes)
	}
	report := r.Steps[0].Result.Output
	if !strings.Contains(report, "@S PASS") || !strings.Contains(report, "@U domain the token TTL is 15m") {
		t.Errorf("subagent report: %q", report)
	}
	if w.read("src/auth/token.go") == "" {
		t.Error("the subagent's write in ownership did not land")
	}
	if _, err := os.Stat(filepath.Join(w.root, "src/api/rogue.go")); err == nil {
		t.Error("a write outside the subagent's ownership landed")
	}
	// role routing: the subagent ran on m2 (analyst), the session on m1
	recs, _ := ReadUsage(a.Journal.Dir)
	byAgent := map[string]UsageRecord{}
	for _, rec := range recs {
		byAgent[rec.Agent] = rec
	}
	if c := byAgent["zone-auth"]; c.Model != "m2/analyst" || c.Role != "analyst" || c.Rank != "zone" {
		t.Errorf("subagent usage: %+v", c)
	}
	if s := byAgent["orchestrator"]; s.Model != "m1/mount" {
		t.Errorf("session usage: %+v", s)
	}
	// the gate's verdict is in log.md; the unsaid landed in the notes and the ontology
	log := w.read(".agent-one/log.md")
	if !strings.Contains(log, "zone-auth — gate pass") || !strings.Contains(log, "@U domain the token TTL is 15m") {
		t.Errorf("log: %s", log)
	}
	notes := w.read(".agent-one/memory/shared/notes.jsonl")
	if !strings.Contains(notes, `"kind":"domain"`) || !strings.Contains(notes, "the token TTL is 15m") {
		t.Errorf("notes: %s", notes)
	}
	if ttl := w.read(".agent-one/ontology/graph/unsaid.ttl"); !strings.Contains(ttl, "the token TTL is 15m") {
		t.Errorf("ontology: %s", ttl)
	}
	// the subagent is visible after the fact
	found := false
	for _, b := range a.Subagent.Agents() {
		if b.Name == "zone-auth" && b.State == "done" && b.Role == "analyst" {
			found = true
		}
	}
	if !found {
		t.Errorf("ephemeral subagents: %+v", a.Subagent.Agents())
	}
	if lines := a.StatusLines(); len(lines) == 0 || strings.Contains(strings.Join(lines, "\n"), "@? off ") {
		t.Errorf("status: %v", lines)
	}
}

// TestDrainJunction: the context reading crosses the stress line mid-run, the drain sends the
// unsaid home and rebuilds the context, and the run continues to its end.
func TestDrainJunction(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	var steps []map[string]interface{}
	for i := 1; i <= 5; i++ {
		// each step's output is bulky, as a real context is; the drain must shrink it
		steps = append(steps, call("bash", map[string]string{"command": "echo step " + string(rune('0'+i)) + "; seq 1 400"}))
	}
	steps = append(steps, text("@S DONE\n@E 12"))
	w.script(".agent-one/tmp/session.json", steps...)
	w.script(".agent-one/tmp/drain.json",
		when("drained messages", text("@S DRAINED\n@F README.md:1 hello\n@U team the drain saw five echoes\n@D 2026-09-26 goal: echo five times\n@D 2026-09-26 next: the fifth echo\n@E 120")),
	)
	w.write(".agent-one/config.yaml", `
models:
  default: m1/mount
  tasks: {drain: m2/drainer}
providers:
  m1: {type: mock, script: .agent-one/tmp/session.json}
  m2: {type: mock, script: .agent-one/tmp/drain.json}
tools: {bash: {sandbox: none}}
policy: {budget: {contextTokens: 200000, stressTokens: 2600}}
compaction: {keepRecentTurns: 1}
ui: {board: {autostart: false}}
`)
	a := w.open()
	e, err := a.Engine()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Run(context.Background(), "echo five times")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != loop.Done || len(r.Steps) != 5 {
		t.Fatalf("run: %s steps %d holes %v", r.Status, len(r.Steps), r.Holes)
	}
	if a.Drainer.Last == nil || a.Drainer.Last.Aborted != "" || a.Drainer.Last.After >= a.Drainer.Last.Before {
		t.Fatalf("drain report: %+v", a.Drainer.Last)
	}
	if !strings.Contains(w.read(".agent-one/memory/shared/notes.jsonl"), "the drain saw five echoes") {
		t.Error("the drain's @U did not land")
	}
	workingNotes := w.read(a.Drainer.WorkingNotesPath(r.RunID)[len(w.root)+1:])
	if !strings.Contains(workingNotes, "goal: echo five times") {
		t.Errorf("workingNotes: %q", workingNotes)
	}
	recs, _ := ReadUsage(a.Journal.Dir)
	drained := false
	for _, rec := range recs {
		if rec.Agent == "drain" && rec.Model == "m2/drainer" {
			drained = true
		}
	}
	if !drained {
		t.Errorf("the drain's call is not journaled under the drain task: %+v", recs)
	}
}

// TestReplaceRanks: a 5-rank workspace in replace mode runs with none of the policy's rank names.
func TestReplaceRanks(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", map[string]string{
		".agent-one/AGENT-ONE.md":              testPolicy,
		".agent-one/log.md":                    "# Chronicle\n\n---\n",
		".agent-one/lead/core/README.md":       "# lead-core\n\n- **Rank:** lead\n- **Owns:** `src/`\n",
		".agent-one/persistent/gate/README.md": "# persistent-gate\n\n- **Rank:** persistent\n- **Owns:** `src/`\n- **Reports to:** lead-core\n",
		".agent-one/coder/auth/README.md":      "# coder-auth\n\n- **Rank:** coder\n- **Owns:** `src/auth/`\n- **Reports to:** persistent-gate\n",
		".agent-one/tester/qa/README.md":       "# tester-qa\n\n- **Rank:** tester\n- **Owns:** `tests/`\n- **Reports to:** persistent-gate\n",
		".agent-one/scribe/log/README.md":      "# scribe-log\n\n- **Rank:** scribe\n- **Owns:** `docs/`\n- **Reports to:** lead-core\n",
		"src/auth/a.go":                        "package auth\n",
	})
	w.script(".agent-one/tmp/session.json",
		when("start", call("dispatch", map[string]string{"agent": "coder-auth", "ask": "findings: write src/auth/b.go", "scope": "src/auth"})),
		text("@S DONE\n@E 12"),
	)
	w.script(".agent-one/tmp/coder.json",
		when("@ASK findings", map[string]interface{}{"calls": []interface{}{
			map[string]interface{}{"name": "write", "input": map[string]string{"path": "src/auth/b.go", "content": "package auth\n"}},
			map[string]interface{}{"name": "write", "input": map[string]string{"path": ".agent-one/coder/auth/README.md", "content": "# coder-auth\n\n- **Rank:** coder\n- **Owns:** `src/auth/`\n- **Reports to:** persistent-gate\n- b.go added\n"}},
			// tester-qa's ground: refused, never lands (a shell write there is caught by the gate instead — workspace.TestSubagentWritesGatedOnce)
			map[string]interface{}{"name": "write", "input": map[string]string{"path": "tests/x_test.go", "content": "package tests\n"}},
		}}),
		text("@S PASS\n@U team coders write under src/auth\n@E 50"),
	)
	w.write(".agent-one/config.yaml", `
models: {default: m1/mount, ranks: {coder: m2/coder}}
providers:
  m1: {type: mock, script: .agent-one/tmp/session.json}
  m2: {type: mock, script: .agent-one/tmp/coder.json}
tools: {bash: {sandbox: none}}
rankSet: replace
ranks:
  lead:   {reportsTo: orchestrator, job: coordinates, agent: ephemeral, role: drafter}
  persistent: {reportsTo: lead, job: holds the gate, holdsGate: true, agent: persistent, role: judge, sideways: true}
  coder:  {reportsTo: persistent, job: writes code, authors: true, agent: ephemeral, role: analyst, tools: ["read", "write", "edit", "bash", "policy"]}
  tester: {reportsTo: persistent, job: writes tests, authors: true, agent: ephemeral, role: analyst}
  scribe: {reportsTo: lead, job: keeps the docs, authors: false, agent: ephemeral, role: drafter}
ui: {board: {autostart: false}}
`)
	a := w.open()
	for _, r := range a.Workspace.Ranks {
		for _, policy := range []string{"coord", "domain", "zone", "service"} {
			if r.Name == policy {
				t.Errorf("policy rank %s loaded in replace mode", policy)
			}
		}
	}
	if len(a.Workspace.Ranks) != 5 || len(a.Workspace.Members) != 5 {
		t.Fatalf("ranks %d members %d: %+v", len(a.Workspace.Ranks), len(a.Workspace.Members), a.Workspace.Members)
	}
	if c := a.Workspace.Member("coder-auth"); c == nil || c.Rank != "coder" || c.Parent != "persistent-gate" {
		t.Fatalf("coder-auth: %+v", c)
	}
	e, err := a.Engine()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Run(context.Background(), "start")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != loop.Done || !strings.Contains(r.Steps[0].Result.Output, "@S PASS") {
		t.Fatalf("dispatch in a replace-mode workspace: %s %v %q", r.Status, r.Holes, r.Steps[0].Result.Output)
	}
	if !strings.Contains(w.read(".agent-one/log.md"), "coder-auth — gate pass") {
		t.Errorf("gate: %s", w.read(".agent-one/log.md"))
	}
	if _, err := os.Stat(filepath.Join(w.root, "tests", "x_test.go")); err == nil {
		t.Error("a coder's write into the tester's ownership landed in a replace-mode workspace")
	}
	if j := w.journalText(".agent-one"); !strings.Contains(j, `"by":"policy"`) || !strings.Contains(j, `"decision":"refused"`) {
		t.Errorf("ownership refusal not journaled:\n%s", j)
	}
	recs, _ := ReadUsage(a.Journal.Dir)
	ok := false
	for _, rec := range recs {
		if rec.Agent == "coder-auth" && rec.Model == "m2/coder" && rec.Rank == "coder" {
			ok = true
		}
	}
	if !ok {
		t.Errorf("rank routing: %+v", recs)
	}
	if dev := strings.Join(a.Cfg.Deviations(), "\n"); !strings.Contains(dev, "rankSet: replace") {
		t.Errorf("deviations: %s", dev)
	}
}

// TestAgentOneWorkspace: the same engine under the other distribution — .agent-one/, AGENT-ONE.md,
// zone-/domain- prefixes, the wire's kind tokens, memory and toolbox in the renamed dir.
func TestAgentOneWorkspace(t *testing.T) {
	lex := workspace.Default()
	policy := strings.Replace(testPolicy, "## Core principles", "## "+lex.Principles, 1)
	w := newTestWorkspace(t, "", map[string]string{
		".agent-one/AGENT-ONE.md":              policy,
		".agent-one/log.md":                    "# Chronicle\n\n---\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Rank:** Domain owner\n- **Owns:** `src/auth/`\n",
		".agent-one/zone/auth/README.md":       "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n",
		"src/auth/a.go":                        "package auth\n",
	})
	w.script(".agent-one/tmp/session.json",
		when("begin", map[string]interface{}{"calls": []interface{}{
			map[string]interface{}{"name": "remember", "input": map[string]string{"text": "the login zone rotates keys weekly", "kind": "team"}},
			map[string]interface{}{"name": "recall", "input": map[string]string{"question": "login zone keys"}},
			map[string]interface{}{"name": "onto", "input": map[string]string{"path": "src/auth/a.go"}},
			map[string]interface{}{"name": "dispatch", "input": map[string]string{"agent": "zone-auth", "ask": "findings: look around"}},
		}}),
		text("@S DONE\n@E 12"),
	)
	w.script(".agent-one/tmp/zone.json", when("@ASK findings", text("@S PASS\n@U domain keys rotate weekly\n@E 40")))
	w.write(".agent-one/config.yaml", `
models: {default: m1/mount, roles: {analyst: m2/zone}}
providers:
  m1: {type: mock, script: .agent-one/tmp/session.json}
  m2: {type: mock, script: .agent-one/tmp/zone.json}
tools: {bash: {sandbox: none}}
ui: {board: {autostart: false}}
`)
	a := w.open()
	if a.Cfg.Dist.Name != "agent-one" || a.Lex.Vocab != "agent-one" || a.Cfg.Dist.WorkspaceDir != ".agent-one" {
		t.Fatalf("dist: %+v %s", a.Cfg.Dist, a.Lex.Vocab)
	}
	if c := a.Workspace.Member("zone-auth"); c == nil || c.Rank != "zone" || c.Parent != "domain-security" {
		t.Fatalf("zone-auth: %+v (roster %+v)", c, a.Workspace.Members)
	}
	// what agent-one says to a person carries none of the vocabulary it was forked from (the
	// words are spelled in pieces so this file passes the same scan)
	leak := regexp.MustCompile(`(?i)` + strings.Join([]string{"ise" + "kai", "rim" + "uru", "veld" + "ora", "sli" + "me", `\borc` + `s?\b`, `\be` + `lf\b`, `\bel` + `ves\b`, "kij" + "in", "great[ -]" + "sage", "raph" + "ael", `\bci` + `el\b`, "temp" + "est"}, "|"))
	for name, text := range map[string]string{"status": strings.Join(a.StatusLines(), "\n"), "explain": a.Cfg.Explain(), "show": a.Cfg.Show(false), "show-yaml": a.Cfg.Show(true)} {
		if m := leak.FindAllString(text, -1); len(m) > 0 {
			t.Errorf("%s leaks the other vocabulary: %v\n%s", name, m, text)
		}
	}
	e, err := a.Engine()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Run(context.Background(), "begin")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != loop.Done || len(r.Steps) != 4 {
		t.Fatalf("run: %s %v", r.Status, r.Holes)
	}
	out := outputs(r)
	if !strings.HasPrefix(out[0], "noted (workspace") {
		t.Errorf("remember: %q", out[0])
	}
	if !strings.Contains(out[1], "rotates keys weekly") {
		t.Errorf("recall through .agent-one: %q", out[1])
	}
	if !strings.Contains(out[2], "owned by zone-auth") {
		t.Errorf("onto owner: %q", out[2])
	}
	if !strings.Contains(out[3], "@U domain keys rotate weekly") {
		t.Errorf("dispatch: %q", out[3])
	}
	notes := w.read(".agent-one/memory/shared/notes.jsonl")
	if !strings.Contains(notes, `"kind":"team"`) || !strings.Contains(notes, `"kind":"domain"`) {
		t.Errorf("canonical kinds on disk: %s", notes)
	}
}
