package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
)

const policyText = `# AgentOne — the policy

Every member reads this file before working.

## Core principles

1. **Docs-as-code** — When anything changes, its doc changes in the same change — or it dies.
2. **Single responsibility** — Each rank does exactly its role.

## Principles

- Context flows down.

## Policies

1. Operator's word is policy.
`

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".agent-one/AGENT-ONE.md":              policyText,
		".agent-one/log.md":                    "# Chronicle\n\nAppend-only.\n\n---\n\n### [2026-09-20T17:54:00Z] orchestrator — Workspace onboarded\n- **Task:** /agent-one\n- **Files:** none\n- **Gate:** n/a\n- **Result:** done\n- **Learned:** —\n",
		".agent-one/coord/core/README.md":      "# coord-core\n\n- **Rank:** Coordinator\n- **Owns:** `src/`\n- **Purpose:** the shared skill\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Rank:** Domain owner\n- **Owns:** `src/auth/`, `src/api/`\n- **Reports to:** coord-core\n- **Verify:** `test -d src`\n",
		".agent-one/zone/auth/README.md":       "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Skills:** drafter\n- **Verify:** `test ! -f src/auth/BROKEN`\n\n## Invariants\n- tokens expire after one hour\n",
		".agent-one/zone/api/README.md":        "# zone-api\n\n- **Rank:** Zone worker\n- **Owns:** `src/api/`\n- **Domain owner:** domain-security\n\n## Verify\n- `true`\n",
		".claude/skills/drafter/SKILL.md":      "---\nname: drafter\ndescription: drafts the answer\n---\ndrafts\n",
		".claude/skills/tdd/SKILL.md":          "---\nname: tdd\ndescription: test first, red green refactor\n---\nshared host skill\n",
		"CLAUDE.md":                            "Read .agent-one/AGENT-ONE.md first.\n",
		"src/auth/login.go":                    "package auth\n",
		"src/api/server.go":                    "package api\n",
		"README.md":                            "hello\n",
	}
	for p, agent := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func open(t *testing.T, dir string) *Workspace {
	t.Helper()
	w, err := Open(dir, Default(), Options{Ontology: true, Instructions: InstructionOptions{Enabled: true, WalkUp: false}})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestLexicon(t *testing.T) {
	a := Default()
	if a.WorkspaceDir != ".agent-one" || a.Policy != "AGENT-ONE.md" || a.Ranks["zone"] != "zone" || a.Ranks["domain"] != "domain" || a.Prefixes["coord"] != "coord-" || a.Unsaid["team"] != "team" || a.Human != "Operator" || a.Principles != "Core principles" || a.GateNA != "Gate: n/a (no domain owners)" || a.Checks[0] != "Right zone worker authored" {
		t.Errorf("lexicon: %+v", a)
	}
	if k, ok := a.Canonical("team"); !ok || k != "team" {
		t.Error("team → team")
	}
	if k, ok := a.Canonical("policy"); !ok || k != "policy" {
		t.Error("canonical accepted on input")
	}
	if _, ok := a.Canonical("vibe"); ok {
		t.Error("unknown kind")
	}
	if a.Token("domain") != "domain" || a.Rank("zone-auth") != "zone" || a.Rank("nobody-x") != "" || a.Rank("domain-x") != "domain" {
		t.Error("tokens and ranks")
	}
	if l := a.Layout(); len(l.Ranks) != 5 || l.Ranks[2].Dir != "zone" || l.Ranks[2].Parent != "domain" || l.Policy != "AGENT-ONE.md" {
		t.Errorf("layout %+v", l)
	}
	if l := a.Loop(); l.Human != "Operator" || l.WorkspaceDir != ".agent-one" || l.Agent != "ephemeral-subagent" {
		t.Errorf("loop lexicon %+v", l)
	}
}

func TestDiscoverAndOpen(t *testing.T) {
	dir := fixture(t)
	root, lex, err := Discover(filepath.Join(dir, "src", "auth"))
	if err != nil || root != dir || lex.Vocab != "agent-one" {
		t.Fatalf("discover: %q %s %v", root, lex.Vocab, err)
	}
	w := open(t, dir)
	if w.Policy == nil || !strings.HasPrefix(w.Policy.Principles, "1. **Docs-as-code**") || len(w.Policy.Sections) != 4 {
		t.Fatalf("policy: %+v", w.Policy)
	}
	if s, ok := w.Policy.Section("policies"); !ok || !strings.Contains(s.Text, "Operator") {
		t.Error("section on demand")
	}
	names := []string{}
	for _, c := range w.Members {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "coord-core,domain-security,zone-api,zone-auth" {
		t.Fatalf("roster %v", names)
	}
	auth := w.Member("zone-auth")
	if auth.Rank != "zone" || auth.Parent != "domain-security" || auth.Doc != ".agent-one/zone/auth/README.md" || auth.Dir != ".agent-one/zone/auth" || strings.Join(auth.Ownership, ",") != "src/auth" || strings.Join(auth.Skills, ",") != "drafter" || strings.Join(auth.Verify, ",") != "test ! -f src/auth/BROKEN" {
		t.Errorf("zone-auth %+v", auth)
	}
	if api := w.Member("api"); api == nil || strings.Join(api.Verify, ",") != "true" || api.Parent != "domain-security" {
		t.Errorf("zone-api by short name %+v", api)
	}
	if w.Member("core") == nil || w.Member("nobody") != nil || !w.DomainOwners() {
		t.Error("lookup")
	}
	if o := w.Owner("src/auth/x.go"); o == nil || o.Name != "zone-auth" {
		t.Errorf("owner %+v", o)
	}
	if o := w.Owner("src/other.go"); o != nil {
		t.Errorf("coord ownership is not ownership: %+v", o)
	}
	if !auth.InOwnership("src/auth/deep/x") || auth.InOwnership("src/api/x") || !auth.InOwnership(".agent-one/zone/auth/README.md") {
		t.Error("ownership test")
	}
	if len(w.Instructions) != 1 || w.Instructions[0].Scope != "project" || !strings.Contains(w.Instructions[0].Text, "Read") {
		t.Errorf("instructions %+v", w.Instructions)
	}
	env := tool.Env{Root: dir}
	lt := w.PolicyTool()
	if r := lt.Run(context.Background(), env, json.RawMessage(`{}`)); r.Err || !strings.Contains(r.Output, "## Core principles") || !strings.Contains(r.Output, "## Policies") {
		t.Errorf("policy tool list %+v", r)
	}
	if r := lt.Run(context.Background(), env, json.RawMessage(`{"section":"principles"}`)); r.Err || !strings.Contains(r.Output, "Context flows down") {
		t.Errorf("policy tool section %+v", r)
	}
	if r := lt.Run(context.Background(), env, json.RawMessage(`{"section":"nope"}`)); !r.Err {
		t.Error("unknown section is an error")
	}
	// agent-one workspace, same engine
	az := t.TempDir()
	for p, agent := range map[string]string{
		".agent-one/AGENT-ONE.md":              "# Agent-Zero\n\n## Core principles\n\n1. **Docs-as-code** — docs change with the code.\n\n## Policies\n\n1. The operator decides.\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Owns:** `src/`\n",
		".agent-one/zone/auth/README.md":       "# zone-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n",
	} {
		full := filepath.Join(az, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(agent), 0o644)
	}
	root, lex, err = Discover(az)
	if err != nil || lex.Vocab != "agent-one" || root != az {
		t.Fatalf("agent-one discover: %v %s", err, lex.Vocab)
	}
	w2, err := Open(az, lex, Options{Ontology: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(w2.Policy.Principles, "1. **Docs-as-code**") {
		t.Errorf("agent-one principles: %q", w2.Policy.Principles)
	}
	if c := w2.Member("zone-auth"); c == nil || c.Rank != "zone" || c.Parent != "domain-security" || strings.Join(c.Ownership, ",") != "src/auth" {
		t.Errorf("agent-one member %+v", c)
	}
	if !w2.DomainOwners() || w2.Owner("src/auth/x").Name != "zone-auth" {
		t.Error("agent-one domain owners / owner")
	}
	if _, err := Open(t.TempDir(), Default(), Options{}); err == nil {
		t.Error("a directory without a workspace dir is not a workspace")
	}
}

func TestLogAppendOnly(t *testing.T) {
	dir := fixture(t)
	p := filepath.Join(dir, ".agent-one", "log.md")
	before, _ := os.ReadFile(p)
	l, err := OpenLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Entry{Author: "zone-auth", Title: "first", Task: "t", Files: []string{"a", "b"}, Gate: "pass", Learned: []string{"x", "y"}}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(after), string(before)) || !strings.Contains(string(after), "] zone-auth — first\n- **Task:** t\n- **Files:** a, b\n- **Gate:** pass\n- **Result:** done\n- **Learned:**\n  - x\n  - y\n") {
		t.Fatalf("append:\n%s", after)
	}
	if err := l.Append(Entry{Title: "second"}); err != nil {
		t.Fatal(err)
	}
	// the past is rewritten under the writer → refuse
	cur, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(cur), "Workspace onboarded", "Workspace rewritten", 1)), 0o644)
	if err := l.Append(Entry{Title: "third"}); err == nil || !strings.Contains(err.Error(), "rewritten") {
		t.Fatalf("rewrite not refused: %v", err)
	}
	os.WriteFile(p, cur[:len(cur)/2], 0o644)
	if err := l.Append(Entry{Title: "third"}); err == nil || !strings.Contains(err.Error(), "shrank") {
		t.Fatalf("truncation not refused: %v", err)
	}
	// a fresh log gets its header
	l2, _ := OpenLog(filepath.Join(dir, "new", "log.md"))
	if err := l2.Append(Entry{Title: "provisioned"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(l2.Path); !strings.HasPrefix(string(b), "# Chronicle") || !strings.Contains(string(b), "— provisioned") {
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
	return Build{Provider: m, Gate: noTTY(), Hooks: h, Ownership: OwnershipOptions{Enabled: true}, Subagent: SubagentOptions{Enabled: true, Unsaid: true}, Journal: "-"}
}

func writeCall(id, path, content string) provider.Response {
	return mock.Call(id, "write", map[string]string{"path": path, "content": content})
}

func TestOwnershipRefusal(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	m := mock.New(writeCall("1", "src/api/x.go", "x"), mock.Text("@S DONE\n@? src/api/x.go needed\n@U team c\n@E 0"))
	e := w.Engine("zone-auth", build(m))
	if names := strings.Join(e.Tools.Names(), ","); names != "read,write,edit,bash,glob,grep,policy" {
		t.Fatalf("a zone worker's shelf: %s", names)
	}
	r, err := e.Run(context.Background(), "x")
	if err != nil || r.Status != loop.Done || len(r.Steps) != 1 || r.Steps[0].Status != "refused" {
		t.Fatalf("%v %+v", err, r)
	}
	res := m.Requests[1].Messages[2].ToolResults[0]
	if !res.IsError || !strings.Contains(res.Content, "outside zone-auth's ownership") || !strings.Contains(res.Content, "one hop up") || !strings.Contains(res.Content, "domain-security") {
		t.Fatalf("refusal %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "api", "x.go")); err == nil {
		t.Fatal("refused write happened")
	}
	// off: allowed
	b := build(mock.New(writeCall("1", "src/api/y.go", "y"), mock.Text("@S DONE\n@U team c\n@E 0")))
	b.Ownership.Enabled = false
	b.Hooks.Gate.Enabled = false
	e = w.Engine("zone-auth", b)
	if r, _ := e.Run(context.Background(), "x"); r.Steps[0].Status != "done" {
		t.Fatalf("ownership off: %+v", r.Steps[0])
	}
	// orchestrator has no ownership to be held to
	e = w.Engine("", build(mock.New(writeCall("1", "notes.txt", "n"), mock.Text("ok"))))
	e.Hooks.EndGate = nil
	if r, _ := e.Run(context.Background(), "x"); r.Steps[0].Status != "done" || !strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
		t.Fatalf("orchestrator: %+v %v", r.Steps[0], e.Tools.Names())
	}
}

func logText(t *testing.T, dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, ".agent-one", "log.md"))
	return string(b)
}

func TestGateVerdicts(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	doc := ".agent-one/zone/auth/README.md"
	// pass: the ownership and its doc change together; the verdict lands in log.md
	m := mock.New(writeCall("1", "src/auth/token.go", "package auth"), mock.Call("2", "edit", map[string]string{"path": doc, "old": "one hour", "new": "fifteen minutes"}),
		mock.Text("@S DONE\n@F src/auth/token.go:1 ttl\n@U domain token TTL is 15m\n@E 0"))
	e := w.Engine("zone-auth", build(m))
	r, _ := e.Run(context.Background(), "@ASK draft +unsaid\nshorten the TTL")
	if r.Status != loop.Done || !strings.HasPrefix(r.Verdict, "pass") {
		t.Fatalf("pass expected: %s %v %v", r.Status, r.Verdict, r.Holes)
	}
	lg := logText(t, dir)
	if !strings.Contains(lg, "] zone-auth — gate pass — @ASK draft +unsaid") || !strings.Contains(lg, "- **Files:** src/auth/token.go, "+doc) || !strings.Contains(lg, "- **Gate:** pass") || !strings.Contains(lg, "@U domain token TTL is 15m") {
		t.Fatalf("log:\n%s", lg)
	}
	// the @U landed: an ontology fact and a shared note (Record sees the final report through Land)
	w.Land("zone-auth", r.Report)
	if b, _ := os.ReadFile(filepath.Join(dir, ".agent-one/ontology/graph/unsaid.ttl")); !strings.Contains(string(b), "token TTL is 15m") {
		t.Error("unsaid fact not asserted")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".agent-one/memory/shared/notes.jsonl")); !strings.Contains(string(b), `"kind":"domain"`) {
		t.Errorf("shared note not written: %s", b)
	}
	// Docs-as-code fail: the ownership changes, the doc does not
	m = mock.New(writeCall("1", "src/auth/other.go", "package auth"), mock.Text("@S DONE\n@U team c\n@E 0"))
	e = w.Engine("zone-auth", build(m))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != loop.Fail || !strings.Contains(r.Verdict, "Doc truthful (Docs-as-code): src/auth/other.go changed under zone-auth's ownership but "+doc+" did not") {
		t.Fatalf("docs-as-code: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	if lg = logText(t, dir); !strings.Contains(lg, "gate fail") || !strings.Contains(lg, "- **Result:** failed — gate") {
		t.Fatalf("fail not logged:\n%s", lg)
	}
	// right zone worker authored: zone-api writes under zone-auth's ground (ownership off so the write lands)
	b := build(mock.New(writeCall("1", "src/auth/z.go", "z"), writeCall("2", doc, "# zone-auth\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n"), mock.Text("@S DONE\n@U team c\n@E 0")))
	b.Ownership.Enabled = false
	e = w.Engine("zone-api", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Right zone worker authored: src/auth/z.go belongs to zone-auth, written by zone-api") {
		t.Fatalf("author: %s %q", r.Status, r.Verdict)
	}
	// invariants: a verify command fails
	os.WriteFile(filepath.Join(dir, "src/auth/BROKEN"), []byte("x"), 0o644)
	m = mock.New(writeCall("1", "src/auth/t.go", "t"), writeCall("2", doc, "# zone-auth\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Verify:** `test ! -f src/auth/BROKEN`\n"), mock.Text("@S DONE\n@U team c\n@E 0"))
	e = w.Engine("zone-auth", build(m))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Invariants hold: zone-auth verify `test ! -f src/auth/BROKEN` exit 1") {
		t.Fatalf("invariants: %s %q", r.Status, r.Verdict)
	}
	// invariants: an agent cannot pass its own gate by rewriting its check — the pre-turn line runs
	m = mock.New(writeCall("1", "src/auth/t2.go", "t"), writeCall("2", doc, "# zone-auth\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Verify:** `true`\n"), mock.Text("@S DONE\n@U team c\n@E 0"))
	e = w.Engine("zone-auth", build(m))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "zone-auth verify `test ! -f src/auth/BROKEN` exit 1") ||
		!strings.Contains(strings.Join(r.Holes, "|"), "was removed or changed this turn") {
		t.Fatalf("a rewritten verify line escaped the gate: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	os.Remove(filepath.Join(dir, "src/auth/BROKEN"))
	// a failed gate goes back to the model once: it fixes what the gate names, the turn passes
	m = mock.New(writeCall("1", "src/auth/r.go", "package auth"), mock.Text("@S DONE\n@U team c\n@E 0"),
		writeCall("2", doc, "# zone-auth\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- r.go added\n"), mock.Text("@S DONE doc updated\n@U team c\n@E 0"))
	b0 := build(m)
	b0.Hooks.Gate.Retries = 1
	e = w.Engine("zone-auth", b0)
	r, _ = e.Run(context.Background(), "x")
	if r.Status != loop.Done || !strings.HasPrefix(r.Verdict, "pass") || !strings.Contains(strings.Join(r.Holes, "|"), "gate failed and was sent back (1 of 1)") {
		t.Fatalf("gate retry: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	// an injected check fails the turn
	b = build(mock.New(writeCall("1", "src/auth/u.go", "u"), writeCall("2", doc, "# zone-auth\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n"), mock.Text("@S DONE\n@U team c\n@E 0")))
	b.Hooks.Gate.Checks = []Check{{Name: "gofmt", Command: "echo 'u.go not formatted'; exit 3"}}
	e = w.Engine("zone-auth", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "check gofmt: `echo 'u.go not formatted'; exit 3` exit 3 — u.go not formatted") {
		t.Fatalf("injected check: %s %q", r.Status, r.Verdict)
	}
	// duties: a commission answered off the wire
	b = build(mock.New(writeCall("1", "src/auth/v.go", "v"), writeCall("2", doc, "# zone-auth\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n"), mock.Text("plain prose")))
	e = w.Engine("zone-auth", b)
	if r, _ = e.Run(context.Background(), "@ASK findings\nlook"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Duties done: the commission asked on the wire") {
		t.Fatalf("duties: %s %q", r.Status, r.Verdict)
	}
	// gate off: a finding, not silence
	b = build(mock.New(writeCall("1", "src/auth/w.go", "w"), mock.Text("@S DONE\n@U team c\n@E 0")))
	b.Hooks.Gate.Enabled = false
	e = w.Engine("zone-auth", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Done || r.Verdict != "off" || !strings.Contains(strings.Join(r.Holes, "|"), "policy.gate is off") {
		t.Fatalf("gate off: %+v", r)
	}
	// a workspace with no domain owners: n/a, and check 4 still runs
	os.RemoveAll(filepath.Join(dir, ".agent-one/domain"))
	w2 := open(t, dir)
	v := w2.Gate(context.Background(), DefaultHooks().Gate, "zone-auth", []string{"src/auth/q.go"}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "fail" || !strings.Contains(v.String(), "Docs-as-code") {
		t.Fatalf("no domain owners, docs-as-code: %+v", v)
	}
	v = w2.Gate(context.Background(), DefaultHooks().Gate, "zone-auth", []string{"src/auth/q.go", doc}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "Gate: n/a (no domain owners)" || len(v.Reasons) != 0 {
		t.Fatalf("no domain owners: %+v", v)
	}
}

func TestSystemPrompt(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	opt := DefaultPrompt()
	opt.Rules = []Rule{{Text: "never push", Scope: "all"}, {Text: "zone workers stay in their zone", Scope: "rank:zone"}, {Text: "only auth", Scope: "member:zone-auth"}, {Text: "domain owners only", Scope: "rank:domain"}}
	e := w.Engine("zone-auth", build(mock.New()))
	p := w.Prompt("zone-auth", opt, e)
	for _, want := range []string{".agent-one/AGENT-ONE.md — Core principles", "1. **Docs-as-code**", "You are zone-auth, an agent working inside", "- never push", "- zone workers stay in their zone", "- only auth",
		"You are zone-auth (zone: authors). Doc: .agent-one/zone/auth/README.md. Owns: src/auth. One hop up: domain-security. Skills held: drafter. Verify: test ! -f src/auth/BROKEN.",
		"@ONTO\nzone-auth ⇒truth domain-security", "Instructions (project, CLAUDE.md)", "@U <policy|team|domain>"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "domain owners only") {
		t.Error("rank:domain rule reached a zone")
	}
	if strings.Contains(p, "## Principles") {
		t.Error("the code must not be in the prompt — only the principles")
	}
	// everything off but the principles
	p = w.Prompt("zone-auth", PromptOptions{Principles: true}, nil)
	if !strings.HasPrefix(p, ".agent-one/AGENT-ONE.md") || strings.Contains(p, "@ONTO") || strings.Contains(p, "Instructions") {
		t.Errorf("principles only:\n%s", p)
	}
	// the hook feeds the provider
	m := mock.New(mock.Text("ok"))
	e = w.Engine("domain-security", build(m))
	e.Run(context.Background(), "hi")
	if sys := m.Requests[0].System; !strings.Contains(sys, "1. **Docs-as-code**") || !strings.Contains(sys, "You are domain-security (domain: holds the gate)") {
		t.Errorf("system via hook:\n%s", sys)
	}
	// rules through a orchestrator session (no rank)
	if !(Rule{Scope: "rank:orchestrator"}).Applies("", "") || (Rule{Scope: "member:x"}).Applies("y", "") || !(Rule{}).Applies("y", "") {
		t.Error("rule scopes")
	}
}

func TestRecallAndRecord(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	rc := w.Recall("zone-auth", "tokens expire after one hour — draft the answer", RecallOptions{Enabled: true, Memory: true, Toolbox: true})
	if len(rc.Anchors) == 0 || !rc.Rebuilt {
		t.Fatalf("recall anchors %+v", rc)
	}
	if !strings.Contains(strings.Join(rc.Anchors, " "), ".agent-one/zone/auth/README.md#") {
		t.Errorf("anchor to the zone worker's doc section: %v", rc.Anchors)
	}
	if !strings.Contains(strings.Join(rc.Tools, "\n"), "@T skill drafter") {
		t.Errorf("toolbox brief: %v %v", rc.Tools, rc.Holes)
	}
	if rc := w.Recall("zone-auth", "x", RecallOptions{}); len(rc.Anchors)+len(rc.Tools) != 0 {
		t.Error("recall off")
	}
	// the hook rides the provider call
	m := mock.New(mock.Text("ok"))
	b := build(m)
	b.Hooks.Recall = RecallOptions{Enabled: true, Memory: true, Toolbox: true}
	e := w.Engine("zone-auth", b)
	e.Run(context.Background(), "tokens expire after one hour — draft the answer")
	if sys := m.Requests[0].SystemText(); !strings.Contains(sys, "@RECALL") || !strings.Contains(sys, "@TOOLS\n@T") {
		t.Errorf("recall block:\n%s", sys)
	}
	// Record: a dispatch step's report carries @U lines → they go home under the dispatched agent
	st := &loop.StepRecord{ID: "s1", Tool: "dispatch", Input: json.RawMessage(`{"agent":"zone-api","ask":"x"}`), Result: tool.Result{Output: "@S DONE\n@U team the api tests need the fixture db\n@U team wire token in agent-one spelling\n@E 0"}}
	holes := w.Record("domain-security", st, RecordOptions{Enabled: true, Unsaid: true})
	if len(holes) != 0 {
		t.Fatalf("record holes %v", holes)
	}
	ttl, _ := os.ReadFile(filepath.Join(dir, ".agent-one/ontology/graph/unsaid.ttl"))
	if !strings.Contains(string(ttl), "ao:zone-api ao:knows") || strings.Count(string(ttl), "ao:Team") != 2 {
		t.Errorf("facts:\n%s", ttl)
	}
	notes, _ := os.ReadFile(filepath.Join(dir, ".agent-one/memory/shared/notes.jsonl"))
	if strings.Count(string(notes), `"kind":"team"`) != 2 || !strings.Contains(string(notes), `"by":"zone-api"`) {
		t.Errorf("notes:\n%s", notes)
	}
	if holes := w.Record("x", st, RecordOptions{}); holes != nil {
		t.Error("record off")
	}
	// a shared note for landed files when asked
	st2 := &loop.StepRecord{ID: "s2", Tool: "write", Wrote: []string{"src/auth/a.go"}, Result: tool.Result{Output: "wrote"}}
	w.Record("zone-auth", st2, RecordOptions{Enabled: true, Landed: true})
	notes, _ = os.ReadFile(filepath.Join(dir, ".agent-one/memory/shared/notes.jsonl"))
	if !strings.Contains(string(notes), `"tag":"landed"`) {
		t.Errorf("landed note:\n%s", notes)
	}
}

func TestDispatch(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	// the mock is shared by parent and Subagent, so the script interleaves: parent → child → parent
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "list the auth files", "scope": "src/auth"}),
		mock.Call("c1", "glob", map[string]string{"pattern": "src/auth/*"}),
		mock.Text("@S DONE\n@F src/auth/login.go:1 the only file\n@U domain login.go is the whole zone\n@E 0"),
		mock.Text("@S DONE\n@F subagent answered\n@U team x\n@E 0"),
	)
	b := build(m)
	b.Budget.Steps = 5
	e := w.Engine("domain-security", b)
	r, err := e.Run(context.Background(), "@ASK findings\nwhat is in auth?")
	if err != nil || r.Status != loop.Done || len(r.Steps) != 1 || r.Steps[0].Status != "done" {
		t.Fatalf("%v %+v", err, r)
	}
	// the commission the Subagent received
	child := m.Requests[1]
	if !strings.Contains(child.System, "You are zone-auth (zone: authors)") || !strings.Contains(child.Messages[0].Text, "@ROOT "+dir) || !strings.Contains(child.Messages[0].Text, "@SCOPE src/auth") || !strings.Contains(child.Messages[0].Text, "@ASK findings +unsaid") || !strings.Contains(child.Messages[0].Text, "@CAP 2048") || !strings.HasSuffix(strings.TrimSpace(child.Messages[0].Text), "list the auth files") {
		t.Fatalf("commission:\n%s\n%s", child.System, child.Messages[0].Text)
	}
	names := []string{}
	for _, d := range child.Tools {
		names = append(names, d.Name)
	}
	if strings.Contains(strings.Join(names, ","), "dispatch") {
		t.Fatalf("a zone worker Subagent may not dispatch: %v", names)
	}
	// the report the dispatcher got
	res := m.Requests[3].Messages[2].ToolResults[0]
	if res.IsError || !strings.HasPrefix(res.Content, "@S DONE\n@U domain login.go is the whole zone\n@F src/auth/login.go:1") || !strings.HasSuffix(res.Content, "@E "+itoa(len(res.Content))) || strings.Contains(res.Content, "no @U") {
		t.Fatalf("report:\n%s", res.Content)
	}
	// the Subagent's spend joined the parent's; the Subagent had the parent's remaining steps
	if r.Usage.Output < 150 {
		t.Errorf("shared spend: %+v", r.Usage)
	}
	// a report with no @U is flagged
	m = mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-api", "ask": "x"}),
		mock.Text("@S DONE\n@F nothing\n@E 0"),
		mock.Text("@S DONE\n@U team x\n@E 0"),
	)
	e = w.Engine("domain-security", build(m))
	e.Run(context.Background(), "x")
	res = m.Requests[2].Messages[2].ToolResults[0]
	if !strings.Contains(res.Content, "@? report carries no @U line") || !strings.Contains(res.Content, "@? dispatch zone-api: report carries no @U line") {
		t.Fatalf("missing @U not flagged:\n%s", res.Content)
	}
	// unknown agent, and a parent whose step budget is spent
	m = mock.New(mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-ghost", "ask": "x"}), mock.Text("@S DONE\n@U team x\n@E 0"))
	e = w.Engine("coord-core", build(m))
	e.Run(context.Background(), "x")
	if res := m.Requests[1].Messages[2].ToolResults[0]; !res.IsError || !strings.Contains(res.Content, `no agent named "zone-ghost"`) {
		t.Fatalf("unknown agent: %+v", res)
	}
	m = mock.New(mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-api", "ask": "x"}), mock.Text("@S DONE\n@U team x\n@E 0"))
	b = build(m)
	b.Budget.Steps = 1
	e = w.Engine("coord-core", b)
	e.Run(context.Background(), "x")
	if res := m.Requests[1].Messages[2].ToolResults[0]; !res.IsError || !strings.Contains(res.Content, "parent step budget spent") {
		t.Fatalf("budget share: %+v", res)
	}
	// depth: a Subagent at MaxDepth 2 may dispatch once more, then no further
	b = build(mock.New())
	b.Subagent.MaxDepth = 2
	e = w.Engine("coord-core", b)
	if !strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
		t.Fatal("coord dispatches")
	}
	b.Depth = 2
	if e = w.Engine("coord-core", b); strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
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

// The orchestrator is not an role: the session's route carries no role (the mount is never routed
// through models.roles and the usage journal never labels orchestrator's own calls), while a Subagent's
// route carries the role its ask names or its rank's default.
func TestSessionRouteHasNoRole(t *testing.T) {
	w := open(t, fixture(t))
	if r, _ := w.Ranks.Get(Orchestrator); r.Role != "" {
		t.Fatalf("orchestrator's rank row carries an role: %q", r.Role)
	}
	var routes []Route
	b := build(mock.New(mock.Text("x")))
	b.Models = func(r Route) provider.Provider { routes = append(routes, r); return nil }
	w.Engine(Orchestrator, b)
	w.Engine("zone-auth", b)
	if len(routes) != 2 || routes[0].Member != Orchestrator || routes[0].Rank != Orchestrator || routes[0].Role != "" || routes[0].Task != "session" {
		t.Fatalf("session route: %+v", routes)
	}
	if routes[1].Role != "analyst" || routes[1].Rank != "zone" {
		t.Fatalf("subagent route: %+v", routes[1])
	}
}

// A Subagent's writes are gated on its own account; its dispatcher's end gate leaves them alone —
// one verdict per landing, never a second gate under the dispatcher's name (binary.md §Ranks and
// agents). The dispatcher is gated only for what it wrote itself, through whatever tool.
func TestSubagentWritesGatedOnce(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	doc := ".agent-one/zone/auth/README.md"
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "@ASK draft +unsaid\nshorten the TTL"}),
		writeCall("c1", "src/auth/token.go", "package auth"), mock.Call("c2", "edit", map[string]string{"path": doc, "old": "one hour", "new": "fifteen minutes"}),
		mock.Text("@S DONE\n@F src/auth/token.go:1 ttl\n@U domain token TTL is 15m\n@E 0"),
		mock.Text("@S DONE\n@F the subagent landed it\n@U team x\n@E 0"),
	)
	e := w.Engine("domain-security", build(m))
	r, err := e.Run(context.Background(), "@ASK findings\nhave the TTL shortened")
	if err != nil || r.Status != loop.Done {
		t.Fatalf("%v %+v", err, r)
	}
	if r.Verdict != "" || len(r.Wrote) != 0 {
		t.Fatalf("the dispatcher was gated for its Subagent's writes: verdict %q wrote %v", r.Verdict, r.Wrote)
	}
	if got := strings.Join(r.Steps[0].Wrote, ","); got != "src/auth/token.go,"+doc {
		t.Fatalf("the dispatch step names the Subagent's writes: %q", got)
	}
	lg := logText(t, dir)
	if strings.Count(lg, "— gate ") != 1 || !strings.Contains(lg, "] zone-auth — gate pass") {
		t.Fatalf("one verdict, the Subagent's:\n%s", lg)
	}
	// the dispatcher's own shell write is still its own, and gated under its name
	m = mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-api", "ask": "look"}),
		mock.Text("@S DONE\n@U team c\n@E 0"),
		mock.Call("b1", "bash", map[string]string{"command": "echo x > src/api/by-domain.go"}),
		mock.Text("@S DONE\n@U team x\n@E 0"),
	)
	e = w.Engine("domain-security", build(m))
	r, _ = e.Run(context.Background(), "x")
	if !strings.HasPrefix(r.Verdict, "fail") || !strings.Contains(r.Verdict, "src/api/by-domain.go changed under zone-api's ownership") || strings.Join(r.Wrote, ",") != "src/api/by-domain.go" {
		t.Fatalf("own shell write gated under the dispatcher's name: %q %v", r.Verdict, r.Wrote)
	}
}

// The log entry's title reads `<body> — gate pass — <ask>`; a workspace with no gate holder says
// `— Gate: n/a (no domain owners) —`, never `gate Gate:`.
func TestLogTitleNoDomainOwners(t *testing.T) {
	dir := fixture(t)
	os.RemoveAll(filepath.Join(dir, ".agent-one/domain"))
	w := open(t, dir)
	m := mock.New(writeCall("1", "notes.txt", "n"), mock.Text("@S DONE\n@U team c\n@E 0"))
	e := w.Engine("coord-core", build(m))
	if r, _ := e.Run(context.Background(), "note it"); r.Status != loop.Done {
		t.Fatalf("%+v", r)
	}
	lg := logText(t, dir)
	if !strings.Contains(lg, "] coord-core — Gate: n/a (no domain owners) — note it") || strings.Contains(lg, "gate Gate") {
		t.Fatalf("log:\n%s", lg)
	}
}

// A turn may not pass by making the tests easier: fewer tests or more skips fail the gate; more
// tests pass.
func TestGateTestsIntact(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	doc := ".agent-one/zone/auth/README.md"
	docBody := "# zone-auth\n- **Territory:** `src/auth/`\n- **Reports to:** domain-security\n"
	two := "package auth\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n\nfunc TestB(t *testing.T) {}\n"
	os.WriteFile(filepath.Join(dir, "src/auth/token_test.go"), []byte(two), 0o644)
	run := func(test string) *loop.Result {
		m := mock.New(writeCall("1", "src/auth/token_test.go", test), writeCall("2", doc, docBody+"- "+test[len(test)-12:]+"\n"), mock.Text("@S DONE\n@U team c\n@E 0"))
		r, _ := w.Engine("zone-auth", build(m)).Run(context.Background(), "x")
		os.WriteFile(filepath.Join(dir, "src/auth/token_test.go"), []byte(two), 0o644)
		return r
	}
	if r := run("package auth\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "src/auth/token_test.go lost 1 test (2 → 1)") {
		t.Fatalf("a deleted test passed the gate: %s %q", r.Status, r.Verdict)
	}
	if r := run("package auth\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) { t.Skip(\"later\") }\n\nfunc TestB(t *testing.T) {}\n"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "gained 1 skip/only marker") {
		t.Fatalf("a skipped test passed the gate: %s %q", r.Status, r.Verdict)
	}
	if r := run(two + "\nfunc TestC(t *testing.T) {}\n"); r.Status != loop.Done || !strings.HasPrefix(r.Verdict, "pass") {
		t.Fatalf("a new test failed the gate: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
}

func TestTestsIntactLanguages(t *testing.T) {
	cases := []struct {
		lang, text   string
		tests, skips int
	}{
		{"js", "describe('a', () => {\n  it('x', () => {})\n  test('y', () => {})\n  it.skip('z', () => {})\n  xit('w', () => {})\n  it.only('v', () => {})\n})", 2, 3},
		{"py", "import pytest\n\ndef test_a():\n    pass\n\n@pytest.mark.skip\ndef test_b():\n    pass\n\nasync def test_c():\n    pytest.skip('no')\n", 3, 2},
		{"go", "func TestA(t *testing.T) { t.Skipf(\"x\") }\nfunc BenchmarkB(b *testing.B) {}\nfunc helper() {}\n", 2, 1},
	}
	for _, c := range cases {
		if n := len(testDecl[c.lang].FindAllString(c.text, -1)); n != c.tests {
			t.Errorf("%s: %d tests, want %d", c.lang, n, c.tests)
		}
		if n := len(testSkip[c.lang].FindAllString(c.text, -1)); n != c.skips {
			t.Errorf("%s: %d skips, want %d", c.lang, n, c.skips)
		}
	}
}
