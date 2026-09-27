package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kaginari/agentic-playground/isekai/tui"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// The junction of the terminal UI with the real engine: the app's host under teatest, the mock
// provider scripted, real tools and the real classifier behind the gate.

type tmSender struct{ tm *teatest.TestModel }

func (s tmSender) Send(m tea.Msg) { s.tm.Send(m) }

type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *screen) follow(r io.Reader) {
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				s.mu.Lock()
				s.buf.Write(b[:n])
				s.mu.Unlock()
			}
			if err == io.EOF {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			if err != nil {
				return
			}
		}
	}()
}

func (s *screen) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ansi.Strip(s.buf.String())
}

func (s *screen) wait(t testing.TB, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(s.text(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %q; the screen so far:\n%s", want, s.text())
		}
		time.Sleep(15 * time.Millisecond)
	}
}

// tuiSession opens the app on a test world and drives its host under teatest.
func tuiSession(t *testing.T, w *testWorld) (*App, *teatest.TestModel, *screen) {
	t.Helper()
	a := w.open()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := tui.New(h, tui.NewTheme(true), h.words)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))
	m.Attach(tm.Send)
	h.attach(tmSender{tm})
	sc := &screen{}
	sc.follow(tm.Output())
	sc.wait(t, "✦ isekai")
	t.Cleanup(func() {
		tm.Send(tui.EvQuit{})
		tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
	})
	return a, tm, sc
}

func enter(tm *teatest.TestModel) { tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter}) }

func TestTUIJunctionTurn(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json",
		when("say hi", map[string]interface{}{"text": "Let me run it.\n\n", "calls": []interface{}{map[string]interface{}{"name": "bash", "input": map[string]string{"command": "echo hi; echo there"}}}}),
		when("there", call("edit", map[string]string{"path": "README.md", "old": "hello", "new": "hello\n\nthe login zone"})),
		when("edited", text("All **done**: `hi` was said and the file edited.\n\n```go\npackage auth\n```\n")),
	)
	w.write(".isekai/config.yaml", mockCfg(".isekai/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("say hi")
	enter(tm)
	sc.wait(t, "> say hi")
	sc.wait(t, "● bash  echo hi; echo there  read")
	sc.wait(t, "⎿ ok · ")
	sc.wait(t, "there")
	sc.wait(t, "● edit  README.md  write")
	sc.wait(t, "+ the login zone")
	sc.wait(t, "hi was said and the file edited")
	sc.wait(t, "✓ gate · ")
	s := sc.text()
	if strings.Index(s, "Let me run it.") > strings.Index(s, "● bash") {
		t.Fatalf("the streamed text lands before the tool block:\n%s", s)
	}
	if !strings.Contains(s, "+2 −0") {
		t.Fatalf("the diff stat is missing:\n%s", s)
	}
	if !strings.Contains(w.read(".isekai/log.md"), "Gate") {
		t.Fatal("the gate verdict did not reach log.md")
	}
	// the footer reads the instruments
	sc.wait(t, "mock/m · ctx ")
	// a slash command through the menu
	tm.Type("/stat")
	sc.wait(t, "❯ /status")
	enter(tm)
	sc.wait(t, "sandbox: none")
}

func TestTUIApprovalYes(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		text("push attempted"),
	)
	w.write(".isekai/config.yaml", mockCfg(".isekai/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "❯ 1. Yes")
	if s := sc.text(); !strings.Contains(s, "outward  bash  git push origin main") || !strings.Contains(s, "don't ask again for bash:git push*") {
		t.Fatalf("approval block:\n%s", s)
	}
	enter(tm)
	sc.wait(t, "✓ Yes")
	sc.wait(t, "push attempted")
	if j := w.journalText(".isekai"); !strings.Contains(j, `"by":"tty"`) || !strings.Contains(j, `"decision":"approved"`) {
		t.Fatalf("the journal does not carry the human's approval:\n%s", j)
	}
	if _, err := readFile(w, ".isekai/config.local.yaml"); err == nil {
		t.Fatal("a plain yes must not write a rule")
	}
}

func TestTUIApprovalDontAskAgain(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		call("bash", map[string]string{"command": "git push origin main"}),
		text("pushed twice"),
	)
	w.write(".isekai/config.yaml", mockCfg(".isekai/tmp/s.json", ""))
	w.write(".isekai/config.local.yaml", "permissions:\n  rules:\n    - {match: \"bash:make *\", action: allow}\n")
	_, tm, sc := tuiSession(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "❯ 1. Yes")
	tm.Type("2")
	sc.wait(t, "rule bash:git push* → allow written to .isekai/config.local.yaml")
	sc.wait(t, "pushed twice")
	local, _ := readFile(w, ".isekai/config.local.yaml")
	if !strings.Contains(local, `bash:git push*`) || !strings.Contains(local, `bash:make *`) {
		t.Fatalf("the local layer lost a rule or gained none:\n%s", local)
	}
	j := w.journalText(".isekai")
	if strings.Count(j, `"by":"tty"`) != 1 || !strings.Contains(j, `"by":"rule"`) {
		t.Fatalf("the second push must pass by the rule, live:\n%s", j)
	}
}

func TestTUIApprovalNoWithReason(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		when("run the tests first", text("Understood: I will run the tests before any push.")),
	)
	w.write(".isekai/config.yaml", mockCfg(".isekai/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "❯ 1. Yes")
	tm.Type("3")
	sc.wait(t, "why not?")
	tm.Type("run the tests first")
	enter(tm)
	sc.wait(t, "✓ No, and tell the model why: run the tests first")
	sc.wait(t, "denied at the gate")
	sc.wait(t, "> run the tests first")
	sc.wait(t, "I will run the tests before any push")
	j := w.journalText(".isekai")
	if !strings.Contains(j, `"by":"tty"`) || !strings.Contains(j, `"decision":"denied"`) {
		t.Fatalf("the journal does not carry the denial:\n%s", j)
	}
	if !strings.Contains(w.read(".isekai/instruments/loop/"+firstJournal(w)), "the human said: run the tests first") {
		t.Fatal("the reason did not reach the model's tool result")
	}
}

func TestTUICourtsAndQuestion(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/session.json",
		when("look", call("dispatch", map[string]interface{}{"body": "slime-auth", "ask": "findings: look at the login zone"})),
		when("@F src/auth/login.go:1 fine", call("dispatch", map[string]interface{}{"body": "slime-api", "ask": "findings: look at the api", "background": true})),
		when("started in the background", call("ask", map[string]interface{}{"question": "Which zone next?", "options": []string{"auth", "api"}})),
		when("api", text("The api zone it is.")),
		when("[court slime-api reported]", text("Both courts reported.")),
	)
	w.script(".isekai/tmp/court.json",
		when("login zone", text("@S PASS\n@F src/auth/login.go:1 fine\n@? no test for refresh\n@U colony courts can run behind\n@E 80")),
		when("the api", call("bash", map[string]string{"command": "sleep 1; echo slept"})),
		when("slept", text("@S PASS\n@F src/api/server.go:1 fine too\n@U territory the api zone serves json\n@E 70")),
	)
	w.write(".isekai/config.yaml", `
models: {default: mock/m, offices: {great-sage: court/c}}
providers:
  mock: {enabled: true, script: .isekai/tmp/session.json}
  court: {type: mock, script: .isekai/tmp/court.json}
tools: {bash: {sandbox: none}, dispatch: {background: true}}
ui: {board: {autostart: false}}
`)
	_, tm, sc := tuiSession(t, w)
	tm.Type("look")
	enter(tm)
	sc.wait(t, "→ court slime-auth started")
	sc.wait(t, "◆ court slime-auth · slime · great-sage · court/c")
	sc.wait(t, "⎿ PASS")
	sc.wait(t, "● src/auth/login.go:1 fine")
	sc.wait(t, "∴ colony: courts can run behind")
	sc.wait(t, "❯ 1. auth")
	tm.Type("2")
	sc.wait(t, "The api zone it is.")
	sc.wait(t, "◆ court slime-api")
	sc.wait(t, "● src/api/server.go:1 fine too")
	sc.wait(t, "↻ continuing with what arrived")
	sc.wait(t, "Both courts reported.")
}

func TestTUIInterruptAndQueue(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json",
		when("slow", call("bash", map[string]string{"command": "sleep 5; echo late"})),
		when("late", text("late came")),
		when("again", text("second turn fine")),
	)
	w.write(".isekai/config.yaml", mockCfg(".isekai/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("slow")
	enter(tm)
	sc.wait(t, "Running bash…")
	tm.Type("a note")
	enter(tm)
	sc.wait(t, "⏎ queued: a note")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	sc.wait(t, "■ interrupted")
	tm.Type("again")
	enter(tm)
	sc.wait(t, "second turn fine")
}

func TestWantsTUI(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json", text("nothing"))
	w.write(".isekai/config.yaml", mockCfg(".isekai/tmp/s.json", ""))
	a := w.open()
	if a.wantsTUI(false) {
		t.Fatal("a pipe must keep the line REPL")
	}
	if a.wantsTUI(true) {
		t.Fatal("--plain must keep the line REPL")
	}
}

func readFile(w *testWorld, p string) (string, error) {
	b, err := os.ReadFile(filepath.Join(w.root, p))
	return string(b), err
}

func firstJournal(w *testWorld) string {
	ents, _ := os.ReadDir(filepath.Join(w.root, ".isekai/instruments/loop"))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			return e.Name()
		}
	}
	return ""
}

func TestTUIStartFailureReachesTheTerminal(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.write(".isekai/config.yaml", "providers:\n  vllm: {type: openai, baseURL: http://127.0.0.1:1/v1, apiKeyEnv: VLLM_API_KEY}\nmodels: {default: vllm/fake/model}\n")
	a := w.open()
	errw := a.Opt.Err
	if code := a.TUI(context.Background()); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if got := errw.(*strings.Builder).String(); !strings.Contains(got, "VLLM_API_KEY is not set") {
		t.Fatalf("a TUI that cannot start must say why on stderr; got %q", got)
	}
}

// /board's view is the real world's: the reasoned ontology as a graph with its bonds and
// knowledge, the triad as config resolves it.
func TestTUIBoardReadsTheWorld(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/s.json", text("nothing"))
	w.write(".isekai/config.yaml", mockCfg(".isekai/tmp/s.json", ""))
	a := w.open()
	h, err := a.tuiHost(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v := h.Board("24h")
	byID := map[string]tui.GraphNode{}
	for _, n := range v.Graph.Nodes {
		byID[n.ID] = n
	}
	slime, ok := byID["slime-auth"]
	if !ok || len(byID) != 5 {
		t.Fatalf("graph nodes %v, want rimuru, elf-core, orc-security, slime-auth, slime-api", v.Graph.Nodes)
	}
	if len(slime.Up) != 1 || slime.Up[0].To != "orc-security" || slime.Up[0].Bond != "truth" || slime.Rank != "slime" {
		t.Fatalf("slime-auth answers %v (rank %q), want orc-security by truth", slime.Up, slime.Rank)
	}
	if byID["rimuru"].Level != 0 || byID["elf-core"].Level != 1 || byID["orc-security"].Level != 2 || slime.Level != 3 {
		t.Fatalf("levels: %+v", byID)
	}
	know := ""
	for _, kv := range slime.Knowledge {
		know += kv.Key + "=" + kv.Value + "\n"
	}
	if !strings.Contains(know, "chain=") || !strings.Contains(know, "rimuru") || !strings.Contains(know, "owns=src/auth") {
		t.Fatalf("slime-auth's knowledge misses the inferred chain or its territory:\n%s", know)
	}
	if !strings.Contains(v.Graph.Summary, "5 creatures") || !strings.Contains(v.Graph.Summary, "reasoned") {
		t.Fatalf("summary %q", v.Graph.Summary)
	}
	if len(v.Offices) != 3 || v.Offices[0].Name != "great-sage" || v.Offices[0].Model != "mock/m" || len(v.Offices[0].Ranks) == 0 {
		t.Fatalf("offices %+v", v.Offices)
	}
}
