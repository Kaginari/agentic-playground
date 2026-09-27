package world

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/provider/mock"
	"github.com/Kaginari/agentic-playground/isekai/tool"
	"github.com/Kaginari/agentic-playground/isekai/wire"
)

const lawText = `# Isekai — the law

Every creature reads this file before working.

## The Crest

1. **Vitality** — When anything changes, its doc changes in the same change — or it dies.
2. **Symbiosis** — Each race does exactly its role.

## Principles

- Context flows down.

## Laws

1. Veldora's word is law.
`

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".isekai/isekai.md":              lawText,
		".isekai/log.md":                 "# Chronicle\n\nAppend-only.\n\n---\n\n### [2026-09-20T17:54:00Z] rimuru — World reincarnated\n- **Task:** /isekai\n- **Files:** none\n- **Gate:** n/a\n- **Result:** done\n- **Learned:** —\n",
		".isekai/elf/core/README.md":     "# elf-core\n\n- **Rank:** Elf\n- **Territory:** `src/`\n- **Purpose:** the shared mind\n",
		".isekai/orc/security/README.md": "# orc-security\n\n- **Rank:** Orc\n- **Territory:** `src/auth/`, `src/api/`\n- **Reports to:** elf-core\n- **Verify:** `test -d src`\n",
		".isekai/slime/auth/README.md":   "# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n- **Minds:** ciel\n- **Verify:** `test ! -f src/auth/BROKEN`\n\n## Traits\n- tokens expire after one hour\n",
		".isekai/slime/api/README.md":    "# slime-api\n\n- **Rank:** Slime\n- **Territory:** `src/api/`\n- **Orc:** orc-security\n\n## Verify\n- `true`\n",
		".claude/skills/ciel/SKILL.md":   "---\nname: ciel\ndescription: drafts the answer\n---\ndrafts\n",
		".claude/skills/tdd/SKILL.md":    "---\nname: tdd\ndescription: test first, red green refactor\n---\nshared host skill\n",
		"CLAUDE.md":                      "Read .isekai/isekai.md first.\n",
		"src/auth/login.go":              "package auth\n",
		"src/api/server.go":              "package api\n",
		"README.md":                      "hello\n",
	}
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func open(t *testing.T, dir string) *World {
	t.Helper()
	w, err := Open(dir, Isekai(), Options{Ontology: true, Instructions: InstructionOptions{Enabled: true, WalkUp: false}})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestLexicon(t *testing.T) {
	if b, err := os.ReadFile(filepath.Join("..", "..", "agent-one", "lexicon.json")); err == nil && string(b) != string(lexiconJSON) {
		t.Error("world/lexicon.json differs from agent-one/lexicon.json — Vitality: change both in the same change")
	}
	i, a := Isekai(), AgentOne()
	if i.WorldDir != ".isekai" || i.Law != "isekai.md" || i.Ranks["slime"] != "slime" || i.Prefixes["orc"] != "orc-" || i.Crest != "The Crest" || i.GateNA != "Gate: n/a (no orcs)" || i.Checks[0] != "Right slime authored" {
		t.Errorf("isekai lexicon: %+v", i)
	}
	if a.WorldDir != ".agent-one" || a.Law != "AGENT-ONE.md" || a.Ranks["slime"] != "zone" || a.Ranks["orc"] != "domain" || a.Prefixes["elf"] != "coord-" || a.Unsaid["colony"] != "team" || a.Human != "Operator" {
		t.Errorf("agent-one lexicon: %+v", a)
	}
	if k, ok := a.Canonical("team"); !ok || k != "colony" {
		t.Error("team → colony")
	}
	if k, ok := a.Canonical("law"); !ok || k != "law" {
		t.Error("canonical accepted on input")
	}
	if _, ok := a.Canonical("vibe"); ok {
		t.Error("unknown kind")
	}
	if a.Token("territory") != "domain" || i.Token("territory") != "territory" || a.Race("zone-auth") != "slime" || i.Race("zone-auth") != "" || i.Race("orc-x") != "orc" {
		t.Error("tokens and races")
	}
	if l := a.Layout(); len(l.Ranks) != 5 || l.Ranks[2].Dir != "zone" || l.Ranks[2].Parent != "orc" || l.Law != "AGENT-ONE.md" {
		t.Errorf("layout %+v", l)
	}
	if l := a.Loop(); l.Human != "Operator" || l.WorldDir != ".agent-one" || l.Body != "ephemeral-subagent" {
		t.Errorf("loop lexicon %+v", l)
	}
	if _, err := ParseLexicon(lexiconJSON, "klingon"); err == nil {
		t.Error("unknown vocabulary")
	}
}

func TestDiscoverAndOpen(t *testing.T) {
	dir := fixture(t)
	root, lex, err := Discover(filepath.Join(dir, "src", "auth"))
	if err != nil || root != dir || lex.Vocab != "isekai" {
		t.Fatalf("discover: %q %s %v", root, lex.Vocab, err)
	}
	w := open(t, dir)
	if w.Law == nil || !strings.HasPrefix(w.Law.Crest, "1. **Vitality**") || len(w.Law.Sections) != 4 {
		t.Fatalf("law: %+v", w.Law)
	}
	if s, ok := w.Law.Section("laws"); !ok || !strings.Contains(s.Text, "Veldora") {
		t.Error("section on demand")
	}
	names := []string{}
	for _, c := range w.Creatures {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "elf-core,orc-security,slime-api,slime-auth" {
		t.Fatalf("roster %v", names)
	}
	auth := w.Creature("slime-auth")
	if auth.Rank != "slime" || auth.Parent != "orc-security" || auth.Doc != ".isekai/slime/auth/README.md" || auth.Dir != ".isekai/slime/auth" || strings.Join(auth.Territory, ",") != "src/auth" || strings.Join(auth.Minds, ",") != "ciel" || strings.Join(auth.Verify, ",") != "test ! -f src/auth/BROKEN" {
		t.Errorf("slime-auth %+v", auth)
	}
	if api := w.Creature("api"); api == nil || strings.Join(api.Verify, ",") != "true" || api.Parent != "orc-security" {
		t.Errorf("slime-api by short name %+v", api)
	}
	if w.Creature("core") == nil || w.Creature("nobody") != nil || !w.Orcs() {
		t.Error("lookup")
	}
	if o := w.Owner("src/auth/x.go"); o == nil || o.Name != "slime-auth" {
		t.Errorf("owner %+v", o)
	}
	if o := w.Owner("src/other.go"); o != nil {
		t.Errorf("elf territory is not ownership: %+v", o)
	}
	if !auth.InTerritory("src/auth/deep/x") || auth.InTerritory("src/api/x") || !auth.InTerritory(".isekai/slime/auth/README.md") {
		t.Error("territory test")
	}
	if len(w.Instructions) != 1 || w.Instructions[0].Scope != "project" || !strings.Contains(w.Instructions[0].Text, "Read") {
		t.Errorf("instructions %+v", w.Instructions)
	}
	env := tool.Env{Root: dir}
	lt := w.LawTool()
	if r := lt.Run(context.Background(), env, json.RawMessage(`{}`)); r.Err || !strings.Contains(r.Output, "## The Crest") || !strings.Contains(r.Output, "## Laws") {
		t.Errorf("law tool list %+v", r)
	}
	if r := lt.Run(context.Background(), env, json.RawMessage(`{"section":"principles"}`)); r.Err || !strings.Contains(r.Output, "Context flows down") {
		t.Errorf("law tool section %+v", r)
	}
	if r := lt.Run(context.Background(), env, json.RawMessage(`{"section":"nope"}`)); !r.Err {
		t.Error("unknown section is an error")
	}
	// agent-one world, same engine
	az := t.TempDir()
	for p, body := range map[string]string{
		".agent-one/AGENT-ONE.md":              "# Agent-Zero\n\n## Core principles\n\n1. **Docs-as-code** — docs change with the code.\n\n## Policies\n\n1. The operator decides.\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Owns:** `src/`\n",
		".agent-one/zone/auth/README.md":       "# zone-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n",
	} {
		full := filepath.Join(az, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	root, lex, err = Discover(az)
	if err != nil || lex.Vocab != "agent-one" || root != az {
		t.Fatalf("agent-one discover: %v %s", err, lex.Vocab)
	}
	w2, err := Open(az, lex, Options{Ontology: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(w2.Law.Crest, "1. **Docs-as-code**") {
		t.Errorf("agent-one crest: %q", w2.Law.Crest)
	}
	if c := w2.Creature("zone-auth"); c == nil || c.Rank != "slime" || c.Parent != "domain-security" || strings.Join(c.Territory, ",") != "src/auth" {
		t.Errorf("agent-one creature %+v", c)
	}
	if !w2.Orcs() || w2.Owner("src/auth/x").Name != "zone-auth" {
		t.Error("agent-one orcs / owner")
	}
	if _, err := Open(t.TempDir(), Isekai(), Options{}); err == nil {
		t.Error("a directory without a world dir is not a world")
	}
}

func TestLogAppendOnly(t *testing.T) {
	dir := fixture(t)
	p := filepath.Join(dir, ".isekai", "log.md")
	before, _ := os.ReadFile(p)
	l, err := OpenLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Entry{Author: "slime-auth", Title: "first", Task: "t", Files: []string{"a", "b"}, Gate: "pass", Learned: []string{"x", "y"}}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(after), string(before)) || !strings.Contains(string(after), "] slime-auth — first\n- **Task:** t\n- **Files:** a, b\n- **Gate:** pass\n- **Result:** done\n- **Learned:**\n  - x\n  - y\n") {
		t.Fatalf("append:\n%s", after)
	}
	if err := l.Append(Entry{Title: "second"}); err != nil {
		t.Fatal(err)
	}
	// the past is rewritten under the writer → refuse
	cur, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(cur), "World reincarnated", "World rewritten", 1)), 0o644)
	if err := l.Append(Entry{Title: "third"}); err == nil || !strings.Contains(err.Error(), "rewritten") {
		t.Fatalf("rewrite not refused: %v", err)
	}
	os.WriteFile(p, cur[:len(cur)/2], 0o644)
	if err := l.Append(Entry{Title: "third"}); err == nil || !strings.Contains(err.Error(), "shrank") {
		t.Fatalf("truncation not refused: %v", err)
	}
	// a fresh log gets its header
	l2, _ := OpenLog(filepath.Join(dir, "new", "log.md"))
	if err := l2.Append(Entry{Title: "born"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(l2.Path); !strings.HasPrefix(string(b), "# Chronicle") || !strings.Contains(string(b), "— born") {
		t.Errorf("fresh log:\n%s", b)
	}
}

func noTTY() *gate.Gate {
	g := gate.New()
	g.IsTTY = func() bool { return false }
	return g
}

func build(m *mock.Provider) Build {
	h := DefaultHooks()
	h.Recall = RecallOptions{}
	h.Gate.Retries = 0 // a failing case fails at once; the retry has its own test
	return Build{Provider: m, Gate: noTTY(), Hooks: h, Territory: TerritoryOptions{Enabled: true}, Court: CourtOptions{Enabled: true, Unsaid: true}, Journal: "-"}
}

func writeCall(id, path, content string) provider.Response {
	return mock.Call(id, "write", map[string]string{"path": path, "content": content})
}

func TestTerritoryRefusal(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	m := mock.New(writeCall("1", "src/api/x.go", "x"), mock.Text("@S DONE\n@? src/api/x.go needed\n@U colony c\n@E 0"))
	e := w.Engine("slime-auth", build(m))
	if names := strings.Join(e.Tools.Names(), ","); names != "read,write,edit,bash,glob,grep,law" {
		t.Fatalf("a slime's shelf: %s", names)
	}
	r, err := e.Run(context.Background(), "x")
	if err != nil || r.Status != loop.Done || len(r.Steps) != 1 || r.Steps[0].Status != "refused" {
		t.Fatalf("%v %+v", err, r)
	}
	res := m.Requests[1].Messages[2].ToolResults[0]
	if !res.IsError || !strings.Contains(res.Content, "outside slime-auth's territory") || !strings.Contains(res.Content, "one hop up") || !strings.Contains(res.Content, "orc-security") {
		t.Fatalf("refusal %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "api", "x.go")); err == nil {
		t.Fatal("refused write happened")
	}
	// off: allowed
	b := build(mock.New(writeCall("1", "src/api/y.go", "y"), mock.Text("@S DONE\n@U colony c\n@E 0")))
	b.Territory.Enabled = false
	b.Hooks.Gate.Enabled = false
	e = w.Engine("slime-auth", b)
	if r, _ := e.Run(context.Background(), "x"); r.Steps[0].Status != "done" {
		t.Fatalf("territory off: %+v", r.Steps[0])
	}
	// rimuru has no territory to be held to
	e = w.Engine("", build(mock.New(writeCall("1", "notes.txt", "n"), mock.Text("ok"))))
	e.Hooks.EndGate = nil
	if r, _ := e.Run(context.Background(), "x"); r.Steps[0].Status != "done" || !strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
		t.Fatalf("rimuru: %+v %v", r.Steps[0], e.Tools.Names())
	}
}

func logText(t *testing.T, dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, ".isekai", "log.md"))
	return string(b)
}

func TestGateVerdicts(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	doc := ".isekai/slime/auth/README.md"
	// pass: the territory and its doc change together; the verdict lands in log.md
	m := mock.New(writeCall("1", "src/auth/token.go", "package auth"), mock.Call("2", "edit", map[string]string{"path": doc, "old": "one hour", "new": "fifteen minutes"}),
		mock.Text("@S DONE\n@F src/auth/token.go:1 ttl\n@U territory token TTL is 15m\n@E 0"))
	e := w.Engine("slime-auth", build(m))
	r, _ := e.Run(context.Background(), "@ASK draft +unsaid\nshorten the TTL")
	if r.Status != loop.Done || !strings.HasPrefix(r.Verdict, "pass") {
		t.Fatalf("pass expected: %s %v %v", r.Status, r.Verdict, r.Holes)
	}
	lg := logText(t, dir)
	if !strings.Contains(lg, "] slime-auth — gate pass — @ASK draft +unsaid") || !strings.Contains(lg, "- **Files:** src/auth/token.go, "+doc) || !strings.Contains(lg, "- **Gate:** pass") || !strings.Contains(lg, "@U territory token TTL is 15m") {
		t.Fatalf("log:\n%s", lg)
	}
	// the @U landed: an ontology fact and a shared note (Record sees the final report through Land)
	w.Land("slime-auth", r.Report)
	if b, _ := os.ReadFile(filepath.Join(dir, ".isekai/ontology/graph/unsaid.ttl")); !strings.Contains(string(b), "token TTL is 15m") {
		t.Error("unsaid fact not asserted")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".isekai/memory/shared/notes.jsonl")); !strings.Contains(string(b), `"kind":"territory"`) {
		t.Errorf("shared note not written: %s", b)
	}
	// Vitality fail: the territory changes, the doc does not
	m = mock.New(writeCall("1", "src/auth/other.go", "package auth"), mock.Text("@S DONE\n@U colony c\n@E 0"))
	e = w.Engine("slime-auth", build(m))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != loop.Fail || !strings.Contains(r.Verdict, "Doc truthful (Vitality): src/auth/other.go changed under slime-auth's territory but "+doc+" did not") {
		t.Fatalf("vitality: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	if lg = logText(t, dir); !strings.Contains(lg, "gate fail") || !strings.Contains(lg, "- **Result:** failed — gate") {
		t.Fatalf("fail not logged:\n%s", lg)
	}
	// right slime authored: slime-api writes under slime-auth's ground (territory off so the write lands)
	b := build(mock.New(writeCall("1", "src/auth/z.go", "z"), writeCall("2", doc, "# slime-auth\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n"), mock.Text("@S DONE\n@U colony c\n@E 0")))
	b.Territory.Enabled = false
	e = w.Engine("slime-api", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Right slime authored: src/auth/z.go belongs to slime-auth, written by slime-api") {
		t.Fatalf("author: %s %q", r.Status, r.Verdict)
	}
	// traits: a verify command fails
	os.WriteFile(filepath.Join(dir, "src/auth/BROKEN"), []byte("x"), 0o644)
	m = mock.New(writeCall("1", "src/auth/t.go", "t"), writeCall("2", doc, "# slime-auth\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n- **Verify:** `test ! -f src/auth/BROKEN`\n"), mock.Text("@S DONE\n@U colony c\n@E 0"))
	e = w.Engine("slime-auth", build(m))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Traits hold: slime-auth verify `test ! -f src/auth/BROKEN` exit 1") {
		t.Fatalf("traits: %s %q", r.Status, r.Verdict)
	}
	// traits: a body cannot pass its own gate by rewriting its check — the pre-turn line runs
	m = mock.New(writeCall("1", "src/auth/t2.go", "t"), writeCall("2", doc, "# slime-auth\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n- **Verify:** `true`\n"), mock.Text("@S DONE\n@U colony c\n@E 0"))
	e = w.Engine("slime-auth", build(m))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "slime-auth verify `test ! -f src/auth/BROKEN` exit 1") ||
		!strings.Contains(strings.Join(r.Holes, "|"), "was removed or changed this turn") {
		t.Fatalf("a rewritten verify line escaped the gate: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	os.Remove(filepath.Join(dir, "src/auth/BROKEN"))
	// a failed gate goes back to the model once: it fixes what the gate names, the turn passes
	m = mock.New(writeCall("1", "src/auth/r.go", "package auth"), mock.Text("@S DONE\n@U colony c\n@E 0"),
		writeCall("2", doc, "# slime-auth\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n- r.go added\n"), mock.Text("@S DONE doc updated\n@U colony c\n@E 0"))
	b0 := build(m)
	b0.Hooks.Gate.Retries = 1
	e = w.Engine("slime-auth", b0)
	r, _ = e.Run(context.Background(), "x")
	if r.Status != loop.Done || !strings.HasPrefix(r.Verdict, "pass") || !strings.Contains(strings.Join(r.Holes, "|"), "gate failed and was sent back (1 of 1)") {
		t.Fatalf("gate retry: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	// an injected check fails the turn
	b = build(mock.New(writeCall("1", "src/auth/u.go", "u"), writeCall("2", doc, "# slime-auth\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n"), mock.Text("@S DONE\n@U colony c\n@E 0")))
	b.Hooks.Gate.Checks = []Check{{Name: "gofmt", Command: "echo 'u.go not formatted'; exit 3"}}
	e = w.Engine("slime-auth", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "check gofmt: `echo 'u.go not formatted'; exit 3` exit 3 — u.go not formatted") {
		t.Fatalf("injected check: %s %q", r.Status, r.Verdict)
	}
	// duties: a commission answered off the wire
	b = build(mock.New(writeCall("1", "src/auth/v.go", "v"), writeCall("2", doc, "# slime-auth\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n"), mock.Text("plain prose")))
	e = w.Engine("slime-auth", b)
	if r, _ = e.Run(context.Background(), "@ASK findings\nlook"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Duties done: the commission asked on the wire") {
		t.Fatalf("duties: %s %q", r.Status, r.Verdict)
	}
	// gate off: a finding, not silence
	b = build(mock.New(writeCall("1", "src/auth/w.go", "w"), mock.Text("@S DONE\n@U colony c\n@E 0")))
	b.Hooks.Gate.Enabled = false
	e = w.Engine("slime-auth", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Done || r.Verdict != "off" || !strings.Contains(strings.Join(r.Holes, "|"), "law.gate is off") {
		t.Fatalf("gate off: %+v", r)
	}
	// a world with no orcs: n/a, and check 4 still runs
	os.RemoveAll(filepath.Join(dir, ".isekai/orc"))
	w2 := open(t, dir)
	v := w2.Gate(context.Background(), DefaultHooks().Gate, "slime-auth", []string{"src/auth/q.go"}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "fail" || !strings.Contains(v.String(), "Vitality") {
		t.Fatalf("no orcs, vitality: %+v", v)
	}
	v = w2.Gate(context.Background(), DefaultHooks().Gate, "slime-auth", []string{"src/auth/q.go", doc}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "Gate: n/a (no orcs)" || len(v.Reasons) != 0 {
		t.Fatalf("no orcs: %+v", v)
	}
}

func TestSystemPrompt(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	opt := DefaultPrompt()
	opt.Rules = []Rule{{Text: "never push", Scope: "all"}, {Text: "slimes stay in their zone", Scope: "rank:slime"}, {Text: "only auth", Scope: "creature:slime-auth"}, {Text: "orcs only", Scope: "rank:orc"}}
	e := w.Engine("slime-auth", build(mock.New()))
	p := w.Prompt("slime-auth", opt, e)
	for _, want := range []string{".isekai/isekai.md — The Crest", "1. **Vitality**", "You are slime-auth, a body working inside", "- never push", "- slimes stay in their zone", "- only auth",
		"You are slime-auth (slime: authors). Doc: .isekai/slime/auth/README.md. Territory: src/auth. One hop up: orc-security. Minds worn: ciel. Verify: test ! -f src/auth/BROKEN.",
		"@ONTO\nslime-auth ⇒truth orc-security", "Instructions (project, CLAUDE.md)", "@U <law|colony|territory>"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "orcs only") {
		t.Error("rank:orc rule reached a slime")
	}
	if strings.Contains(p, "## Principles") {
		t.Error("the code must not be in the prompt — only the crest")
	}
	// everything off but the crest
	p = w.Prompt("slime-auth", PromptOptions{Crest: true}, nil)
	if !strings.HasPrefix(p, ".isekai/isekai.md") || strings.Contains(p, "@ONTO") || strings.Contains(p, "Instructions") {
		t.Errorf("crest only:\n%s", p)
	}
	// the hook feeds the provider
	m := mock.New(mock.Text("ok"))
	e = w.Engine("orc-security", build(m))
	e.Run(context.Background(), "hi")
	if sys := m.Requests[0].System; !strings.Contains(sys, "1. **Vitality**") || !strings.Contains(sys, "You are orc-security (orc: holds the gate)") {
		t.Errorf("system via hook:\n%s", sys)
	}
	// rules through a rimuru session (no race)
	if !(Rule{Scope: "rank:rimuru"}).Applies("", "") || (Rule{Scope: "creature:x"}).Applies("y", "") || !(Rule{}).Applies("y", "") {
		t.Error("rule scopes")
	}
}

func TestRecallAndRecord(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	rc := w.Recall("slime-auth", "tokens expire after one hour — draft the answer", RecallOptions{Enabled: true, Memory: true, Toolbox: true})
	if len(rc.Anchors) == 0 || !rc.Rebuilt {
		t.Fatalf("recall anchors %+v", rc)
	}
	if !strings.Contains(strings.Join(rc.Anchors, " "), ".isekai/slime/auth/README.md#") {
		t.Errorf("anchor to the slime's doc section: %v", rc.Anchors)
	}
	if !strings.Contains(strings.Join(rc.Tools, "\n"), "@T mind ciel") {
		t.Errorf("toolbox brief: %v %v", rc.Tools, rc.Holes)
	}
	if rc := w.Recall("slime-auth", "x", RecallOptions{}); len(rc.Anchors)+len(rc.Tools) != 0 {
		t.Error("recall off")
	}
	// the hook rides the provider call
	m := mock.New(mock.Text("ok"))
	b := build(m)
	b.Hooks.Recall = RecallOptions{Enabled: true, Memory: true, Toolbox: true}
	e := w.Engine("slime-auth", b)
	e.Run(context.Background(), "tokens expire after one hour — draft the answer")
	if sys := m.Requests[0].SystemText(); !strings.Contains(sys, "@RECALL") || !strings.Contains(sys, "@TOOLS\n@T") {
		t.Errorf("recall block:\n%s", sys)
	}
	// Record: a dispatch step's report carries @U lines → they go home under the dispatched body
	st := &loop.StepRecord{ID: "s1", Tool: "dispatch", Input: json.RawMessage(`{"body":"slime-api","ask":"x"}`), Result: tool.Result{Output: "@S DONE\n@U colony the api tests need the fixture db\n@U team wire token in agent-one spelling\n@E 0"}}
	holes := w.Record("orc-security", st, RecordOptions{Enabled: true, Unsaid: true})
	if len(holes) != 0 {
		t.Fatalf("record holes %v", holes)
	}
	ttl, _ := os.ReadFile(filepath.Join(dir, ".isekai/ontology/graph/unsaid.ttl"))
	if !strings.Contains(string(ttl), "is:slime-api is:knows") || strings.Count(string(ttl), "is:Colony") != 2 {
		t.Errorf("facts:\n%s", ttl)
	}
	notes, _ := os.ReadFile(filepath.Join(dir, ".isekai/memory/shared/notes.jsonl"))
	if strings.Count(string(notes), `"kind":"colony"`) != 2 || !strings.Contains(string(notes), `"by":"slime-api"`) {
		t.Errorf("notes:\n%s", notes)
	}
	if holes := w.Record("x", st, RecordOptions{}); holes != nil {
		t.Error("record off")
	}
	// a shared note for landed files when asked
	st2 := &loop.StepRecord{ID: "s2", Tool: "write", Wrote: []string{"src/auth/a.go"}, Result: tool.Result{Output: "wrote"}}
	w.Record("slime-auth", st2, RecordOptions{Enabled: true, Landed: true})
	notes, _ = os.ReadFile(filepath.Join(dir, ".isekai/memory/shared/notes.jsonl"))
	if !strings.Contains(string(notes), `"tag":"landed"`) {
		t.Errorf("landed note:\n%s", notes)
	}
}

func TestDispatch(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	// the mock is shared by parent and Court, so the script interleaves: parent → child → parent
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"body": "slime-auth", "ask": "list the auth files", "scope": "src/auth"}),
		mock.Call("c1", "glob", map[string]string{"pattern": "src/auth/*"}),
		mock.Text("@S DONE\n@F src/auth/login.go:1 the only file\n@U territory login.go is the whole zone\n@E 0"),
		mock.Text("@S DONE\n@F court answered\n@U colony x\n@E 0"),
	)
	b := build(m)
	b.Budget.Steps = 5
	e := w.Engine("orc-security", b)
	r, err := e.Run(context.Background(), "@ASK findings\nwhat is in auth?")
	if err != nil || r.Status != loop.Done || len(r.Steps) != 1 || r.Steps[0].Status != "done" {
		t.Fatalf("%v %+v", err, r)
	}
	// the commission the Court received
	child := m.Requests[1]
	if !strings.Contains(child.System, "You are slime-auth (slime: authors)") || !strings.Contains(child.Messages[0].Text, "@ROOT "+dir) || !strings.Contains(child.Messages[0].Text, "@SCOPE src/auth") || !strings.Contains(child.Messages[0].Text, "@ASK findings +unsaid") || !strings.Contains(child.Messages[0].Text, "@CAP 2048") || !strings.HasSuffix(strings.TrimSpace(child.Messages[0].Text), "list the auth files") {
		t.Fatalf("commission:\n%s\n%s", child.System, child.Messages[0].Text)
	}
	names := []string{}
	for _, d := range child.Tools {
		names = append(names, d.Name)
	}
	if strings.Contains(strings.Join(names, ","), "dispatch") {
		t.Fatalf("a slime Court may not dispatch: %v", names)
	}
	// the report the dispatcher got
	res := m.Requests[3].Messages[2].ToolResults[0]
	if res.IsError || !strings.HasPrefix(res.Content, "@S DONE\n@U territory login.go is the whole zone\n@F src/auth/login.go:1") || !strings.HasSuffix(res.Content, "@E "+itoa(len(res.Content))) || strings.Contains(res.Content, "no @U") {
		t.Fatalf("report:\n%s", res.Content)
	}
	// the Court's spend joined the parent's; the Court had the parent's remaining steps
	if r.Usage.Output < 150 {
		t.Errorf("shared spend: %+v", r.Usage)
	}
	// a report with no @U is flagged
	m = mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"body": "slime-api", "ask": "x"}),
		mock.Text("@S DONE\n@F nothing\n@E 0"),
		mock.Text("@S DONE\n@U colony x\n@E 0"),
	)
	e = w.Engine("orc-security", build(m))
	e.Run(context.Background(), "x")
	res = m.Requests[2].Messages[2].ToolResults[0]
	if !strings.Contains(res.Content, "@? report carries no @U line") || !strings.Contains(res.Content, "@? dispatch slime-api: report carries no @U line") {
		t.Fatalf("missing @U not flagged:\n%s", res.Content)
	}
	// unknown body, and a parent whose step budget is spent
	m = mock.New(mock.Call("d1", "dispatch", map[string]interface{}{"body": "slime-ghost", "ask": "x"}), mock.Text("@S DONE\n@U colony x\n@E 0"))
	e = w.Engine("elf-core", build(m))
	e.Run(context.Background(), "x")
	if res := m.Requests[1].Messages[2].ToolResults[0]; !res.IsError || !strings.Contains(res.Content, `no body named "slime-ghost"`) {
		t.Fatalf("unknown body: %+v", res)
	}
	m = mock.New(mock.Call("d1", "dispatch", map[string]interface{}{"body": "slime-api", "ask": "x"}), mock.Text("@S DONE\n@U colony x\n@E 0"))
	b = build(m)
	b.Budget.Steps = 1
	e = w.Engine("elf-core", b)
	e.Run(context.Background(), "x")
	if res := m.Requests[1].Messages[2].ToolResults[0]; !res.IsError || !strings.Contains(res.Content, "parent step budget spent") {
		t.Fatalf("budget share: %+v", res)
	}
	// depth: a Court at MaxDepth 2 may dispatch once more, then no further
	b = build(mock.New())
	b.Court.MaxDepth = 2
	e = w.Engine("elf-core", b)
	if !strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
		t.Fatal("elf dispatches")
	}
	b.Depth = 2
	if e = w.Engine("elf-core", b); strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
		t.Fatal("depth limit")
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestMergeHooks(t *testing.T) {
	calls := []string{}
	a := loop.Hooks{Record: func(ctx context.Context, s *loop.Session, st *loop.StepRecord) []string {
		calls = append(calls, "a")
		return []string{"ha"}
	},
		EndGate: func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
			return "pass", nil, nil
		}}
	b := loop.Hooks{Record: func(ctx context.Context, s *loop.Session, st *loop.StepRecord) []string {
		calls = append(calls, "b")
		return []string{"hb"}
	},
		EndGate: func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
			return "checked", []string{"h"}, nil
		}}
	h := MergeHooks(a, b)
	if hs := h.Record(context.Background(), nil, nil); strings.Join(hs, ",") != "ha,hb" || strings.Join(calls, ",") != "a,b" {
		t.Error("record chain")
	}
	if v, hs, err := h.EndGate(context.Background(), nil, nil); v != "pass · checked" || len(hs) != 1 || err != nil {
		t.Error("gate chain")
	}
	if h.Drain != nil || h.System != nil {
		t.Error("unset stays unset")
	}
}

// The throne is not an office: the session's route carries no office (the mount is never routed
// through models.offices and the usage journal never labels rimuru's own calls), while a Court's
// route carries the office its ask names or its rank's default.
func TestSessionRouteHasNoOffice(t *testing.T) {
	w := open(t, fixture(t))
	if r, _ := w.Ranks.Get(Rimuru); r.Office != "" {
		t.Fatalf("rimuru's rank row carries an office: %q", r.Office)
	}
	var routes []Route
	b := build(mock.New(mock.Text("x")))
	b.Models = func(r Route) provider.Provider { routes = append(routes, r); return nil }
	w.Engine(Rimuru, b)
	w.Engine("slime-auth", b)
	if len(routes) != 2 || routes[0].Creature != Rimuru || routes[0].Rank != Rimuru || routes[0].Office != "" || routes[0].Task != "session" {
		t.Fatalf("session route: %+v", routes)
	}
	if routes[1].Office != "great-sage" || routes[1].Rank != "slime" {
		t.Fatalf("court route: %+v", routes[1])
	}
}

// A Court's writes are gated on its own account; its dispatcher's end gate leaves them alone —
// one verdict per landing, never a second gate under the dispatcher's name (binary.md §Ranks and
// bodies). The dispatcher is gated only for what it wrote itself, through whatever tool.
func TestCourtWritesGatedOnce(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	doc := ".isekai/slime/auth/README.md"
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"body": "slime-auth", "ask": "@ASK draft +unsaid\nshorten the TTL"}),
		writeCall("c1", "src/auth/token.go", "package auth"), mock.Call("c2", "edit", map[string]string{"path": doc, "old": "one hour", "new": "fifteen minutes"}),
		mock.Text("@S DONE\n@F src/auth/token.go:1 ttl\n@U territory token TTL is 15m\n@E 0"),
		mock.Text("@S DONE\n@F the court landed it\n@U colony x\n@E 0"),
	)
	e := w.Engine("orc-security", build(m))
	r, err := e.Run(context.Background(), "@ASK findings\nhave the TTL shortened")
	if err != nil || r.Status != loop.Done {
		t.Fatalf("%v %+v", err, r)
	}
	if r.Verdict != "" || len(r.Wrote) != 0 {
		t.Fatalf("the dispatcher was gated for its Court's writes: verdict %q wrote %v", r.Verdict, r.Wrote)
	}
	if got := strings.Join(r.Steps[0].Wrote, ","); got != "src/auth/token.go,"+doc {
		t.Fatalf("the dispatch step names the Court's writes: %q", got)
	}
	lg := logText(t, dir)
	if strings.Count(lg, "— gate ") != 1 || !strings.Contains(lg, "] slime-auth — gate pass") {
		t.Fatalf("one verdict, the Court's:\n%s", lg)
	}
	// the dispatcher's own shell write is still its own, and gated under its name
	m = mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"body": "slime-api", "ask": "look"}),
		mock.Text("@S DONE\n@U colony c\n@E 0"),
		mock.Call("b1", "bash", map[string]string{"command": "echo x > src/api/by-orc.go"}),
		mock.Text("@S DONE\n@U colony x\n@E 0"),
	)
	e = w.Engine("orc-security", build(m))
	r, _ = e.Run(context.Background(), "x")
	if !strings.HasPrefix(r.Verdict, "fail") || !strings.Contains(r.Verdict, "src/api/by-orc.go changed under slime-api's territory") || strings.Join(r.Wrote, ",") != "src/api/by-orc.go" {
		t.Fatalf("own shell write gated under the dispatcher's name: %q %v", r.Verdict, r.Wrote)
	}
}

// The log entry's title reads `<body> — gate pass — <ask>`; a world with no gate holder says
// `— Gate: n/a (no orcs) —`, never `gate Gate:`.
func TestLogTitleNoOrcs(t *testing.T) {
	dir := fixture(t)
	os.RemoveAll(filepath.Join(dir, ".isekai/orc"))
	w := open(t, dir)
	m := mock.New(writeCall("1", "notes.txt", "n"), mock.Text("@S DONE\n@U colony c\n@E 0"))
	e := w.Engine("elf-core", build(m))
	if r, _ := e.Run(context.Background(), "note it"); r.Status != loop.Done {
		t.Fatalf("%+v", r)
	}
	lg := logText(t, dir)
	if !strings.Contains(lg, "] elf-core — Gate: n/a (no orcs) — note it") || strings.Contains(lg, "gate Gate") {
		t.Fatalf("log:\n%s", lg)
	}
}
