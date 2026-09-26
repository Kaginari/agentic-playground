package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/instrument"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/provider/mock"
	"github.com/Kaginari/agentic-playground/isekai/tool"
)

func engine(t *testing.T, m *mock.Provider) *Engine {
	t.Helper()
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".isekai"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("hello world\n"), 0o644)
	g := gate.New()
	g.IsTTY = func() bool { return false }
	return &Engine{Provider: m, Tools: tool.Builtins(), Gate: g, Root: root, As: "slime-test", Unsaid: true}
}

func read(id string) provider.Response {
	return mock.Call(id, "read", map[string]string{"path": "README.md"})
}

func events(t *testing.T, e *Engine, id string) []Event {
	t.Helper()
	ev, err := ReadJournal(filepath.Join(e.JournalDir(), id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestTurnAndJournal(t *testing.T) {
	m := mock.New(read("1"), mock.Call("2", "write", map[string]string{"path": "out.txt", "content": "x"}), mock.Text("@S DONE\n@F out.txt:1 x\n@U colony c\n@E 0"))
	e := engine(t, m)
	var trace strings.Builder
	e.Trace = &trace
	r, err := e.Run(context.Background(), "do it")
	if err != nil || r.Status != Done || r.Exit() != 0 {
		t.Fatalf("%v %+v", err, r)
	}
	if len(r.Steps) != 2 || r.Steps[0].ID != "s1" || r.Steps[1].ID != "s2" || r.Steps[1].Wrote[0] != "out.txt" || r.Turns != 3 {
		t.Fatalf("steps %+v", r.Steps)
	}
	if !r.IsWire || r.Report.Unsaid[0].Text != "c" || len(r.Holes) != 0 {
		t.Fatalf("report %+v %v", r.Report, r.Holes)
	}
	if r.Context.Zone != instrument.OK || r.Usage.Output == 0 {
		t.Fatalf("readings %+v %+v", r.Context, r.Usage)
	}
	if !strings.Contains(trace.String(), "→ s1 read [read] README.md") || !strings.Contains(trace.String(), "→ s2 write [write] out.txt") {
		t.Fatalf("trace %q", trace.String())
	}
	ev := events(t, e, r.RunID)
	var ts []string
	for _, x := range ev {
		ts = append(ts, x["t"].(string))
	}
	seq := strings.Join(ts, ",")
	if !strings.HasPrefix(seq, "run,turn,perceive,gate,act,act,verify,record,turn,perceive,gate,act,act,verify,record,turn,end") {
		t.Fatalf("journal beats: %s", seq)
	}
	if ev[0]["as"] != "slime-test" || ev[0]["ask"] != "do it" || ev[0]["provider"] != "mock" {
		t.Fatalf("run line %v", ev[0])
	}
	st := State("", ev)
	if st.Status != "DONE" || st.Turns != 3 || len(st.Steps) != 2 || st.Steps[1].Tool != "write" || st.Steps[1].Class != "write" {
		t.Fatalf("state %+v", st)
	}
	runs, _ := Runs(e.JournalDir())
	if len(runs) != 1 || runs[0].ID != r.RunID {
		t.Fatalf("runs %+v", runs)
	}
	// conversation shape sent to the provider
	last := m.Requests[2].Messages
	if len(last) != 5 || last[2].ToolResults[0].ID != "1" || last[4].ToolResults[0].ID != "2" || !strings.Contains(last[2].ToolResults[0].Content, "hello world") {
		t.Fatalf("messages %+v", last)
	}
	if !strings.Contains(m.Requests[0].System, "slime-test") || !strings.Contains(m.Requests[0].System, "@U") {
		t.Fatalf("system %q", m.Requests[0].System)
	}
}

func TestDenialStopsTheTurn(t *testing.T) {
	m := mock.New(mock.Calls(provider.ToolCall{ID: "1", Name: "bash", Input: json.RawMessage(`{"command":"git push"}`)}, provider.ToolCall{ID: "2", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}), mock.Text("never"))
	e := engine(t, m)
	r, _ := e.Run(context.Background(), "publish")
	if r.Status != Denied || r.Exit() != 4 || len(r.Steps) != 1 || r.Steps[0].Status != "denied" || len(m.Requests) != 1 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Holes[0], "s1 denied at the gate (outward: git ↔ remote") {
		t.Fatalf("hole %v", r.Holes)
	}
	if _, err := os.Stat(filepath.Join(e.JournalDir(), r.RunID+".transcript.json")); err != nil {
		t.Fatal("transcript kept for resume")
	}
	// resume: the open calls are answered as interrupted, the model goes on
	m2 := mock.New(mock.Text("@S DONE\n@U law x\n@E 0"))
	e.Provider = m2
	r2, err := e.Resume(context.Background(), r.RunID, "skip it")
	if err != nil || r2.Status != Done || r2.RunID != r.RunID {
		t.Fatalf("%v %+v", err, r2)
	}
	msgs := m2.Requests[0].Messages
	if len(msgs) != 3 || len(msgs[2].ToolResults) != 2 || !strings.Contains(msgs[2].Text, "skip it") {
		t.Fatalf("resumed conversation %+v", msgs)
	}
	if _, err := e.Resume(context.Background(), "no-such-run", ""); err == nil {
		t.Fatal("missing transcript is an error")
	}
}

func TestPreApprovalAndDryRun(t *testing.T) {
	m := mock.New(mock.Call("1", "bash", map[string]string{"command": "echo hi", "class": "outward"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	e := engine(t, m)
	e.Gate.Approve, _ = gate.ParseApprove("outward")
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Done || r.Steps[0].Gate.By != "pre-approved" || r.Steps[0].Effective != "outward" {
		t.Fatalf("%+v", r.Steps[0])
	}
	e.Gate.Approve = nil
	e.Gate.DryRun = true
	m = mock.New(mock.Calls(
		provider.ToolCall{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)},
		provider.ToolCall{ID: "2", Name: "write", Input: json.RawMessage(`{"path":"w.txt","content":"x"}`)},
		provider.ToolCall{ID: "3", Name: "bash", Input: json.RawMessage(`{"command":"rm -rf x"}`)},
	), mock.Text("@S DRY\n@U law x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Dry || r.Steps[0].Status != "done" || r.Steps[1].Status != "would-run" || r.Steps[2].Status != "would-ask" || r.Journal != "" {
		t.Fatalf("%+v journal=%q", r.Steps, r.Journal)
	}
	if _, err := os.Stat(filepath.Join(e.Root, "w.txt")); err == nil {
		t.Fatal("dry run must not write")
	}
	if runs, _ := Runs(e.JournalDir()); len(runs) != 1 {
		t.Fatalf("dry run must not journal: %d journals", len(runs))
	}
}

func TestBudgetsAreReadings(t *testing.T) {
	m := mock.New(mock.Calls(provider.ToolCall{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}, provider.ToolCall{ID: "2", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}), mock.Text("x"))
	e := engine(t, m)
	e.Budget.Steps = 1
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Checkpoint || r.Exit() != 5 || !strings.Contains(r.Holes[0], "step budget reached (1) before s2") || !strings.Contains(r.Holes[0], "isekai resume "+r.RunID) {
		t.Fatalf("%+v", r)
	}
	e.Budget.Steps = 0
	e.Budget.Minutes = 0.000001
	e.Provider = mock.New(read("1"), mock.Text("x"))
	time.Sleep(time.Millisecond)
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Checkpoint || !strings.Contains(r.Holes[0], "wall-clock budget") {
		t.Fatalf("%+v", r)
	}
	e.Budget.Minutes = -1
	e.Budget.Retries = 1
	e.Provider = mock.New(mock.Call("1", "bash", map[string]string{"command": "false"}), mock.Call("2", "bash", map[string]string{"command": "false"}), mock.Text("x"))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Escalate || r.Exit() != 3 || len(r.Steps) != 2 || !strings.Contains(r.Holes[0], "2 consecutive failed acts") {
		t.Fatalf("%+v", r)
	}
	// a success resets the failure count
	e.Provider = mock.New(mock.Call("1", "bash", map[string]string{"command": "false"}), read("2"), mock.Call("3", "bash", map[string]string{"command": "false"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Done {
		t.Fatalf("%+v", r)
	}
	// context stress
	e.Provider = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 200000}}, mock.Text("x"))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Checkpoint || !strings.Contains(r.Holes[0], "stress zone (200000 ≥ 180000") {
		t.Fatalf("%+v", r)
	}
	ev := events(t, e, r.RunID)
	found := false
	for _, x := range ev {
		if x["t"] == "budget" {
			found = true
		}
	}
	if !found {
		t.Fatal("budget line journaled")
	}
	// a Perceive hook overrides the reading
	e.Hooks.Perceive = func(s *Session) *instrument.Context {
		c := instrument.Budget{}.Reading(provider.Usage{Input: 10})
		return &c
	}
	e.Provider = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 200000}}, mock.Text("@S DONE\n@U law x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Done {
		t.Fatalf("perceive hook: %+v", r)
	}
	// a Drain hook may continue past the stress line
	e.Hooks.Perceive = nil
	drained := 0
	e.Hooks.Drain = func(ctx context.Context, s *Session, c instrument.Context) (bool, error) {
		drained++
		s.Last = provider.Usage{Input: 1}
		return true, nil
	}
	e.Provider = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 200000}}, mock.Text("@S DONE\n@U law x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Done || drained != 1 {
		t.Fatalf("drain hook: %+v %d", r, drained)
	}
}

func TestHooks(t *testing.T) {
	m := mock.New(mock.Call("1", "write", map[string]string{"path": "a.txt", "content": "x"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	e := engine(t, m)
	var recorded []string
	e.Hooks.System = func(s *Session) string { return "CUSTOM SYSTEM" }
	e.Hooks.Recall = func(ctx context.Context, s *Session, ask string) Recall {
		return Recall{Anchors: []string{"README.md#top"}, Tools: []string{"@T mind x — p — load≈5tok"}, Holes: []string{"index stale"}, Rebuilt: true}
	}
	e.Hooks.Record = func(ctx context.Context, s *Session, st *StepRecord) []string {
		recorded = append(recorded, st.ID)
		return []string{"note failed"}
	}
	e.Hooks.EndGate = func(ctx context.Context, s *Session, r *Result) (string, []string, error) {
		return "pass", []string{"doc not touched"}, nil
	}
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Done || r.Verdict != "pass" || len(recorded) != 1 {
		t.Fatalf("%+v %v", r, recorded)
	}
	holes := strings.Join(r.Holes, "|")
	for _, want := range []string{"recall: index stale", "s1 record: note failed", "doc not touched"} {
		if !strings.Contains(holes, want) {
			t.Fatalf("missing %q in %q", want, holes)
		}
	}
	sys := m.Requests[0].System
	if !strings.HasPrefix(sys, "CUSTOM SYSTEM") || !strings.Contains(sys, "@RECALL README.md#top") || !strings.Contains(sys, "@TOOLS\n@T mind x") {
		t.Fatalf("system %q", sys)
	}
	// policy refusal
	e.Hooks = Hooks{}
	e.Policy = func(a tool.Access) error {
		if a.Class >= tool.Write {
			return errorf("territory")
		}
		return nil
	}
	m = mock.New(mock.Call("1", "write", map[string]string{"path": "b.txt", "content": "x"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Done || r.Steps[0].Status != "refused" || !m.Requests[1].Messages[2].ToolResults[0].IsError {
		t.Fatalf("%+v", r.Steps[0])
	}
	// an EndGate error fails the turn
	e.Policy = nil
	e.Hooks.EndGate = func(ctx context.Context, s *Session, r *Result) (string, []string, error) {
		return "fail", nil, errorf("traits broken")
	}
	e.Provider = mock.New(mock.Call("1", "write", map[string]string{"path": "c.txt", "content": "x"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Fail || r.Exit() != 2 || !strings.Contains(r.Holes[0], "traits broken") {
		t.Fatalf("%+v", r)
	}
}

func TestUnsaidAndEmit(t *testing.T) {
	e := engine(t, mock.New(mock.Text("@S DONE\n@F a:1 f\n@E 0")))
	r, _ := e.Run(context.Background(), "x")
	if len(r.Holes) != 1 || !strings.Contains(r.Holes[0], "no @U") {
		t.Fatalf("%v", r.Holes)
	}
	out := r.Emit(0)
	if !strings.HasPrefix(out, "@S DONE\n@? report carries no @U") || !strings.HasSuffix(out, "@E "+itoa(len(out))) {
		t.Fatalf("%q", out)
	}
	e.Unsaid = false
	e.Provider = mock.New(mock.Text("plain prose answer"))
	r, _ = e.Run(context.Background(), "x")
	if r.IsWire || len(r.Holes) != 0 || r.Emit(0) != "plain prose answer" {
		t.Fatalf("%+v", r)
	}
	e.Provider = mock.New(mock.Call("1", "bash", map[string]string{"command": "git push"}))
	r, _ = e.Run(context.Background(), "x")
	if !strings.HasPrefix(r.Emit(0), "@? s1 denied") {
		t.Fatalf("a denied turn with no wire text still names the hole: %q", r.Emit(0))
	}
	// provider error → FAIL
	e.Provider = mock.New()
	if r, _ = e.Run(context.Background(), "x"); r.Status != Fail || !strings.Contains(r.Holes[0], "exhausted") {
		t.Fatalf("%+v", r)
	}
	// journaling off and lexicon
	e.Journal = "-"
	e.Lexicon = Lexicon{Body: "worker", Human: "the operator", WorldDir: ".agent-zero"}
	e.As = ""
	e.Provider = mock.New(mock.Text("ok"))
	m := e.Provider.(*mock.Provider)
	r, _ = e.Run(context.Background(), "x")
	if r.Journal != "" || e.JournalDir() != "" || !strings.Contains(m.Requests[0].System, "You are worker") || !strings.Contains(m.Requests[0].System, "the operator") {
		t.Fatalf("lexicon/journal: %q %q", r.Journal, m.Requests[0].System)
	}
	if _, err := e.NewSession().Turn(context.Background(), "  "); err == nil {
		t.Fatal("empty ask")
	}
}

func TestSessionAcrossTurns(t *testing.T) {
	m := mock.New(mock.Text("one"), read("1"), mock.Text("two"))
	e := engine(t, m)
	e.Unsaid = false
	s := e.NewSession()
	r1, _ := s.Turn(context.Background(), "first")
	r2, _ := s.Turn(context.Background(), "second")
	if r1.Text != "one" || r2.Text != "two" || r2.RunID != r1.RunID || len(s.Messages) != 6 || s.Turns != 3 || s.Steps != 1 {
		t.Fatalf("%+v %+v %d", r1, r2, len(s.Messages))
	}
	ev := events(t, e, s.RunID)
	runs, ends := 0, 0
	for _, x := range ev {
		switch x["t"] {
		case "run":
			runs++
		case "end":
			ends++
		}
	}
	if runs != 1 || ends != 2 {
		t.Fatalf("one run line, one end per turn: %d %d", runs, ends)
	}
}

type errorf string

func (e errorf) Error() string { return string(e) }

func itoa(n int) string { return fmt.Sprint(n) }
