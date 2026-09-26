package toolbox

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

	"github.com/Kaginari/agentic-playground/isekai/memory"
)

// fixture writes the selftest world (the one toolbox.js's selftest builds) under a temp dir.
func fixture(t *testing.T) (world string) {
	t.Helper()
	W := filepath.Join(t.TempDir(), "world")
	writeFile(filepath.Join(W, ".isekai", "name"), "fixture-world\n")
	writeFile(filepath.Join(W, ".isekai", "isekai.md"), "# Law\n\n## Laws\nVeldora's word is law.\n")
	writeFile(filepath.Join(W, ".isekai", "slime", "auth", "README.md"), "# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-api\n- **Purpose:** ground truth of the login zone; wears the wire mind\n\n## Thoughts\n- 2026-09-22 born\n")
	writeFile(filepath.Join(W, ".isekai", "orc", "api", "README.md"), "# orc-api\n\n- **Rank:** Orc\n- **Territory:** `src/api/`\n- **Reports to:** elf-core\n- **Purpose:** rules the api domain\n")
	writeFile(filepath.Join(W, ".claude", "skills", "wire", "SKILL.md"), "---\nname: wire\ndescription: How to speak the wire between machine mouths.\n---\n# Wire\n\n## Report\nA court body reports back over the wire. BODYSENTINEL-WIRE\n\n## Hygiene\nNo greetings.\n")
	writeFile(filepath.Join(W, ".claude", "skills", "deploy-ship", "SKILL.md"), "---\nname: deploy-ship\ndescription: Push a release to the production cluster and watch the rollout.\ntriggers:\n  - ship it\n  - rollout\n---\n# Deploy\n\nBODYSENTINEL-DEPLOY\n")
	writeFile(filepath.Join(W, ".claude", "skills", "standards-check", "SKILL.md"), "---\nname: standards-check\ndescription: \"Review the branch changes against the repo coding standards and report findings. Use when asked to \\\"review since X\\\".\"\n---\n# Review\n\n## Standards\nBODYSENTINEL-REVIEW standards section.\n\n## Spec\nSpec section text.\n\n## Report\nReport section text.\n")
	writeFile(filepath.Join(W, ".opencode", "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: Lives in both homes; one entry, two sources.\n---\n# Dup\n")
	writeFile(filepath.Join(W, ".claude", "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: Lives in both homes; one entry, two sources.\n---\n# Dup\n")
	heavy := "Render slides and decks to pdf; check a slide; export the deck. " + strings.Repeat("Slides decks pdf render export slide deck check. ", 40)
	writeFile(filepath.Join(W, ".claude", "skills", "big", "SKILL.md"), "---\nname: big\ndescription: \""+heavy+"\"\n---\n# Big\n")
	writeFile(filepath.Join(W, ".claude", "skills", "cheap", "SKILL.md"), "---\nname: cheap\ndescription: Render a deck.\n---\n# Cheap\n")
	writeFile(filepath.Join(W, ".claude", "skills", "mid", "SKILL.md"), "---\nname: mid\ndescription: >\n  Render slides and decks to pdf and check\n  every slide of the deck before export.\n---\n# Mid\n")
	writeFile(filepath.Join(W, ".claude", "commands", "genesis.md"), "---\ndescription: Genesis — birth Elves, Orcs and Slimes from observed need\n---\n# /genesis\n")
	writeFile(filepath.Join(W, ".opencode", "commands", "isekai.md"), "---\ndescription: Reincarnate a directory as a living world\n---\n# /isekai\n")
	writeFile(filepath.Join(W, ".isekai", "tools", "x.sh"), "#!/bin/bash\n# x.sh — proving-grounds helper: runs the world's tests under .isekai/tmp/.\n#\n# Usage: x.sh\necho hi\n")
	writeFile(filepath.Join(W, ".isekai", "tools", "memo.js"), "#!/usr/bin/env node\n// memo.js — the memory instrument: remember a note, recall by meaning.\n// Second header line.\n'use strict';\n\nconsole.log(1)\n")
	writeFile(filepath.Join(W, ".isekai", "tools", "block.js"), "/**\n * block.js — a tool with a block header.\n * Second line.\n */\nconsole.log(1)\n")
	writeFile(filepath.Join(W, ".claude", "agents", "orc-api.md"), "---\nname: orc-api\ndescription: Orc of the api domain — holds the gate for slime-auth.\nmode: subagent\nmodel: sonnet\n---\n# orc-api\n\nBODYSENTINEL-ORC\n")
	writeFile(filepath.Join(W, "bin", "soffice"), "#!/bin/sh\necho fake\n")
	os.Chmod(filepath.Join(W, "bin", "soffice"), 0o755)
	l1, _ := memory.MarshalJS(memory.OJ{memory.P("kind", "external"), memory.P("name", "soffice"), memory.P("description", "LibreOffice headless: converts a pptx deck to pdf."), memory.P("triggers", []string{"pdf", "libreoffice"}), memory.P("cost", memory.OJ{memory.P("resident", 30), memory.P("full", 60)}), memory.P("usage", "soffice --headless --convert-to pdf <file>")})
	l2, _ := memory.MarshalJS(memory.OJ{memory.P("kind", "external"), memory.P("name", "ollama"), memory.P("path", filepath.Join(W, "nowhere", "ollama")), memory.P("description", "Local embedding model server."), memory.P("triggers", []string{"embedding"}), memory.P("installed", true)})
	writeFile(filepath.Join(W, ".isekai", "toolbox", "extra.jsonl"), string(l1)+"\n"+string(l2)+"\n{not json}\n")
	return W
}

func TestSelftest(t *testing.T) {
	var out bytes.Buffer
	n, err := SelftestIn(t.TempDir(), &out)
	if err != nil {
		t.Fatalf("selftest: %v\n%s", err, out.String())
	}
	if !strings.HasPrefix(out.String(), "@S PASS") || n < 45 {
		t.Fatalf("selftest output: %s", out.String())
	}
}

func TestFrontmatterAndHeader(t *testing.T) {
	fm, body := Frontmatter("---\nname: x\ndescription: >\n  folded line\n  second\ntriggers:\n  - \"ship it\"\n  - rollout\nwhen: a, b\n---\n# T\nbody\n")
	if fm["description"] != "folded line second" || body != "# T\nbody\n" || !reflect.DeepEqual(fm["triggers"], []string{"ship it", "rollout"}) || !reflect.DeepEqual(asList(fm["when"]), []string{"a", "b"}) {
		t.Errorf("frontmatter: %#v body=%q", fm, body)
	}
	if fm, body := Frontmatter("---\n---\nx"); body != "---\n---\nx" || len(fm) != 0 {
		t.Errorf("a fence with nothing between is no frontmatter in the JS either: body=%q", body)
	}
	if fm, body := Frontmatter("no fm\n---\nx"); len(fm) != 0 || body != "no fm\n---\nx" {
		t.Errorf("no frontmatter")
	}
	if h := HeaderOf("#!/usr/bin/env node\n// a.js — does a thing.\n// more\n'use strict';\nconst x = 1;\n// not header\n"); h != "a.js — does a thing.\nmore" {
		t.Errorf("HeaderOf line: %q", h)
	}
	if h := HeaderOf("/**\n * b.js — block.\n * two */\ncode\n"); h != "b.js — block.\ntwo" {
		t.Errorf("HeaderOf block: %q", h)
	}
	if stem("rendering") != "render" || stem("slides") != "slid" || stem("bus") != "bus" || stem("tested") != "test" {
		t.Errorf("stem")
	}
	tr := triggersOf("code-review", map[string]any{}, `Review the changes. Use when asked to "review since X".`)
	if len(tr) != 3 || tr[1].T != "code review" || tr[2].T != "review since X" || tr[2].Src != "quoted" {
		t.Errorf("triggersOf: %+v", tr)
	}
}

func TestAPI(t *testing.T) {
	W := fixture(t)
	w, err := Open(W)
	if err != nil {
		t.Fatal(err)
	}
	w.Path = filepath.Join(W, "bin")
	if Rg := w.LoadRegistry(); !Rg.Live || Rg.Why == "" {
		t.Fatal("no registry must be answered live and named")
	}
	ir, err := w.Index()
	if err != nil || ir.Reg.N != 15 || ir.ByKind["mind"] != 7 || ir.ByKind["tool"] != 3 || ir.ByKind["external"] != 2 {
		t.Fatalf("index: %+v %v", ir.ByKind, err)
	}
	e, _, err := w.Explain("mid", "")
	if err != nil || e.Description != "Render slides and decks to pdf and check every slide of the deck before export." {
		t.Fatalf("folded description: %+v %v", e, err)
	}
	if e, _, _ := w.Explain("block.js", ""); e == nil || e.Description != "a tool with a block header. Second line. /" {
		t.Fatalf("block header description: %+v", e)
	}
	b, err := w.Brief("speak on the wire, then ship it", PickOpts{As: "slime-auth"})
	if err != nil || len(b.Picks) < 2 || b.Picks[0].Name != "wire" || !strings.HasPrefix(b.Head, "@TOOLS as=slime-auth k=") {
		t.Fatalf("brief: %+v %v", b, err)
	}
	for _, l := range b.Lines {
		if strings.Contains(l, "BODYSENTINEL") {
			t.Fatal("a body crossed in level 1")
		}
	}
	lr, err := w.Load("standards-check", LoadOpts{As: "orc-api", Sec: intPtr(3)})
	if err != nil || lr.Text != "## Spec\nSpec section text.\n" || lr.Sec != 3 {
		t.Fatalf("load sec: %+v %v", lr, err)
	}
	if _, err := w.Load("standards-check", LoadOpts{Sec: intPtr(9)}); err == nil {
		t.Fatal("sec past the end must FAIL")
	}
	st := w.Status(BudgetDefault)
	if st.Loaded.Live || st.Stale || st.Ins.Loads != 1 || st.Ins.Offers != 1 || st.Ext != 2 || len(st.Missing) != 1 {
		t.Fatalf("status: %+v", st)
	}
	if _, err := w.DoPick("x", PickOpts{Kinds: []string{"hat"}}); err == nil {
		t.Fatal("kind hat must FAIL")
	}
	if _, err := w.DoPick("x", PickOpts{Min: math.NaN()}); err == nil {
		t.Fatal("NaN min must FAIL")
	}
}

func intPtr(n int) *int { return &n }

// ---- interop with toolbox.js

func runJS(t *testing.T, node, tool, pathEnv, world string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(node, append([]string{tool, world}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+pathEnv)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return so.String() + se.String(), code
}

func runGo(world, pathEnv string, args ...string) (string, int) {
	var so, se bytes.Buffer
	code := cli(append([]string{world}, args...), &so, &se, pathEnv)
	return so.String() + se.String(), code
}

func jsonEqual(a, b string, tol float64) bool {
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

// stripAt drops the `at` stamps from a JSON document before comparing.
func stripAt(s string) string {
	var v map[string]any
	if json.Unmarshal([]byte(lastLine(s)), &v) != nil {
		return s
	}
	delete(v, "at")
	if reg, ok := v["registry"].(map[string]any); ok {
		delete(reg, "builtAt")
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestInteropWithToolboxJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH — interop test skipped")
	}
	tool, _ := filepath.Abs(filepath.Join("..", "..", ".isekai", "tools", "toolbox.js"))
	if !memory.Exists(tool) {
		t.Skipf("%s not found — interop test skipped", tool)
	}
	W := fixture(t)
	// the world ships the JS instrument, as this one does: both mouths then name it in their hints
	src, err := os.ReadFile(tool)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(filepath.Join(W, ".isekai", "tools", "toolbox.js"), string(src))
	pathEnv := filepath.Join(W, "bin") // soffice present, git absent — for both mouths
	compare := func(label string, args ...string) {
		t.Helper()
		js, jc := runJS(t, node, tool, pathEnv, W, args...)
		gо, gc := runGo(W, pathEnv, args...)
		if jc != gc {
			t.Errorf("%s %v: exit js=%d go=%d\n--- js\n%s--- go\n%s", label, args, jc, gc, js, gо)
			return
		}
		if strings.Contains(strings.Join(args, " "), "--json") {
			if !jsonEqual(stripAt(js), stripAt(gо), 1e-9) {
				t.Errorf("%s %v: json differs\n--- js\n%s--- go\n%s", label, args, js, gо)
			}
		} else if js != gо {
			t.Errorf("%s %v: wire differs\n--- js\n%s--- go\n%s", label, args, js, gо)
		}
	}
	asks := [][]string{
		{"review the branch changes against the coding standards, then ship it"},
		{"render the deck to pdf and check slide 8", "--budget", "100000", "-k", "10"},
		{"render the deck to pdf and check slide 8", "--budget", "120", "-k", "10"},
		{"speak on the wire", "--as", "slime-auth"},
		{"the api gate", "--as", "slime-auth", "--kind", "body"},
		{"leave a note"},
		{"embedding server"},
		{"the of and"},
	}
	// no registry: both answer from a live harvest
	compare("live", "pick", "speak on the wire", "--json")
	// JS builds the registry; the Go port reads it and picks identically
	if out, code := runJS(t, node, tool, pathEnv, W, "index"); code != 0 {
		t.Fatalf("js index: %s", out)
	}
	for _, a := range asks {
		compare("js-registry", append([]string{"pick"}, a...)...)
		compare("js-registry", append(append([]string{"pick"}, a...), "--json")...)
		compare("js-registry", append([]string{"brief"}, a...)...)
		compare("js-registry", append(append([]string{"brief"}, a...), "--json")...)
	}
	for _, n := range []string{"wire", "dup", "standards-check", "x.sh", "memo", "block.js", "orc-api", "soffice", "ollama"} {
		compare("js-registry", "explain", n)
		compare("js-registry", "explain", n, "--json")
		compare("js-registry", "load", n)
		compare("js-registry", "load", n, "--map")
		compare("js-registry", "load", n, "--map", "--json")
	}
	compare("js-registry", "load", "standards-check", "--sec", "3", "--as", "orc-api", "--json")
	compare("js-registry", "load", "standards-check", "--sec", "0")
	compare("js-registry", "status")
	compare("js-registry", "status", "--json")
	compare("js-registry", "status", "--budget", "0")
	// Go rebuilds the registry; the JS reads it, sees it fresh, and answers identically
	if out, code := runGo(W, pathEnv, "index"); code != 0 {
		t.Fatalf("go index: %s", out)
	}
	js, _ := runJS(t, node, tool, pathEnv, W, "status", "--json")
	var st map[string]any
	json.Unmarshal([]byte(lastLine(js)), &st)
	if reg := st["registry"].(map[string]any); reg["present"] != true || reg["stale"] != false {
		t.Fatalf("js does not accept the Go registry as fresh: %s", js)
	}
	for _, a := range asks {
		compare("go-registry", append([]string{"pick"}, a...)...)
		compare("go-registry", append(append([]string{"brief"}, a...), "--json")...)
	}
	for _, n := range []string{"wire", "x.sh", "soffice", "ollama"} {
		compare("go-registry", "explain", n, "--json")
		compare("go-registry", "load", n, "--json")
	}
	compare("go-registry", "status")
	compare("go-registry", "index")
	compare("go-registry", "index", "--json")
	// the loads journal is one file both write and both read back
	js, _ = runJS(t, node, tool, pathEnv, W, "status", "--json")
	gо, _ := runGo(W, pathEnv, "status", "--json")
	if !jsonEqual(stripAt(js), stripAt(gо), 1e-9) {
		t.Errorf("journal readback differs\n%s\n%s", js, gо)
	}
	// FAILs agree
	for _, bad := range [][]string{{"pick", "x", "-k", "0"}, {"pick", "x", "--budget", "0"}, {"pick", "x", "--kind", "hat"}, {"pick", "x", "--min", "-1"}, {"load"}, {"load", "nope"}, {"load", "standards-check", "--sec", "9"}, {"explain", "nope"}, {"nope"}} {
		compare("fail", bad...)
	}
	_ = strconv.Itoa
}

// An empty registry's hole names the world dir the toolbox was opened in, never the other
// distribution's.
func TestEmptyRegistryHoleNamesTheWorldDir(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755)
	w, err := OpenIn(root, ".agent-one")
	if err != nil {
		t.Fatal(err)
	}
	st := w.Status(math.NaN())
	holes := strings.Join(st.Holes, "\n")
	if !strings.Contains(holes, "registry empty") || strings.Contains(holes, ".isekai") || !strings.Contains(holes, ".agent-one/tools") {
		t.Fatalf("holes: %q", holes)
	}
}
