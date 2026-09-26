package onto

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fixture writes a small world under dir: one elf, one orc, two slimes, a
// worn mind and an unworn one. broken adds a slime with no orc and an
// overlapping territory.
func fixture(dir string, broken bool) error {
	files := map[string]string{
		".isekai/isekai.md":                  "# law\n",
		".isekai/elf/core/README.md":         "# elf-core\n\n- **Rank:** Elf\n- **Territory:** `src/`\n- **Purpose:** the shared mind\n",
		".isekai/orc/security/README.md":     "# orc-security\n\n- **Rank:** Orc\n- **Territory:** `src/auth/`, `src/api/`\n- **Reports to:** elf-core\n- **Purpose:** rules the security domain; wears great-sage\n",
		".isekai/slime/auth/README.md":       "# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-security\n- **Minds:** ciel\n\n## Traits\n- tokens expire after one hour\n",
		".isekai/slime/api/README.md":        "# slime-api\n\n- **Rank:** Slime\n- **Territory:** `src/api/`\n- **Orc:** orc-security\n",
		".claude/skills/great-sage/SKILL.md": "---\nname: great-sage\n---\nreads\n",
		".claude/skills/ciel/SKILL.md":       "---\nname: ciel\n---\ndrafts\n",
		".claude/skills/raphael/SKILL.md":    "---\nname: raphael\n---\nverdicts, worn by nobody here\n",
	}
	if broken {
		files[".isekai/slime/orphan/README.md"] = "# slime-orphan\n\n- **Rank:** Slime\n- **Territory:** `src/auth/login/`\n"
	}
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Selftest runs the in-binary checks on a throwaway world and answers how
// many passed; err names the first failure.
func Selftest() (passed int, err error) {
	dir, err := os.MkdirTemp("", "isekai-onto-")
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
	pre, perr := Parse("t.ttl", "@prefix is: <isekai:> .\nis:x a is:Slime ; is:owns \"a\", \"b\" ; is:n 15 ; is:t \"hi\"@en .", g, nil)
	if e := check(perr == nil && pre["is"] == NS && g.Len() == 5, "parse a small document"); e != nil {
		return fail(e)
	}
	_, perr = Parse("t.ttl", "@prefix is: <isekai:> .\nis:x a is:Slime\nis:y a is:Orc .", New(), nil)
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
	sg.Add(Triple{Is("s"), pTruth, Is("o")})
	sg.Add(Triple{Is("o"), pVerdict, Is("e")})
	Infer(sg)
	if e := check(sg.Has(Triple{Is("s"), rdfType, cSlime}) && sg.Has(Triple{Is("o"), rdfType, cCreature}), "domain, range and subclass typing"); e != nil {
		return fail(e)
	}
	if e := check(sg.Has(Triple{Is("s"), pAbove, Is("e")}) && sg.Has(Triple{Is("e"), pBelow, Is("s")}), "subproperty, transitive and inverse closure"); e != nil {
		return fail(e)
	}
	n1 := sg.Len()
	Infer(sg)
	if e := check(sg.Len() == n1, "reasoning is at a fixpoint"); e != nil {
		return fail(e)
	}

	// a healthy world
	if err := fixture(dir, false); err != nil {
		return passed, err
	}
	w, err := Load(dir)
	if err != nil {
		return passed, err
	}
	if e := check(len(w.Graph.Instances(cSlime)) == 2 && w.Graph.Has(Triple{Is("slime-auth"), pTruth, Is("orc-security")}) && w.Graph.Has(Triple{Is("slime-api"), pTruth, Is("orc-security")}), "creatures and truth bonds derived from docs"); e != nil {
		return fail(e)
	}
	if e := check(w.Graph.Has(Triple{Is("orc-security"), pVerdict, Is("elf-core")}) && w.Graph.Has(Triple{Is("elf-core"), pReports, tRimuru}), "verdict and reports bonds"); e != nil {
		return fail(e)
	}
	if e := check(w.Graph.Has(Triple{Is("slime-auth"), pWears, Is("mind-ciel")}) && w.Graph.Has(Triple{Is("orc-security"), pWears, Is("mind-great-sage")}), "worn minds from field and prose"); e != nil {
		return fail(e)
	}
	if e := check(w.Graph.Has(Triple{Is("slime-auth"), pOwns, L("src/auth")}), "territory from the doc"); e != nil {
		return fail(e)
	}
	fs := w.Validate()
	if e := check(len(fs) == 1 && fs[0].Shape == "MindWorn" && fs[0].Subject == Is("mind-raphael"), "healthy world: only the unworn mind is a finding"); e != nil {
		return fail(e)
	}
	if e := check(len(w.Graph.Asserted()) > 0 && !w.Graph.Derived(w.Graph.Asserted()[0]) && w.Graph.Derived(Triple{Is("slime-auth"), pTruth, Is("orc-security")}), "derived triples are marked, never asserted"); e != nil {
		return fail(e)
	}

	// the unsaid, and the flow
	Now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }
	defer func() { Now = time.Now }()
	if _, err := w.AssertUnsaid("slime-auth", "territory", "token TTL is 15m"); err != nil {
		return passed, err
	}
	if _, err := w.AssertUnsaid("elf-core", "law", "nobody pushes"); err != nil {
		return passed, err
	}
	if _, err := w.AssertUnsaid("elf-core", "colony", "elf gossip"); err != nil {
		return passed, err
	}
	unsaid, _ := os.ReadFile(filepath.Join(OntologyDir(dir), "graph", "unsaid.ttl"))
	if e := check(strings.Count(string(unsaid), "is:knows") == 3 && strings.HasPrefix(string(unsaid), "#"), "unsaid.ttl is appended, header once"); e != nil {
		return fail(e)
	}
	w, err = Load(dir)
	if err != nil {
		return passed, err
	}
	lines, err := w.Project("orc-security", 1000)
	if err != nil {
		return passed, err
	}
	joined := strings.Join(lines, "\n")
	if e := check(strings.Contains(joined, "slime-auth ⇒truth orc-security · token TTL is 15m (territory)"), "analysis flows up one hop"); e != nil {
		return fail(e)
	}
	if e := check(strings.Contains(joined, "elf-core ⇐verdict orc-security · nobody pushes (law)") && !strings.Contains(joined, "elf gossip"), "wisdom (law) flows down; colony does not"); e != nil {
		return fail(e)
	}
	if e := check(!strings.Contains(joined, "isekai:") && !strings.Contains(joined, "is:"), "no IRIs or prefixes in a projection"); e != nil {
		return fail(e)
	}
	elf, _ := w.Project("elf-core", 1000)
	if e := check(strings.Contains(strings.Join(elf, "\n"), "slime-auth ⇒truth orc-security ⇒verdict elf-core · token TTL is 15m (territory)"), "analysis flows up two hops"); e != nil {
		return fail(e)
	}
	small, _ := w.Project("orc-security", Tokens(lines[0]))
	if e := check(len(small) == 1 && small[0] == lines[0], "budget cuts nearest-first"); e != nil {
		return fail(e)
	}

	// a broken world
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
	if e := check(shapes["SlimeTruth"] && shapes["SlimeTerritory"] && shapes["MindWorn"], "broken world: no-orc slime, overlapping territory, unworn mind"); e != nil {
		return fail(e)
	}
	var out, errOut bytes.Buffer
	code := CLI([]string{"--root", dir, "check"}, &out, &errOut)
	if e := check(code == 1 && strings.HasPrefix(out.String(), "@S FAIL") && strings.Contains(out.String(), "@E "), "cli check answers on the wire"); e != nil {
		return fail(e)
	}
	return passed, nil
}
