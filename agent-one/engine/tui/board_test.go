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
			{Name: "orchestrator", Rank: "orchestrator", Model: "openrouter/google/gemma-4-31b-it:free", State: "thinking", Started: at.Add(-3 * time.Minute), CtxTokens: 24000, CtxLimit: 262144, Input: 21000, Output: 3000, USD: &usd},
			{Name: "domain-api", Rank: "domain", Role: "judge", Model: "openrouter/google/gemma-4-31b-it:free", State: "waiting on gate", Started: at.Add(-50 * time.Second), CtxTokens: 9000, CtxLimit: 262144, Input: 8000, Output: 1000, USD: &usd},
			{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "openrouter/thinkingmachines/inkling-small:free", State: "done", Started: at.Add(-2 * time.Minute), CtxTokens: 240000, CtxLimit: 262144, Input: 5000, Output: 400},
		},
		Graph: GraphView{Summary: "7 members · 2 skills · 1 facts · 131 triples (reasoned) · 1 finding",
			Nodes: []GraphNode{
				{ID: "orchestrator", Name: "orchestrator", Rank: "orchestrator", Level: 0, Knowledge: []KV{{"is a", "Member ⊂ Orchestrator"}}},
				{ID: "coord-core", Name: "coord-core", Rank: "coord", Level: 1, Up: []GraphBond{{"orchestrator", "reports"}}, Knowledge: []KV{{"doc", ".agent-one/coord/core/coord-core.md"}, {"holds", "writing-for-agents"}}},
				{ID: "service-ci", Name: "service-ci", Rank: "service", Level: 1, Up: []GraphBond{{"orchestrator", "reports"}}},
				{ID: "domain-api", Name: "domain-api", Rank: "domain", Level: 2, Up: []GraphBond{{"coord-core", "verdict"}}, Knowledge: []KV{{"owns", "src/api/"}, {"sees", "1 fact by the flow rules"}, {"", "territory: tokens expire after 15 minutes"}}},
				{ID: "domain-db", Name: "domain-db", Rank: "domain", Level: 2, Up: []GraphBond{{"coord-core", "verdict"}}},
				{ID: "zone-auth", Name: "zone-auth", Rank: "zone", Level: 3, Up: []GraphBond{{"domain-api", "truth"}}, Knowledge: []KV{{"owns", "src/auth/"}, {"knows", "1 fact"}}},
				{ID: "zone-schema", Name: "zone-schema", Rank: "zone", Level: 3, Up: []GraphBond{{"domain-db", "truth"}}, Findings: 1, Knowledge: []KV{{"finding", "zone-schema has 0 truth edges (ZoneTruth)"}}},
			}},
		Roles: []OfficeRow{
			{Name: "analyst", Role: "reads", Model: "openrouter/thinkingmachines/inkling-small:free", Fallback: "openrouter/google/gemma-4-26b-a4b-it:free", Origin: "~/.config/agent-one/config.yaml:22", Ranks: []string{"zone"}, Live: 1},
			{Name: "judge", Role: "verdicts", Model: "openrouter/google/gemma-4-31b-it:free", Fallback: "openrouter/thinkingmachines/inkling-small:free", Ranks: []string{"domain", "service", "dark-coord"}, Live: 1},
			{Name: "drafter", Role: "drafts", Model: "openrouter/thinkingmachines/inkling:free", Ranks: []string{"coord"}},
		},
		Usage: UsageView{Range: "24h", Calls: 42, Input: 120000, Output: 18000, Cache: 30000,
			ByBody:   []UsageRow{{Key: "orchestrator", Calls: 20, Tokens: 90000}, {Key: "domain-api", Calls: 12, Tokens: 50000}, {Key: "zone-auth", Calls: 10, Tokens: 28000}},
			ByModel:  []UsageRow{{Key: "openrouter/google/gemma-4-31b-it:free", Calls: 32, Tokens: 140000}, {Key: "openrouter/thinkingmachines/inkling-small:free", Calls: 10, Tokens: 28000}},
			ByOffice: []UsageRow{{Key: "judge", Calls: 12, Tokens: 50000}, {Key: "analyst", Calls: 10, Tokens: 28000}},
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
			"agents": {"orchestrator", "domain-api", "waiting on gate", "zone-auth", "CONTEXT"},
			"graph":  {"◆ orchestrator", "◆ coord-core", "◆ domain-api", "◆ zone-auth", "◇ zone-schema", "reasoned", "bonds"},
			"roles":  {"analyst", "reads", "judge", "verdicts", "drafter", "drafts", "serves"},
			"usage":  {"168k tokens", "42 calls", "by agent", "by model", "by role"},
		}
		for i, name := range []string{"agents", "graph", "roles", "usage"} {
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
	if m.board.sel != "orchestrator" {
		t.Fatalf("the walk starts at the root, got %q", m.board.sel)
	}
	for _, k := range []rune{tea.KeyDown, tea.KeyDown, tea.KeyDown} {
		m.Update(tea.KeyPressMsg{Code: k})
	}
	if !strings.HasPrefix(m.board.sel, "zone-") {
		t.Fatalf("three levels down is a zone, got %q", m.board.sel)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	s := screen(m)
	if !strings.Contains(s, "answers") || !strings.Contains(s, "‹truth›") {
		t.Fatalf("the card shows the selected zone's bond up:\n%s", s)
	}
}

func TestBoardHoldsBlocksUntilClosed(t *testing.T) {
	m := boardAt(t, 80, 24)
	for len(m.prints) > 0 { // the welcome
		<-m.prints
	}
	m.Update(EvNotice{Text: "a subagent finished"})
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
		if h.id == "domain-db" {
			target = h
		}
	}
	if target.id == "" {
		t.Fatal("domain-db drew no hit")
	}
	m.Update(tea.MouseClickMsg{X: target.x0 + 1, Y: target.line - m.board.lastOff + 2, Button: tea.MouseLeft})
	if m.board.sel != "domain-db" {
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
	m.footer.Workspace = "/home/u/proj"
	if v := m.View(); v.WindowTitle != "agent-one · proj · idle" || v.ProgressBar != nil {
		t.Fatalf("idle: %q %v", v.WindowTitle, v.ProgressBar)
	}
	m.busy, m.verb = true, "Thinking…"
	if v := m.View(); v.WindowTitle != "agent-one · proj · thinking" || v.ProgressBar == nil || v.ProgressBar.State != tea.ProgressBarIndeterminate {
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
	m.Toast("✓ zone-auth done", m.theme.pill("ok"))
	for i := 0; i < 120 && m.stepToasts(); i++ {
	}
	if s := ansi.Strip(m.Render()); !strings.Contains(s, "✓ zone-auth done") {
		t.Fatalf("the toast is drawn once it slid in:\n%s", s)
	}
	m.toasts[0].born = time.Now().Add(-toastLife - time.Second)
	m.stepToasts()
	if len(m.toasts) != 0 || strings.Contains(ansi.Strip(m.Render()), "zone-auth done") {
		t.Fatal("an expired toast stays")
	}
}
