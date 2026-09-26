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

func runCLI(fn func(args []string, stdout, stderr io.Writer, home string) int, world, home string, args ...string) selfRun {
	var so, se bytes.Buffer
	code := fn(append([]string{world}, args...), &so, &se, home)
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

// Selftest builds a throwaway world (under .isekai/tmp/ when cwd is a world — Law 5 — else the
// OS temp dir), drives every command against it, and removes it. Returns the checks passed.
func Selftest() (int, error) {
	root, _ := os.Getwd()
	if !Exists(filepath.Join(root, ".isekai")) {
		root = os.TempDir()
	}
	return SelftestIn(root, io.Discard)
}

// SelftestIn runs the selftest with its throwaway under root and reports on out the way the JS does.
func SelftestIn(root string, out io.Writer) (int, error) {
	T := filepath.Join(root, ".isekai", "tmp", "memory-selftest")
	if !Exists(filepath.Join(root, ".isekai")) {
		T = filepath.Join(root, "isekai-memory-selftest")
	}
	W, H := filepath.Join(T, "world"), filepath.Join(T, "home")
	realHome, _ := os.UserHomeDir()
	realMachine := fileSize(filepath.Join(realHome, ".isekai", "shared", "notes.jsonl"))
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
		writeFile(filepath.Join(W, ".isekai", "name"), "selftest-world\n")
		writeFile(filepath.Join(W, ".isekai", "isekai.md"), "# Law\r\n\r\n## The gate\r\nNo change lands without its Orc's pass. The constructor word is here on purpose.\r\n\r\n## Laws\r\nVeldora's word is law.\r\n")
		writeFile(filepath.Join(W, ".isekai", "log.md"), "# Chronicle\n\n    ### [YYYY-MM-DD] format example, indented — not an entry\n\n### [2026-09-20 17:54] rimuru — World reincarnated\n- **Task:** /isekai\n\n### [2026-09-22T01:00:00+02:00] rimuru — Court body reports back over the wire\n- **Learned:** a court body reports back over the wire with @S @F @? @E lines only\n")
		writeFile(filepath.Join(W, ".isekai", "slime", "auth", "README.md"), "# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-api\n- **Purpose:** ground truth of the login zone; wears the wire mind\n\n## Traits\n- tokens expire after one hour\n\n## Thoughts\n\n### [2026-09-22]\nBorn by /genesis.\n")
		writeFile(filepath.Join(W, ".isekai", "orc", "api", "README.md"), "# orc-api\n\n- **Rank:** Orc\n- **Territory:** `src/api/`\n- **Reports to:** elf-core\n- **Purpose:** rules the api domain\n\n## Thoughts\n- 2026-09-22 gate verdicts on the auth zone\n")
		writeFile(filepath.Join(W, ".isekai", "canon", "notes.md"), "# Canon\n\n## Login tokens\nThe orc-api gate checks the login token zone owned by slime-auth.\n\n## Big\n"+strings.Repeat("lorem ipsum dolor ", 12000)+"\n")
		writeFile(filepath.Join(W, ".isekai", "tools", "x.sh"), "# x.sh — a tool header about the proving grounds\necho hi\n")
		var thoughts []string
		for _, i := range []int{1, 2, 3, 4, 5, 6} {
			thoughts = append(thoughts, fmt.Sprintf("- 2026-09-2%d thought %d", i%3, i))
		}
		writeFile(filepath.Join(W, ".claude", "skills", "wire", "SKILL.md"), "---\nname: wire\ndescription: how to speak the wire\n---\n# Wire\n\n## Report\nA court body reports back over the wire: @S opens, @F per finding, @? per hole, @E closes.\n\n## Thoughts\n"+strings.Join(thoughts, "\n")+"\n")
		writeFile(filepath.Join(W, "README.md"), "# selftest world\n\n## About\nA throwaway world for the memory selftest.\n")
		// index
		r := run("index", "--json")
		ok(r.code == 0 && r.str("@S") == "INDEXED" && r.num("episodic") == 2 && r.num("procedural") >= 3 && r.num("semantic") >= 7, "index: "+head(r.out, 200))
		tmpLeft := false
		for _, e := range readDirSorted(filepath.Join(W, ".isekai", "memory", "long"), false) {
			if strings.HasSuffix(e.Name(), ".tmp") {
				tmpLeft = true
			}
		}
		ok(Exists(filepath.Join(W, ".isekai", "memory", "long", "index.json")) && !tmpLeft, "index: file present, no temp left")
		// recall: miss, then hit; both the log entry and the mind section must be in the top 3
		r = run("recall", "how does a court body report back over the wire", "-k", "3", "--json")
		ok(r.code == 0 && r.str("@S") == "MISS" && len(r.arr("results")) == 3, "recall miss: "+head(r.out, 200))
		hasLog, hasMind, clean := false, false, true
		for _, x := range r.arr("results") {
			m := x.(map[string]any)
			src, _ := m["src"].(string)
			if strings.HasSuffix(src, "log.md") && m["rec"].(float64) > 0 {
				hasLog = true
			}
			if strings.HasSuffix(src, "SKILL.md") {
				hasMind = true
			}
			if s := m["sim"].(float64); !(s > 0 && s <= 1) {
				clean = false
			}
		}
		ok(hasLog && hasMind, "recall: log entry (with recency) and wire mind in top 3")
		ok(clean, "recall: sims in (0,1], no NaN")
		first, _ := MarshalJS(r.path("results"))
		r = run("recall", "how does a court body report back over the wire", "-k", "3", "--json")
		again, _ := MarshalJS(r.path("results"))
		ok(r.str("@S") == "HIT" && r.num("cacheSim") >= 0.92 && string(first) == string(again), "recall hit: "+head(r.out, 120))
		r = run("recall", "how does a court body report back over the wire", "-k", "2", "--json")
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
			if m["title"] == "law › The gate" {
				gate = true
			}
		}
		ok(allSem && gate, "recall --kind: CRLF law sections harvested clean")
		r = run("recall", "x", "-k", "0")
		ok(r.code == 2 && strings.Contains(r.out, "@S FAIL"), "recall: -k 0 is a FAIL, not a guess")
		// remember (world), concurrent remembers, relation boost as slime-auth
		r = run("remember", "login tokens expire hourly", "--as", "slime-auth", "--tag", "auth", "--kind", "territory", "--json")
		ok(r.str("@S") == "REMEMBERED" && r.str("scope") == "world" && r.str("tag") == "auth" && r.str("kind") == "territory", "remember: "+head(r.out, 120))
		r = run("remember", "the gate is run twice in practice, once before lunch", "--as", "orc-api", "--kind", "colony", "--json")
		ok(r.str("@S") == "REMEMBERED" && r.str("kind") == "colony", "remember --kind colony: "+head(r.out, 120))
		ok(run("remember", "x", "--kind", "tribal").code == 2, "remember --kind tribal (not an isekai name) is a FAIL")
		r = run("remember", "plain note", "--as", "body-x", "--json")
		ok(r.path("kind") == nil && has(r.j, "kind"), "remember without --kind stores kind null")
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				run("remember", fmt.Sprintf("parallel note %d", i), "--as", fmt.Sprintf("body-%d", i))
			}(i)
		}
		wg.Wait()
		notes := ReadJSONL(filepath.Join(W, ".isekai", "memory", "shared", "notes.jsonl"))
		texts := map[string]bool{}
		for _, raw := range notes {
			var n Note
			json.Unmarshal(raw, &n)
			texts[n.Text] = true
		}
		ok(len(notes) == 15 && len(texts) == 15, fmt.Sprintf("remember: 15 parse-clean lines after 12 concurrent appends, got %d", len(notes)))
		r = run("recall", "login tokens", "--as", "slime-auth", "--json")
		var canon, note map[string]any
		for _, x := range r.arr("results") {
			m := x.(map[string]any)
			if src, _ := m["src"].(string); strings.HasSuffix(src, "canon/notes.md") && canon == nil {
				canon = m
			}
			if m["kind"] == "shared" && note == nil {
				note = m
			}
		}
		ok(canon != nil && canon["rel"].(float64) >= 0.25, fmt.Sprintf("relation: canon section naming slime-auth + its orc gets ≥0.25, got %v", canon))
		ok(note != nil && note["rel"].(float64) >= 0.05 && note["rec"].(float64) > 0, fmt.Sprintf("relation: own shared note gets +0.05 and recency, got %v", note))
		r = run("recall", "login tokens", "--tier", "shared", "--json")
		onlyShared := len(r.arr("results")) > 0
		for _, x := range r.arr("results") {
			if x.(map[string]any)["kind"] != "shared" {
				onlyShared = false
			}
		}
		ok(onlyShared, "recall --tier shared: notes only")
		r = run("recall", "gate lunch tokens", "--kind", "colony", "--json")
		res := r.arr("results")
		ok(len(res) == 1 && strings.Contains(res[0].(map[string]any)["title"].(string), "colony") && strings.Contains(res[0].(map[string]any)["snippet"].(string), "lunch"), "recall --kind colony: only the colony note, got "+head(r.out, 160))
		r = run("recall", "login tokens", "--kind", "territory", "--tier", "shared", "--json")
		res = r.arr("results")
		ok(len(res) == 1 && strings.Contains(res[0].(map[string]any)["title"].(string), "territory"), "recall --kind territory --tier shared: only the territory note")
		r = run("recall", "anything at all", "--kind", "law", "--json")
		ok(len(r.arr("results")) == 0 && r.holesMatch("no shared note carries kind law"), "recall --kind law with no law note is a @?, not a guess")
		ok(run("recall", "x", "--kind", "colony", "--tier", "long").code == 2, "recall --kind colony --tier long is a FAIL (unsaid kinds live in shared)")
		ok(run("recall", "x", "--kind", "tribal").code == 2, "recall --kind tribal is a FAIL")
		r = run("recall", "login", "--as", "Slime/Auth Zone!")
		ok(r.code == 0 && Exists(filepath.Join(W, ".isekai", "memory", "short", "slime-auth-zone-.jsonl")), "recall: an odd creature name gets a sanitised cache file")
		// machine tier lands in the throwaway HOME, never the real one
		r = run("remember", "machine note", "--machine", "--json")
		ok(r.str("scope") == "machine" && Exists(filepath.Join(H, ".isekai", "shared", "notes.jsonl")), "remember --machine: written under the throwaway home")
		// status
		r = run("status", "--json")
		ok(r.path("long", "indexed") == true && r.path("long", "stale") == false && r.num("shared", "world") == 15 && r.num("shared", "machine") == 1, "status: "+head(r.out, 200))
		ok(r.num("shared", "unsaid", "territory") == 1 && r.num("shared", "unsaid", "colony") == 1 && r.num("shared", "unsaid", "law") == 0, "status: shared notes by unsaid kind")
		ok(regexp.MustCompile(`SHARED  world 15 · machine 1 · unsaid law 0 · colony 1 · territory 1`).MatchString(run("status").out), "status text: unsaid kinds on the SHARED line")
		stressedWire := false
		for _, s := range r.arr("short", "workingMemory", "stressed") {
			if s == "wire" {
				stressedWire = true
			}
		}
		ok(r.num("short", "workingMemory", "desks") == 3 && stressedWire, "status: 3 desks, wire STRESSED")
		ok(r.path("short", "contextWindow", "available") == false && r.holesMatch("context window"), "status: no transcript is a @? finding")
		ok(r.num("short", "semanticCache", "entries") == 7 && r.num("short", "semanticCache", "hits") == 1, fmt.Sprintf("status: cache entries/hits, got %v", r.path("short", "semanticCache")))
		// stale: a source changed after the build
		f, _ := os.OpenFile(filepath.Join(W, ".isekai", "log.md"), os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString("\n### [2026-09-22 02:00] rimuru — Later entry\n- **Task:** staleness\n")
		f.Close()
		os.Chtimes(filepath.Join(W, ".isekai", "log.md"), time.Now(), time.Now().Add(2*time.Second))
		r = run("status", "--json")
		ok(r.path("long", "stale") == true && r.holesMatch(`older than its sources.*log\.md`), fmt.Sprintf("status stale: %v", r.path("@?")))
		r = run("recall", "later entry", "--json")
		ok(r.holesMatch("older than its sources"), "recall: stale index is a @?")
		ok(run("index", "--json").num("episodic") == 3 && run("status", "--json").path("long", "stale") == false, "index: rebuild clears stale")
		// forget
		r = run("forget", "--short", "--json")
		ok(r.str("@S") == "FORGOT" && r.num("n") == 7 && !Exists(filepath.Join(W, ".isekai", "memory", "short", "rimuru.jsonl")), "forget: "+head(r.out, 120))
		ok(run("forget").code == 2, "forget without --short is a FAIL")
		// empty world: every silence is a finding, nothing crashes
		E := filepath.Join(T, "empty")
		os.MkdirAll(filepath.Join(E, ".isekai"), 0o755)
		runE := func(args ...string) selfRun { return runCLI(cli, E, H, args...) }
		ok(runE("recall", "anything", "--json").holesMatch("no long-term index"), "empty world: recall without index is a @?")
		r = runE("index", "--json")
		ok(r.code == 0 && r.num("n") == 0 && len(r.arr("@?")) == 1, "empty world: index of nothing is a @?")
		r = runE("status", "--json")
		ok(r.code == 0 && r.path("long", "indexed") == true && r.num("long", "episodic") == 0 && r.num("short", "workingMemory", "desks") == 0, "empty world: status parses")
	}()
	ok(fileSize(filepath.Join(realHome, ".isekai", "shared", "notes.jsonl")) == realMachine, "the real ~/.isekai/shared was not touched")
	ok(!Exists(T), "throwaway world removed")
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
