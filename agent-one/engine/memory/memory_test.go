package memory

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// fixture writes the selftest workspace (the same one memory.js's selftest builds) under dir.
func fixture(t *testing.T) (workspace, home string) {
	t.Helper()
	T := t.TempDir()
	workspace, home = filepath.Join(T, "workspace"), filepath.Join(T, "home")
	os.MkdirAll(home, 0o755)
	writeFile(filepath.Join(workspace, ".agent-one", "name"), "fixture-workspace\n")
	writeFile(filepath.Join(workspace, ".agent-one", "AGENT-ONE.md"), "# Policy\r\n\r\n## The gate\r\nNo change lands without its Domain owner's pass. The constructor word is here on purpose.\r\n\r\n## Policies\r\nOperator's word is policy.\r\n")
	writeFile(filepath.Join(workspace, ".agent-one", "log.md"), "# Chronicle\n\n    ### [YYYY-MM-DD] format example, indented — not an entry\n\n### [2026-09-20 17:54] orchestrator — Workspace onboarded\n- **Task:** /agent-one\n\n### [2026-09-22T01:00:00+02:00] orchestrator — Subagent agent reports back over the wire\n- **Learned:** a ephemeral subagent reports back over the wire with @S @F @? @E lines only\n")
	writeFile(filepath.Join(workspace, ".agent-one", "zone", "auth", "README.md"), "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-api\n- **Purpose:** ground truth of the login zone; holds the wire skill\n\n## Invariants\n- tokens expire after one hour\n\n## Working notes\n\n### [2026-09-22]\nProvisioned by /provision.\n")
	writeFile(filepath.Join(workspace, ".agent-one", "domain", "api", "README.md"), "# domain-api\n\n- **Rank:** Domain owner\n- **Owns:** `src/api/`\n- **Reports to:** coord-core\n- **Purpose:** rules the api domain\n\n## Working notes\n- 2026-09-22 gate verdicts on the auth zone\n")
	writeFile(filepath.Join(workspace, ".agent-one", "design", "notes.md"), "# Design\n\n## Login tokens\nThe domain-api gate checks the login token zone owned by zone-auth.\n\n## Big\n"+strings.Repeat("lorem ipsum dolor ", 1200)+"\n")
	writeFile(filepath.Join(workspace, ".agent-one", "tools", "x.sh"), "# x.sh — a tool header about the sandbox\necho hi\n")
	writeFile(filepath.Join(workspace, ".claude", "skills", "wire", "SKILL.md"), "---\nname: wire\ndescription: how to speak the wire\n---\n# Wire\n\n## Report\nA ephemeral subagent reports back over the wire: @S opens, @F per finding, @? per hole, @E closes.\n\n## Working notes\n- 2026-09-21 note 1\n- 2026-09-22 note 2\n")
	writeFile(filepath.Join(workspace, ".claude", "commands", "provisioning.md"), "---\ndescription: Provisioning — provisioning Coordinators, Domain owners and Zone workers from observed need\n---\n# /provision\n\n## Steps\nScan, then provisioning.\n")
	writeFile(filepath.Join(workspace, "README.md"), "# fixture workspace\n\n## About\nA throwaway workspace for the memory tests.\n")
	return workspace, home
}

func TestSelftest(t *testing.T) {
	var out bytes.Buffer
	n, err := SelftestIn(t.TempDir(), &out)
	if err != nil {
		t.Fatalf("selftest: %v\n%s", err, out.String())
	}
	if !strings.HasPrefix(out.String(), "@S PASS") || n < 40 {
		t.Fatalf("selftest output: %s", out.String())
	}
}

var tenth, fifth = 0.1, 0.2

func TestJSHelpers(t *testing.T) {
	for _, c := range []struct {
		x float64
		d int
		s string
	}{{0.03125, 4, "0.0313"}, {1.005, 2, "1.00"}, {0.5, 4, "0.5000"}, {2, 0, "2"}, {0.15 + 0.10, 2, "0.25"}} {
		if got := ToFixedStr(c.x, c.d); got != c.s {
			t.Errorf("ToFixedStr(%v,%d)=%s want %s", c.x, c.d, got, c.s)
		}
	}
	for _, c := range []struct {
		x float64
		s string
	}{{1e-7, "1e-7"}, {1e21, "1e+21"}, {tenth + fifth, "0.30000000000000004"}, {3, "3"}, {0.5, "0.5"}} {
		if got := JSNum(c.x); got != c.s {
			t.Errorf("JSNum(%v)=%s want %s", c.x, got, c.s)
		}
	}
	want := []string{"a", "a_b", "a-b", "a1", "abc", "b", "B", "README.md"}
	got := []string{"b", "a", "B", "a-b", "a_b", "a1", "README.md", "abc"}
	for i := range got {
		for j := i + 1; j < len(got); j++ {
			if LocaleCompare(got[i], got[j]) > 0 {
				got[i], got[j] = got[j], got[i]
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LocaleCompare order %v want %v", got, want)
	}
	if toks := Tokens("The ephemeral-subagent's @S line: memory.js, 2026, a1 -x- e.g."); !reflect.DeepEqual(toks, []string{"ephemeral-subagent", "@s", "line", "memory.js", "a1", "e.g"}) {
		t.Errorf("Tokens: %v", toks)
	}
	if Locale(1234567) != "1,234,567" || Locale(12) != "12" {
		t.Errorf("Locale: %s %s", Locale(1234567), Locale(12))
	}
	if n, ok := JSParseInt("3abc"); !ok || n != 3 {
		t.Errorf("JSParseInt")
	}
	if _, ok := JSParseInt(""); ok {
		t.Errorf("JSParseInt('') must be NaN")
	}
	if FirstSentence("Push a release. Watch it roll.") != "Push a release." || FirstSentence("e.g. a thing and so on") != "e.g. a thing and so on" {
		t.Errorf("FirstSentence")
	}
	if !NameIn("holds the wire skill", "wire") || NameIn("rewired", "wire") || NameIn("wire-tap", "wire") {
		t.Errorf("NameIn")
	}
}

func TestSectionsAndWorkingNotes(t *testing.T) {
	m := SectionMap("pre\n# A\ntext\n```\n## not a heading\n```\n## B ##\nmore\n")
	if m.Preamble != "pre" || len(m.Sections) != 2 || m.Sections[1].T != "B" || m.Sections[0].Text != "# A\ntext\n```\n## not a heading\n```" {
		t.Errorf("SectionMap: %+v", m)
	}
	if n := countNotes("# x\n\n## Working notes\n- 2026-09-01 a\n* 2026-09-02 b\n### [2026-09-03]\nc\n- no date\n\n## Next\n- 2026-09-04 d\n"); n != 3 {
		t.Errorf("countNotes=%d want 3", n)
	}
}

func TestAPI(t *testing.T) {
	workspace, home := fixture(t)
	w, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	w.Home = home
	if _, why := w.LoadIndex(); why == "" {
		t.Fatal("no index must be a hole")
	}
	ir, err := w.Index()
	if err != nil || ir.Episodic != 2 || ir.Procedural != 6 || ir.Semantic < 7 {
		t.Fatalf("index: %+v %v", ir, err)
	}
	r, err := w.Recall("how does a ephemeral subagent report back over the wire", RecallOpts{K: 3})
	if err != nil || r.S != "MISS" || len(r.Results) != 3 {
		t.Fatalf("recall: %+v %v", r, err)
	}
	if r2, _ := w.Recall("how does a ephemeral subagent report back over the wire", RecallOpts{K: 3}); r2.S != "HIT" || !reflect.DeepEqual(r2.Results, r.Results) {
		t.Fatalf("cache hit expected: %+v", r2)
	}
	if _, err := w.Recall("x", RecallOpts{Kind: "team", Tier: "long"}); err == nil {
		t.Fatal("team+long must FAIL")
	}
	if _, err := w.Recall("x", RecallOpts{Kind: "tribal"}); err == nil {
		t.Fatal("kind tribal must FAIL")
	}
	rm, err := w.Remember("login tokens expire hourly", RememberOpts{As: "zone-auth", Tag: "auth", Kind: "domain"})
	if err != nil || rm.Scope != "workspace" || *rm.Note.Kind != "domain" {
		t.Fatalf("remember: %+v %v", rm, err)
	}
	r, _ = w.Recall("login tokens", RecallOpts{As: "zone-auth"})
	var design, note *Result
	for i := range r.Results {
		if strings.HasSuffix(r.Results[i].Src, "design/notes.md") && design == nil {
			design = &r.Results[i]
		}
		if r.Results[i].Kind == "shared" {
			note = &r.Results[i]
		}
	}
	if design == nil || design.Rel < 0.25 || note == nil || note.Rel < 0.05 || note.Rec <= 0 {
		t.Fatalf("relation boosts: design=%+v note=%+v", design, note)
	}
	st := w.Status("zone-auth")
	if !st.Long.Indexed || st.Long.Stale || st.Shared.Workspace != 1 || st.Shared.Unsaid.Domain != 1 || st.Short.WorkingMemory.WorkingNotes != 3 {
		t.Fatalf("status: %+v", st)
	}
	if f, _ := w.Forget("orchestrator"); f.N != 1 {
		t.Fatalf("forget: %+v", f)
	}
	if strings.Contains(RdOr(filepath.Join(home, ".agent-one", "shared", "notes.jsonl")), "login") {
		t.Fatal("a workspace note must not land in the machine tier")
	}
}

// ---- interop with memory.js

func runGo(workspace, home string, args ...string) (string, int) {
	var so, se bytes.Buffer
	code := cli(append([]string{workspace}, args...), &so, &se, home)
	return so.String() + se.String(), code
}

// WireEqual compares two wire texts token by token; numbers within tol are equal (the recency
// prior moves by ~1e-8 per second between the two runs, so a 4-decimal score may flip a digit).
func WireEqual(a, b string, tol float64) bool {
	ta, tb := strings.Fields(a), strings.Fields(b)
	if len(ta) != len(tb) {
		return false
	}
	num := func(s string) (float64, bool) {
		s = strings.Trim(s, "(),")
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	for i := range ta {
		if ta[i] == tb[i] {
			continue
		}
		fa, oka := num(ta[i])
		fb, okb := num(tb[i])
		if !oka || !okb || math.Abs(fa-fb) > tol {
			return false
		}
	}
	return true
}

// JSONEqual compares two JSON documents structurally with a numeric tolerance.
func JSONEqual(a, b string, tol float64) bool {
	var va, vb any
	if json.Unmarshal([]byte(strings.TrimSpace(a)), &va) != nil || json.Unmarshal([]byte(strings.TrimSpace(b)), &vb) != nil {
		return false
	}
	var eq func(x, y any) bool
	eq = func(x, y any) bool {
		switch xv := x.(type) {
		case map[string]any:
			yv, ok := y.(map[string]any)
			if !ok || len(xv) != len(yv) {
				return false
			}
			for k := range xv {
				if _, ok := yv[k]; !ok || !eq(xv[k], yv[k]) {
					return false
				}
			}
			return true
		case []any:
			yv, ok := y.([]any)
			if !ok || len(xv) != len(yv) {
				return false
			}
			for i := range xv {
				if !eq(xv[i], yv[i]) {
					return false
				}
			}
			return true
		case float64:
			yv, ok := y.(float64)
			return ok && math.Abs(xv-yv) <= tol
		}
		return reflect.DeepEqual(x, y)
	}
	return eq(va, vb)
}

func lastLine(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	return l[len(l)-1]
}
