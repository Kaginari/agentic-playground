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

	"github.com/Kaginari/agent-one/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"
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

// tuiSession opens the app on a test workspace and drives its host under teatest.
func tuiSession(t *testing.T, w *testWorkspace) (*App, *teatest.TestModel, *screen) {
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
	sc.wait(t, "✦ agent-one")
	t.Cleanup(func() {
		tm.Send(tui.EvQuit{})
		tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
	})
	return a, tm, sc
}

func enter(tm *teatest.TestModel) { tm.Send(tea.KeyMsg{Type: tea.KeyEnter}) }

func TestTUIJunctionTurn(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("say hi", map[string]interface{}{"text": "Let me run it.\n\n", "calls": []interface{}{map[string]interface{}{"name": "bash", "input": map[string]string{"command": "echo hi; echo there"}}}}),
		when("there", call("edit", map[string]string{"path": "README.md", "old": "hello", "new": "hello\n\nthe login zone"})),
		when("edited", text("All **done**: `hi` was said and the file edited.\n\n```go\npackage auth\n```\n")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
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
	if !strings.Contains(w.read(".agent-one/log.md"), "Gate") {
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
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		text("push attempted"),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
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
	if j := w.journalText(".agent-one"); !strings.Contains(j, `"by":"tty"`) || !strings.Contains(j, `"decision":"approved"`) {
		t.Fatalf("the journal does not carry the operator's approval:\n%s", j)
	}
	if _, err := readFile(w, ".agent-one/config.local.yaml"); err == nil {
		t.Fatal("a plain yes must not write a rule")
	}
}

func TestTUIApprovalDontAskAgain(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		call("bash", map[string]string{"command": "git push origin main"}),
		text("pushed twice"),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	w.write(".agent-one/config.local.yaml", "permissions:\n  rules:\n    - {match: \"bash:make *\", action: allow}\n")
	_, tm, sc := tuiSession(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "❯ 1. Yes")
	tm.Type("2")
	sc.wait(t, "rule bash:git push* → allow written to .agent-one/config.local.yaml")
	sc.wait(t, "pushed twice")
	local, _ := readFile(w, ".agent-one/config.local.yaml")
	if !strings.Contains(local, `bash:git push*`) || !strings.Contains(local, `bash:make *`) {
		t.Fatalf("the local layer lost a rule or gained none:\n%s", local)
	}
	j := w.journalText(".agent-one")
	if strings.Count(j, `"by":"tty"`) != 1 || !strings.Contains(j, `"by":"rule"`) {
		t.Fatalf("the second push must pass by the rule, live:\n%s", j)
	}
}

func TestTUIApprovalNoWithReason(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		when("run the tests first", text("Understood: I will run the tests before any push.")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
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
	j := w.journalText(".agent-one")
	if !strings.Contains(j, `"by":"tty"`) || !strings.Contains(j, `"decision":"denied"`) {
		t.Fatalf("the journal does not carry the denial:\n%s", j)
	}
	if !strings.Contains(w.read(".agent-one/instruments/loop/"+firstJournal(w)), "the operator said: run the tests first") {
		t.Fatal("the reason did not reach the model's tool result")
	}
}

func TestTUISubagentsAndQuestion(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/session.json",
		when("look", call("dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "findings: look at the login zone"})),
		when("@F src/auth/login.go:1 fine", call("dispatch", map[string]interface{}{"agent": "zone-api", "ask": "findings: look at the api", "background": true})),
		when("started in the background", call("ask", map[string]interface{}{"question": "Which zone next?", "options": []string{"auth", "api"}})),
		when("api", text("The api zone it is.")),
		when("[subagent zone-api reported]", text("Both subagents reported.")),
	)
	w.script(".agent-one/tmp/subagent.json",
		when("login zone", text("@S PASS\n@F src/auth/login.go:1 fine\n@? no test for refresh\n@U team subagents can run behind\n@E 80")),
		when("the api", call("bash", map[string]string{"command": "sleep 1; echo slept"})),
		when("slept", text("@S PASS\n@F src/api/server.go:1 fine too\n@U domain the api zone serves json\n@E 70")),
	)
	w.write(".agent-one/config.yaml", `
models: {default: mock/m, roles: {analyst: subagent/c}}
providers:
  mock: {enabled: true, script: .agent-one/tmp/session.json}
  subagent: {type: mock, script: .agent-one/tmp/subagent.json}
tools: {bash: {sandbox: none}, dispatch: {background: true}}
ui: {board: {autostart: false}}
`)
	_, tm, sc := tuiSession(t, w)
	tm.Type("look")
	enter(tm)
	sc.wait(t, "→ subagent zone-auth started")
	sc.wait(t, "◆ subagent zone-auth · zone · analyst · subagent/c")
	sc.wait(t, "⎿ PASS")
	sc.wait(t, "● src/auth/login.go:1 fine")
	sc.wait(t, "∴ team: subagents can run behind")
	sc.wait(t, "❯ 1. auth")
	tm.Type("2")
	sc.wait(t, "The api zone it is.")
	sc.wait(t, "◆ subagent zone-api")
	sc.wait(t, "● src/api/server.go:1 fine too")
	sc.wait(t, "↻ continuing with what arrived")
	sc.wait(t, "Both subagents reported.")
}

func TestTUIInterruptAndQueue(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("slow", call("bash", map[string]string{"command": "sleep 5; echo late"})),
		when("late", text("late came")),
		when("again", text("second turn fine")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("slow")
	enter(tm)
	sc.wait(t, "Running bash…")
	tm.Type("a note")
	enter(tm)
	sc.wait(t, "⏎ queued: a note")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	sc.wait(t, "■ interrupted")
	tm.Type("again")
	enter(tm)
	sc.wait(t, "second turn fine")
}

func TestWantsTUI(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", text("nothing"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	if a.wantsTUI(false) {
		t.Fatal("a pipe must keep the line REPL")
	}
	if a.wantsTUI(true) {
		t.Fatal("--plain must keep the line REPL")
	}
}

func readFile(w *testWorkspace, p string) (string, error) {
	b, err := os.ReadFile(filepath.Join(w.root, p))
	return string(b), err
}

func firstJournal(w *testWorkspace) string {
	ents, _ := os.ReadDir(filepath.Join(w.root, ".agent-one/instruments/loop"))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			return e.Name()
		}
	}
	return ""
}
