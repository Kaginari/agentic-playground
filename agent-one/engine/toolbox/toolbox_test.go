package toolbox

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/memory"
)

// fixture writes the selftest workspace (the one toolbox.js's selftest builds) under a temp dir.
func fixture(t *testing.T) (workspace string) {
	t.Helper()
	W := filepath.Join(t.TempDir(), "workspace")
	writeFile(filepath.Join(W, ".agent-one", "name"), "fixture-workspace\n")
	writeFile(filepath.Join(W, ".agent-one", "AGENT-ONE.md"), "# Policy\n\n## Policies\nOperator's word is policy.\n")
	writeFile(filepath.Join(W, ".agent-one", "zone", "auth", "README.md"), "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-api\n- **Purpose:** ground truth of the login zone; holds the wire skill\n\n## Working notes\n- 2026-09-22 provisioned\n")
	writeFile(filepath.Join(W, ".agent-one", "domain", "api", "README.md"), "# domain-api\n\n- **Rank:** Domain owner\n- **Owns:** `src/api/`\n- **Reports to:** coord-core\n- **Purpose:** rules the api domain\n")
	writeFile(filepath.Join(W, ".claude", "skills", "wire", "SKILL.md"), "---\nname: wire\ndescription: How to speak the wire between machine mouths.\n---\n# Wire\n\n## Report\nA ephemeral subagent reports back over the wire. BODYSENTINEL-WIRE\n\n## Hygiene\nNo greetings.\n")
	writeFile(filepath.Join(W, ".claude", "skills", "deploy-ship", "SKILL.md"), "---\nname: deploy-ship\ndescription: Push a release to the production cluster and watch the rollout.\ntriggers:\n  - ship it\n  - rollout\n---\n# Deploy\n\nBODYSENTINEL-DEPLOY\n")
	writeFile(filepath.Join(W, ".claude", "skills", "standards-check", "SKILL.md"), "---\nname: standards-check\ndescription: \"Review the branch changes against the repo coding standards and report findings. Use when asked to \\\"review since X\\\".\"\n---\n# Review\n\n## Standards\nBODYSENTINEL-REVIEW standards section.\n\n## Spec\nSpec section text.\n\n## Report\nReport section text.\n")
	writeFile(filepath.Join(W, ".opencode", "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: Lives in both homes; one entry, two sources.\n---\n# Dup\n")
	writeFile(filepath.Join(W, ".claude", "skills", "dup", "SKILL.md"), "---\nname: dup\ndescription: Lives in both homes; one entry, two sources.\n---\n# Dup\n")
	heavy := "Render slides and decks to pdf; check a slide; export the deck. " + strings.Repeat("Slides decks pdf render export slide deck check. ", 40)
	writeFile(filepath.Join(W, ".claude", "skills", "big", "SKILL.md"), "---\nname: big\ndescription: \""+heavy+"\"\n---\n# Big\n")
	writeFile(filepath.Join(W, ".claude", "skills", "cheap", "SKILL.md"), "---\nname: cheap\ndescription: Render a deck.\n---\n# Cheap\n")
	writeFile(filepath.Join(W, ".claude", "skills", "mid", "SKILL.md"), "---\nname: mid\ndescription: >\n  Render slides and decks to pdf and check\n  every slide of the deck before export.\n---\n# Mid\n")
	writeFile(filepath.Join(W, ".claude", "commands", "provisioning.md"), "---\ndescription: Provisioning — provisioning Coordinators, Domain owners and Zone workers from observed need\n---\n# /provision\n")
	writeFile(filepath.Join(W, ".opencode", "commands", "AGENT-ONE.md"), "---\ndescription: Onboard a directory as a managed workspace\n---\n# /agent-one\n")
	writeFile(filepath.Join(W, ".agent-one", "tools", "x.sh"), "#!/bin/bash\n# x.sh — proving-grounds helper: runs the workspace's tests under .agent-one/tmp/.\n#\n# Usage: x.sh\necho hi\n")
	writeFile(filepath.Join(W, ".agent-one", "tools", "memo.js"), "#!/usr/bin/env node\n// memo.js — the memory instrument: remember a note, recall by meaning.\n// Second header line.\n'use strict';\n\nconsole.log(1)\n")
	writeFile(filepath.Join(W, ".agent-one", "tools", "block.js"), "/**\n * block.js — a tool with a block header.\n * Second line.\n */\nconsole.log(1)\n")
	writeFile(filepath.Join(W, ".claude", "agents", "domain-api.md"), "---\nname: domain-api\ndescription: Domain owner of the api domain — holds the gate for zone-auth.\nmode: subagent\nmodel: sonnet\n---\n# domain-api\n\nBODYSENTINEL-DOMAIN\n")
	writeFile(filepath.Join(W, "bin", "soffice"), "#!/bin/sh\necho fake\n")
	os.Chmod(filepath.Join(W, "bin", "soffice"), 0o755)
	l1, _ := memory.MarshalJS(memory.OJ{memory.P("kind", "external"), memory.P("name", "soffice"), memory.P("description", "LibreRole headless: converts a pptx deck to pdf."), memory.P("triggers", []string{"pdf", "libreoffice"}), memory.P("cost", memory.OJ{memory.P("resident", 30), memory.P("full", 60)}), memory.P("usage", "soffice --headless --convert-to pdf <file>")})
	l2, _ := memory.MarshalJS(memory.OJ{memory.P("kind", "external"), memory.P("name", "ollama"), memory.P("path", filepath.Join(W, "nowhere", "ollama")), memory.P("description", "Local embedding model server."), memory.P("triggers", []string{"embedding"}), memory.P("installed", true)})
	writeFile(filepath.Join(W, ".agent-one", "toolbox", "extra.jsonl"), string(l1)+"\n"+string(l2)+"\n{not json}\n")
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
	fm, agent := Frontmatter("---\nname: x\ndescription: >\n  folded line\n  second\ntriggers:\n  - \"ship it\"\n  - rollout\nwhen: a, b\n---\n# T\nagent\n")
	if fm["description"] != "folded line second" || agent != "# T\nagent\n" || !reflect.DeepEqual(fm["triggers"], []string{"ship it", "rollout"}) || !reflect.DeepEqual(asList(fm["when"]), []string{"a", "b"}) {
		t.Errorf("frontmatter: %#v agent=%q", fm, agent)
	}
	if fm, agent := Frontmatter("---\n---\nx"); agent != "---\n---\nx" || len(fm) != 0 {
		t.Errorf("a fence with nothing between is no frontmatter in the JS either: agent=%q", agent)
	}
	if fm, agent := Frontmatter("no fm\n---\nx"); len(fm) != 0 || agent != "no fm\n---\nx" {
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
	if err != nil || ir.Reg.N != 15 || ir.ByKind["skill"] != 7 || ir.ByKind["tool"] != 3 || ir.ByKind["external"] != 2 {
		t.Fatalf("index: %+v %v", ir.ByKind, err)
	}
	e, _, err := w.Explain("mid", "")
	if err != nil || e.Description != "Render slides and decks to pdf and check every slide of the deck before export." {
		t.Fatalf("folded description: %+v %v", e, err)
	}
	if e, _, _ := w.Explain("block.js", ""); e == nil || e.Description != "a tool with a block header. Second line. /" {
		t.Fatalf("block header description: %+v", e)
	}
	b, err := w.Brief("speak on the wire, then ship it", PickOpts{As: "zone-auth"})
	if err != nil || len(b.Picks) < 2 || b.Picks[0].Name != "wire" || !strings.HasPrefix(b.Head, "@TOOLS as=zone-auth k=") {
		t.Fatalf("brief: %+v %v", b, err)
	}
	for _, l := range b.Lines {
		if strings.Contains(l, "BODYSENTINEL") {
			t.Fatal("a agent crossed in level 1")
		}
	}
	lr, err := w.Load("standards-check", LoadOpts{As: "domain-api", Sec: intPtr(3)})
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
	if _, err := w.DoPick("x", PickOpts{Kinds: []string{"vibe"}}); err == nil {
		t.Fatal("kind vibe must FAIL")
	}
	if _, err := w.DoPick("x", PickOpts{Min: math.NaN()}); err == nil {
		t.Fatal("NaN min must FAIL")
	}
}

func intPtr(n int) *int { return &n }

// ---- interop with toolbox.js

func runGo(workspace, pathEnv string, args ...string) (string, int) {
	var so, se bytes.Buffer
	code := cli(append([]string{workspace}, args...), &so, &se, pathEnv)
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

func TestEmptyRegistryHoleNamesTheWorkspaceDir(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755)
	w, err := OpenIn(root, ".agent-one")
	if err != nil {
		t.Fatal(err)
	}
	st := w.Status(math.NaN())
	holes := strings.Join(st.Holes, "\n")
	if !strings.Contains(holes, "registry empty") || !strings.Contains(holes, ".agent-one/tools") {
		t.Fatalf("holes: %q", holes)
	}
}
