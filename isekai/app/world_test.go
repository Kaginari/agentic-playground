package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/world"
)

const testLaw = `# The law

Every creature reads this file before working.

## The Crest

1. **Vitality** — When anything changes, its doc changes in the same change — or it dies.
2. **Containment** — Territory inward; nothing leaves outward without the human agreeing.

## Laws

1. Veldora's word is law.
`

// testWorld writes a world (files are root-relative; "~/" is the fake home) with a config and
// mock scripts, and opens the App on it with a fake environment and no TTY.
type testWorld struct {
	t    *testing.T
	root string
	home string
	dist string
}

func newTestWorld(t *testing.T, dist string, files map[string]string) *testWorld {
	t.Helper()
	dir := t.TempDir()
	w := &testWorld{t: t, root: filepath.Join(dir, "world"), home: filepath.Join(dir, "home"), dist: dist}
	_ = os.MkdirAll(w.home, 0o755)
	for p, s := range files {
		w.write(p, s)
	}
	return w
}

func (w *testWorld) write(p, s string) {
	full := filepath.Join(w.root, p)
	if strings.HasPrefix(p, "~/") {
		full = filepath.Join(w.home, p[2:])
	}
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *testWorld) read(p string) string {
	b, _ := os.ReadFile(filepath.Join(w.root, p))
	return string(b)
}

func (w *testWorld) script(name string, steps ...map[string]interface{}) string {
	b, _ := json.MarshalIndent(steps, "", " ")
	w.write(name, string(b))
	return name
}

func (w *testWorld) open() *App {
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

func isekaiCreatures() map[string]string {
	return map[string]string{
		".isekai/isekai.md":              testLaw,
		".isekai/log.md":                 "# Chronicle\n\nAppend-only.\n\n---\n",
		".isekai/elf/core/README.md":     "# elf-core\n\n- **Rank:** Elf\n- **Territory:** `src/`\n- **Purpose:** the shared mind\n",
		".isekai/orc/security/README.md": "# orc-security\n\n- **Rank:** Orc\n- **Territory:** `src/auth/`, `src/api/`\n- **Reports to:** elf-core\n",
		".isekai/slime/auth/README.md":   "# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n\n## Traits\n- tokens expire after one hour\n",
		".isekai/slime/api/README.md":    "# slime-api\n\n- **Rank:** Slime\n- **Territory:** `src/api/`\n- **Reports to:** orc-security\n",
		"src/auth/login.go":              "package auth\n",
		"src/api/server.go":              "package api\n",
		"README.md":                      "hello\n",
	}
}

// TestDispatchJunction is rung 3's junction: the real shelf (rung 2) under the real world —
// a Court dispatched with office routing to a distinct model, territory enforced, the gate's
// verdict in log.md, the unsaid landed.
func TestDispatchJunction(t *testing.T) {
	files := isekaiCreatures()
	w := newTestWorld(t, "isekai", files)
	w.script(".isekai/tmp/session.json",
		when("do the work", call("dispatch", map[string]string{"body": "slime-auth", "ask": "findings on the login zone: land the token file", "scope": "src/auth"})),
		text("@S DONE\n@F the court reported\n@E 34"),
	)
	w.script(".isekai/tmp/sage.json",
		when("@ASK findings", map[string]interface{}{"calls": []interface{}{
			map[string]interface{}{"name": "write", "input": map[string]string{"path": "src/auth/token.go", "content": "package auth // ttl 15m\n"}},
			map[string]interface{}{"name": "write", "input": map[string]string{"path": ".isekai/slime/auth/README.md", "content": files[".isekai/slime/auth/README.md"] + "- token ttl is 15m\n"}},
		}}),
		call("write", map[string]string{"path": "src/api/rogue.go", "content": "package api\n"}),
		text("@S PASS\n@F src/auth/token.go:1 token ttl is 15m\n@U territory the token TTL is 15m\n@E 80"),
	)
	w.write(".isekai/config.yaml", `
models:
  default: m1/mount
  offices: {great-sage: m2/sage}
providers:
  m1: {type: mock, script: .isekai/tmp/session.json}
  m2: {type: mock, script: .isekai/tmp/sage.json}
tools: {bash: {sandbox: none}}
ui: {board: {autostart: false}}
`)
	a := w.open()
	if a.Lex.Vocab != "isekai" || len(a.World.Creatures) != 4 {
		t.Fatalf("world: %s %d creatures", a.Lex.Vocab, len(a.World.Creatures))
	}
	e, err := a.Engine()
	if err != nil {
		t.Fatal(err)
	}
	if names := strings.Join(e.Tools.Names(), ","); !strings.Contains(names, "dispatch") || !strings.Contains(names, "recall") || !strings.Contains(names, "law") || !strings.Contains(names, "desk") {
		t.Fatalf("rimuru's shelf: %s", names)
	}
	r, err := e.Run(context.Background(), "do the work")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != loop.Done || len(r.Steps) != 1 {
		t.Fatalf("session: %s %v", r.Status, r.Holes)
	}
	report := r.Steps[0].Result.Output
	if !strings.Contains(report, "@S PASS") || !strings.Contains(report, "@U territory the token TTL is 15m") {
		t.Errorf("court report: %q", report)
	}
	if w.read("src/auth/token.go") == "" {
		t.Error("the court's write in territory did not land")
	}
	if _, err := os.Stat(filepath.Join(w.root, "src/api/rogue.go")); err == nil {
		t.Error("a write outside the court's territory landed")
	}
	// office routing: the court ran on m2 (great-sage), the session on m1
	recs, _ := ReadUsage(a.Journal.Dir)
	byBody := map[string]UsageRecord{}
	for _, rec := range recs {
		byBody[rec.Body] = rec
	}
	if c := byBody["slime-auth"]; c.Model != "m2/sage" || c.Office != "great-sage" || c.Rank != "slime" {
		t.Errorf("court usage: %+v", c)
	}
	if s := byBody["rimuru"]; s.Model != "m1/mount" {
		t.Errorf("session usage: %+v", s)
	}
	// the gate's verdict is in log.md; the unsaid landed in the notes and the ontology
	log := w.read(".isekai/log.md")
	if !strings.Contains(log, "slime-auth — gate pass") || !strings.Contains(log, "@U territory the token TTL is 15m") {
		t.Errorf("log: %s", log)
	}
	notes := w.read(".isekai/memory/shared/notes.jsonl")
	if !strings.Contains(notes, `"kind":"territory"`) || !strings.Contains(notes, "the token TTL is 15m") {
		t.Errorf("notes: %s", notes)
	}
	if ttl := w.read(".isekai/ontology/graph/unsaid.ttl"); !strings.Contains(ttl, "the token TTL is 15m") {
		t.Errorf("ontology: %s", ttl)
	}
	// the court is visible after the fact
	found := false
	for _, b := range a.Court.Bodies() {
		if b.Name == "slime-auth" && b.State == "done" && b.Office == "great-sage" {
			found = true
		}
	}
	if !found {
		t.Errorf("court bodies: %+v", a.Court.Bodies())
	}
	if lines := a.StatusLines(); len(lines) == 0 || strings.Contains(strings.Join(lines, "\n"), "@? off ") {
		t.Errorf("status: %v", lines)
	}
}

// TestDrainJunction: the context reading crosses the stress line mid-run, the drain sends the
// unsaid home and rebuilds the context, and the run continues to its end.
func TestDrainJunction(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	var steps []map[string]interface{}
	for i := 1; i <= 5; i++ {
		// each step's output is bulky, as a real context is; the drain must shrink it
		steps = append(steps, call("bash", map[string]string{"command": "echo step " + string(rune('0'+i)) + "; seq 1 400"}))
	}
	steps = append(steps, text("@S DONE\n@E 12"))
	w.script(".isekai/tmp/session.json", steps...)
	w.script(".isekai/tmp/drain.json",
		when("drained messages", text("@S DRAINED\n@F README.md:1 hello\n@U colony the drain saw five echoes\n@D 2026-09-26 goal: echo five times\n@D 2026-09-26 next: the fifth echo\n@E 120")),
	)
	w.write(".isekai/config.yaml", `
models:
  default: m1/mount
  tasks: {drain: m2/drainer}
providers:
  m1: {type: mock, script: .isekai/tmp/session.json}
  m2: {type: mock, script: .isekai/tmp/drain.json}
tools: {bash: {sandbox: none}}
law: {budget: {contextTokens: 200000, stressTokens: 2600}}
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
	if !strings.Contains(w.read(".isekai/memory/shared/notes.jsonl"), "the drain saw five echoes") {
		t.Error("the drain's @U did not land")
	}
	desk := w.read(a.Drainer.DeskPath(r.RunID)[len(w.root)+1:])
	if !strings.Contains(desk, "goal: echo five times") {
		t.Errorf("desk: %q", desk)
	}
	recs, _ := ReadUsage(a.Journal.Dir)
	drained := false
	for _, rec := range recs {
		if rec.Body == "drain" && rec.Model == "m2/drainer" {
			drained = true
		}
	}
	if !drained {
		t.Errorf("the drain's call is not journaled under the drain task: %+v", recs)
	}
}

// TestReplaceRanks: a 5-rank world in replace mode runs with none of the law's rank names.
func TestReplaceRanks(t *testing.T) {
	w := newTestWorld(t, "isekai", map[string]string{
		".isekai/isekai.md":             testLaw,
		".isekai/log.md":                "# Chronicle\n\n---\n",
		".isekai/lead/core/README.md":   "# lead-core\n\n- **Rank:** lead\n- **Territory:** `src/`\n",
		".isekai/keeper/gate/README.md": "# keeper-gate\n\n- **Rank:** keeper\n- **Territory:** `src/`\n- **Reports to:** lead-core\n",
		".isekai/coder/auth/README.md":  "# coder-auth\n\n- **Rank:** coder\n- **Territory:** `src/auth/`\n- **Reports to:** keeper-gate\n",
		".isekai/tester/qa/README.md":   "# tester-qa\n\n- **Rank:** tester\n- **Territory:** `tests/`\n- **Reports to:** keeper-gate\n",
		".isekai/scribe/log/README.md":  "# scribe-log\n\n- **Rank:** scribe\n- **Territory:** `docs/`\n- **Reports to:** lead-core\n",
		"src/auth/a.go":                 "package auth\n",
	})
	w.script(".isekai/tmp/session.json",
		when("start", call("dispatch", map[string]string{"body": "coder-auth", "ask": "findings: write src/auth/b.go", "scope": "src/auth"})),
		text("@S DONE\n@E 12"),
	)
	w.script(".isekai/tmp/coder.json",
		when("@ASK findings", map[string]interface{}{"calls": []interface{}{
			map[string]interface{}{"name": "write", "input": map[string]string{"path": "src/auth/b.go", "content": "package auth\n"}},
			map[string]interface{}{"name": "write", "input": map[string]string{"path": ".isekai/coder/auth/README.md", "content": "# coder-auth\n\n- **Rank:** coder\n- **Territory:** `src/auth/`\n- **Reports to:** keeper-gate\n- b.go added\n"}},
		}}),
		text("@S PASS\n@U colony coders write under src/auth\n@E 50"),
	)
	w.write(".isekai/config.yaml", `
models: {default: m1/mount, ranks: {coder: m2/coder}}
providers:
  m1: {type: mock, script: .isekai/tmp/session.json}
  m2: {type: mock, script: .isekai/tmp/coder.json}
tools: {bash: {sandbox: none}}
rankSet: replace
ranks:
  lead:   {reportsTo: rimuru, job: coordinates, body: court, office: ciel}
  keeper: {reportsTo: lead, job: holds the gate, holdsGate: true, body: keeper, office: raphael, sideways: true}
  coder:  {reportsTo: keeper, job: writes code, authors: true, body: court, office: great-sage, tools: ["read", "write", "edit", "bash", "law"]}
  tester: {reportsTo: keeper, job: writes tests, authors: true, body: court, office: great-sage}
  scribe: {reportsTo: lead, job: keeps the docs, authors: false, body: court, office: ciel}
ui: {board: {autostart: false}}
`)
	a := w.open()
	for _, r := range a.World.Ranks {
		for _, law := range []string{"elf", "orc", "slime", "kijin"} {
			if r.Name == law {
				t.Errorf("law rank %s loaded in replace mode", law)
			}
		}
	}
	if len(a.World.Ranks) != 5 || len(a.World.Creatures) != 5 {
		t.Fatalf("ranks %d creatures %d: %+v", len(a.World.Ranks), len(a.World.Creatures), a.World.Creatures)
	}
	if c := a.World.Creature("coder-auth"); c == nil || c.Rank != "coder" || c.Parent != "keeper-gate" {
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
		t.Fatalf("dispatch in a replace-mode world: %s %v %q", r.Status, r.Holes, r.Steps[0].Result.Output)
	}
	if !strings.Contains(w.read(".isekai/log.md"), "coder-auth — gate pass") {
		t.Errorf("gate: %s", w.read(".isekai/log.md"))
	}
	recs, _ := ReadUsage(a.Journal.Dir)
	ok := false
	for _, rec := range recs {
		if rec.Body == "coder-auth" && rec.Model == "m2/coder" && rec.Rank == "coder" {
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

// TestAgentOneWorld: the same engine under the other distribution — .agent-one/, AGENT-ONE.md,
// zone-/domain- prefixes, the wire's kind tokens, memory and toolbox in the renamed dir.
func TestAgentOneWorld(t *testing.T) {
	lex := world.AgentOne()
	law := strings.Replace(testLaw, "## The Crest", "## "+lex.Crest, 1)
	w := newTestWorld(t, "", map[string]string{
		".agent-one/AGENT-ONE.md":            law,
		".agent-one/log.md":                  "# Chronicle\n\n---\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Rank:** Domain owner\n- **Owns:** `src/auth/`\n",
		".agent-one/zone/auth/README.md":     "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n",
		"src/auth/a.go":                      "package auth\n",
	})
	w.script(".agent-one/tmp/session.json",
		when("begin", map[string]interface{}{"calls": []interface{}{
			map[string]interface{}{"name": "remember", "input": map[string]string{"text": "the login zone rotates keys weekly", "kind": "team"}},
			map[string]interface{}{"name": "recall", "input": map[string]string{"question": "login zone keys"}},
			map[string]interface{}{"name": "onto", "input": map[string]string{"path": "src/auth/a.go"}},
			map[string]interface{}{"name": "dispatch", "input": map[string]string{"body": "zone-auth", "ask": "findings: look around"}},
		}}),
		text("@S DONE\n@E 12"),
	)
	w.script(".agent-one/tmp/zone.json", when("@ASK findings", text("@S PASS\n@U domain keys rotate weekly\n@E 40")))
	w.write(".agent-one/config.yaml", `
models: {default: m1/mount, offices: {analyst: m2/zone}}
providers:
  m1: {type: mock, script: .agent-one/tmp/session.json}
  m2: {type: mock, script: .agent-one/tmp/zone.json}
tools: {bash: {sandbox: none}}
ui: {board: {autostart: false}}
`)
	a := w.open()
	if a.Cfg.Dist.Name != "agent-one" || a.Lex.Vocab != "agent-one" || a.Cfg.Dist.WorldDir != ".agent-one" {
		t.Fatalf("dist: %+v %s", a.Cfg.Dist, a.Lex.Vocab)
	}
	if c := a.World.Creature("zone-auth"); c == nil || c.Rank != "slime" || c.Parent != "domain-security" {
		t.Fatalf("zone-auth: %+v (roster %+v)", c, a.World.Creatures)
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
	if !strings.HasPrefix(out[0], "noted (world") {
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
	if !strings.Contains(notes, `"kind":"colony"`) || !strings.Contains(notes, `"kind":"territory"`) {
		t.Errorf("canonical kinds on disk: %s", notes)
	}
	if _, err := os.Stat(filepath.Join(w.root, ".isekai")); err == nil {
		t.Error("an .isekai/ dir appeared in an agent-one world")
	}
}
