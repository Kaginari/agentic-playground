package toolbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/memory"
)

type selfRun struct {
	code int
	out  string
	j    map[string]any
}

func runCLI(workspace, pathEnv string, args ...string) selfRun {
	var so, se bytes.Buffer
	code := cli(append([]string{workspace}, args...), &so, &se, pathEnv)
	r := selfRun{code: code, out: so.String() + se.String()}
	lines := strings.Split(strings.TrimSpace(so.String()), "\n")
	json.Unmarshal([]byte(lines[len(lines)-1]), &r.j)
	return r
}

func (r selfRun) path(keys ...string) any {
	var cur any = r.j
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}
func (r selfRun) num(keys ...string) float64 { f, _ := r.path(keys...).(float64); return f }
func (r selfRun) str(keys ...string) string  { s, _ := r.path(keys...).(string); return s }
func (r selfRun) arr(keys ...string) []any   { a, _ := r.path(keys...).([]any); return a }
func (r selfRun) holesMatch(pat string) bool {
	re := regexp.MustCompile(pat)
	for _, h := range r.arr("@?") {
		if s, ok := h.(string); ok && re.MatchString(s) {
			return true
		}
	}
	return false
}
func pickNames(r selfRun) []string {
	var out []string
	for _, p := range r.arr("picks") {
		out = append(out, p.(map[string]any)["name"].(string))
	}
	return out
}
func pickBy(r selfRun, name string) map[string]any {
	for _, p := range r.arr("picks") {
		if m := p.(map[string]any); m["name"] == name {
			return m
		}
	}
	return nil
}
func indexOf(ss []string, s string) int {
	for i, x := range ss {
		if x == s {
			return i
		}
	}
	return -1
}
func head(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func writeFile(p, s string) {
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(s), 0o644)
}

// Selftest builds a throwaway workspace (under .agent-one/tmp/ when cwd is a workspace — Policy 5 — else the
// OS temp dir), drives every command against it, and removes it. Returns the checks passed.
func Selftest() (int, error) {
	root, _ := os.Getwd()
	if !memory.Exists(filepath.Join(root, ".agent-one")) {
		root = os.TempDir()
	}
	return SelftestIn(root, io.Discard)
}

// SelftestIn runs the selftest with its throwaway under root and reports on out the way the JS does.
func SelftestIn(root string, out io.Writer) (int, error) {
	T := filepath.Join(root, ".agent-one", "tmp", "toolbox-selftest")
	if !memory.Exists(filepath.Join(root, ".agent-one")) {
		T = filepath.Join(root, "agent-one-toolbox-selftest")
	}
	W := filepath.Join(T, "workspace")
	var fails []string
	checks := 0
	ok := func(cond bool, what string) {
		checks++
		if !cond {
			fails = append(fails, what)
		}
	}
	run := func(args ...string) selfRun { return runCLI(W, filepath.Join(W, "bin"), args...) }
	func() {
		defer os.RemoveAll(T)
		os.RemoveAll(T)
		writeFile(filepath.Join(W, ".agent-one", "name"), "selftest-workspace\n")
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
		writeFile(filepath.Join(W, ".claude", "skills", "mid", "SKILL.md"), "---\nname: mid\ndescription: Render slides and decks to pdf and check every slide of the deck before export.\n---\n# Mid\n")
		writeFile(filepath.Join(W, ".claude", "commands", "provisioning.md"), "---\ndescription: Provisioning — provisioning Coordinators, Domain owners and Zone workers from observed need\n---\n# /provision\n")
		writeFile(filepath.Join(W, ".opencode", "commands", "AGENT-ONE.md"), "---\ndescription: Onboard a directory as a managed workspace\n---\n# /agent-one\n")
		writeFile(filepath.Join(W, ".agent-one", "tools", "x.sh"), "#!/bin/bash\n# x.sh — proving-grounds helper: runs the workspace's tests under .agent-one/tmp/.\n#\n# Usage: x.sh\necho hi\n")
		writeFile(filepath.Join(W, ".agent-one", "tools", "memo.js"), "#!/usr/bin/env node\n// memo.js — the memory instrument: remember a note, recall by meaning.\n// Second header line.\n\nconsole.log(1)\n")
		writeFile(filepath.Join(W, ".claude", "agents", "domain-api.md"), "---\nname: domain-api\ndescription: Domain owner of the api domain — holds the gate for zone-auth.\nmode: subagent\nmodel: sonnet\n---\n# domain-api\n\nBODYSENTINEL-DOMAIN\n")
		writeFile(filepath.Join(W, "bin", "soffice"), "#!/bin/sh\necho fake\n")
		os.Chmod(filepath.Join(W, "bin", "soffice"), 0o755)
		l1, _ := memory.MarshalJS(memory.OJ{memory.P("kind", "external"), memory.P("name", "soffice"), memory.P("description", "LibreRole headless: converts a pptx deck to pdf."), memory.P("triggers", []string{"pdf", "libreoffice"}), memory.P("cost", memory.OJ{memory.P("resident", 30), memory.P("full", 60)}), memory.P("usage", "soffice --headless --convert-to pdf <file>")})
		l2, _ := memory.MarshalJS(memory.OJ{memory.P("kind", "external"), memory.P("name", "ollama"), memory.P("path", filepath.Join(W, "nowhere", "ollama")), memory.P("description", "Local embedding model server."), memory.P("triggers", []string{"embedding"}), memory.P("installed", true)})
		writeFile(filepath.Join(W, ".agent-one", "toolbox", "extra.jsonl"), string(l1)+"\n"+string(l2)+"\n{not json}\n")
		// index: counts per kind, dup merged, external installed measured not believed
		r := run("index", "--json")
		ok(r.code == 0 && r.str("@S") == "INDEXED" && r.num("skill") == 7 && r.num("command") == 2 && r.num("tool") == 2 && r.num("agent") == 1 && r.num("external") == 2 && r.num("n") == 14, "index counts: "+head(r.out, 200))
		ok(r.holesMatch("ollama is not installed.*claims"), "index: an external that is not installed is a @?, against its own claim")
		tmpLeft := false
		es, _ := os.ReadDir(filepath.Join(W, ".agent-one", "toolbox"))
		for _, e := range es {
			if strings.HasSuffix(e.Name(), ".tmp") {
				tmpLeft = true
			}
		}
		ok(memory.Exists(filepath.Join(W, ".agent-one", "toolbox", "registry.json")) && !tmpLeft, "index: registry present, no temp left")
		r = run("explain", "dup", "--json")
		ok(len(r.arr("srcs")) == 2 && r.num("cost", "resident") > 0 && r.num("cost", "full") >= r.num("cost", "resident"), "explain dup: two sources, costs known, got "+head(r.out, 160))
		r = run("explain", "wire", "--json")
		trig := func(r selfRun, t, src string) bool {
			for _, x := range r.arr("triggers") {
				m := x.(map[string]any)
				if (t == "" || m["t"] == t) && m["src"] == src {
					return true
				}
			}
			return false
		}
		holders := fmt.Sprint(r.arr("holders"))
		ok(r.str("serves") == "zone" && strings.Contains(holders, "zone-auth") && trig(r, "wire", "name") && r.num("sections") == 3, "explain wire: zone lane from its holder, name trigger, 3 sections — got "+head(r.out, 200))
		r = run("explain", "standards-check", "--json")
		ok(trig(r, "review since X", "quoted"), "explain standards-check: a quoted phrase is a derived trigger — got "+head(r.out, 300))
		ok(trig(run("explain", "x", "--json"), "", "sentence") && trig(run("explain", "deploy-ship", "--json"), "ship it", "frontmatter"), "explain: first-sentence and frontmatter triggers")
		ok(run("explain", "nope").code == 2, "explain: an unknown name is a FAIL")
		// pick by trigger beats pick by meaning
		r = run("pick", "review the branch changes against the coding standards, then ship it", "--json")
		p := r.arr("picks")
		ok(len(p) >= 2 && p[0].(map[string]any)["name"] == "deploy-ship" && strings.HasPrefix(p[0].(map[string]any)["why"].(string), `trigger "ship it"`) && p[1].(map[string]any)["name"] == "standards-check" && p[1].(map[string]any)["why"] == "meaning" && p[1].(map[string]any)["sim"].(float64) > 0.4,
			"trigger beats meaning: "+head(r.out, 300))
		clean := true
		for _, x := range p {
			if s := x.(map[string]any)["sim"].(float64); s < 0 || s > 1 {
				clean = false
			}
		}
		ok(clean, "pick: sims in [0,1], no NaN")
		// level 1 is descriptions only: no agent text crosses in pick or brief
		pickText := run("pick", "review the branch changes", "-k", "3").out
		ok(strings.HasPrefix(pickText, "@S PICK k=") && regexp.MustCompile(`@T skill standards-check — \.claude/skills/standards-check/SKILL\.md — \d+tok \(load≈\d+\)`).MatchString(pickText) && regexp.MustCompile(`@E \d+`).MatchString(pickText) && !strings.Contains(pickText, "BODYSENTINEL"), "pick wire: "+head(pickText, 200))
		briefText := run("brief", "speak on the wire, then ship it", "--as", "zone-auth").out
		ok(regexp.MustCompile(`^@TOOLS as=zone-auth k=\d+ cost=\d+/1500 — level 2 on decision only: agent-one toolbox load <name>`).MatchString(briefText) && strings.Contains(briefText, "Skill <name>") &&
			regexp.MustCompile(`(?m)@T skill wire — .*load≈\d+tok — How to speak the wire between machine mouths\.$`).MatchString(briefText) && regexp.MustCompile(`(?m)@T skill deploy-ship — .* — ⟨ship it · rollout⟩$`).MatchString(briefText) && !strings.Contains(briefText, "BODYSENTINEL"), "brief: "+head(briefText, 300))
		r = run("brief", "speak on the wire, then ship it", "--as", "zone-auth", "--json")
		linesOK := len(r.arr("lines")) == len(r.arr("picks"))
		for _, l := range r.arr("lines") {
			if !strings.HasPrefix(l.(string), "@T ") {
				linesOK = false
			}
		}
		priced := true
		for _, x := range r.arr("picks") {
			m := x.(map[string]any)
			if _, ok := m["resident"].(float64); !ok {
				priced = false
			}
			if _, ok := m["full"].(float64); !ok {
				priced = false
			}
			if _, ok := m["text"]; ok {
				priced = false
			}
		}
		ok(r.str("@S") == "TOOLS" && r.str("as") == "zone-auth" && r.num("k") == float64(len(r.arr("picks"))) && linesOK && strings.HasPrefix(r.str("head"), "@TOOLS as=zone-auth") && priced && !strings.Contains(r.out, "BODYSENTINEL"), "brief --json: head, @T lines, priced picks, no agent — "+head(r.out, 200))
		// budget: the cut drops the lowest-SCORING picks, not the cheapest; the dropped ones are a @?
		all := run("pick", "render the deck to pdf and check slide 8", "--budget", "100000", "-k", "10", "--json")
		names := pickNames(all)
		ok(len(names) > 0 && names[0] == "soffice" && indexOf(names, "cheap") > indexOf(names, "big") && indexOf(names, "cheap") > indexOf(names, "mid"), "budget: unbudgeted order (declared trigger first, the cheapest-cost entry behind the dearer ones) is "+strings.Join(names, ","))
		cheap := pickBy(all, "cheap")
		minOther := 1e9
		for _, x := range all.arr("picks") {
			m := x.(map[string]any)
			if m["name"] != "cheap" && m["resident"].(float64) < minOther {
				minOther = m["resident"].(float64)
			}
		}
		ok(cheap != nil && cheap["resident"].(float64) < minOther, "budget: cheap is the cheapest yet scores below big and mid")
		ap := all.arr("picks")
		top2 := ap[0].(map[string]any)["resident"].(float64) + ap[1].(map[string]any)["resident"].(float64)
		r = run("pick", "render the deck to pdf and check slide 8", "--budget", fmt.Sprint(int(top2)+1), "-k", "10", "--json")
		ok(strings.Join(pickNames(r), ",") == strings.Join(names[:2], ",") && r.num("cost") <= top2+1 && pickBy(r, "cheap") == nil, "budget cut keeps the two best scores, drops the two lowest — not the cheapest-cost: "+strings.Join(pickNames(r), ","))
		ok(r.holesMatch(`\d fit the turn but not the budget`) && r.holesMatch("cheap"), fmt.Sprintf("budget: the over-budget picks are a @?, got %v", r.path("@?")))
		ok(run("pick", "render the deck", "--budget", "0").code == 2 && run("pick", "render", "-k", "0").code == 2, "pick: --budget 0 and -k 0 are FAILs")
		// --as relation boost: the skill zone-auth holds is pulled closer; its lane too
		r = run("pick", "speak on the wire", "--as", "zone-auth", "--json")
		wire := pickBy(r, "wire")
		ok(wire != nil && wire["rel"].(float64) >= 0.23 && pickNames(r)[0] == "wire", fmt.Sprintf("--as zone-auth: wire held+lane ≥ 0.23, got %v", wire))
		w0 := pickBy(run("pick", "speak on the wire", "--json"), "wire")
		ok(w0 != nil && w0["rel"].(float64) == 0, "--as orchestrator: no relation boost")
		r = run("pick", "the api gate", "--as", "zone-auth", "--kind", "agent", "--json")
		ok(len(r.arr("picks")) == 1 && pickNames(r)[0] == "domain-api" && pickBy(r, "domain-api")["rel"].(float64) >= 0.1, "--kind agent + parent bond: "+head(r.out, 160))
		ok(run("pick", "x", "--kind", "vibe").code == 2 && run("pick", "x", "--min", "-1").code == 2, "pick: --kind vibe and --min -1 are FAILs")
		// the floor: one incidental word in common is not a fit
		one := pickNames(run("pick", "render the deck to pdf, check slide 8 and leave a note", "-k", "20", "--json"))
		two := run("pick", "leave a note", "--json")
		ok(indexOf(one, "memo.js") < 0 && len(two.arr("picks")) == 1 && pickNames(two)[0] == "memo.js" && pickBy(two, "memo.js")["matched"].(float64) == 1, fmt.Sprintf("fit rule: one word of seven is no fit; one of two is — got %v %v", one, pickNames(two)))
		floor := true
		for _, x := range run("pick", "render the deck to pdf", "--min", "0.9", "--json").arr("picks") {
			if x.(map[string]any)["score"].(float64) < 0.9 {
				floor = false
			}
		}
		ok(floor, "--min: a score floor")
		// holes: empty ask, a missing external
		r = run("pick", "the of and", "--json")
		ok(r.code == 0 && len(r.arr("picks")) == 0 && r.holesMatch("no content words"), "empty ask: "+head(r.out, 160))
		r = run("pick", "embedding server", "--json")
		ok(len(pickNames(r)) > 0 && pickNames(r)[0] == "ollama" && pickBy(r, "ollama")["installed"] == false && r.holesMatch("ollama.*not installed"), "missing external is picked but named a hole: "+head(r.out, 200))
		// level 2: load whole, load a section, map; every load is one journal line; a map is not a load
		journal := func() []map[string]any {
			var out []map[string]any
			for _, raw := range memory.ReadJSONL(filepath.Join(W, ".agent-one", "instruments", "toolbox", "loads.jsonl")) {
				var m map[string]any
				json.Unmarshal(raw, &m)
				if m["ev"] == "load" {
					out = append(out, m)
				}
			}
			return out
		}
		before := len(journal())
		r = run("load", "standards-check", "--json")
		ok(r.str("@S") == "LOAD" && r.str("sec") == "all" && strings.Contains(r.str("text"), "BODYSENTINEL-REVIEW") && strings.HasPrefix(r.str("text"), "---\nname: standards-check") && r.num("tokens") == float64(TOK(r.str("text"))), "load whole: "+head(r.out, 160))
		r = run("load", "standards-check", "--sec", "3", "--as", "domain-api", "--json")
		ok(r.num("sec") == 3 && r.str("text") == "## Spec\nSpec section text.\n" && !strings.Contains(r.str("text"), "BODYSENTINEL"), fmt.Sprintf("load --sec 3 is exactly that section, got %q", r.str("text")))
		ok(strings.HasPrefix(run("load", "standards-check", "--sec", "0", "--json").str("text"), "---\nname: standards-check"), "load --sec 0 is the preamble (frontmatter)")
		ok(run("load", "standards-check", "--sec", "9").code == 2, "load --sec past the end is a FAIL that points at --map")
		r = run("load", "standards-check", "--map", "--json")
		ok(r.str("@S") == "MAP" && len(r.arr("sections")) == 4 && r.arr("sections")[2].(map[string]any)["title"] == "Spec", "load --map: "+head(r.out, 160))
		ok(strings.HasPrefix(run("load", "standards-check", "--map").out, "@S MAP standards-check sections=4"), "load --map wire form")
		loadText := run("load", "wire").out
		ok(regexp.MustCompile(`^@S LOAD skill wire sec=all tokens=\d+ — \.claude/skills/wire/SKILL\.md\n---\nname: wire`).MatchString(loadText) && strings.Contains(loadText, "BODYSENTINEL-WIRE") && regexp.MustCompile(`\n@E \d+\n$`).MatchString(loadText), "load wire form: "+head(loadText, 120))
		ok(strings.Contains(run("load", "x.sh", "--json").str("text"), "x.sh — proving-grounds helper") && !strings.Contains(run("load", "x.sh", "--json").str("text"), "echo hi"), "load tool: the header, not the code")
		ok(run("load", "soffice", "--json").str("text") == "soffice --headless --convert-to pdf <file>", "load external: its usage notes")
		ok(run("load", "ollama", "--json").holesMatch("no usage notes"), "load external without usage: description + @?")
		ok(run("load", "BODYSENTINEL").code == 2 && run("load").code == 2, "load: unknown name / no name are FAILs")
		loads := journal()
		shaped, domainSec3 := true, false
		for _, l := range loads {
			if l["at"] == nil || l["by"] == nil || l["name"] == nil || l["sec"] == nil {
				shaped = false
			}
			if _, ok := l["tokens"].(float64); !ok {
				shaped = false
			}
			if l["by"] == "domain-api" && l["sec"] == 3.0 {
				domainSec3 = true
			}
		}
		ok(len(loads) == before+8 && shaped && domainSec3, fmt.Sprintf("journal: one line per load (%d), map not counted, {at, by, name, tokens, sec}", len(loads)-before))
		// status: offered vs loaded, from the journal
		r = run("status", "--json")
		ok(r.path("registry", "present") == true && r.path("registry", "stale") == false && r.num("registry", "n") == 14 && r.num("cost", "residentIfAllInjected") > 0 && r.num("cost", "fullIfAllLoaded") > r.num("cost", "residentIfAllInjected"), "status: "+head(r.out, 200))
		ok(r.num("instrument", "loads") == 8 && r.num("instrument", "offers") >= 8 && r.num("instrument", "offered") > r.num("instrument", "loads") && r.num("instrument", "loadedTokens") > 0 && r.num("instrument", "residentTokens") > 0, fmt.Sprintf("status instrument: %v", r.path("instrument")))
		ok(r.num("externals", "installed") == 1 && fmt.Sprint(r.arr("externals", "missing")) == "[ollama]", "status: externals measured")
		ok(regexp.MustCompile(`LOADS    offered \d+ \(\d+ distinct, \d+ picks\) · loaded 8 \(\d+ distinct\) · resident cost [\d,]+ tok · loaded cost [\d,]+ tok`).MatchString(run("status").out), "status text: offered vs loaded line")
		// stale: a source changed after the build → @? everywhere; rebuild clears it
		os.Chtimes(filepath.Join(W, ".claude", "skills", "wire", "SKILL.md"), time.Now(), time.Now().Add(5*time.Second))
		r = run("status", "--json")
		ok(r.path("registry", "stale") == true && r.holesMatch("older than its sources.*wire"), fmt.Sprintf("status stale: %v", r.path("@?")))
		ok(run("pick", "wire", "--json").holesMatch("older than its sources") && run("load", "wire", "--json").holesMatch("older than its sources"), "pick and load: a stale registry is a @?")
		ok(run("index", "--json").num("n") == 14 && run("status", "--json").path("registry", "stale") == false, "index: rebuild clears stale")
		// no registry: answered live, named as a hole; empty workspace: every silence is a finding
		os.Remove(filepath.Join(W, ".agent-one", "toolbox", "registry.json"))
		r = run("pick", "speak on the wire", "--json")
		ok(len(r.arr("picks")) > 0 && r.holesMatch("no registry on disk"), "no registry: live harvest + @?")
		E := filepath.Join(T, "empty")
		os.MkdirAll(filepath.Join(E, ".agent-one"), 0o755)
		runE := func(args ...string) selfRun { return runCLI(E, "", args...) }
		r = runE("index", "--json")
		ok(r.code == 0 && r.num("n") == 0 && len(r.arr("@?")) == 1, "empty workspace: index of nothing is a @?")
		r = runE("pick", "anything", "--json")
		ok(r.code == 0 && len(r.arr("picks")) == 0 && r.holesMatch("registry empty"), "empty workspace: pick is a @?")
		ok(runE("status", "--json").code == 0, "empty workspace: status parses")
	}()
	ok(!memory.Exists(T), "throwaway workspace removed")
	if len(fails) > 0 {
		fmt.Fprintln(out, "@S FAIL")
		for _, f := range fails {
			fmt.Fprintf(out, "@F selftest — %s\n", f)
		}
		return checks - len(fails), fmt.Errorf("%d of %d checks failed", len(fails), checks)
	}
	fmt.Fprintf(out, "@S PASS %d checks · go %s\n", checks, runtime.Version())
	return checks, nil
}
