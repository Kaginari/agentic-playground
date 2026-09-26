package onto

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fixture writes a small workspace under dir: one coordinator, one domain owner, two zone workers, held
// skills, a shared (unprefixed, unheld) skill and a rank-prefixed unheld skill.
// broken adds a zone worker with no domain owner and an overlapping ownership.
func fixture(dir string, broken bool) error {
	files := map[string]string{
		".agent-one/AGENT-ONE.md":              "# policy\n",
		".agent-one/coord/core/README.md":      "# coord-core\n\n- **Rank:** Coordinator\n- **Owns:** `src/`\n- **Purpose:** the shared skill\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Rank:** Domain owner\n- **Owns:** `src/auth/`, `src/api/`\n- **Reports to:** coord-core\n- **Purpose:** rules the security domain; holds analyst\n",
		".agent-one/zone/auth/README.md":       "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Skills:** drafter\n\n## Invariants\n- tokens expire after one hour\n",
		".agent-one/zone/api/README.md":        "# zone-api\n\n- **Rank:** Zone worker\n- **Owns:** `src/api/`\n- **Domain owner:** domain-security\n",
		".claude/skills/analyst/SKILL.md":      "---\nname: analyst\n---\nreads\n",
		".claude/skills/drafter/SKILL.md":      "---\nname: drafter\n---\ndrafts\n",
		".claude/skills/judge/SKILL.md":        "---\nname: judge\n---\nverdicts, held by nobody here — a shared host skill, not a member's skill\n",
		".claude/skills/zone-lonely/SKILL.md":  "---\nname: zone-lonely\n---\na rank-prefixed skill held by nobody\n",
	}
	if broken {
		files[".agent-one/zone/orphan/README.md"] = "# zone-orphan\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/login/`\n"
	}
	for p, agent := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(agent), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Selftest runs the in-binary checks on a throwaway workspace and answers how
// many passed; err names the first failure.
func Selftest() (passed int, err error) {
	dir, err := os.MkdirTemp("", "agent-one-onto-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	check := func(ok bool, what string) error {
		if !ok {
			return fmt.Errorf("selftest: %s", what)
		}
		passed++
		return nil
	}
	fail := func(e error) (int, error) { return passed, e }

	// parser
	g := New()
	pre, perr := Parse("t.ttl", "@prefix ao: <agent-one:> .\nao:x a ao:ZoneWorker ; ao:owns \"a\", \"b\" ; ao:n 15 ; ao:t \"hi\"@en .", g, nil)
	if e := check(perr == nil && pre["ao"] == NS && g.Len() == 5, "parse a small document"); e != nil {
		return fail(e)
	}
	_, perr = Parse("t.ttl", "@prefix ao: <agent-one:> .\nao:x a ao:ZoneWorker\nao:y a ao:DomainOwner .", New(), nil)
	se, isSyntax := perr.(*SyntaxError)
	if e := check(isSyntax && se.Line == 3, "syntax error carries the line number"); e != nil {
		return fail(e)
	}
	_, perr = Parse("t.ttl", "ex:x a ex:Y .", New(), nil)
	if e := check(perr != nil && strings.Contains(perr.Error(), "undeclared prefix"), "undeclared prefix is an error"); e != nil {
		return fail(e)
	}
	var buf bytes.Buffer
	if e := check(Write(&buf, g.All(), pre) == nil, "write turtle"); e != nil {
		return fail(e)
	}
	g2 := New()
	_, perr = Parse("round.ttl", buf.String(), g2, nil)
	if e := check(perr == nil && len(g2.All()) == len(g.All()) && cmpTriple(g2.All()[0], g.All()[0]) == 0, "round trip write → parse"); e != nil {
		return fail(e)
	}

	// schema + reasoning
	sg := New()
	_, perr = Parse("schema.ttl", DefaultSchema, sg, nil)
	if e := check(perr == nil, "built-in schema parses"); e != nil {
		return fail(e)
	}
	sg.Add(Triple{Ao("s"), pTruth, Ao("o")})
	sg.Add(Triple{Ao("o"), pVerdict, Ao("e")})
	Infer(sg)
	if e := check(sg.Has(Triple{Ao("s"), rdfType, cZone}) && sg.Has(Triple{Ao("o"), rdfType, cMember}), "domain, range and subclass typing"); e != nil {
		return fail(e)
	}
	if e := check(sg.Has(Triple{Ao("s"), pAbove, Ao("e")}) && sg.Has(Triple{Ao("e"), pBelow, Ao("s")}), "subproperty, transitive and inverse closure"); e != nil {
		return fail(e)
	}
	n1 := sg.Len()
	Infer(sg)
	if e := check(sg.Len() == n1, "reasoning is at a fixpoint"); e != nil {
		return fail(e)
	}

	// a healthy workspace
	if err := fixture(dir, false); err != nil {
		return passed, err
	}
	w, err := Load(dir)
	if err != nil {
		return passed, err
	}
	if e := check(len(w.Graph.Instances(cZone)) == 2 && w.Graph.Has(Triple{Ao("zone-auth"), pTruth, Ao("domain-security")}) && w.Graph.Has(Triple{Ao("zone-api"), pTruth, Ao("domain-security")}), "members and truth bonds derived from docs"); e != nil {
		return fail(e)
	}
	if e := check(w.Graph.Has(Triple{Ao("domain-security"), pVerdict, Ao("coord-core")}) && w.Graph.Has(Triple{Ao("coord-core"), pReports, tOrchestrator}), "verdict and reports bonds"); e != nil {
		return fail(e)
	}
	if e := check(w.Graph.Has(Triple{Ao("zone-auth"), pHolds, Ao("skill-drafter")}) && w.Graph.Has(Triple{Ao("domain-security"), pHolds, Ao("skill-analyst")}), "held skills from field and prose"); e != nil {
		return fail(e)
	}
	if e := check(w.Graph.Has(Triple{Ao("zone-auth"), pOwns, L("src/auth")}), "ownership from the doc"); e != nil {
		return fail(e)
	}
	fs := w.Validate()
	if e := check(len(fs) == 1 && fs[0].Shape == "SkillHeld" && fs[0].Subject == Ao("skill-zone-lonely"), "healthy workspace: only the unheld rank-prefixed skill is a finding"); e != nil {
		return fail(e)
	}
	if e := check(w.Graph.Has(Triple{Ao("skill-judge"), pShared, Bool(true)}) && !w.Graph.Has(Triple{Ao("skill-zone-lonely"), pShared, Bool(true)}), "a skill with no rank prefix is shared and exempt from SkillHeld"); e != nil {
		return fail(e)
	}
	if e := check(len(w.Graph.Asserted()) > 0 && !w.Graph.Derived(w.Graph.Asserted()[0]) && w.Graph.Derived(Triple{Ao("zone-auth"), pTruth, Ao("domain-security")}), "derived triples are marked, never asserted"); e != nil {
		return fail(e)
	}

	// the unsaid, and the flow
	Now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }
	defer func() { Now = time.Now }()
	if _, err := w.AssertUnsaid("zone-auth", "domain", "token TTL is 15m"); err != nil {
		return passed, err
	}
	if _, err := w.AssertUnsaid("coord-core", "policy", "nobody pushes"); err != nil {
		return passed, err
	}
	if _, err := w.AssertUnsaid("coord-core", "team", "coord gossip"); err != nil {
		return passed, err
	}
	unsaid, _ := os.ReadFile(filepath.Join(OntologyDir(dir), "graph", "unsaid.ttl"))
	if e := check(strings.Count(string(unsaid), "ao:knows") == 3 && strings.HasPrefix(string(unsaid), "#"), "unsaid.ttl is appended, header once"); e != nil {
		return fail(e)
	}
	w, err = Load(dir)
	if err != nil {
		return passed, err
	}
	lines, err := w.Project("domain-security", 1000)
	if err != nil {
		return passed, err
	}
	joined := strings.Join(lines, "\n")
	if e := check(strings.Contains(joined, "zone-auth ⇒truth domain-security · token TTL is 15m (domain)"), "analysis flows up one hop"); e != nil {
		return fail(e)
	}
	if e := check(strings.Contains(joined, "coord-core ⇐verdict domain-security · nobody pushes (policy)") && !strings.Contains(joined, "coord gossip"), "wisdom (policy) flows down; team does not"); e != nil {
		return fail(e)
	}
	if e := check(!strings.Contains(joined, "agent-one:") && !strings.Contains(joined, "ao:"), "no IRIs or prefixes in a projection"); e != nil {
		return fail(e)
	}
	coord, _ := w.Project("coord-core", 1000)
	if e := check(strings.Contains(strings.Join(coord, "\n"), "zone-auth ⇒truth domain-security ⇒verdict coord-core · token TTL is 15m (domain)"), "analysis flows up two hops"); e != nil {
		return fail(e)
	}
	small, _ := w.Project("domain-security", Tokens(lines[0]))
	if e := check(len(small) == 1 && small[0] == lines[0], "budget cuts nearest-first"); e != nil {
		return fail(e)
	}

	// a broken workspace
	if err := fixture(dir, true); err != nil {
		return passed, err
	}
	w, err = Load(dir)
	if err != nil {
		return passed, err
	}
	fs = w.Validate()
	shapes := map[string]bool{}
	for _, f := range fs {
		shapes[f.Shape] = true
	}
	if e := check(shapes["ZoneTruth"] && shapes["ZoneOwnership"] && shapes["SkillHeld"], "broken workspace: no-domain zone, overlapping ownership, unheld skill"); e != nil {
		return fail(e)
	}
	var out, errOut bytes.Buffer
	code := CLI([]string{"--root", dir, "check"}, &out, &errOut)
	if e := check(code == 1 && strings.HasPrefix(out.String(), "@S FAIL") && strings.Contains(out.String(), "@E "), "cli check answers on the wire"); e != nil {
		return fail(e)
	}
	return passed, nil
}
