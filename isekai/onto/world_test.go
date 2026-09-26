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

func newWorld(t *testing.T, broken bool) (string, *World) {
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
	b, err := os.ReadFile(filepath.Join("..", "..", ".isekai", "ontology", "schema.ttl"))
	if err != nil {
		t.Skip("no world schema beside this module")
	}
	if string(b) != DefaultSchema {
		t.Error(".isekai/ontology/schema.ttl differs from onto/schema.ttl — Vitality: change both in the same change")
	}
}

func TestReasoner(t *testing.T) {
	g := New()
	if _, err := Parse("schema.ttl", DefaultSchema, g, nil); err != nil {
		t.Fatal(err)
	}
	g.Add(Triple{Is("s"), pTruth, Is("o")})
	g.Add(Triple{Is("o"), pVerdict, Is("e")})
	g.Add(Triple{Is("e"), pReports, tRimuru})
	g.Add(Triple{Is("s"), pWears, Is("m")})
	g.Add(Triple{Is("s"), pKnows, Is("f")})
	g.Add(Triple{Is("f"), rdfType, Is("Law")})
	Infer(g)
	for _, want := range []Triple{
		{Is("s"), rdfType, cSlime}, {Is("o"), rdfType, Is("Orc")}, {Is("e"), rdfType, Is("Elf")},
		{Is("s"), rdfType, cCreature}, {Is("m"), rdfType, cMind}, {Is("f"), rdfType, cFact},
		{Is("s"), pAbove, tRimuru}, {tRimuru, pBelow, Is("s")}, {Is("m"), Is("wornBy"), Is("s")},
		{Is("f"), Is("knownBy"), Is("s")}, {Is("Slime"), pSubClassOf, Is("Creature")},
	} {
		if !g.Has(want) {
			t.Errorf("not inferred: %v", want)
		}
		if g.Has(want) && !g.Derived(want) && want.P != pSubClassOf {
			t.Errorf("inferred triple not marked derived: %v", want)
		}
	}
	if g.Has(Triple{Is("s"), rdfType, Is("Orc")}) {
		t.Error("over-inference: slime typed as orc")
	}
	n := g.Len()
	if Infer(g) != 0 || g.Len() != n {
		t.Error("not a fixpoint")
	}
}

func TestDeriveAndValidate(t *testing.T) {
	_, w := newWorld(t, false)
	g := w.Graph
	for _, want := range []Triple{
		{Is("slime-auth"), pTruth, Is("orc-security")},
		{Is("slime-api"), pTruth, Is("orc-security")},
		{Is("orc-security"), pVerdict, Is("elf-core")},
		{Is("elf-core"), pReports, tRimuru},
		{Is("slime-auth"), pWears, Is("mind-ciel")},
		{Is("orc-security"), pWears, Is("mind-great-sage")},
		{Is("slime-auth"), pOwns, L("src/auth")},
		{Is("orc-security"), pOwns, L("src/api")},
		{Is("mind-raphael"), rdfType, cMind},
		{Is("slime-auth"), pDoc, Is("doc-slime-auth")},
		{Is("doc-slime-auth"), pPath, L(".isekai/slime/auth/README.md")},
		{tRimuru, rdfType, cRimuru},
	} {
		if !g.Has(want) {
			t.Errorf("not derived: %v", want)
		} else if !g.Derived(want) {
			t.Errorf("world triple should be derived, not asserted: %v", want)
		}
	}
	if len(g.Asserted()) == 0 {
		t.Error("schema triples should be asserted")
	}
	fs := w.Validate()
	if len(fs) != 1 || fs[0].Shape != "MindWorn" || !strings.Contains(fs[0].String(), ".claude/skills/slime-lonely/SKILL.md:1: mind-slime-lonely has 0 wornBy edges, wants at least 1 (MindWorn)") {
		t.Errorf("healthy world findings: %v", fs)
	}
	if !g.Has(Triple{Is("mind-raphael"), pShared, Bool(true)}) || !g.Has(Triple{Is("mind-ciel"), pShared, Bool(true)}) || g.Has(Triple{Is("mind-slime-lonely"), pShared, Bool(true)}) {
		t.Error("shared: a skill with no race prefix is a shared host tool; a race-prefixed one is a mind")
	}
	if w.Docs[Is("slime-auth")].Lines["parent"] != 5 {
		t.Errorf("field line: %v", w.Docs[Is("slime-auth")].Lines)
	}
}

func TestValidateBroken(t *testing.T) {
	_, w := newWorld(t, true)
	var got []string
	for _, f := range w.Validate() {
		got = append(got, f.String())
	}
	want := []string{
		".claude/skills/slime-lonely/SKILL.md:1: mind-slime-lonely has 0 wornBy edges, wants at least 1 (MindWorn)",
		".isekai/slime/auth/README.md:4: slime-auth owns src/auth, overlapping src/auth/login of slime-orphan (SlimeTerritory)",
		".isekai/slime/orphan/README.md:1: slime-orphan has 0 truth edges, wants exactly 1 (SlimeTruth)",
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
	os.WriteFile(filepath.Join(dir, ".isekai/slime/api/README.md"), []byte("# slime-api\n- **Territory:** `src/api/`\n- **Reports to:** orc-ghost\n"), 0o644)
	w, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range w.Validate() {
		if f.Shape == "BondTarget" && strings.Contains(f.String(), ".isekai/slime/api/README.md:3: slime-api above orc-ghost, which has no doc") {
			found = true
		}
	}
	if !found {
		t.Errorf("dangling bond not found: %v", w.Validate())
	}
}

func TestFlowAndProjection(t *testing.T) {
	dir, w := newWorld(t, false)
	Now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }
	defer func() { Now = time.Now }()
	for _, u := range [][3]string{
		{"slime-auth", "territory", "token TTL is 15m"},
		{"orc-security", "colony", "gate runs go vet first"},
		{"elf-core", "law", "nobody pushes"},
		{"elf-core", "colony", "elf gossip"},
	} {
		if _, err := w.AssertUnsaid(u[0], u[1], u[2]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.AssertUnsaid("slime-auth", "vibe", "x"); err == nil {
		t.Error("bad kind accepted")
	}
	if _, err := w.AssertUnsaid("slime-nobody", "law", "x"); err == nil {
		t.Error("unknown body accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(OntologyDir(dir), "graph", "unsaid.ttl"))
	if c := strings.Count(string(raw), "@prefix"); c != 1 {
		t.Errorf("prefix block written %d times", c)
	}
	if !strings.Contains(string(raw), "is:said \"2026-09-26T12:00:00Z\"") || strings.Count(string(raw), "<isekai:") != 1 {
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
	lines, err := w.Project("orc-security", 1000)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"orc-security ⇒verdict elf-core",
		"orc-security ⇌wears great-sage",
		"orc-security owns src/api",
		"orc-security owns src/auth",
		"orc-security · gate runs go vet first (colony)",
		"slime-api ⇒truth orc-security",
		"slime-auth ⇒truth orc-security",
		"elf-core ⇐verdict orc-security · nobody pushes (law)",
		"slime-auth ⇒truth orc-security · token TTL is 15m (territory)",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("projection:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	elf, _ := w.Project("elf-core", 1000)
	j := strings.Join(elf, "\n")
	if !strings.Contains(j, "slime-auth ⇒truth orc-security ⇒verdict elf-core · token TTL is 15m (territory)") || !strings.Contains(j, "orc-security ⇒verdict elf-core · gate runs go vet first (colony)") {
		t.Errorf("elf projection:\n%s", j)
	}
	slime, _ := w.Project("auth", 1000)
	j = strings.Join(slime, "\n")
	if !strings.Contains(j, "elf-core ⇐verdict orc-security ⇐truth slime-auth · nobody pushes (law)") || strings.Contains(j, "gate runs") || strings.Contains(j, "gossip") {
		t.Errorf("slime projection (law down, colony not):\n%s", j)
	}
	rim, _ := w.Project("rimuru", 1000)
	if !strings.Contains(strings.Join(rim, "\n"), "slime-auth ⇒truth orc-security ⇒verdict elf-core ⇒reports rimuru · token TTL is 15m (territory)") {
		t.Errorf("rimuru sees everything:\n%s", strings.Join(rim, "\n"))
	}
	// budget: chars/4, nearest first
	budget := Tokens(want[0]) + Tokens(want[1])
	cut, _ := w.Project("orc-security", budget)
	if len(cut) != 2 {
		t.Errorf("budget %d kept %d lines: %v", budget, len(cut), cut)
	}
	if Tokens("abcd") != 1 || Tokens("abcde") != 2 || Tokens("") != 0 {
		t.Error("token rule")
	}
	if _, err := w.Project("nobody", 10); err == nil {
		t.Error("unknown creature projected")
	}
}

func TestCLI(t *testing.T) {
	dir, _ := newWorld(t, false)
	run := func(args ...string) (int, string, string) {
		var o, e bytes.Buffer
		code := CLI(append([]string{"--root", dir}, args...), &o, &e)
		return code, o.String(), e.String()
	}
	code, out, _ := run("check")
	if code != 1 || !strings.HasPrefix(out, "@S FAIL 1 findings · 5 creatures · 4 minds · 0 facts") || !strings.Contains(out, "@F .claude/skills/slime-lonely/SKILL.md:1: mind-slime-lonely") || strings.Contains(out, "mind-raphael") || !strings.Contains(out, "@? .isekai/ontology/schema.ttl missing") {
		t.Errorf("check: %d\n%s", code, out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "@E "+itoa(strings.LastIndex(out, "@E "))) {
		t.Errorf("@E byte count: %s", out)
	}
	code, out, _ = run("assert", "slime-auth", "territory", "token", "TTL", "is", "15m")
	if code != 0 || !strings.Contains(out, "@S PASS asserted\n@F .isekai/ontology/graph/unsaid.ttl: slime-auth knows fact-") {
		t.Errorf("assert: %d\n%s", code, out)
	}
	code, _, errOut := run("assert", "slime-auth", "vibe", "x")
	if code != 1 || !strings.Contains(errOut, "@? kind must be") {
		t.Errorf("bad assert: %d %s", code, errOut)
	}
	code, out, _ = run("project", "orc-security", "--budget", "100")
	if code != 0 || !strings.Contains(out, "token TTL is 15m (territory)") || strings.Contains(out, "@") {
		t.Errorf("project: %d\n%s", code, out)
	}
	code, _, errOut = run("project", "nobody")
	if code != 1 || !strings.Contains(errOut, "@? unknown creature") {
		t.Errorf("project unknown: %d %s", code, errOut)
	}
	code, out, _ = run("show")
	if code != 0 || !strings.Contains(out, "is:slime-auth a is:Creature, is:Slime ;") || !strings.Contains(out, "# ") {
		t.Errorf("show: %d\n%s", code, out[:300])
	}
	if code, _, e := run("bogus"); code != 2 || !strings.Contains(e, "usage") {
		t.Error("unknown command")
	}
	if code, _, e := run(); code != 2 || !strings.Contains(e, "usage") {
		t.Error("no command")
	}
	// a syntax error in a graph file is a load failure with a position
	os.WriteFile(filepath.Join(OntologyDir(dir), "graph", "bad.ttl"), []byte("@prefix is: <isekai:> .\nis:x is:p\n"), 0o644)
	code, _, errOut = run("check")
	if code != 2 || !strings.Contains(errOut, ".isekai/ontology/graph/bad.ttl:3:1: expected an object") {
		t.Errorf("bad graph file: %d %s", code, errOut)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestLayoutAgentZero(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".agent-zero/AGENT-ZERO.md":             "# rules\n",
		".agent-zero/coord/core/README.md":      "# coord-core\n\n- **Owns:** `src/`\n",
		".agent-zero/domain/security/README.md": "# domain-security\n\n- **Owns:** `src/auth/`\n- **Reports to:** coord-core\n",
		".agent-zero/zone/auth/README.md":       "# zone-auth\n\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Minds:** ciel\n",
		".claude/skills/ciel/SKILL.md":          "---\nname: ciel\n---\ndrafts\n",
	}
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	l := Layout{WorldDir: ".agent-zero", Law: "AGENT-ZERO.md", Ranks: []RankDir{{"elf", "coord", "rimuru"}, {"orc", "domain", "elf"}, {"slime", "zone", "orc"}, {"kijin", "service", "rimuru"}}}
	if r, err := FindRootIn(filepath.Join(dir, "src"), l); err != nil || r != dir {
		t.Fatalf("FindRootIn: %q %v", r, err)
	}
	if _, err := FindRoot(dir); err == nil {
		t.Error("the isekai layout must not find an agent-zero world")
	}
	w, err := LoadLayout(dir, l)
	if err != nil {
		t.Fatal(err)
	}
	g := w.Graph
	for _, want := range []Triple{
		{Is("zone-auth"), rdfType, cSlime}, {Is("zone-auth"), pTruth, Is("domain-security")},
		{Is("domain-security"), pVerdict, Is("coord-core")}, {Is("coord-core"), pReports, tRimuru},
		{Is("zone-auth"), pOwns, L("src/auth")}, {Is("zone-auth"), pWears, Is("mind-ciel")},
		{Is("doc-rimuru"), pPath, L(".agent-zero/AGENT-ZERO.md")}, {Is("doc-zone-auth"), pPath, L(".agent-zero/zone/auth/README.md")},
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
		".isekai/isekai.md":             "# law\n",
		".isekai/lead/main/README.md":   "# lead-main\n\n- **Territory:** `src/`\n",
		".isekai/keeper/core/README.md": "# keeper-core\n\n- **Territory:** `src/`\n- **Reports to:** lead-main\n",
		".isekai/coder/auth/README.md":  "# coder-auth\n\n- **Territory:** `src/auth/`\n- **Reports to:** keeper-core\n- **Minds:** ciel\n",
		".isekai/writer/docs/README.md": "# writer-docs\n\n- **Territory:** `docs/`\n- keeper-core rules here\n",
		".isekai/auditor/eye/README.md": "# auditor-eye\n\n- **Territory:** `.isekai/log.md`\n",
		".claude/skills/ciel/SKILL.md":  "---\nname: ciel\n---\ndrafts\n",
	}
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	l := Layout{Ranks: []RankDir{{"lead", "lead", "rimuru"}, {"keeper", "keeper", "lead"}, {"coder", "coder", "keeper"}, {"writer", "writer", "keeper"}, {"auditor", "auditor", "rimuru"}}}
	w, err := LoadLayout(dir, l)
	if err != nil {
		t.Fatal(err)
	}
	g := w.Graph
	for _, want := range []Triple{
		{Is("Coder"), pSubClassOf, cCreature}, {Is("coder-auth"), rdfType, Is("Coder")}, {Is("coder-auth"), rdfType, cCreature},
		{Is("coder-auth"), pAbove, Is("keeper-core")}, {Is("writer-docs"), pAbove, Is("keeper-core")}, {Is("keeper-core"), pAbove, Is("lead-main")},
		{Is("lead-main"), pReports, tRimuru}, {Is("auditor-eye"), pReports, tRimuru}, {Is("coder-auth"), pAbove, tRimuru},
		{Is("coder-auth"), pWears, Is("mind-ciel")}, {Is("auditor-eye"), pOwns, L(".isekai/log.md")},
	} {
		if !g.Has(want) {
			t.Errorf("not derived: %v", want)
		}
	}
	if g.Has(Triple{Is("coder-auth"), pTruth, Is("keeper-core")}) || len(g.Instances(cSlime)) != 0 {
		t.Error("custom ranks take the generic bond, never a law rank's class")
	}
	if fs := w.Validate(); len(fs) != 0 {
		t.Errorf("findings: %v", fs)
	}
	if _, err := w.AssertUnsaid("coder-auth", "territory", "auth uses argon2"); err != nil {
		t.Fatal(err)
	}
	lines, _ := w.Project("lead-main", 1000)
	if j := strings.Join(lines, "\n"); !strings.Contains(j, "coder-auth ⇒above keeper-core ⇒above lead-main · auth uses argon2 (territory)") {
		t.Errorf("flow along custom bonds:\n%s", j)
	}
	if ClassOf("dark_elf") != Is("DarkElf") || ClassOf("slime") != cSlime || ClassOf("") != cCreature {
		t.Error("ClassOf")
	}
}
