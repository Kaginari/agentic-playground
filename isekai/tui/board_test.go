package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func sampleBoard() BoardView {
	usd := 0.0
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	return BoardView{At: at,
		Agents: []AgentRow{
			{Name: "rimuru", Rank: "rimuru", Model: "openrouter/google/gemma-4-31b-it:free", State: "thinking", Started: at.Add(-3 * time.Minute), CtxTokens: 24000, CtxLimit: 262144, Input: 21000, Output: 3000, USD: &usd},
			{Name: "orc-api", Rank: "orc", Office: "raphael", Model: "openrouter/google/gemma-4-31b-it:free", State: "waiting on gate", Started: at.Add(-50 * time.Second), CtxTokens: 9000, CtxLimit: 262144, Input: 8000, Output: 1000, USD: &usd},
			{Name: "slime-auth", Rank: "slime", Office: "great-sage", Model: "openrouter/thinkingmachines/inkling-small:free", State: "done", Started: at.Add(-2 * time.Minute), CtxTokens: 240000, CtxLimit: 262144, Input: 5000, Output: 400},
		},
		Graph: GraphView{Summary: "7 creatures · 2 minds · 1 facts · 131 triples (reasoned) · 1 finding",
			Nodes: []GraphNode{
				{ID: "rimuru", Name: "rimuru", Rank: "rimuru", Level: 0, Knowledge: []KV{{"is a", "Creature ⊂ Rimuru"}}},
				{ID: "elf-core", Name: "elf-core", Rank: "elf", Level: 1, Up: []GraphBond{{"rimuru", "reports"}}, Knowledge: []KV{{"doc", ".isekai/elf/core/elf-core.md"}, {"wears", "writing-for-agents"}}},
				{ID: "kijin-ci", Name: "kijin-ci", Rank: "kijin", Level: 1, Up: []GraphBond{{"rimuru", "reports"}}},
				{ID: "orc-api", Name: "orc-api", Rank: "orc", Level: 2, Up: []GraphBond{{"elf-core", "verdict"}}, Knowledge: []KV{{"owns", "src/api/"}, {"sees", "1 fact by the flow rules"}, {"", "territory: tokens expire after 15 minutes"}}},
				{ID: "orc-db", Name: "orc-db", Rank: "orc", Level: 2, Up: []GraphBond{{"elf-core", "verdict"}}},
				{ID: "slime-auth", Name: "slime-auth", Rank: "slime", Level: 3, Up: []GraphBond{{"orc-api", "truth"}}, Knowledge: []KV{{"owns", "src/auth/"}, {"knows", "1 fact"}}},
				{ID: "slime-schema", Name: "slime-schema", Rank: "slime", Level: 3, Up: []GraphBond{{"orc-db", "truth"}}, Findings: 1, Knowledge: []KV{{"finding", "slime-schema has 0 truth edges (SlimeTruth)"}}},
			}},
		Offices: []OfficeRow{
			{Name: "great-sage", Role: "reads", Model: "openrouter/thinkingmachines/inkling-small:free", Fallback: "openrouter/google/gemma-4-26b-a4b-it:free", Origin: "~/.config/isekai/config.yaml:22", Ranks: []string{"slime"}, Live: 1},
			{Name: "raphael", Role: "verdicts", Model: "openrouter/google/gemma-4-31b-it:free", Fallback: "openrouter/thinkingmachines/inkling-small:free", Ranks: []string{"orc", "kijin", "dark-elf"}, Live: 1},
			{Name: "ciel", Role: "drafts", Model: "openrouter/thinkingmachines/inkling:free", Ranks: []string{"elf"}},
		},
		Usage: UsageView{Range: "24h", Calls: 42, Input: 120000, Output: 18000, Cache: 30000,
			ByBody:   []UsageRow{{Key: "rimuru", Calls: 20, Tokens: 90000}, {Key: "orc-api", Calls: 12, Tokens: 50000}, {Key: "slime-auth", Calls: 10, Tokens: 28000}},
			ByModel:  []UsageRow{{Key: "openrouter/google/gemma-4-31b-it:free", Calls: 32, Tokens: 140000}, {Key: "openrouter/thinkingmachines/inkling-small:free", Calls: 10, Tokens: 28000}},
			ByOffice: []UsageRow{{Key: "raphael", Calls: 12, Tokens: 50000}, {Key: "great-sage", Calls: 10, Tokens: 28000}},
			ByDay:    []UsageRow{{Key: "2026-09-25", Tokens: 20000}, {Key: "2026-09-26", Tokens: 90000}, {Key: "2026-09-27", Tokens: 58000}},
		},
	}
}

func (h *fakeHost) Board(rng string) BoardView {
	v := sampleBoard()
	v.Usage.Range = rng
	return v
}

// boardAt opens the board on a model of the given size and returns it ready to draw.
func boardAt(t *testing.T, w, h int) *Model {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.openBoard()
	m.Update(evBoard{m.host.Board("24h")})
	return m
}

func screen(m *Model) string { return ansi.Strip(m.Render()) }

func TestBoardPages(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {140, 40}} {
		m := boardAt(t, size[0], size[1])
		pages := map[string][]string{
			"agents":  {"rimuru", "orc-api", "waiting on gate", "slime-auth", "CONTEXT"},
			"graph":   {"◆ rimuru", "◆ elf-core", "◆ orc-api", "◆ slime-auth", "◇ slime-schema", "reasoned", "bonds"},
			"offices": {"great-sage", "reads", "raphael", "verdicts", "ciel", "drafts", "serves"},
			"usage":   {"168k tokens", "42 calls", "by body", "by model", "by office"},
		}
		for i, name := range []string{"agents", "graph", "offices", "usage"} {
			m.Update(tea.KeyPressMsg{Code: rune('1' + i), Text: string(rune('1' + i))})
			s := screen(m)
			if dir := os.Getenv("BOARD_SHOTS"); dir != "" {
				_ = os.WriteFile(filepath.Join(dir, name+"-"+itoa(size[0])+".txt"), []byte(s), 0o644)
			}
			lines := strings.Split(s, "\n")
			if len(lines) != size[1] {
				t.Fatalf("%s at %dx%d: %d lines, want the full screen", name, size[0], size[1], len(lines))
			}
			for _, l := range lines {
				if ansi.StringWidth(l) > size[0] {
					t.Fatalf("%s at %d cols: a line overflows: %q", name, size[0], l)
				}
			}
			for _, want := range pages[name] {
				if !strings.Contains(s, want) {
					t.Fatalf("%s at %dx%d: missing %q\n%s", name, size[0], size[1], want, s)
				}
			}
		}
	}
}

func TestBoardGraphWalk(t *testing.T) {
	m := boardAt(t, 140, 40)
	m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	screen(m)
	if m.board.sel != "rimuru" {
		t.Fatalf("the walk starts at the root, got %q", m.board.sel)
	}
	for _, k := range []rune{tea.KeyDown, tea.KeyDown, tea.KeyDown} {
		m.Update(tea.KeyPressMsg{Code: k})
	}
	if !strings.HasPrefix(m.board.sel, "slime-") {
		t.Fatalf("three levels down is a slime, got %q", m.board.sel)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	s := screen(m)
	if !strings.Contains(s, "answers") || !strings.Contains(s, "‹truth›") {
		t.Fatalf("the card shows the selected slime's bond up:\n%s", s)
	}
}

func TestBoardHoldsBlocksUntilClosed(t *testing.T) {
	m := boardAt(t, 80, 24)
	for len(m.prints) > 0 { // the welcome
		<-m.prints
	}
	m.Update(EvNotice{Text: "a court finished"})
	if len(m.prints) != 0 || len(m.held) != 1 {
		t.Fatalf("a block printed under the board: prints %d held %d", len(m.prints), len(m.held))
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.board != nil || len(m.prints) != 1 {
		t.Fatalf("closing the board prints what was held: board %v prints %d", m.board != nil, len(m.prints))
	}
}

func TestBoardMouse(t *testing.T) {
	m := boardAt(t, 140, 40)
	screen(m)
	// a click on the third tab
	x := m.board.tabX[2][0] + 1
	m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	if m.board.page != pageOffices {
		t.Fatalf("tab click: page %d", m.board.page)
	}
	// a click on a graph node selects it
	m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	screen(m)
	var target hit
	for _, h := range m.board.hits {
		if h.id == "orc-db" {
			target = h
		}
	}
	if target.id == "" {
		t.Fatal("orc-db drew no hit")
	}
	m.Update(tea.MouseClickMsg{X: target.x0 + 1, Y: target.line - m.board.lastOff + 2, Button: tea.MouseLeft})
	if m.board.sel != "orc-db" {
		t.Fatalf("node click selected %q", m.board.sel)
	}
	// a click on the third agent's row, then again: the detail opens
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	screen(m)
	row := m.board.hits[2]
	click := tea.MouseClickMsg{X: 4, Y: row.line - m.board.lastOff + 2, Button: tea.MouseLeft}
	m.Update(click)
	m.Update(click)
	if m.board.cursor[pageAgents] != 2 || !m.board.detail {
		t.Fatalf("row click: cursor %d detail %v", m.board.cursor[pageAgents], m.board.detail)
	}
}

func TestTerminalIntegration(t *testing.T) {
	th := NewTheme(true)
	out := th.Tool(ToolView{Name: "edit", Summary: "src/auth/token.go", Class: "write", Status: "done", Link: "file:///w/src/auth/token.go"}, 100)
	if !strings.Contains(out, "\x1b]8;;file:///w/src/auth/token.go") {
		t.Fatalf("no OSC 8 link on the path: %q", out)
	}
	m := New(&fakeHost{}, th, DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.footer.World = "/home/u/proj"
	if v := m.View(); v.WindowTitle != "isekai · proj · idle" || v.ProgressBar != nil {
		t.Fatalf("idle: %q %v", v.WindowTitle, v.ProgressBar)
	}
	m.busy, m.verb = true, "Thinking…"
	if v := m.View(); v.WindowTitle != "isekai · proj · thinking" || v.ProgressBar == nil || v.ProgressBar.State != tea.ProgressBarIndeterminate {
		t.Fatalf("busy: %q %v", v.WindowTitle, v.ProgressBar)
	}
}

func TestFuzzyAndPalette(t *testing.T) {
	if _, _, ok := fuzzy("sessions", "ssn"); !ok {
		t.Fatal("ssn is a subsequence of sessions")
	}
	if _, _, ok := fuzzy("board", "bx"); ok {
		t.Fatal("bx matched board")
	}
	b1, _, _ := fuzzy("board", "bo")
	b2, _, _ := fuzzy("rebooted", "bo")
	if b1 <= b2 {
		t.Fatalf("a word-start run must outrank a mid-word one: %d vs %d", b1, b2)
	}
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if m.palette == nil {
		t.Fatal("ctrl+k opens the palette")
	}
	for _, r := range "boa" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if s := ansi.Strip(m.Render()); !strings.Contains(s, "❯ boa") || !strings.Contains(s, "/board") {
		t.Fatalf("the palette draws its query and its best hit:\n%s", s)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.palette != nil || m.board == nil {
		t.Fatalf("enter on /board runs it: palette %v board %v", m.palette != nil, m.board != nil)
	}
}

func TestToastLives(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Toast("✓ slime-auth done", m.theme.pill("ok"))
	for i := 0; i < 120 && m.stepToasts(); i++ {
	}
	if s := ansi.Strip(m.Render()); !strings.Contains(s, "✓ slime-auth done") {
		t.Fatalf("the toast is drawn once it slid in:\n%s", s)
	}
	m.toasts[0].born = time.Now().Add(-toastLife - time.Second)
	m.stepToasts()
	if len(m.toasts) != 0 || strings.Contains(ansi.Strip(m.Render()), "slime-auth done") {
		t.Fatal("an expired toast stays")
	}
}

func drainPrints(m *Model) string {
	var out []string
	for len(m.prints) > 0 {
		out = append(out, ansi.Strip((<-m.prints).text))
	}
	return strings.Join(out, "\n")
}

func TestThinkingShowsThenFolds(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.bgKnown = true
	m.Update(EvTurnStart{Text: "why"})
	drainPrints(m)
	m.Update(EvStream{Body: "rimuru", Kind: "thinking", Text: "The gate runs after the turn, so the verify lines"})
	if s := ansi.Strip(m.Render()); !strings.Contains(s, "∴ thinking") || !strings.Contains(s, "verify lines") {
		t.Fatalf("the thinking is shown as it arrives:\n%s", s)
	}
	m.Update(EvDelta{Text: "Because the gate reloads the docs.\n\n"})
	if p := drainPrints(m); !strings.Contains(p, "∴ Thought for") || !strings.Contains(p, "ctrl+o to expand") {
		t.Fatalf("the answer folds the thinking into a block: %q", p)
	}
	if s := ansi.Strip(m.Render()); strings.Contains(s, "∴ thinking") {
		t.Fatal("the folded thinking stays live")
	}
	m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if p := drainPrints(m); !strings.Contains(p, "verify lines") {
		t.Fatalf("ctrl+o expands the thought: %q", p)
	}
}

func TestBodyView(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	m.Update(EvTurnStart{Text: "fix auth"})
	m.Update(EvCourt{Court: CourtView{Name: "slime-auth", Rank: "slime", Ask: "@ASK draft\nshorten the TTL", State: "thinking"}})
	m.Update(EvStream{Body: "slime-auth", Kind: "thinking", Text: "The TTL lives in token.go."})
	m.Update(EvBodyStep{Body: "slime-auth", Tool: ToolView{ID: "s1", Name: "read", Summary: "src/auth/token.go", Class: "read", Status: "done", Output: "const ttl = time.Hour"}, End: true})
	if s := ansi.Strip(m.Render()); !strings.Contains(s, "ctrl+t to watch") {
		t.Fatalf("a running court is announced with the key to watch it:\n%s", s)
	}
	m.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	s := ansi.Strip(m.Render())
	for _, want := range []string{"rimuru", "slime-auth", "The TTL lives in token.go", "read  src/auth/token.go", "shorten the TTL"} {
		if !strings.Contains(s, want) {
			t.Fatalf("the body view opens on the live court; missing %q:\n%s", want, s)
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.bview.sel != 0 {
		t.Fatalf("tab cycles to the session, sel %d", m.bview.sel)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.bview != nil {
		t.Fatal("esc closes the body view")
	}
}

func TestGateWearsTheOrc(t *testing.T) {
	v := Verb("isekai", "rimuru", "gating", "", 3)
	found := false
	for _, w := range isekaiVerbs["orc"] {
		if v == w+"…" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the gate speaks the orc's words, got %q", v)
	}
	if a := Verb("agent-one", "orchestrator", "gating", "", 1); !strings.HasSuffix(a, "…") || strings.Contains(strings.ToLower(a), "orc") {
		t.Fatalf("agent-one's gate: %q", a)
	}
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(EvTurnStart{Text: "x"})
	m.Update(EvState{Body: "rimuru", State: "gating"})
	if s := ansi.Strip(m.Render()); !strings.Contains(s, "orc · the gate weighs the turn's writes") {
		t.Fatalf("the gate shows the orc:\n%s", s)
	}
}
