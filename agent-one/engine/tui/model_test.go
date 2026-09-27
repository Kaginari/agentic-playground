package tui

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"
)

// fakeHost records what the program asks of the app.
type fakeHost struct {
	mu          sync.Mutex
	submitted   []string
	queued      []string
	slashed     []string
	interrupted int
	refuse      string
}

func (h *fakeHost) Welcome() Welcome {
	return Welcome{Dist: "agent-one", Version: "vtest", Workspace: "/w", Model: "mock/m", Session: "s1"}
}
func (h *fakeHost) Footer() FooterView {
	return FooterView{Model: "mock/m", Ctx: "ctx 3%", Cost: "no calls yet", Workspace: "/w", Tokens: 1500}
}
func (h *fakeHost) Submit(text string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refuse != "" {
		return h.refuse
	}
	h.submitted = append(h.submitted, text)
	return ""
}
func (h *fakeHost) Queue(text string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.queued = append(h.queued, text)
	return "queued for the next tool step"
}
func (h *fakeHost) Interrupt() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.interrupted++
}
func (h *fakeHost) Slash(line string) ([]string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.slashed = append(h.slashed, line)
	if line == "/quit-now" {
		return nil, true
	}
	return []string{"answer to " + line}, false
}
func (h *fakeHost) Commands() []MenuItem {
	return []MenuItem{{"agents", "the live subagent"}, {"status", "the board"}, {"send", "a line for a agent"}, {"review", "Review $ARGUMENTS"}}
}
func (h *fakeHost) Complete(prefix string) []string {
	var out []string
	for _, p := range []string{"src/auth/login.go", "src/auth/token.go", "src/api/server.go"} {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

// tail accumulates the program's output so a test can wait for text across events.
type tail struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (t *tail) follow(r io.Reader) {
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				t.mu.Lock()
				t.buf.Write(b[:n])
				t.mu.Unlock()
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

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return ansi.Strip(t.buf.String())
}

func (t *tail) wait(tb testing.TB, want string) {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(t.String(), want) {
		if time.Now().After(deadline) {
			tb.Fatalf("waited for %q, output:\n%s", want, t.String())
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func start(t *testing.T, h *fakeHost) (*teatest.TestModel, *Model, *tail) {
	t.Helper()
	m := New(h, NewTheme(true), DefaultWords())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.Attach(tm.Send)
	out := &tail{}
	out.follow(tm.Output())
	out.wait(t, "✦ agent-one")
	return tm, m, out
}

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func TestTurnStreamsAndLands(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("explain the gate")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> explain the gate")
	out.wait(t, "esc to interrupt")
	tm.Send(EvState{Agent: "orchestrator", State: "thinking"})
	tm.Send(EvDelta{Text: "The gate has **four** checks.\n\nSecond paragraph "})
	out.wait(t, "four")
	tm.Send(EvDelta{Text: "done.\n"})
	tm.Send(EvToolStart{Tool: ToolView{ID: "s1", Name: "bash", Summary: "go test ./...", Class: "read"}})
	out.wait(t, "Running bash…")
	tm.Send(EvToolEnd{Tool: ToolView{ID: "s1", Name: "bash", Summary: "go test ./...", Class: "read", Status: "done", Ms: 40, Output: "ok\n"}})
	out.wait(t, "⎿ ok · 40ms · 1 line")
	tm.Send(EvTurnDone{Status: "DONE", Streamed: true, Verdict: "n/a (no domain owners)", LogRel: ".agent-one/log.md"})
	out.wait(t, "✓ gate · n/a (no domain owners) · .agent-one/log.md")
	s := out.String()
	if strings.Index(s, "Second paragraph") > strings.Index(s, "● bash") {
		t.Fatalf("the streamed text must land above the tool block:\n%s", s)
	}
	h.mu.Lock()
	sub := h.submitted
	h.mu.Unlock()
	if len(sub) != 1 || sub[0] != "explain the gate" {
		t.Fatalf("submitted %v", sub)
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestQueuedInterruptAndCtrlC(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("first")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> first")
	tm.Type("also this")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "⏎ queued: also this")
	tm.Send(key(tea.KeyEsc))
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		n := h.interrupted
		h.mu.Unlock()
		if n == 1 || time.Now().After(deadline) {
			if n != 1 {
				t.Fatalf("esc did not interrupt (%d)", n)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	tm.Send(EvTurnDone{Status: "FAIL", Interrupted: true})
	out.wait(t, "■ interrupted")
	h.mu.Lock()
	q := h.queued
	h.mu.Unlock()
	if len(q) != 1 || q[0] != "also this" {
		t.Fatalf("queued %v", q)
	}
	tm.Send(key(tea.KeyCtrlC))
	out.wait(t, "ctrl+c again to exit")
	tm.Send(key(tea.KeyCtrlC))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestSlashMenuAndCompletion(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("/")
	out.wait(t, "/agents")
	tm.Type("st")
	out.wait(t, "❯ /status")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "answer to /status")
	h.mu.Lock()
	sl := h.slashed
	h.mu.Unlock()
	if len(sl) != 1 || sl[0] != "/status" {
		t.Fatalf("slashed %v", sl)
	}
	// /help is the UI's own
	tm.Type("/help")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "ctrl+o        expand the last collapsed block")
	// @path completion: tab opens, tab accepts the first candidate
	tm.Type("look at @src/auth/l")
	tm.Send(key(tea.KeyTab))
	out.wait(t, "src/auth/login.go")
	tm.Send(key(tea.KeyTab))
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> look at @src/auth/login.go")
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestHistoryNewlineAndPaste(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("one")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> one")
	tm.Send(EvTurnDone{Status: "DONE", Text: "fine"})
	out.wait(t, "fine")
	tm.Send(key(tea.KeyUp))
	tm.Type(" more\\")
	tm.Send(key(tea.KeyEnter)) // "\" + enter is a newline
	tm.Type("second line")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "second line")
	h.mu.Lock()
	sub := h.submitted
	h.mu.Unlock()
	if len(sub) != 2 || sub[1] != "one more\nsecond line" {
		t.Fatalf("history + newline: %q", sub)
	}
	tm.Send(EvTurnDone{Status: "DONE", Text: "ok"})
	out.wait(t, "ok")
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb\nc\nd"), Paste: true})
	out.wait(t, "[pasted 4 lines #1]")
	tm.Send(key(tea.KeyEnter))
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		n := len(h.submitted)
		last := ""
		if n > 0 {
			last = h.submitted[n-1]
		}
		h.mu.Unlock()
		if n == 3 {
			if last != "a\nb\nc\nd" {
				t.Fatalf("paste expanded to %q", last)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the paste was not submitted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestCollapsedExpandsOnCtrlO(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	var lines []string
	for i := 1; i <= 12; i++ {
		lines = append(lines, "row "+itoa(i))
	}
	tm.Send(EvToolStart{Tool: ToolView{ID: "s1", Name: "read", Summary: "x", Class: "read"}})
	tm.Send(EvToolEnd{Tool: ToolView{ID: "s1", Name: "read", Summary: "x", Class: "read", Status: "done", Ms: 1, Output: strings.Join(lines, "\n")}})
	out.wait(t, "… +4 lines (ctrl+o to expand)")
	if strings.Contains(out.String(), "row 12") {
		t.Fatal("collapsed output shows every line")
	}
	tm.Send(key(tea.KeyCtrlO))
	out.wait(t, "row 12")
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestSubagentBlocks(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Send(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "mock/c", State: "queued", Ask: "findings: look"}})
	out.wait(t, "→ subagent zone-auth started")
	tm.Send(EvState{Agent: "zone-auth", State: "tool"})
	out.wait(t, "◆ subagent zone-auth · zone · analyst · mock/c")
	tm.Send(EvSubagent{Subagent: SubagentView{Name: "zone-auth", State: "done", Elapsed: time.Second, Report: &ReportView{Status: "PASS", Findings: []string{"src/auth/login.go:1 fine"}, Unsaid: []string{"team: subagents can run behind"}}}})
	out.wait(t, "∴ team: subagents can run behind")
	if !strings.Contains(out.String(), "⎿ PASS") {
		t.Fatalf("report status missing:\n%s", out.String())
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestChoiceBlock(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	reply := make(chan ChoiceAnswer, 1)
	tm.Send(EvChoice{View: ChoiceView{Title: "Approval", Class: "outward", Tool: "bash", Summary: "git push origin main", Why: "outward",
		Options: []string{"Yes", "Yes, and don't ask again for bash:git push*", "No, and tell the model why"}, Reasons: []string{"", "", "why not?"}}, Reply: reply})
	out.wait(t, "❯ 1. Yes")
	tm.Send(key(tea.KeyDown))
	tm.Send(key(tea.KeyDown))
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "why not?")
	tm.Type("tests first")
	tm.Send(key(tea.KeyEnter))
	select {
	case a := <-reply:
		if a.Index != 2 || a.Text != "tests first" {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no answer")
	}
	out.wait(t, "✓ No, and tell the model why: tests first")
	// a digit answers at once
	reply2 := make(chan ChoiceAnswer, 1)
	tm.Send(EvChoice{View: ChoiceView{Title: "Question", Why: "which?", Options: []string{"a", "b"}, Free: true}, Reply: reply2})
	out.wait(t, "… or type an answer")
	tm.Type("2")
	select {
	case a := <-reply2:
		if a.Index != 1 {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no answer")
	}
	// free text on a question
	reply3 := make(chan ChoiceAnswer, 1)
	tm.Send(EvChoice{View: ChoiceView{Title: "Question", Why: "name?", Options: []string{"a"}, Free: true}, Reply: reply3})
	out.wait(t, "name?")
	tm.Type("zed")
	tm.Send(key(tea.KeyEnter))
	select {
	case a := <-reply3:
		if a.Index != -1 || a.Text != "zed" {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no answer")
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestSafeCut(t *testing.T) {
	s := "para one\n\n```go\nx\n\ny\n```\n\npara three\n\n- a\n- b\n\nlast"
	cut := safeCut(s)
	head := s[:cut]
	if s[cut:] != "last" || !strings.HasSuffix(head, "\n\n") {
		t.Fatalf("cut at %d: %q", cut, head)
	}
	if strings.Count(head, "```")%2 != 0 {
		t.Fatal("cut inside a fence")
	}
	if safeCut("no blank line yet") != 0 {
		t.Fatal("cut without a paragraph")
	}
}

func (t *tail) raw() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}

// A width change clears the screen and the scrollback once the drag settles, then prints the
// transcript again at the new width: the terminal's own rewrap of the live area left ghosts.
func TestResizeReprintsAtTheNewWidth(t *testing.T) {
	h := &fakeHost{}
	tm, m, out := start(t, h)
	long := strings.Repeat("word ", 30)
	tm.Send(EvNotice{Text: long})
	out.wait(t, "word word")
	for _, w := range []int{70, 60, 50, 40} { // a drag: one reprint, not four
		tm.Send(tea.WindowSizeMsg{Width: w, Height: 24})
	}
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(out.raw(), "\x1b[3J") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no reprint after the width changed")
		}
		time.Sleep(15 * time.Millisecond)
	}
	time.Sleep(3 * reflowDelay)
	raw := out.raw()
	if n := strings.Count(raw, "\x1b[3J"); n != 1 {
		t.Fatalf("%d reprints for one drag, want 1", n)
	}
	after := ansi.Strip(raw[strings.Index(raw, "\x1b[3J"):])
	if !strings.Contains(after, "word") {
		t.Fatalf("the reprint lost the transcript:\n%s", after)
	}
	for _, ln := range strings.Split(after, "\n") {
		if strings.Contains(ln, "word") && ansi.StringWidth(strings.TrimRight(ln, " \r")) > 40 {
			t.Fatalf("a reprinted line is wider than the new 40 columns: %q", ln)
		}
	}
	if m.lastWidth != 40 {
		t.Fatalf("lastWidth %d, want 40", m.lastWidth)
	}
	// the same width again is not a change
	tm.Send(tea.WindowSizeMsg{Width: 40, Height: 30})
	time.Sleep(3 * reflowDelay)
	if n := strings.Count(out.raw(), "\x1b[3J"); n != 1 {
		t.Fatalf("a height-only change reprinted (%d)", n)
	}
}
