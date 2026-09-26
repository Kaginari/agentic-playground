package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/loop"
)

// syncBuffer is a bytes.Buffer safe for a writer goroutine and a reading test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestEmbeddedLawsMatchTheWorld(t *testing.T) {
	for _, c := range []struct{ file, dist string }{{"../../.isekai/isekai.md", "isekai"}, {"../../agent-one/AGENT-ONE.md", "agent-one"}} {
		want, err := os.ReadFile(c.file)
		if err != nil {
			t.Skipf("%s: %v", c.file, err)
		}
		if string(want) != LawText(c.dist) {
			t.Errorf("app/law/%s differs from %s — Vitality: copy it in the same change", filepath.Base(c.file), c.file)
		}
	}
}

// mainIO runs Main with piped stdin and a fake environment rooted at the test world.
func (w *testWorld) main(dist string, stdin string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	env := func(k string) string {
		if k == "HOME" {
			return w.home
		}
		return ""
	}
	code := Main(dist, append([]string{"--root", w.root, "--quiet", "--no-board"}, args...), IO{In: strings.NewReader(stdin), Out: &out, Err: &errb, Env: env}, Version{Version: "test", Commit: "abc", Date: "today"})
	return code, out.String(), errb.String()
}

func TestCLIRunAndSessions(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/session.json",
		when("say hi", call("bash", map[string]string{"command": "echo hi > hi.txt && cat hi.txt"})),
		when("hi\n", text("@S DONE\n@F hi.txt:1 hi\n@E 24")),
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		when("continue", text("@S DONE resumed\n@E 20")),
	)
	w.write(".isekai/config.yaml", `
models: {default: mock/m}
providers: {mock: {enabled: true, script: .isekai/tmp/session.json}}
tools: {bash: {sandbox: none}}
ontology: {enabled: false}
ui: {board: {autostart: false}}
`)
	// version
	code, out, _ := w.main("isekai", "", "version")
	if code != 0 || !strings.HasPrefix(out, "isekai test (abc, today, ") {
		t.Errorf("version: %d %q", code, out)
	}
	// run --json: a JSON line, exit 0, the usage journal in the schema
	code, out, errb := w.main("isekai", "", "run", "--json", "--session", "s-run", "say hi")
	if code != 0 {
		t.Fatalf("run: %d %s %s", code, out, errb)
	}
	var res map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil || res["@S"] != "DONE" || res["exit"].(float64) != 0 {
		t.Fatalf("run --json: %v %q", err, out)
	}
	if u, _ := res["usage"].(map[string]interface{}); u["input"].(float64) <= 0 {
		t.Errorf("usage in the result: %v", res["usage"])
	}
	if w.read("hi.txt") != "hi\n" {
		t.Error("the run's write did not land")
	}
	journal := w.read(".isekai/instruments/usage/s-run.jsonl")
	var rec map[string]interface{}
	_ = json.Unmarshal([]byte(strings.Split(journal, "\n")[0]), &rec)
	for _, k := range []string{"input", "output", "cacheRead", "cacheWrite", "usd"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("usage record lacks %s: %s", k, journal)
		}
	}
	if rec["usd"] != nil {
		t.Errorf("mock is unpriced: usd must be null, got %v", rec["usd"])
	}
	// a denied outward act exits 4 (no TTY, no pre-approval)
	code, out, _ = w.main("isekai", "", "run", "--session", "s-deny", "push it")
	if code != 4 || !strings.Contains(out, "denied at the gate") {
		t.Errorf("denied run: %d %q", code, out)
	}
	// sessions lists both; resume continues the first with an ask
	code, out, _ = w.main("isekai", "", "sessions")
	if code != 0 || !strings.Contains(out, "s-run") || !strings.Contains(out, "s-deny") || !strings.Contains(out, "say hi") {
		t.Errorf("sessions: %d %q", code, out)
	}
	code, out, errb = w.main("isekai", "", "resume", "s-run", "continue")
	if code != 0 || !strings.Contains(out, "@S DONE resumed") {
		t.Errorf("resume: %d %q %q", code, out, errb)
	}
	events, err := NewSessionStore(filepath.Join(w.home, ".local", "share", "isekai", "sessions"), w.root).Read("s-run")
	if err != nil || len(events) < 4 {
		t.Fatalf("session file: %v %d", err, len(events))
	}
	kinds := ""
	for _, e := range events {
		kinds += e.Kind + ","
	}
	if !strings.Contains(kinds, "user,assistant,tool,assistant,") || !strings.Contains(kinds, "resume,") {
		t.Errorf("session kinds: %s", kinds)
	}
	// status: the honesty rule names the switch and its origin, then the board
	code, out, _ = w.main("isekai", "", "status")
	if code != 0 || !strings.Contains(out, "@? off ontology.enabled — ") || !strings.Contains(out, "config.yaml:5") || !strings.Contains(out, "sandbox: none") || !strings.Contains(out, "runs:") {
		t.Errorf("status: %d %q", code, out)
	}
	// usage rolls the journal up
	code, out, _ = w.main("isekai", "", "usage")
	if code != 0 || !strings.Contains(out, "by body:") || !strings.Contains(out, "rimuru") {
		t.Errorf("usage: %d %q", code, out)
	}
	code, out, _ = w.main("isekai", "", "usage", "--session", "s-deny")
	if code != 0 || !strings.Contains(out, "1 calls") {
		t.Errorf("usage --session: %d %q", code, out)
	}
	// config check answers on the wire
	code, out, _ = w.main("isekai", "", "config", "check")
	if code != 0 || !strings.HasPrefix(out, "@S PASS isekai") || !strings.Contains(out, "@? off ontology.enabled") {
		t.Errorf("config check: %d %q", code, out)
	}
	// the instrument CLIs find the world
	for _, args := range [][]string{{"memory", "status"}, {"toolbox", "status"}, {"onto", "check"}} {
		code, out, errb := w.main("isekai", "", args...)
		if code != 0 && !strings.Contains(out+errb, "@S") {
			t.Errorf("%v: %d %q %q", args, code, out, errb)
		}
	}
	// selftest through the CLI
	code, out, errb = w.main("isekai", "", "selftest")
	if code != 0 || !strings.HasPrefix(out, "@S PASS ") {
		t.Errorf("selftest: %d %q %q", code, out, errb)
	}
	// unknown command, missing ask
	if code, _, errb := w.main("isekai", "", "nonsense"); code != 2 || !strings.Contains(errb, "unknown command") {
		t.Errorf("unknown: %d %q", code, errb)
	}
	if code, _, errb := w.main("isekai", "", "run"); code != 2 || !strings.Contains(errb, "needs an ask") {
		t.Errorf("no ask: %d %q", code, errb)
	}
}

func TestInitBench(t *testing.T) {
	for _, dist := range []string{"isekai", "agent-one"} {
		w := newTestWorld(t, dist, nil)
		_ = os.MkdirAll(w.root, 0o755)
		_ = os.WriteFile(filepath.Join(w.root, "outside.txt"), []byte("x"), 0o644)
		code, out, errb := w.main(dist, "", "init", "--bench")
		if code != 0 || !strings.HasPrefix(out, "@S OK "+dist+" founded") {
			t.Fatalf("%s init: %d %q %q", dist, code, out, errb)
		}
		d, _ := parseDist(dist)
		for _, p := range []string{d.LawFile, "log.md", "instruments/usage", "instruments/loop", "memory/shared/notes.jsonl"} {
			if _, err := os.Stat(filepath.Join(w.root, d.WorldDir, p)); err != nil {
				t.Errorf("%s: %s missing", dist, p)
			}
		}
		if law := w.read(d.WorldDir + "/" + d.LawFile); !strings.Contains(law, "## "+lexiconFor(dist).Crest) {
			t.Errorf("%s: the seeded law has no crest heading %q", dist, lexiconFor(dist).Crest)
		}
		code, out, _ = w.main(dist, "", "init", "--bench")
		if code != 0 || !strings.Contains(out, "nothing touched") {
			t.Errorf("%s: init is not idempotent: %q", dist, out)
		}
		ents, _ := os.ReadDir(w.root)
		if len(ents) != 2 {
			t.Errorf("%s: init wrote outside the world dir: %v", dist, ents)
		}
		// a run on the founded world with the config from the env (the bench's path)
		w.write(d.WorldDir+"/tmp/s.json", `[{"calls":[{"name":"bash","input":{"command":"printf 'hello isekai\n' > hello.txt && cat hello.txt"}}]},{"text":"@S DONE wrote hello.txt\n@E 30"}]`)
		var out2, errb2 bytes.Buffer
		env := func(k string) string {
			switch k {
			case "HOME":
				return w.home
			case d.EnvPrefix + "CONFIG_CONTENT":
				return "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: " + d.WorldDir + "/tmp/s.json}}\ntools: {profile: minimal, bash: {sandbox: none}}\nui: {board: {autostart: false}}\n"
			}
			return ""
		}
		code = Main(dist, []string{"--root", w.root, "--quiet", "run", "--json", "Create the file hello.txt"}, IO{In: strings.NewReader(""), Out: &out2, Err: &errb2, Env: env}, Version{})
		if code != 0 || !strings.Contains(out2.String(), `"@S":"DONE"`) || w.read("hello.txt") != "hello isekai\n" {
			t.Errorf("%s bench-style run: %d %q %q", dist, code, out2.String(), errb2.String())
		}
	}
}

func TestREPLLive(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/session.json",
		when("first", call("bash", map[string]string{"command": "echo one"})),
		when("a note from the human", text("@S DONE the note arrived at the tool step\n@E 40")),
		when("second", call("dispatch", map[string]interface{}{"body": "slime-auth", "ask": "findings: look", "background": true})),
		when("started in the background", text("@S DONE court running\n@E 30")),
		when("[court slime-auth reported]", text("@S DONE woke with the report\n@E 40")),
		when("Review src carefully", text("@S DONE reviewed src\n@E 30")),
	)
	w.script(".isekai/tmp/court.json", when("@ASK findings", text("@S PASS\n@F src/auth/login.go:1 fine\n@U colony courts can run behind\n@E 60")))
	w.write(".claude/commands/review.md", "Review $ARGUMENTS carefully\n")
	w.write(".isekai/config.yaml", `
models: {default: mock/m, offices: {great-sage: court/c}}
providers:
  mock: {enabled: true, script: .isekai/tmp/session.json}
  court: {type: mock, script: .isekai/tmp/court.json}
tools: {bash: {sandbox: none}, dispatch: {background: true}}
ui: {board: {autostart: false}}
`)
	a, err := New(Options{Dist: "isekai", Root: w.root, Cwd: w.root, Home: w.home, Env: func(k string) string { return map[string]string{"HOME": w.home}[k] },
		In: strings.NewReader(""), Out: &strings.Builder{}, Err: &strings.Builder{}, Quiet: true, IsTTY: func() bool { return false }, NoBoard: true, Session: "s-live"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	// mid-turn input: a line queued before the tool step rides the next tool-result message
	e, _ := a.Engine()
	s := e.NewSession()
	a.Inbox = func(ls *loop.Session) []string { return a.Court.Inbox(bodyName(ls)) }
	_ = a.Court.Send("rimuru", "a note from the human")
	r, err := s.Turn(context.Background(), "first")
	if err != nil || r.Status != loop.Done || !strings.Contains(r.Text, "the note arrived at the tool step") {
		t.Fatalf("inbox delivery: %v %s %q", err, r.Status, r.Text)
	}
	// the REPL over a pipe: a background court wakes the session; slash commands answer
	var out, errb syncBuffer
	pr, pw := io.Pipe()
	a.Opt.In, a.Opt.Out, a.Opt.Err = pr, &out, &errb
	done := make(chan int, 1)
	go func() { done <- a.REPL(context.Background()) }()
	say := func(line string, wantAfter string) {
		t.Helper()
		_, _ = io.WriteString(pw, line+"\n")
		if wantAfter == "" {
			return
		}
		deadline := time.Now().Add(20 * time.Second)
		for !strings.Contains(out.String()+errb.String(), wantAfter) {
			if time.Now().After(deadline) {
				t.Fatalf("waiting for %q after %q:\n%s%s", wantAfter, line, out.String(), errb.String())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	say("second", "woke with the report")
	say("/agents", "great-sage")
	say("/status", "sandbox: none")
	say("/usage", "by body:")
	say("/sessions", "s-live")
	say("/review src", "reviewed src")
	say("/help", "/agents · /send")
	say("/nonsense", "no command /nonsense")
	say("/quit", "")
	pw.Close()
	code := <-done
	all := out.String() + errb.String()
	if code != 0 {
		t.Fatalf("repl: %d\n%s", code, all)
	}
	for _, want := range []string{"court running", "→ slime-auth started", "← slime-auth reported", "woke with the report", "slime-auth", "great-sage", "by body:", "s-live", "reviewed src", "/agents · /send", "no command /nonsense"} {
		if !strings.Contains(all, want) {
			t.Errorf("repl output lacks %q:\n%s", want, all)
		}
	}
	if !strings.Contains(w.read(".isekai/memory/shared/notes.jsonl"), "courts can run behind") {
		t.Error("the background court's @U did not land")
	}
}

func TestBenchOnMock(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.write(".isekai/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true}}\ntools: {bash: {sandbox: none}}\nui: {board: {autostart: false}}\n")
	code, out, errb := w.main("isekai", "", "bench")
	if code != 0 || !strings.Contains(out, "@S PASS") {
		t.Fatalf("bench: %d\n%s\n%s", code, out, errb)
	}
	for _, task := range []string{"edit in territory + gate", "dispatch a Court", "survive a drain", "custom tool call"} {
		if !strings.Contains(out, task) {
			t.Errorf("bench table lacks %q:\n%s", task, out)
		}
	}
	if !strings.Contains(out, "anthropic") || !strings.Contains(out, "ANTHROPIC_API_KEY is not set") {
		t.Errorf("real providers are skipped by name, never faked:\n%s", out)
	}
}

// TestRunIgnoresAnOpenStdin: with the ask on the command line, run never reads stdin — an
// inherited pipe that never closes (CI, another agent, cron) must not hang it.
func TestRunIgnoresAnOpenStdin(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json", when("say hi", text("@S DONE hi\n@E 10")))
	w.write(".isekai/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: .isekai/tmp/s.json}}\nui: {board: {autostart: false}}\n")
	r, pw := io.Pipe() // never written, never closed
	defer pw.Close()
	done := make(chan int, 1)
	go func() {
		var out, errb bytes.Buffer
		done <- Main("isekai", []string{"--root", w.root, "--quiet", "--no-board", "run", "say hi"}, IO{In: r, Out: &out, Err: &errb, Env: func(k string) string {
			if k == "HOME" {
				return w.home
			}
			return ""
		}}, Version{Version: "test"})
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("run exited %d", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("run blocked on an open stdin")
	}
}
