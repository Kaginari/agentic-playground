package world

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/provider/mock"
)

func TestRankTable(t *testing.T) {
	rs := Ranks(DefaultRanks(Isekai()))
	if err := rs.Validate(); err != nil {
		t.Fatal(err)
	}
	if r, ok := rs.Get("slime"); !ok || !r.Authors || r.HoldsGate || r.ReportsTo != "orc" || r.Dir != "slime" || r.Prefix != "slime-" {
		t.Errorf("slime row %+v", r)
	}
	if r, ok := rs.Get("orc"); !ok || r.Authors || !r.HoldsGate || !r.Sideways {
		t.Errorf("orc row %+v", r)
	}
	if r, _ := rs.Get(Rimuru); len(r.Tools) != 1 || r.Tools[0] != "*" {
		t.Error("rimuru holds everything")
	}
	if rs.Of("slime-x") != "slime" || rs.Of("dark-elf-y") != "dark_elf" || rs.Of("rimuru") != "" || rs.Of("elf-z") != "elf" {
		t.Error("Of")
	}
	if !rs.Below("slime", "elf") || !rs.Below("slime", "orc") || rs.Below("elf", "orc") || !rs.Below("orc", Rimuru) || rs.Below("orc", "orc") {
		t.Error("Below")
	}
	if !rs.GateAbove("slime") || rs.GateAbove("elf") || !rs.GateAbove("kijin") {
		t.Error("GateAbove")
	}
	if strings.Join(rs.Tools("slime"), ",") != "read,write,edit,bash,glob,grep,law" || strings.Join(rs.Tools("high_orc"), ",") != strings.Join(AllTools, ",") || strings.Join(rs.Tools("nobody"), ",") != strings.Join(AllTools, ",") {
		t.Error("Tools")
	}
	az := Ranks(DefaultRanks(AgentOne()))
	if r, _ := az.Get("slime"); r.Dir != "zone" || r.Prefix != "zone-" || az.Of("domain-x") != "orc" {
		t.Errorf("agent-one dirs %+v", r)
	}
	bad := []Ranks{
		{{Name: "a", ReportsTo: "b"}},                                    // unknown parent
		{{Name: "a", ReportsTo: "b"}, {Name: "b", ReportsTo: "a"}},       // a cycle never reaches rimuru
		{{Name: "a", ReportsTo: Rimuru, Authors: true}},                  // authors with no gate above
		{{Name: "a", ReportsTo: Rimuru}, {Name: "a", ReportsTo: Rimuru}}, // twice
		{{Name: "a", ReportsTo: Rimuru, Base: "zz"}},                     // unknown base
		{{Name: "a", ReportsTo: Rimuru, Dir: "a", Prefix: "b-"}},         // prefix ≠ dir-
		{{Name: Rimuru, ReportsTo: Rimuru}},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("bad table %d accepted", i)
		}
	}
	if OfficeOf("@ASK findings +unsaid") != "great-sage" || OfficeOf("verdict on x") != "raphael" || OfficeOf("draft") != "ciel" || OfficeOf("look around") != "great-sage" {
		t.Error("OfficeOf")
	}
}

// customRanks is a hierarchy with zero built-in names: a coordinator, a gate holder under it,
// two authoring ranks under that, and an auditor beside — config's `rankSet: replace`.
func customRanks() Ranks {
	return Ranks{
		{Name: "lead", ReportsTo: Rimuru, Job: "coordinates", Body: "court", Office: "ciel", Tools: AllTools, Dir: "lead", Prefix: "lead-"},
		{Name: "keeper", ReportsTo: "lead", Job: "holds the gate", HoldsGate: true, Body: "court", Office: "raphael", Tools: AllTools, Dir: "keeper", Prefix: "keeper-"},
		{Name: "coder", ReportsTo: "keeper", Job: "authors code", Authors: true, Body: "court", Office: "great-sage", Tools: baseTools, Dir: "coder", Prefix: "coder-"},
		{Name: "writer", ReportsTo: "keeper", Job: "authors docs", Authors: true, Body: "court", Office: "ciel", Tools: baseTools, Dir: "writer", Prefix: "writer-"},
		{Name: "auditor", ReportsTo: Rimuru, Job: "reads verdicts", Body: "keeper", Office: "raphael", Tools: []string{"read", "grep", "glob", "law"}, Dir: "auditor", Prefix: "auditor-"},
	}
}

func customWorld(t *testing.T) (string, *World) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".isekai/isekai.md":             lawText,
		".isekai/lead/main/README.md":   "# lead-main\n\n- **Territory:** `src/`, `docs/`\n",
		".isekai/keeper/core/README.md": "# keeper-core\n\n- **Territory:** `src/`, `docs/`\n- **Reports to:** lead-main\n- **Verify:** `test -d src`\n",
		".isekai/coder/auth/README.md":  "# coder-auth\n\n- **Territory:** `src/auth/`\n- **Reports to:** keeper-core\n- **Verify:** `test ! -f src/auth/BROKEN`\n",
		".isekai/writer/docs/README.md": "# writer-docs\n\n- **Territory:** `docs/`\n- **Reports to:** keeper-core\n",
		".isekai/auditor/eye/README.md": "# auditor-eye\n\n- **Territory:** `.isekai/log.md`\n",
		"src/auth/login.go":             "package auth\n",
		"docs/index.md":                 "# docs\n",
	}
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	w, err := Open(dir, Isekai(), Options{Ontology: true, Ranks: customRanks()})
	if err != nil {
		t.Fatal(err)
	}
	return dir, w
}

func TestCustomRanksReplace(t *testing.T) {
	dir, w := customWorld(t)
	names := []string{}
	for _, c := range w.Creatures {
		names = append(names, c.Name+":"+c.Rank+"→"+c.Parent)
	}
	if got := strings.Join(names, " "); got != "auditor-eye:auditor→rimuru coder-auth:coder→keeper-core keeper-core:keeper→lead-main lead-main:lead→rimuru writer-docs:writer→keeper-core" {
		t.Fatalf("roster: %s", got)
	}
	if !w.GateHolders() || w.Owner("src/auth/x.go").Name != "coder-auth" || w.Owner("docs/a.md").Name != "writer-docs" || w.Owner("src/other.go").Name != "keeper-core" {
		t.Error("owners by Authors / HoldsGate, no rank names in logic")
	}
	if w.RankOf("coder-auth").Name != "coder" || w.RankOf("nobody").Name != Rimuru || w.RankOf("").Name != Rimuru {
		t.Error("RankOf")
	}
	// the roster's shelves come from the table
	if e := w.Engine("auditor-eye", build(mock.New())); strings.Join(e.Tools.Names(), ",") != "read,grep,glob,law" {
		t.Errorf("auditor shelf %v", e.Tools.Names())
	}
	if e := w.Engine("coder-auth", build(mock.New())); strings.Contains(strings.Join(e.Tools.Names(), ","), "dispatch") {
		t.Error("an authoring rank does not dispatch")
	}
	// territory: an authoring rank is held to its ground; the refusal points one hop up
	m := mock.New(writeCall("1", "docs/new.md", "x"), mock.Text("@S DONE\n@? docs/new.md is writer-docs' ground\n@U colony c\n@E 0"))
	e := w.Engine("coder-auth", build(m))
	r, _ := e.Run(context.Background(), "x")
	if r.Steps[0].Status != "refused" || !strings.Contains(m.Requests[1].Messages[2].ToolResults[0].Content, "outside coder-auth's territory") || !strings.Contains(m.Requests[1].Messages[2].ToolResults[0].Content, "to keeper-core") {
		t.Fatalf("territory: %+v %s", r.Steps[0], m.Requests[1].Messages[2].ToolResults[0].Content)
	}
	// the gate: Vitality fails without the doc, passes with it, and the verdict names the table's ranks only through the creatures
	doc := ".isekai/coder/auth/README.md"
	e = w.Engine("coder-auth", build(mock.New(writeCall("1", "src/auth/token.go", "t"), mock.Text("@S DONE\n@U colony c\n@E 0"))))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Doc truthful (Vitality): src/auth/token.go changed under coder-auth's territory but "+doc+" did not") {
		t.Fatalf("vitality: %s %q", r.Status, r.Verdict)
	}
	e = w.Engine("coder-auth", build(mock.New(writeCall("1", "src/auth/token.go", "t"), writeCall("2", doc, "# coder-auth\n\n- **Territory:** `src/auth/`\n- **Reports to:** keeper-core\n"), mock.Text("@S DONE\n@U colony c\n@E 0"))))
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Done || !strings.HasPrefix(r.Verdict, "pass") {
		t.Fatalf("pass: %s %q %v", r.Status, r.Verdict, r.Holes)
	}
	if lg := logText(t, dir); !strings.Contains(lg, "] coder-auth — gate pass") {
		t.Fatalf("log:\n%s", lg)
	}
	// right author: the other authoring rank writes on coder ground (territory off so it lands)
	b := build(mock.New(writeCall("1", "src/auth/z.go", "z"), writeCall("2", doc, "# coder-auth\n\n- **Territory:** `src/auth/`\n- **Reports to:** keeper-core\n"), mock.Text("@S DONE\n@U colony c\n@E 0")))
	b.Territory.Enabled = false
	e = w.Engine("writer-docs", b)
	if r, _ = e.Run(context.Background(), "x"); r.Status != loop.Fail || !strings.Contains(r.Verdict, "Right slime authored: src/auth/z.go belongs to coder-auth, written by writer-docs") {
		t.Fatalf("author: %s %q", r.Status, r.Verdict)
	}
	// no gate holder on the roster: n/a wording is generic, check 4 still runs
	os.RemoveAll(filepath.Join(dir, ".isekai/keeper"))
	w2, _ := Open(dir, Isekai(), Options{Ontology: true, Ranks: customRanks()})
	if v := w2.Gate(context.Background(), DefaultHooks().Gate, "coder-auth", []string{"src/auth/q.go", doc}, nil, true, "x"); v.Word != "Gate: n/a (no gate holder)" {
		t.Fatalf("n/a: %+v", v)
	}
	// dispatch: the coordinator dispatches down; the office rides the ask; an upward dispatch is refused
	_, w = customWorld(t)
	var routes []Route
	m = mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"body": "coder-auth", "ask": "judge the token code", "office": "raphael"}),
		mock.Text("@S DONE\n@V token.go:1 sound\n@U territory tokens are opaque\n@E 0"),
		mock.Text("@S DONE\n@U colony x\n@E 0"),
	)
	b = build(m)
	b.Models = func(r Route) provider.Provider { routes = append(routes, r); return nil }
	e = w.Engine("lead-main", b)
	if r, _ = e.Run(context.Background(), "@ASK draft\nrun the review"); r.Status != loop.Done || r.Steps[0].Status != "done" {
		t.Fatalf("dispatch: %+v", r)
	}
	if c := m.Requests[1].Messages[0].Text; !strings.Contains(c, "@ASK verdict +unsaid") {
		t.Fatalf("the office rides the commission:\n%s", c)
	}
	if len(routes) != 2 || routes[0].Rank != "lead" || routes[0].Office != "ciel" || routes[0].Task != "session" || routes[1] != (Route{Office: "raphael", Rank: "coder", Creature: "coder-auth", Task: "dispatch"}) {
		t.Fatalf("routes %+v", routes)
	}
	m = mock.New(mock.Call("d1", "dispatch", map[string]interface{}{"body": "lead-main", "ask": "x"}), mock.Text("@S DONE\n@U colony x\n@E 0"))
	e = w.Engine("keeper-core", build(m))
	e.Run(context.Background(), "x")
	if res := m.Requests[1].Messages[2].ToolResults[0]; !res.IsError || !strings.Contains(res.Content, "may not dispatch lead-main") {
		t.Fatalf("upward dispatch: %+v", res)
	}
	// a table that breaks the law's shape is refused at open
	broken := customRanks()
	broken[1].HoldsGate = false
	if _, err := Open(dir, Isekai(), Options{Ranks: broken}); err == nil || !strings.Contains(err.Error(), "Law 3") {
		t.Errorf("broken table accepted: %v", err)
	}
}

func TestHomes(t *testing.T) {
	dir, w := customWorld(t)
	h := w.Homes()
	if h.Root != dir || h.WorldDir != ".isekai" || !strings.HasPrefix(h.Crest(), "1. **Vitality**") {
		t.Fatalf("homes %+v", h)
	}
	if err := h.Assert("coder-auth", "territory", "argon2 everywhere"); err != nil {
		t.Fatal(err)
	}
	if err := h.Remember("coder-auth", "territory", "argon2 everywhere"); err != nil {
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
