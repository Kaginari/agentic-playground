package memory

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
	"sync"
	"time"
)

// selfRun is one CLI call in-process: exit code, stdout+stderr, and the last stdout line as JSON.
type selfRun struct {
	code int
	out  string
	j    map[string]any
}

func runCLI(fn func(args []string, stdout, stderr io.Writer, home string) int, workspace, home string, args ...string) selfRun {
	var so, se bytes.Buffer
	code := fn(append([]string{workspace}, args...), &so, &se, home)
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
	if !Exists(filepath.Join(root, ".agent-one")) {
		root = os.TempDir()
	}
	return SelftestIn(root, io.Discard)
}

// SelftestIn runs the selftest with its throwaway under root and reports on out the way the JS does.
func SelftestIn(root string, out io.Writer) (int, error) {
	T := filepath.Join(root, ".agent-one", "tmp", "memory-selftest")
	if !Exists(filepath.Join(root, ".agent-one")) {
		T = filepath.Join(root, "agent-one-memory-selftest")
	}
	W, H := filepath.Join(T, "workspace"), filepath.Join(T, "home")
	realHome, _ := os.UserHomeDir()
	realMachine := fileSize(filepath.Join(realHome, ".agent-one", "shared", "notes.jsonl"))
	var fails []string
	checks := 0
	ok := func(cond bool, what string) {
		checks++
		if !cond {
			fails = append(fails, what)
		}
	}
	run := func(args ...string) selfRun { return runCLI(cli, W, H, args...) }
	func() {
		defer os.RemoveAll(T)
		os.RemoveAll(T)
		writeFile(filepath.Join(W, ".agent-one", "name"), "selftest-workspace\n")
		writeFile(filepath.Join(W, ".agent-one", "AGENT-ONE.md"), "# Policy\r\n\r\n## The gate\r\nNo change lands without its Domain owner's pass. The constructor word is here on purpose.\r\n\r\n## Policies\r\nOperator's word is policy.\r\n")
		writeFile(filepath.Join(W, ".agent-one", "log.md"), "# Chronicle\n\n    ### [YYYY-MM-DD] format example, indented — not an entry\n\n### [2026-09-20 17:54] orchestrator — Workspace onboarded\n- **Task:** /agent-one\n\n### [2026-09-22T01:00:00+02:00] orchestrator — Subagent agent reports back over the wire\n- **Learned:** a ephemeral subagent reports back over the wire with @S @F @? @E lines only\n")
		writeFile(filepath.Join(W, ".agent-one", "zone", "auth", "README.md"), "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-api\n- **Purpose:** ground truth of the login zone; holds the wire skill\n\n## Invariants\n- tokens expire after one hour\n\n## Working notes\n\n### [2026-09-22]\nProvisioned by /provision.\n")
		writeFile(filepath.Join(W, ".agent-one", "domain", "api", "README.md"), "# domain-api\n\n- **Rank:** Domain owner\n- **Owns:** `src/api/`\n- **Reports to:** coord-core\n- **Purpose:** rules the api domain\n\n## Working notes\n- 2026-09-22 gate verdicts on the auth zone\n")
		writeFile(filepath.Join(W, ".agent-one", "design", "notes.md"), "# Design\n\n## Login tokens\nThe domain-api gate checks the login token zone owned by zone-auth.\n\n## Big\n"+strings.Repeat("lorem ipsum dolor ", 12000)+"\n")
		writeFile(filepath.Join(W, ".agent-one", "tools", "x.sh"), "# x.sh — a tool header about the sandbox\necho hi\n")
		var notes []string
		for _, i := range []int{1, 2, 3, 4, 5, 6} {
			notes = append(notes, fmt.Sprintf("- 2026-09-2%d note %d", i%3, i))
		}
		writeFile(filepath.Join(W, ".claude", "skills", "wire", "SKILL.md"), "---\nname: wire\ndescription: how to speak the wire\n---\n# Wire\n\n## Report\nA ephemeral subagent reports back over the wire: @S opens, @F per finding, @? per hole, @E closes.\n\n## Working notes\n"+strings.Join(notes, "\n")+"\n")
		writeFile(filepath.Join(W, "README.md"), "# selftest workspace\n\n## About\nA throwaway workspace for the memory selftest.\n")
		// index
		r := run("index", "--json")
		ok(r.code == 0 && r.str("@S") == "INDEXED" && r.num("episodic") == 2 && r.num("procedural") >= 3 && r.num("semantic") >= 7, "index: "+head(r.out, 200))
		tmpLeft := false
		for _, e := range readDirSorted(filepath.Join(W, ".agent-one", "memory", "long"), false) {
			if strings.HasSuffix(e.Name(), ".tmp") {
				tmpLeft = true
			}
		}
		ok(Exists(filepath.Join(W, ".agent-one", "memory", "long", "index.json")) && !tmpLeft, "index: file present, no temp left")
		// recall: miss, then hit; both the log entry and the skill section must be in the top 3
		r = run("recall", "how does a ephemeral subagent report back over the wire", "-k", "3", "--json")
		ok(r.code == 0 && r.str("@S") == "MISS" && len(r.arr("results")) == 3, "recall miss: "+head(r.out, 200))
		hasLog, hasSkill, clean := false, false, true
		for _, x := range r.arr("results") {
			m := x.(map[string]any)
			src, _ := m["src"].(string)
			if strings.HasSuffix(src, "log.md") && m["rec"].(float64) > 0 {
				hasLog = true
			}
			if strings.HasSuffix(src, "SKILL.md") {
				hasSkill = true
			}
			if s := m["sim"].(float64); !(s > 0 && s <= 1) {
				clean = false
			}
		}
		ok(hasLog && hasSkill, "recall: log entry (with recency) and wire skill in top 3")
		ok(clean, "recall: sims in (0,1], no NaN")
		first, _ := MarshalJS(r.path("results"))
		r = run("recall", "how does a ephemeral subagent report back over the wire", "-k", "3", "--json")
		again, _ := MarshalJS(r.path("results"))
		ok(r.str("@S") == "HIT" && r.num("cacheSim") >= 0.92 && string(first) == string(again), "recall hit: "+head(r.out, 120))
		r = run("recall", "how does a ephemeral subagent report back over the wire", "-k", "2", "--json")
		ok(r.str("@S") == "MISS" && len(r.arr("results")) == 2, "recall: a different -k is a different answer, not a cache hit")
		r = run("recall", "the of and", "--json")
		ok(r.code == 0 && len(r.arr("results")) == 0 && r.holesMatch("no content words"), "recall stop words: "+head(r.out, 120))
		r = run("recall", "constructor", "--json")
		ok(r.code == 0 && r.j != nil, "recall: a prototype-named word is a safe key")
		r = run("recall", "the gate", "--kind", "semantic", "--json")
		allSem, gate := len(r.arr("results")) > 0, false
		for _, x := range r.arr("results") {
			m := x.(map[string]any)
			if m["kind"] != "semantic" {
				allSem = false
			}
			if m["title"] == "policy › The gate" {
				gate = true
			}
		}
		ok(allSem && gate, "recall --kind: CRLF policy sections harvested clean")
		r = run("recall", "x", "-k", "0")
		ok(r.code == 2 && strings.Contains(r.out, "@S FAIL"), "recall: -k 0 is a FAIL, not a guess")
		// remember (workspace), concurrent remembers, relation boost as zone-auth
		r = run("remember", "login tokens expire hourly", "--as", "zone-auth", "--tag", "auth", "--kind", "domain", "--json")
		ok(r.str("@S") == "REMEMBERED" && r.str("scope") == "workspace" && r.str("tag") == "auth" && r.str("kind") == "domain", "remember: "+head(r.out, 120))
		r = run("remember", "the gate is run twice in practice, once before lunch", "--as", "domain-api", "--kind", "team", "--json")
		ok(r.str("@S") == "REMEMBERED" && r.str("kind") == "team", "remember --kind team: "+head(r.out, 120))
		ok(run("remember", "x", "--kind", "tribal").code == 2, "remember --kind tribal (not an agent-one name) is a FAIL")
		r = run("remember", "plain note", "--as", "agent-x", "--json")
		ok(r.path("kind") == nil && has(r.j, "kind"), "remember without --kind stores kind null")
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				run("remember", fmt.Sprintf("parallel note %d", i), "--as", fmt.Sprintf("agent-%d", i))
			}(i)
		}
		wg.Wait()
		lines := ReadJSONL(filepath.Join(W, ".agent-one", "memory", "shared", "notes.jsonl"))
		texts := map[string]bool{}
		for _, raw := range lines {
			var n Note
			json.Unmarshal(raw, &n)
			texts[n.Text] = true
		}
		ok(len(lines) == 15 && len(texts) == 15, fmt.Sprintf("remember: 15 parse-clean lines after 12 concurrent appends, got %d", len(lines)))
		r = run("recall", "login tokens", "--as", "zone-auth", "--json")
		var design, note map[string]any
		for _, x := range r.arr("results") {
			m := x.(map[string]any)
			if src, _ := m["src"].(string); strings.HasSuffix(src, "design/notes.md") && design == nil {
				design = m
			}
			if m["kind"] == "shared" && note == nil {
				note = m
			}
		}
		ok(design != nil && design["rel"].(float64) >= 0.25, fmt.Sprintf("relation: design section naming zone-auth + its domain owner gets ≥0.25, got %v", design))
		ok(note != nil && note["rel"].(float64) >= 0.05 && note["rec"].(float64) > 0, fmt.Sprintf("relation: own shared note gets +0.05 and recency, got %v", note))
		r = run("recall", "login tokens", "--tier", "shared", "--json")
		onlyShared := len(r.arr("results")) > 0
		for _, x := range r.arr("results") {
			if x.(map[string]any)["kind"] != "shared" {
				onlyShared = false
			}
		}
		ok(onlyShared, "recall --tier shared: notes only")
		r = run("recall", "gate lunch tokens", "--kind", "team", "--json")
		res := r.arr("results")
		ok(len(res) == 1 && strings.Contains(res[0].(map[string]any)["title"].(string), "team") && strings.Contains(res[0].(map[string]any)["snippet"].(string), "lunch"), "recall --kind team: only the team note, got "+head(r.out, 160))
		r = run("recall", "login tokens", "--kind", "domain", "--tier", "shared", "--json")
		res = r.arr("results")
		ok(len(res) == 1 && strings.Contains(res[0].(map[string]any)["title"].(string), "domain"), "recall --kind domain --tier shared: only the ownership note")
		r = run("recall", "anything at all", "--kind", "policy", "--json")
		ok(len(r.arr("results")) == 0 && r.holesMatch("no shared note carries kind policy"), "recall --kind policy with no policy note is a @?, not a guess")
		ok(run("recall", "x", "--kind", "team", "--tier", "long").code == 2, "recall --kind team --tier long is a FAIL (unsaid kinds live in shared)")
		ok(run("recall", "x", "--kind", "tribal").code == 2, "recall --kind tribal is a FAIL")
		r = run("recall", "login", "--as", "Zone/Auth Zone!")
		ok(r.code == 0 && Exists(filepath.Join(W, ".agent-one", "memory", "short", "zone-auth-zone-.jsonl")), "recall: an odd member name gets a sanitised cache file")
		// machine tier lands in the throwaway HOME, never the real one
		r = run("remember", "machine note", "--machine", "--json")
		ok(r.str("scope") == "machine" && Exists(filepath.Join(H, ".agent-one", "shared", "notes.jsonl")), "remember --machine: written under the throwaway home")
		// status
		r = run("status", "--json")
		ok(r.path("long", "indexed") == true && r.path("long", "stale") == false && r.num("shared", "workspace") == 15 && r.num("shared", "machine") == 1, "status: "+head(r.out, 200))
		ok(r.num("shared", "unsaid", "domain") == 1 && r.num("shared", "unsaid", "team") == 1 && r.num("shared", "unsaid", "policy") == 0, "status: shared notes by unsaid kind")
		ok(regexp.MustCompile(`SHARED  workspace 15 · machine 1 · unsaid policy 0 · team 1 · domain 1`).MatchString(run("status").out), "status text: unsaid kinds on the SHARED line")
		stressedWire := false
		for _, s := range r.arr("short", "workingMemory", "stressed") {
			if s == "wire" {
				stressedWire = true
			}
		}
		ok(r.num("short", "workingMemory", "workingNotes") == 3 && stressedWire, "status: 3 workingNotes, wire STRESSED")
		ok(r.path("short", "contextWindow", "available") == false && r.holesMatch("context window"), "status: no transcript is a @? finding")
		ok(r.num("short", "semanticCache", "entries") == 7 && r.num("short", "semanticCache", "hits") == 1, fmt.Sprintf("status: cache entries/hits, got %v", r.path("short", "semanticCache")))
		// stale: a source changed after the build
		f, _ := os.OpenFile(filepath.Join(W, ".agent-one", "log.md"), os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString("\n### [2026-09-22 02:00] orchestrator — Later entry\n- **Task:** staleness\n")
		f.Close()
		os.Chtimes(filepath.Join(W, ".agent-one", "log.md"), time.Now(), time.Now().Add(2*time.Second))
		r = run("status", "--json")
		ok(r.path("long", "stale") == true && r.holesMatch(`older than its sources.*log\.md`), fmt.Sprintf("status stale: %v", r.path("@?")))
		r = run("recall", "later entry", "--json")
		ok(r.holesMatch("older than its sources"), "recall: stale index is a @?")
		ok(run("index", "--json").num("episodic") == 3 && run("status", "--json").path("long", "stale") == false, "index: rebuild clears stale")
		// forget
		r = run("forget", "--short", "--json")
		ok(r.str("@S") == "FORGOT" && r.num("n") == 7 && !Exists(filepath.Join(W, ".agent-one", "memory", "short", "orchestrator.jsonl")), "forget: "+head(r.out, 120))
		ok(run("forget").code == 2, "forget without --short is a FAIL")
		// empty workspace: every silence is a finding, nothing crashes
		E := filepath.Join(T, "empty")
		os.MkdirAll(filepath.Join(E, ".agent-one"), 0o755)
		runE := func(args ...string) selfRun { return runCLI(cli, E, H, args...) }
		ok(runE("recall", "anything", "--json").holesMatch("no long-term index"), "empty workspace: recall without index is a @?")
		r = runE("index", "--json")
		ok(r.code == 0 && r.num("n") == 0 && len(r.arr("@?")) == 1, "empty workspace: index of nothing is a @?")
		r = runE("status", "--json")
		ok(r.code == 0 && r.path("long", "indexed") == true && r.num("long", "episodic") == 0 && r.num("short", "workingMemory", "workingNotes") == 0, "empty workspace: status parses")
	}()
	ok(fileSize(filepath.Join(realHome, ".agent-one", "shared", "notes.jsonl")) == realMachine, "the real ~/.agent-one/shared was not touched")
	ok(!Exists(T), "throwaway workspace removed")
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
