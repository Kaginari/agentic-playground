package compact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/instrument"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/provider/mock"
	"github.com/Kaginari/agentic-playground/isekai/tool"
)

// transcript is a long mock session: many reads (README twice), a grep, a write, a bash step,
// a mid-way answer with an open @?, a second ask, more steps — the last two exchanges kept.
func transcript(t *testing.T, m *mock.Provider) (*loop.Session, string) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".isekai"), 0o755)
	big := strings.Repeat("the quick brown fox jumps over the lazy dog\n", 200)
	os.WriteFile(filepath.Join(root, "README.md"), []byte(big), 0o644)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("alpha\nbeta\n"), 0o644)
	g := gate.New()
	g.IsTTY = func() bool { return false }
	e := &loop.Engine{Provider: m, Tools: tool.Builtins(), Gate: g, Root: root, As: "slime-test"}
	s := e.NewSession()
	s.Ask = "@ASK findings +unsaid\nread the readme and fix the notes"
	s.Turns = 6
	call := func(id, name string, in map[string]interface{}) provider.ToolCall {
		raw, _ := json.Marshal(in)
		return provider.ToolCall{ID: id, Name: name, Input: raw}
	}
	numbered := func(text string) string {
		var b strings.Builder
		for i, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			b.WriteString("     " + itoa(i+1) + "\t" + l + "\n")
		}
		return b.String()
	}
	s.Messages = []provider.Message{
		{Role: provider.User, Text: s.Ask},
		{Role: provider.Assistant, Text: "reading", ToolCalls: []provider.ToolCall{call("r1", "read", map[string]interface{}{"path": "README.md"}), call("g1", "grep", map[string]interface{}{"pattern": "fox", "path": "."})}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "r1", Content: numbered(big)}, {ID: "g1", Content: strings.Repeat("README.md:1:the quick brown fox\n", 50)}}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{call("r2", "read", map[string]interface{}{"path": "notes.txt"})}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "r2", Content: numbered("alpha\nbeta\n")}}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{call("w1", "write", map[string]interface{}{"path": "notes.txt", "content": "alpha\nbeta\ngamma\n"})}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "w1", Content: "wrote notes.txt (17 bytes)"}}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{call("b1", "bash", map[string]interface{}{"command": "go test ./..."})}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "b1", Content: strings.Repeat("ok  \tpkg\t0.01s\n", 40) + "FAIL\n[exit 1, 2s]", IsError: true}}},
		{Role: provider.Assistant, Text: "@S PARTIAL\n@? the failing package is not named in the output\n@E 0"},
		{Role: provider.User, Text: "keep going"},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{call("r3", "read", map[string]interface{}{"path": "README.md", "offset": 3, "limit": 2})}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "r3", Content: "     3\tthe quick brown fox jumps over the lazy dog\n     4\tthe quick brown fox jumps over the lazy dog\n"}}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{call("b2", "bash", map[string]interface{}{"command": "echo tail"})}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "b2", Content: "tail"}}},
	}
	s.Last = provider.Usage{Input: 190000}
	return s, root
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

type homes struct {
	notes, facts []string
	episodes     []string
}

func (h *homes) Homes(root string) Homes {
	return Homes{Root: root, Crest: func() string { return "1. **Vitality** — the crest" },
		Remember: func(as, kind, text string) error { h.notes = append(h.notes, as+"|"+kind+"|"+text); return nil },
		Assert:   func(as, kind, text string) error { h.facts = append(h.facts, as+"|"+kind+"|"+text); return nil },
		Episode: func(as string, wrote []string, desk []string) error {
			h.episodes = append(h.episodes, as+"|"+strings.Join(wrote, ","))
			return nil
		},
	}
}

func TestDrain(t *testing.T) {
	m := mock.New(mock.Text("@S DONE\n@F README.md:1 two hundred fox lines\n@U colony the notes file is the real todo list\n@U territory README is generated\n@D 2026-09-26 goal: fix the notes\n@D 2026-09-26 next: name the failing package\n@E 0"))
	s, root := transcript(t, m)
	h := &homes{}
	opt := Defaults()
	opt.KeepTurns = 2
	d := New(opt, h.Homes(root))
	d.Now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }
	before := Tokens(s.Messages)
	ok, err := d.Drain(context.Background(), s, instrument.Context{Available: true, Tokens: 190000, Stress: 180000})
	if err != nil || !ok {
		t.Fatalf("%v %+v", err, d.Last)
	}
	rep := d.Last
	if rep.After >= before || rep.Before != before || Tokens(s.Messages) != rep.After {
		t.Fatalf("tokens: before %d after %d", rep.Before, rep.After)
	}
	if rep.After*4 > before {
		t.Errorf("a drain of reads should cut deep: %d → %d", before, rep.After)
	}
	if len(s.Messages) != 5 || s.Messages[1].Role != provider.Assistant || s.Messages[1].ToolCalls[0].ID != "r3" || s.Messages[4].ToolResults[0].ID != "b2" {
		t.Fatalf("kept tail: %d messages, %+v", len(s.Messages), s.Messages[1])
	}
	head := s.Messages[0].Text
	if !strings.HasPrefix(head, s.Ask) {
		t.Error("the ask is not first, verbatim")
	}
	if !strings.Contains(head, "@OPEN\n@? the failing package is not named in the output") {
		t.Errorf("open hole lost:\n%s", head)
	}
	if !strings.Contains(head, "@D 2026-09-26 goal: fix the notes") || !strings.Contains(head, "@DESK .isekai/instruments/desk/"+s.RunID+".md") {
		t.Errorf("desk:\n%s", head)
	}
	// pointers: README read twice collapses to one (the latest), notes read once, one grep
	var ptrs []string
	for _, l := range strings.Split(head, "\n") {
		if strings.HasPrefix(l, "@P ") {
			ptrs = append(ptrs, l)
		}
	}
	if len(ptrs) != 3 || !strings.Contains(ptrs[0], "README.md:1-200 · sha256:") || !strings.Contains(ptrs[1], "grep fox in . · sha256:") || !strings.Contains(ptrs[2], "notes.txt:1-2 · sha256:") {
		t.Errorf("pointer table: %v", ptrs)
	}
	for _, p := range rep.Pointers {
		if p.Tool == "read" && (p.Digest == "missing" || len(p.Digest) != 8) {
			t.Errorf("pointer %+v does not resolve", p)
		}
	}
	if strings.Contains(head, "changed since") {
		t.Error("nothing changed yet")
	}
	// the unsaid and the findings went home; the episode was recorded for the write
	if len(h.facts) != 2 || h.facts[0] != "slime-test|colony|the notes file is the real todo list" || len(h.notes) != 3 || !strings.HasSuffix(h.notes[2], "|@F README.md:1 two hundred fox lines") {
		t.Errorf("homes: %v %v", h.facts, h.notes)
	}
	if len(h.episodes) != 1 || h.episodes[0] != "slime-test|notes.txt" {
		t.Errorf("episode: %v", h.episodes)
	}
	// the one model call was commissioned on the wire, over the pointerized transcript
	req := m.Requests[0]
	if !strings.Contains(req.System, "1. **Vitality**") || !strings.Contains(req.Messages[0].Text, "@ASK findings +unsaid") || !strings.Contains(req.Messages[0].Text, "@CAP 4096") || !strings.Contains(req.Messages[0].Text, "result r1: → p1") || strings.Contains(req.Messages[0].Text, "lazy dog\n     5") {
		t.Errorf("model pass request:\n%s", req.Messages[0].Text[:600])
	}
	if !strings.Contains(req.Messages[0].Text, "bash go test ./... · error · tail") {
		t.Errorf("spent output not trimmed:\n%s", req.Messages[0].Text)
	}
	// the desk file, and the perceive reading after the drain
	desk, _ := os.ReadFile(d.DeskPath(s.RunID))
	if !strings.Contains(string(desk), "## Thoughts\n- 2026-09-26 goal: fix the notes\n- 2026-09-26 next") {
		t.Errorf("desk file:\n%s", desk)
	}
	if s.Last.Input != rep.After || s.Engine.Budget.Context.Reading(s.Last).Stressed() {
		t.Error("the reading did not drop")
	}
	// the drained turns are in the short tier and recallable
	hits := RecallDrained(d.DrainedPath(s.RunID), "failing package output", 2)
	if len(hits) == 0 || !strings.Contains(hits[0].Text, "failing package") {
		t.Errorf("recall drained: %+v", hits)
	}
	r := d.RecallTool().Run(context.Background(), tool.Env{Root: root}, json.RawMessage(`{"question":"grep fox","run":"`+s.RunID+`"}`))
	if r.Err || !strings.Contains(r.Output, "drained #") || !strings.Contains(r.Output, "fox") {
		t.Errorf("drained tool: %+v", r)
	}
	// a pointer whose file changed since is flagged, not trusted
	os.WriteFile(filepath.Join(root, "README.md"), []byte("rewritten\n"), 0o644)
	flags := d.Check(rep.Pointers)
	if len(flags) != 1 || !strings.HasPrefix(flags[0], "p1 README.md: changed since: now sha256:") {
		t.Errorf("digest mismatch: %v", flags)
	}
	if line := rep.Pointers[0].Line(d.flag(rep.Pointers[0])); !strings.Contains(line, "(changed since") {
		t.Error("flag rendered")
	}
	// a second drain on the remaining tail: the mock has nothing more → the call fails → abort keeps the context
	kept := s.Messages
	s.Messages = append(s.Messages, provider.Message{Role: provider.Assistant, Text: "more"}, provider.Message{Role: provider.User, Text: "go"})
	m.Script = nil
	if ok, err := d.Drain(context.Background(), s, instrument.Context{}); ok || err == nil || len(s.Messages) != len(kept)+2 {
		t.Fatalf("abort should keep the old context: %v %v", ok, err)
	}
}

func TestVerifyAbortsAndKeeps(t *testing.T) {
	m := mock.New(mock.Text("@S DONE\n@U colony x\n@D 2026-09-26 goal\n@E 0"))
	s, root := transcript(t, m)
	d := New(Defaults(), Homes{Root: root})
	d.Opt.KeepTurns = 2
	old := s.Messages
	os.Remove(filepath.Join(root, "notes.txt")) // a read pointer that will not resolve
	ok, err := d.Drain(context.Background(), s, instrument.Context{})
	if ok || err == nil || !strings.Contains(err.Error(), "pointer p3 does not resolve") || len(s.Messages) != len(old) || s.Last.Input != 190000 {
		t.Fatalf("verify: %v %v", ok, err)
	}
	if d.Last.Aborted == "" {
		t.Error("abort journaled in the report")
	}
	// verify off: the same drain goes through
	d.Opt.Passes.Verify.Enabled = false
	m.Script = []provider.Response{mock.Text("@S DONE\n@U colony x\n@E 0")}
	if ok, err := d.Drain(context.Background(), s, instrument.Context{}); !ok || err != nil {
		t.Fatalf("verify off: %v %v", ok, err)
	}
	// no homes wired: the @U had nowhere to go — a flag, never silence
	if f := strings.Join(d.Last.Flags, "|"); !strings.Contains(f, "had no home") {
		t.Errorf("flags: %v", d.Last.Flags)
	}
}

func TestPassesOffAndSummary(t *testing.T) {
	// every pass off but pointerize: no model call, a mechanical desk
	m := mock.New()
	s, root := transcript(t, m)
	opt := Defaults()
	opt.Passes = Passes{Pointerize: Switch{true}, Verify: Switch{true}}
	opt.KeepTurns = 1
	d := New(opt, Homes{Root: root})
	if ok, err := d.Drain(context.Background(), s, instrument.Context{}); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	if len(m.Requests) != 0 || len(d.Last.Pointers) != 3 || !strings.Contains(s.Messages[0].Text, "@D 20") || strings.Join(d.Last.Passes, ",") != "pointerize,verify" {
		t.Fatalf("passes off: %d calls, %+v", len(m.Requests), d.Last)
	}
	if _, err := os.Stat(d.DeskPath(s.RunID)); err == nil {
		t.Error("desk pass off writes no desk file")
	}
	// trimSpent alone shrinks the bash output the model would read; the transcript is smaller
	m = mock.New()
	s, root = transcript(t, m)
	opt.Passes = Passes{TrimSpent: Switch{true}}
	d = New(opt, Homes{Root: root})
	if ok, _ := d.Drain(context.Background(), s, instrument.Context{}); !ok || strings.Join(d.Last.Passes, ",") != "trimSpent" {
		t.Fatalf("trim only: %+v", d.Last)
	}
	// the summary strategy: one model-written summary, the ask kept, tokens down
	m = mock.New(mock.Text("SUMMARY: read the readme (200 fox lines), notes.txt gained gamma, go test failed on an unnamed package."))
	s, root = transcript(t, m)
	opt = Defaults()
	opt.Strategy, opt.KeepTurns = "summary", 2
	d = New(opt, Homes{Root: root})
	before := Tokens(s.Messages)
	if ok, err := d.Drain(context.Background(), s, instrument.Context{}); !ok || err != nil {
		t.Fatalf("summary: %v %v", ok, err)
	}
	if d.Last.After >= before || !strings.HasPrefix(s.Messages[0].Text, s.Ask+"\n\n@SUMMARY\nSUMMARY: read the readme") || len(s.Messages) != 5 || len(d.Last.Pointers) != 0 {
		t.Fatalf("summary rebuild: %+v\n%s", d.Last, s.Messages[0].Text)
	}
	if !strings.Contains(m.Requests[0].Messages[0].Text, "lazy dog") {
		t.Error("the summary reads the raw turns")
	}
	// off entirely
	d.Opt.Enabled = false
	if ok, err := d.Drain(context.Background(), s, instrument.Context{}); ok || err != nil {
		t.Error("disabled drain does nothing")
	}
}

func TestTriggerAndPerceive(t *testing.T) {
	if (Trigger{}).Threshold(180000) != 180000 || (Trigger{Tokens: 150000}).Threshold(180000) != 150000 || (Trigger{Tokens: 150000, Fraction: 0.8, Window: 100000}).Threshold(180000) != 80000 || (Trigger{Fraction: 0.9, Window: 1000000}).Threshold(180000) != 180000 {
		t.Error("the lower line wins")
	}
	m := mock.New()
	s, root := transcript(t, m)
	opt := Defaults()
	opt.Trigger = Trigger{Tokens: 100000}
	d := New(opt, Homes{Root: root})
	s.Last = provider.Usage{Input: 120000}
	c := d.Perceive(s)
	if c == nil || !c.Stressed() || c.Stress != 100000 || c.Zone != instrument.STRESS {
		t.Fatalf("perceive at the trigger: %+v", c)
	}
	s.Last = provider.Usage{Input: 80000}
	if c := d.Perceive(s); c.Stressed() || c.Zone != instrument.NEAR {
		t.Errorf("below the trigger: %+v", c)
	}
	s.Turns = 0
	if d.Perceive(s) != nil {
		t.Error("no reading before a call")
	}
	h := d.Hooks()
	if h.Perceive == nil || h.Drain == nil {
		t.Error("hooks")
	}
	// through the loop: the engine drains at the trigger and goes on
	m = mock.New(
		provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 1000}},
		provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "2", Name: "read", Input: json.RawMessage(`{"path":"notes.txt"}`)}}}, Usage: provider.Usage{Input: 120000}},
		mock.Text("@S DONE\n@U colony drained\n@D 2026-09-26 goal\n@E 0"), // the drain's call
		mock.Text("@S DONE\n@U colony fin\n@E 0"),
	)
	s2, root := transcript(t, m)
	e := s2.Engine
	d = New(opt, Homes{Root: root})
	d.Opt.KeepTurns = 1
	e.Hooks = d.Hooks()
	e.Journal = ""
	r, err := e.Run(context.Background(), "read both files")
	if err != nil || r.Status != loop.Done || len(r.Steps) != 2 {
		t.Fatalf("%v %+v", err, r)
	}
	if d.Last == nil || d.Last.Aborted != "" || len(d.Last.Pointers) != 1 {
		t.Fatalf("drain through the loop: %+v", d.Last)
	}
	if !strings.HasPrefix(m.Requests[3].Messages[0].Text, "read both files\n\n@DESK") || len(m.Requests[3].Messages) != 3 {
		t.Errorf("rebuilt context reached the provider: %d messages\n%s", len(m.Requests[3].Messages), m.Requests[3].Messages[0].Text)
	}
	ev, _ := loop.ReadJournal(filepath.Join(e.JournalDir(), r.RunID+".jsonl"))
	seen := map[string]bool{}
	for _, x := range ev {
		seen[x["t"].(string)] = true
	}
	if !seen["drain-report"] || !seen["drain"] {
		t.Errorf("drain journaled: %v", seen)
	}
}
