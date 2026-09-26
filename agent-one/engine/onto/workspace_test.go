package onto

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newWorkspace(t *testing.T, broken bool) (string, *Workspace) {
	t.Helper()
	dir := t.TempDir()
	if err := fixture(dir, broken); err != nil {
		t.Fatal(err)
	}
	w, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, w
}

func TestSchemaOnDiskMatchesBuiltIn(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".agent-one", "ontology", "schema.ttl"))
	if err != nil {
		t.Skip("no workspace schema beside this module")
	}
	if string(b) != DefaultSchema {
		t.Error(".agent-one/ontology/schema.ttl differs from onto/schema.ttl — Docs-as-code: change both in the same change")
	}
}

func TestReasoner(t *testing.T) {
	g := New()
	if _, err := Parse("schema.ttl", DefaultSchema, g, nil); err != nil {
		t.Fatal(err)
	}
	g.Add(Triple{Ao("s"), pTruth, Ao("o")})
	g.Add(Triple{Ao("o"), pVerdict, Ao("e")})
	g.Add(Triple{Ao("e"), pReports, tOrchestrator})
	g.Add(Triple{Ao("s"), pHolds, Ao("m")})
	g.Add(Triple{Ao("s"), pKnows, Ao("f")})
	g.Add(Triple{Ao("f"), rdfType, Ao("Policy")})
	Infer(g)
	for _, want := range []Triple{
		{Ao("s"), rdfType, cZone}, {Ao("o"), rdfType, Ao("DomainOwner")}, {Ao("e"), rdfType, Ao("Coordinator")},
		{Ao("s"), rdfType, cMember}, {Ao("m"), rdfType, cSkill}, {Ao("f"), rdfType, cFact},
		{Ao("s"), pAbove, tOrchestrator}, {tOrchestrator, pBelow, Ao("s")}, {Ao("m"), Ao("heldBy"), Ao("s")},
		{Ao("f"), Ao("knownBy"), Ao("s")}, {Ao("ZoneWorker"), pSubClassOf, Ao("Member")},
	} {
		if !g.Has(want) {
			t.Errorf("not inferred: %v", want)
		}
		if g.Has(want) && !g.Derived(want) && want.P != pSubClassOf {
			t.Errorf("inferred triple not marked derived: %v", want)
		}
	}
	if g.Has(Triple{Ao("s"), rdfType, Ao("DomainOwner")}) {
		t.Error("over-inference: zone typed as domain")
	}
	n := g.Len()
	if Infer(g) != 0 || g.Len() != n {
		t.Error("not a fixpoint")
	}
}

func TestDeriveAndValidate(t *testing.T) {
	_, w := newWorkspace(t, false)
	g := w.Graph
	for _, want := range []Triple{
		{Ao("zone-auth"), pTruth, Ao("domain-security")},
		{Ao("zone-api"), pTruth, Ao("domain-security")},
		{Ao("domain-security"), pVerdict, Ao("coord-core")},
		{Ao("coord-core"), pReports, tOrchestrator},
		{Ao("zone-auth"), pHolds, Ao("skill-drafter")},
		{Ao("domain-security"), pHolds, Ao("skill-analyst")},
		{Ao("zone-auth"), pOwns, L("src/auth")},
		{Ao("domain-security"), pOwns, L("src/api")},
		{Ao("skill-judge"), rdfType, cSkill},
		{Ao("zone-auth"), pDoc, Ao("doc-zone-auth")},
		{Ao("doc-zone-auth"), pPath, L(".agent-one/zone/auth/README.md")},
		{tOrchestrator, rdfType, cOrchestrator},
	} {
		if !g.Has(want) {
			t.Errorf("not derived: %v", want)
		} else if !g.Derived(want) {
			t.Errorf("workspace triple should be derived, not asserted: %v", want)
		}
	}
	if len(g.Asserted()) == 0 {
		t.Error("schema triples should be asserted")
	}
	fs := w.Validate()
	if len(fs) != 1 || fs[0].Shape != "SkillHeld" || !strings.Contains(fs[0].String(), ".claude/skills/zone-lonely/SKILL.md:1: skill-zone-lonely has 0 heldBy edges, wants at least 1 (SkillHeld)") {
		t.Errorf("healthy workspace findings: %v", fs)
	}
	if !g.Has(Triple{Ao("skill-judge"), pShared, Bool(true)}) || !g.Has(Triple{Ao("skill-drafter"), pShared, Bool(true)}) || g.Has(Triple{Ao("skill-zone-lonely"), pShared, Bool(true)}) {
		t.Error("shared: a skill with no rank prefix is a shared host tool; a rank-prefixed one is a skill")
	}
	if w.Docs[Ao("zone-auth")].Lines["parent"] != 5 {
		t.Errorf("field line: %v", w.Docs[Ao("zone-auth")].Lines)
	}
}

func TestValidateBroken(t *testing.T) {
	_, w := newWorkspace(t, true)
	var got []string
	for _, f := range w.Validate() {
		got = append(got, f.String())
	}
	want := []string{
		".claude/skills/zone-lonely/SKILL.md:1: skill-zone-lonely has 0 heldBy edges, wants at least 1 (SkillHeld)",
		".agent-one/zone/auth/README.md:4: zone-auth owns src/auth, overlapping src/auth/login of zone-orphan (ZoneOwnership)",
		".agent-one/zone/orphan/README.md:1: zone-orphan has 0 truth edges, wants exactly 1 (ZoneTruth)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDanglingBond(t *testing.T) {
	dir := t.TempDir()
	if err := fixture(dir, false); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".agent-one/zone/api/README.md"), []byte("# zone-api\n- **Owns:** `src/api/`\n- **Reports to:** domain-ghost\n"), 0o644)
	w, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range w.Validate() {
		if f.Shape == "BondTarget" && strings.Contains(f.String(), ".agent-one/zone/api/README.md:3: zone-api above domain-ghost, which has no doc") {
			found = true
		}
	}
	if !found {
		t.Errorf("dangling bond not found: %v", w.Validate())
	}
}

func TestFlowAndProjection(t *testing.T) {
	dir, w := newWorkspace(t, false)
	Now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }
	defer func() { Now = time.Now }()
	for _, u := range [][3]string{
		{"zone-auth", "domain", "token TTL is 15m"},
		{"domain-security", "team", "gate runs go vet first"},
		{"coord-core", "policy", "nobody pushes"},
		{"coord-core", "team", "coord gossip"},
	} {
		if _, err := w.AssertUnsaid(u[0], u[1], u[2]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.AssertUnsaid("zone-auth", "vibe", "x"); err == nil {
		t.Error("bad kind accepted")
	}
	if _, err := w.AssertUnsaid("zone-nobody", "policy", "x"); err == nil {
		t.Error("unknown agent accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(OntologyDir(dir), "graph", "unsaid.ttl"))
	if c := strings.Count(string(raw), "@prefix"); c != 1 {
		t.Errorf("prefix block written %d times", c)
	}
	if !strings.Contains(string(raw), "ao:said \"2026-09-26T12:00:00Z\"") || strings.Count(string(raw), "<agent-one:") != 1 {
		t.Errorf("unsaid.ttl:\n%s", raw)
	}
	// reload: the file is truth
	w, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(w.Graph.Instances(cFact)); n != 4 {
		t.Errorf("facts after reload: %d", n)
	}
	lines, err := w.Project("domain-security", 1000)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"domain-security ⇒verdict coord-core",
		"domain-security ⇌holds analyst",
		"domain-security owns src/api",
		"domain-security owns src/auth",
		"domain-security · gate runs go vet first (team)",
		"zone-api ⇒truth domain-security",
		"zone-auth ⇒truth domain-security",
		"coord-core ⇐verdict domain-security · nobody pushes (policy)",
		"zone-auth ⇒truth domain-security · token TTL is 15m (domain)",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("projection:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	coord, _ := w.Project("coord-core", 1000)
	j := strings.Join(coord, "\n")
	if !strings.Contains(j, "zone-auth ⇒truth domain-security ⇒verdict coord-core · token TTL is 15m (domain)") || !strings.Contains(j, "domain-security ⇒verdict coord-core · gate runs go vet first (team)") {
		t.Errorf("coord projection:\n%s", j)
	}
	zone, _ := w.Project("auth", 1000)
	j = strings.Join(zone, "\n")
	if !strings.Contains(j, "coord-core ⇐verdict domain-security ⇐truth zone-auth · nobody pushes (policy)") || strings.Contains(j, "gate runs") || strings.Contains(j, "gossip") {
		t.Errorf("zone projection (policy down, team not):\n%s", j)
	}
	rim, _ := w.Project("orchestrator", 1000)
	if !strings.Contains(strings.Join(rim, "\n"), "zone-auth ⇒truth domain-security ⇒verdict coord-core ⇒reports orchestrator · token TTL is 15m (domain)") {
		t.Errorf("orchestrator sees everything:\n%s", strings.Join(rim, "\n"))
	}
	// budget: chars/4, nearest first
	budget := Tokens(want[0]) + Tokens(want[1])
	cut, _ := w.Project("domain-security", budget)
	if len(cut) != 2 {
		t.Errorf("budget %d kept %d lines: %v", budget, len(cut), cut)
	}
	if Tokens("abcd") != 1 || Tokens("abcde") != 2 || Tokens("") != 0 {
		t.Error("token rule")
	}
	if _, err := w.Project("nobody", 10); err == nil {
		t.Error("unknown member projected")
	}
}

func TestCLI(t *testing.T) {
	dir, _ := newWorkspace(t, false)
	run := func(args ...string) (int, string, string) {
		var o, e bytes.Buffer
		code := CLI(append([]string{"--root", dir}, args...), &o, &e)
		return code, o.String(), e.String()
	}
	code, out, _ := run("check")
	if code != 1 || !strings.HasPrefix(out, "@S FAIL 1 findings · 5 members · 4 skills · 0 facts") || !strings.Contains(out, "@F .claude/skills/zone-lonely/SKILL.md:1: skill-zone-lonely") || strings.Contains(out, "skill-judge") || !strings.Contains(out, "@? .agent-one/ontology/schema.ttl missing") {
		t.Errorf("check: %d\n%s", code, out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "@E "+itoa(strings.LastIndex(out, "@E "))) {
		t.Errorf("@E byte count: %s", out)
	}
	code, out, _ = run("assert", "zone-auth", "domain", "token", "TTL", "is", "15m")
	if code != 0 || !strings.Contains(out, "@S PASS asserted\n@F .agent-one/ontology/graph/unsaid.ttl: zone-auth knows fact-") {
		t.Errorf("assert: %d\n%s", code, out)
	}
	code, _, errOut := run("assert", "zone-auth", "vibe", "x")
	if code != 1 || !strings.Contains(errOut, "@? kind must be") {
		t.Errorf("bad assert: %d %s", code, errOut)
	}
	code, out, _ = run("project", "domain-security", "--budget", "100")
	if code != 0 || !strings.Contains(out, "token TTL is 15m (domain)") || strings.Contains(out, "@") {
		t.Errorf("project: %d\n%s", code, out)
	}
	code, _, errOut = run("project", "nobody")
	if code != 1 || !strings.Contains(errOut, "@? unknown member") {
		t.Errorf("project unknown: %d %s", code, errOut)
	}
	code, out, _ = run("show")
	if code != 0 || !strings.Contains(out, "ao:zone-auth a ao:Member, ao:ZoneWorker ;") || !strings.Contains(out, "# ") {
		t.Errorf("show: %d\n%s", code, out[:300])
	}
	if code, _, e := run("bogus"); code != 2 || !strings.Contains(e, "usage") {
		t.Error("unknown command")
	}
	if code, _, e := run(); code != 2 || !strings.Contains(e, "usage") {
		t.Error("no command")
	}
	// a syntax error in a graph file is a load failure with a position
	os.WriteFile(filepath.Join(OntologyDir(dir), "graph", "bad.ttl"), []byte("@prefix ao: <agent-one:> .\nao:x ao:p\n"), 0o644)
	code, _, errOut = run("check")
	if code != 2 || !strings.Contains(errOut, ".agent-one/ontology/graph/bad.ttl:3:1: expected an object") {
		t.Errorf("bad graph file: %d %s", code, errOut)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestLayoutAgentOne(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".agent-one/AGENT-ONE.md":              "# rules\n",
		".agent-one/coord/core/README.md":      "# coord-core\n\n- **Owns:** `src/`\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Owns:** `src/auth/`\n- **Reports to:** coord-core\n",
		".agent-one/zone/auth/README.md":       "# zone-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Skills:** drafter\n",
		".claude/skills/drafter/SKILL.md":      "---\nname: drafter\n---\ndrafts\n",
	}
	for p, agent := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(agent), 0o644)
	}
	l := Layout{WorkspaceDir: ".agent-one", Policy: "AGENT-ONE.md", Ranks: []RankDir{{"coord", "coord", "orchestrator"}, {"domain", "domain", "coord"}, {"zone", "zone", "domain"}, {"service", "service", "orchestrator"}}}
	if r, err := FindRootIn(filepath.Join(dir, "src"), l); err != nil || r != dir {
		t.Fatalf("FindRootIn: %q %v", r, err)
	}
	if r, err := FindRoot(dir); err != nil || r != dir {
		t.Errorf("the default layout finds the workspace: %q %v", r, err)
	}
	w, err := LoadLayout(dir, l)
	if err != nil {
		t.Fatal(err)
	}
	g := w.Graph
	for _, want := range []Triple{
		{Ao("zone-auth"), rdfType, cZone}, {Ao("zone-auth"), pTruth, Ao("domain-security")},
		{Ao("domain-security"), pVerdict, Ao("coord-core")}, {Ao("coord-core"), pReports, tOrchestrator},
		{Ao("zone-auth"), pOwns, L("src/auth")}, {Ao("zone-auth"), pHolds, Ao("skill-drafter")},
		{Ao("doc-orchestrator"), pPath, L(".agent-one/AGENT-ONE.md")}, {Ao("doc-zone-auth"), pPath, L(".agent-one/zone/auth/README.md")},
	} {
		if !g.Has(want) {
			t.Errorf("not derived: %v", want)
		}
	}
	if fs := w.Validate(); len(fs) != 0 {
		t.Errorf("findings: %v", fs)
	}
	if _, err := w.Project("zone-auth", 500); err != nil {
		t.Error(err)
	}
}

func TestSelftest(t *testing.T) {
	n, err := Selftest()
	if err != nil {
		t.Fatalf("after %d checks: %v", n, err)
	}
	if n < 20 {
		t.Errorf("only %d checks", n)
	}
}

func TestLayoutCustomRanks(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".agent-one/AGENT-ONE.md":              "# policy\n",
		".agent-one/lead/main/README.md":       "# lead-main\n\n- **Owns:** `src/`\n",
		".agent-one/persistent/core/README.md": "# persistent-core\n\n- **Owns:** `src/`\n- **Reports to:** lead-main\n",
		".agent-one/coder/auth/README.md":      "# coder-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** persistent-core\n- **Skills:** drafter\n",
		".agent-one/writer/docs/README.md":     "# writer-docs\n\n- **Owns:** `docs/`\n- persistent-core rules here\n",
		".agent-one/auditor/eye/README.md":     "# auditor-eye\n\n- **Owns:** `.agent-one/log.md`\n",
		".claude/skills/drafter/SKILL.md":      "---\nname: drafter\n---\ndrafts\n",
	}
	for p, agent := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(agent), 0o644)
	}
	l := Layout{Ranks: []RankDir{{"lead", "lead", "orchestrator"}, {"persistent", "persistent", "lead"}, {"coder", "coder", "persistent"}, {"writer", "writer", "persistent"}, {"auditor", "auditor", "orchestrator"}}}
	w, err := LoadLayout(dir, l)
	if err != nil {
		t.Fatal(err)
	}
	g := w.Graph
	for _, want := range []Triple{
		{Ao("Coder"), pSubClassOf, cMember}, {Ao("coder-auth"), rdfType, Ao("Coder")}, {Ao("coder-auth"), rdfType, cMember},
		{Ao("coder-auth"), pAbove, Ao("persistent-core")}, {Ao("writer-docs"), pAbove, Ao("persistent-core")}, {Ao("persistent-core"), pAbove, Ao("lead-main")},
		{Ao("lead-main"), pReports, tOrchestrator}, {Ao("auditor-eye"), pReports, tOrchestrator}, {Ao("coder-auth"), pAbove, tOrchestrator},
		{Ao("coder-auth"), pHolds, Ao("skill-drafter")}, {Ao("auditor-eye"), pOwns, L(".agent-one/log.md")},
	} {
		if !g.Has(want) {
			t.Errorf("not derived: %v", want)
		}
	}
	if g.Has(Triple{Ao("coder-auth"), pTruth, Ao("persistent-core")}) || len(g.Instances(cZone)) != 0 {
		t.Error("custom ranks take the generic bond, never a policy rank's class")
	}
	if fs := w.Validate(); len(fs) != 0 {
		t.Errorf("findings: %v", fs)
	}
	if _, err := w.AssertUnsaid("coder-auth", "domain", "auth uses argon2"); err != nil {
		t.Fatal(err)
	}
	lines, _ := w.Project("lead-main", 1000)
	if j := strings.Join(lines, "\n"); !strings.Contains(j, "coder-auth ⇒above persistent-core ⇒above lead-main · auth uses argon2 (domain)") {
		t.Errorf("flow along custom bonds:\n%s", j)
	}
	if ClassOf("auditor") != Ao("Auditor") || ClassOf("zone") != cZone || ClassOf("") != cMember {
		t.Error("ClassOf")
	}
}
