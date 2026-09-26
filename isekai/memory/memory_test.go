package memory

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// fixture writes the selftest world (the same one memory.js's selftest builds) under dir.
func fixture(t *testing.T) (world, home string) {
	t.Helper()
	T := t.TempDir()
	world, home = filepath.Join(T, "world"), filepath.Join(T, "home")
	os.MkdirAll(home, 0o755)
	writeFile(filepath.Join(world, ".isekai", "name"), "fixture-world\n")
	writeFile(filepath.Join(world, ".isekai", "isekai.md"), "# Law\r\n\r\n## The gate\r\nNo change lands without its Orc's pass. The constructor word is here on purpose.\r\n\r\n## Laws\r\nVeldora's word is law.\r\n")
	writeFile(filepath.Join(world, ".isekai", "log.md"), "# Chronicle\n\n    ### [YYYY-MM-DD] format example, indented — not an entry\n\n### [2026-09-20 17:54] rimuru — World reincarnated\n- **Task:** /isekai\n\n### [2026-09-22T01:00:00+02:00] rimuru — Court body reports back over the wire\n- **Learned:** a court body reports back over the wire with @S @F @? @E lines only\n")
	writeFile(filepath.Join(world, ".isekai", "slime", "auth", "README.md"), "# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-api\n- **Purpose:** ground truth of the login zone; wears the wire mind\n\n## Traits\n- tokens expire after one hour\n\n## Thoughts\n\n### [2026-09-22]\nBorn by /genesis.\n")
	writeFile(filepath.Join(world, ".isekai", "orc", "api", "README.md"), "# orc-api\n\n- **Rank:** Orc\n- **Territory:** `src/api/`\n- **Reports to:** elf-core\n- **Purpose:** rules the api domain\n\n## Thoughts\n- 2026-09-22 gate verdicts on the auth zone\n")
	writeFile(filepath.Join(world, ".isekai", "canon", "notes.md"), "# Canon\n\n## Login tokens\nThe orc-api gate checks the login token zone owned by slime-auth.\n\n## Big\n"+strings.Repeat("lorem ipsum dolor ", 1200)+"\n")
	writeFile(filepath.Join(world, ".isekai", "tools", "x.sh"), "# x.sh — a tool header about the proving grounds\necho hi\n")
	writeFile(filepath.Join(world, ".claude", "skills", "wire", "SKILL.md"), "---\nname: wire\ndescription: how to speak the wire\n---\n# Wire\n\n## Report\nA court body reports back over the wire: @S opens, @F per finding, @? per hole, @E closes.\n\n## Thoughts\n- 2026-09-21 thought 1\n- 2026-09-22 thought 2\n")
	writeFile(filepath.Join(world, ".claude", "commands", "genesis.md"), "---\ndescription: Genesis — birth Elves, Orcs and Slimes from observed need\n---\n# /genesis\n\n## Steps\nScan, then birth.\n")
	writeFile(filepath.Join(world, "README.md"), "# fixture world\n\n## About\nA throwaway world for the memory tests.\n")
	return world, home
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
	if toks := Tokens("The court-body's @S line: memory.js, 2026, a1 -x- e.g."); !reflect.DeepEqual(toks, []string{"court-body", "@s", "line", "memory.js", "a1", "e.g"}) {
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
	if !NameIn("wears the wire mind", "wire") || NameIn("rewired", "wire") || NameIn("wire-tap", "wire") {
		t.Errorf("NameIn")
	}
}

func TestSectionsAndDesks(t *testing.T) {
	m := SectionMap("pre\n# A\ntext\n```\n## not a heading\n```\n## B ##\nmore\n")
	if m.Preamble != "pre" || len(m.Sections) != 2 || m.Sections[1].T != "B" || m.Sections[0].Text != "# A\ntext\n```\n## not a heading\n```" {
		t.Errorf("SectionMap: %+v", m)
	}
	if n := countThoughts("# x\n\n## Thoughts\n- 2026-09-01 a\n* 2026-09-02 b\n### [2026-09-03]\nc\n- no date\n\n## Next\n- 2026-09-04 d\n"); n != 3 {
		t.Errorf("countThoughts=%d want 3", n)
	}
}

func TestAPI(t *testing.T) {
	world, home := fixture(t)
	w, err := Open(world)
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
	r, err := w.Recall("how does a court body report back over the wire", RecallOpts{K: 3})
	if err != nil || r.S != "MISS" || len(r.Results) != 3 {
		t.Fatalf("recall: %+v %v", r, err)
	}
	if r2, _ := w.Recall("how does a court body report back over the wire", RecallOpts{K: 3}); r2.S != "HIT" || !reflect.DeepEqual(r2.Results, r.Results) {
		t.Fatalf("cache hit expected: %+v", r2)
	}
	if _, err := w.Recall("x", RecallOpts{Kind: "colony", Tier: "long"}); err == nil {
		t.Fatal("colony+long must FAIL")
	}
	if _, err := w.Recall("x", RecallOpts{Kind: "tribal"}); err == nil {
		t.Fatal("kind tribal must FAIL")
	}
	rm, err := w.Remember("login tokens expire hourly", RememberOpts{As: "slime-auth", Tag: "auth", Kind: "territory"})
	if err != nil || rm.Scope != "world" || *rm.Note.Kind != "territory" {
		t.Fatalf("remember: %+v %v", rm, err)
	}
	r, _ = w.Recall("login tokens", RecallOpts{As: "slime-auth"})
	var canon, note *Result
	for i := range r.Results {
		if strings.HasSuffix(r.Results[i].Src, "canon/notes.md") && canon == nil {
			canon = &r.Results[i]
		}
		if r.Results[i].Kind == "shared" {
			note = &r.Results[i]
		}
	}
	if canon == nil || canon.Rel < 0.25 || note == nil || note.Rel < 0.05 || note.Rec <= 0 {
		t.Fatalf("relation boosts: canon=%+v note=%+v", canon, note)
	}
	st := w.Status("slime-auth")
	if !st.Long.Indexed || st.Long.Stale || st.Shared.World != 1 || st.Shared.Unsaid.Territory != 1 || st.Short.WorkingMemory.Desks != 3 {
		t.Fatalf("status: %+v", st)
	}
	if f, _ := w.Forget("rimuru"); f.N != 1 {
		t.Fatalf("forget: %+v", f)
	}
	if strings.Contains(RdOr(filepath.Join(home, ".isekai", "shared", "notes.jsonl")), "login") {
		t.Fatal("a world note must not land in the machine tier")
	}
}

// ---- interop with memory.js

func nodeTool(t *testing.T, name string) (node, tool string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH — interop test skipped")
	}
	tool, _ = filepath.Abs(filepath.Join("..", "..", ".isekai", "tools", name))
	if !Exists(tool) {
		t.Skipf("%s not found — interop test skipped", tool)
	}
	return node, tool
}

func runJS(t *testing.T, node, tool, home, world string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(node, append([]string{tool, world}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return so.String() + se.String(), code
}

func runGo(world, home string, args ...string) (string, int) {
	var so, se bytes.Buffer
	code := cli(append([]string{world}, args...), &so, &se, home)
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

func TestInteropWithMemoryJS(t *testing.T) {
	node, tool := nodeTool(t, "memory.js")
	world, home := fixture(t)
	queries := [][]string{
		{"how does a court body report back over the wire", "-k", "3"},
		{"login tokens", "--as", "slime-auth"},
		{"the gate", "--kind", "semantic"},
		{"proving grounds", "--tier", "long", "-k", "2"},
		{"the of and"},
		{"anything", "--kind", "law"},
	}
	compare := func(label string, args ...string) {
		t.Helper()
		js, jc := runJS(t, node, tool, home, world, args...)
		gо, gc := runGo(world, home, args...)
		if jc != gc {
			t.Errorf("%s %v: exit js=%d go=%d\n--- js\n%s--- go\n%s", label, args, jc, gc, js, gо)
			return
		}
		if strings.Contains(strings.Join(args, " "), "--json") {
			if !JSONEqual(lastLine(js), lastLine(gо), 2e-4) {
				t.Errorf("%s %v: json differs\n--- js\n%s--- go\n%s", label, args, js, gо)
			}
		} else if !WireEqual(js, gо, 2e-4) {
			t.Errorf("%s %v: wire differs\n--- js\n%s--- go\n%s", label, args, js, gо)
		}
	}
	// JS builds the index; the Go port must read it and rank identically
	if out, code := runJS(t, node, tool, home, world, "index"); code != 0 {
		t.Fatalf("js index: %s", out)
	}
	for _, q := range queries {
		compare("js-index", append(append([]string{"recall"}, q...), "--no-cache")...)
		compare("js-index", append(append([]string{"recall"}, q...), "--no-cache", "--json")...)
	}
	compare("js-index", "status")
	// Go rebuilds the index; the JS must read it, see it fresh, and rank identically
	if out, code := runGo(world, home, "index"); code != 0 {
		t.Fatalf("go index: %s", out)
	}
	js, _ := runJS(t, node, tool, home, world, "status", "--json")
	var st map[string]any
	json.Unmarshal([]byte(lastLine(js)), &st)
	if long := st["long"].(map[string]any); long["indexed"] != true || long["stale"] != false {
		t.Fatalf("js does not accept the Go index as fresh: %s", js)
	}
	for _, q := range queries {
		compare("go-index", append(append([]string{"recall"}, q...), "--no-cache")...)
	}
	compare("go-index", "status")
	// notes cross both ways
	runGo(world, home, "remember", "login tokens expire hourly", "--as", "slime-auth", "--tag", "auth", "--kind", "territory")
	runJS(t, node, tool, home, world, "remember", "the gate is run twice in practice, once before lunch", "--as", "orc-api", "--kind", "colony")
	compare("notes", "recall", "login tokens", "--as", "slime-auth", "--no-cache", "--json")
	compare("notes", "recall", "gate lunch tokens", "--kind", "colony", "--no-cache")
	compare("notes", "recall", "login tokens", "--tier", "shared", "--no-cache")
	compare("notes", "status")
	// the semantic cache crosses both ways: a JS recall is a Go HIT with the same results, and back
	q := []string{"recall", "how does a court body report back over the wire", "-k", "3", "--json"}
	jsMiss, _ := runJS(t, node, tool, home, world, q...)
	goHit, _ := runGo(world, home, q...)
	var jm, gh map[string]any
	json.Unmarshal([]byte(lastLine(jsMiss)), &jm)
	json.Unmarshal([]byte(lastLine(goHit)), &gh)
	if jm["@S"] != "MISS" || gh["@S"] != "HIT" || !reflect.DeepEqual(jm["results"], gh["results"]) {
		t.Errorf("cache js→go: %s\n%s", jsMiss, goHit)
	}
	q2 := []string{"recall", "login tokens", "--as", "orc-api", "--json"}
	goMiss, _ := runGo(world, home, q2...)
	jsHit, _ := runJS(t, node, tool, home, world, q2...)
	json.Unmarshal([]byte(lastLine(goMiss)), &gh)
	json.Unmarshal([]byte(lastLine(jsHit)), &jm)
	if gh["@S"] != "MISS" || jm["@S"] != "HIT" || !reflect.DeepEqual(jm["results"], gh["results"]) {
		t.Errorf("cache go→js: %s\n%s", goMiss, jsHit)
	}
	compare("cache", "status", "--as", "orc-api")
	runGo(world, home, "recall", "login tokens", "--as", "body-z")
	if out, _ := runJS(t, node, tool, home, world, "forget", "--short", "--as", "body-z"); out != "@S FORGOT 1 cached recall for body-z\n" {
		t.Errorf("js forgets a Go-written cache: %s", out)
	}
	runJS(t, node, tool, home, world, "recall", "login tokens", "--as", "body-z")
	if out, _ := runGo(world, home, "forget", "--short", "--as", "body-z"); out != "@S FORGOT 1 cached recall for body-z\n" {
		t.Errorf("go forgets a JS-written cache: %s", out)
	}
	// FAILs agree
	for _, bad := range [][]string{{"recall", "x", "-k", "0"}, {"recall", "x", "--kind", "colony", "--tier", "long"}, {"remember", "x", "--kind", "tribal"}, {"forget"}, {"nope"}} {
		compare("fail", bad...)
	}
}
