package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
)

func TestRankTable(t *testing.T) {
	rs := Ranks(DefaultRanks(Default()))
	if err := rs.Validate(); err != nil {
		t.Fatal(err)
	}
	if r, ok := rs.Get("zone"); !ok || !r.Authors || r.HoldsGate || r.ReportsTo != "domain" || r.Dir != "zone" || r.Prefix != "zone-" {
		t.Errorf("zone row %+v", r)
	}
	if r, ok := rs.Get("domain"); !ok || r.Authors || !r.HoldsGate || !r.Sideways {
		t.Errorf("domain row %+v", r)
	}
	if r, _ := rs.Get(Orchestrator); len(r.Tools) != 1 || r.Tools[0] != "*" {
		t.Error("orchestrator holds everything")
	}
	if rs.Of("zone-x") != "zone" || rs.Of("auditor-y") != "auditor" || rs.Of("orchestrator") != "" || rs.Of("coord-z") != "coord" {
		t.Error("Of")
	}
	if !rs.Below("zone", "coord") || !rs.Below("zone", "domain") || rs.Below("coord", "domain") || !rs.Below("domain", Orchestrator) || rs.Below("domain", "domain") {
		t.Error("Below")
	}
	if !rs.GateAbove("zone") || rs.GateAbove("coord") || !rs.GateAbove("service") {
		t.Error("GateAbove")
	}
	if strings.Join(rs.Tools("zone"), ",") != "read,write,edit,bash,glob,grep,policy" || strings.Join(rs.Tools("principal_domain_owner"), ",") != strings.Join(AllTools, ",") || strings.Join(rs.Tools("nobody"), ",") != strings.Join(AllTools, ",") {
		t.Error("Tools")
	}
	az := Ranks(DefaultRanks(Default()))
	if r, _ := az.Get("zone"); r.Dir != "zone" || r.Prefix != "zone-" || az.Of("domain-x") != "domain" {
		t.Errorf("agent-one dirs %+v", r)
	}
	bad := []Ranks{
		{{Name: "a", ReportsTo: "b"}},                                                // unknown parent
		{{Name: "a", ReportsTo: "b"}, {Name: "b", ReportsTo: "a"}},                   // a cycle never reaches orchestrator
		{{Name: "a", ReportsTo: Orchestrator, Authors: true}},                        // authors with no gate above
		{{Name: "a", ReportsTo: Orchestrator}, {Name: "a", ReportsTo: Orchestrator}}, // twice
		{{Name: "a", ReportsTo: Orchestrator, Base: "zz"}},                           // unknown base
		{{Name: "a", ReportsTo: Orchestrator, Dir: "a", Prefix: "b-"}},               // prefix ≠ dir-
		{{Name: Orchestrator, ReportsTo: Orchestrator}},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("bad table %d accepted", i)
		}
	}
	if RoleOf("@ASK findings +unsaid") != "analyst" || RoleOf("verdict on x") != "judge" || RoleOf("draft") != "drafter" || RoleOf("look around") != "analyst" {
		t.Error("RoleOf")
	}
}

// customRanks is a hierarchy with zero built-in names: a coordinator, a gate holder under it,
// two authoring ranks under that, and an auditor beside — config's `rankSet: replace`.
func customRanks() Ranks {
	return Ranks{
		{Name: "lead", ReportsTo: Orchestrator, Job: "coordinates", Agent: "ephemeral", Role: "drafter", Tools: AllTools, Dir: "lead", Prefix: "lead-"},
		{Name: "persistent", ReportsTo: "lead", Job: "holds the gate", HoldsGate: true, Agent: "ephemeral", Role: "judge", Tools: AllTools, Dir: "persistent", Prefix: "persistent-"},
		{Name: "coder", ReportsTo: "persistent", Job: "authors code", Authors: true, Agent: "ephemeral", Role: "analyst", Tools: baseTools, Dir: "coder", Prefix: "coder-"},
		{Name: "writer", ReportsTo: "persistent", Job: "authors docs", Authors: true, Agent: "ephemeral", Role: "drafter", Tools: baseTools, Dir: "writer", Prefix: "writer-"},
		{Name: "auditor", ReportsTo: Orchestrator, Job: "reads verdicts", Agent: "persistent", Role: "judge", Tools: []string{"read", "grep", "glob", "policy"}, Dir: "auditor", Prefix: "auditor-"},
	}
}

func customWorkspace(t *testing.T) (string, *Workspace) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".agent-one/AGENT-ONE.md":              policyText,
		".agent-one/lead/main/README.md":       "# lead-main\n\n- **Owns:** `src/`, `docs/`\n",
		".agent-one/persistent/core/README.md": "# persistent-core\n\n- **Owns:** `src/`, `docs/`\n- **Reports to:** lead-main\n- **Verify:** `test -d src`\n",
		".agent-one/coder/auth/README.md":      "# coder-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** persistent-core\n- **Verify:** `test ! -f src/auth/BROKEN`\n",
		".agent-one/writer/docs/README.md":     "# writer-docs\n\n- **Owns:** `docs/`\n- **Reports to:** persistent-core\n",
		".agent-one/auditor/eye/README.md":     "# auditor-eye\n\n- **Owns:** `.agent-one/log.md`\n",
		"src/auth/login.go":                    "package auth\n",
		"docs/index.md":                        "# docs\n",
	}
	for p, agent := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(agent), 0o644)
	}
	w, err := Open(dir, Default(), Options{Ontology: true, Ranks: customRanks()})
	if err != nil {
		t.Fatal(err)
	}
	return dir, w
}

func TestCustomRanksReplace(t *testing.T) {
	dir, w := customWorkspace(t)
	names := []string{}
	for _, c := range w.Members {
		names = append(names, c.Name+":"+c.Rank+"→"+c.Parent)
	}
	if got := strings.Join(names, " "); got != "auditor-eye:auditor→orchestrator coder-auth:coder→persistent-core lead-main:lead→orchestrator persistent-core:persistent→lead-main writer-docs:writer→persistent-core" {
		t.Fatalf("roster: %s", got)
	}
	if !w.GateHolders() || w.Owner("src/auth/x.go").Name != "coder-auth" || w.Owner("docs/a.md").Name != "writer-docs" || w.Owner("src/other.go").Name != "persistent-core" {
		t.Error("owners by Authors / HoldsGate, no rank names in logic")
	}
	if w.RankOf("coder-auth").Name != "coder" || w.RankOf("nobody").Name != Orchestrator || w.RankOf("").Name != Orchestrator {
		t.Error("RankOf")
	}
	// the roster's shelves come from the table
	if e := w.Engine("auditor-eye", build(mock.New())); strings.Join(e.Tools.Names(), ",") != "read,grep,glob,policy" {
		t.Errorf("auditor shelf %v", e.Tools.Names())
	}
	if e := w.Engine("coder-auth", build(mock.New())); strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
		t.Error("an authoring rank does not dispatch")
	}
	// ownership: an authoring rank is held to its ground; the refusal points one hop up
	m := mock.New(writeCall("1", "docs/new.md", "x"), mock.Text("@S DONE\n@? docs/new.md is writer-docs' ground\n@U team c\n@E 0"))
	e := w.Engine("coder-auth", build(m))
	r, _ := e.Run(context.Background(), "x")
	if r.Steps[0].Status != "refused" || !strings.Contains(m.Requests[1].Messages[2].ToolResults[0].Content, "outside coder-auth's ownership") || !strings.Contains(m.Requests[1].Messages[2].ToolResults[0].Content, "to persistent-core") {
		t.Fatalf("ownership: %+v %s", r.Steps[0], m.Requests[1].Messages[2].ToolResults[0].Content)
	}
	// the gate: Docs-as-code fails without the doc, passes with it, and the verdict names the table's ranks only through the members
	doc := ".agent-one/coder/auth/README.md"
	e = w.Engine("coder-auth", build(mock.New(writeCall("1", "src/auth/token.go", "t"), mock.Text("@S DONE\n@U team c\n@E 0"))))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Doc truthful (Docs-as-code): src/auth/token.go changed under coder-auth's ownership but "+doc+" did not") {
		t.Fatalf("docs-as-code: %s %q", r.Status, r.Verdict)
	}
	e = w.Engine("coder-auth", build(mock.New(writeCall("1", "src/auth/token.go", "t"), writeCall("2", doc, "# coder-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** persistent-core\n"), mock.Text("@S DONE\n@U team c\n@E 0"))))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Done || !strings.HasPrefix(r.Verdict, "pass") {
		t.Fatalf("pass: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	if lg := logText(t, dir); !strings.Contains(lg, "] coder-auth — gate pass") {
		t.Fatalf("log:\n%s", lg)
	}
	// right author: the other authoring rank writes on coder ground (ownership off so it lands)
	b := build(mock.New(writeCall("1", "src/auth/z.go", "z"), writeCall("2", doc, "# coder-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** persistent-core\n"), mock.Text("@S DONE\n@U team c\n@E 0")))
	b.Ownership.Enabled = false
	e = w.Engine("writer-docs", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Right zone worker authored: src/auth/z.go belongs to coder-auth, written by writer-docs") {
		t.Fatalf("author: %s %q", r.Status, r.Verdict)
	}
	// no gate holder on the roster: n/a wording is generic, check 4 still runs
	os.RemoveAll(filepath.Join(dir, ".agent-one/persistent"))
	w2, _ := Open(dir, Default(), Options{Ontology: true, Ranks: customRanks()})
	if v := w2.Gate(context.Background(), DefaultHooks().Gate, "coder-auth", []string{"src/auth/q.go", doc}, nil, true, "x"); v.Word != "Gate: n/a (no gate holder)" {
		t.Fatalf("n/a: %+v", v)
	}
	// dispatch: the coordinator dispatches down; the role rides the ask; an upward dispatch is refused
	_, w = customWorkspace(t)
	var routes []Route
	m = mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "coder-auth", "ask": "judge the token code", "role": "judge"}),
		mock.Text("@S DONE\n@V token.go:1 sound\n@U domain tokens are opaque\n@E 0"),
		mock.Text("@S DONE\n@U team x\n@E 0"),
	)
	b = build(m)
	b.Models = func(r Route) provider.Provider { routes = append(routes, r); return nil }
	e = w.Engine("lead-main", b)
	if r, _ = e.Run(context.Background(), "@ASK draft\nrun the review"); r.Status != loop.Done || r.Steps[0].Status != "done" {
		t.Fatalf("dispatch: %+v", r)
	}
	if c := m.Requests[1].Messages[0].Text; !strings.Contains(c, "@ASK verdict +unsaid") {
		t.Fatalf("the role rides the commission:\n%s", c)
	}
	if len(routes) != 2 || routes[0].Rank != "lead" || routes[0].Role != "drafter" || routes[0].Task != "session" || routes[1] != (Route{Role: "judge", Rank: "coder", Member: "coder-auth", Task: "dispatch"}) {
		t.Fatalf("routes %+v", routes)
	}
	m = mock.New(mock.Call("d1", "dispatch", map[string]interface{}{"agent": "lead-main", "ask": "x"}), mock.Text("@S DONE\n@U team x\n@E 0"))
	e = w.Engine("persistent-core", build(m))
	e.Run(context.Background(), "x")
	if res := m.Requests[1].Messages[2].ToolResults[0]; !res.IsError || !strings.Contains(res.Content, "may not dispatch lead-main") {
		t.Fatalf("upward dispatch: %+v", res)
	}
	// a table that breaks the policy's shape is refused at open
	broken := customRanks()
	broken[1].HoldsGate = false
	if _, err := Open(dir, Default(), Options{Ranks: broken}); err == nil || !strings.Contains(err.Error(), "Policy 3") {
		t.Errorf("broken table accepted: %v", err)
	}
}

func TestHomes(t *testing.T) {
	dir, w := customWorkspace(t)
	h := w.Homes()
	if h.Root != dir || h.WorkspaceDir != ".agent-one" || !strings.HasPrefix(h.Principles(), "1. **Docs-as-code**") {
		t.Fatalf("homes %+v", h)
	}
	if err := h.Assert("coder-auth", "domain", "argon2 everywhere"); err != nil {
		t.Fatal(err)
	}
	if err := h.Remember("coder-auth", "domain", "argon2 everywhere"); err != nil {
		t.Fatal(err)
	}
	if err := h.Episode("coder-auth", []string{"src/auth/x.go"}, []string{"2026-09-26 goal: x"}); err != nil {
		t.Fatal(err)
	}
	if lg := logText(t, dir); !strings.Contains(lg, "] coder-auth — drained: changes landed mid-session") || !strings.Contains(lg, "- **Files:** src/auth/x.go") {
		t.Fatalf("episode:\n%s", lg)
	}
	if w.ScopeOf("coder-auth") != "coder" {
		t.Error("ScopeOf")
	}
}
