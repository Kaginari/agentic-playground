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
			{Name: "domain-api", Rank: "domain", Office: "judge", Model: "openrouter/google/gemma-4-31b-it:free", State: "waiting on gate", Started: at.Add(-50 * time.Second), CtxTokens: 9000, CtxLimit: 262144, Input: 8000, Output: 1000, USD: &usd},
			{Name: "zone-auth", Rank: "zone", Office: "analyst", Model: "openrouter/thinkingmachines/inkling-small:free", State: "done", Started: at.Add(-2 * time.Minute), CtxTokens: 240000, CtxLimit: 262144, Input: 5000, Output: 400},
		},
		Graph: GraphView{Summary: "7 members · 2 skills · 1 facts · 131 triples (reasoned) · 1 finding",
			Nodes: []GraphNode{
				{ID: "orchestrator", Name: "orchestrator", Rank: "orchestrator", Level: 0, Knowledge: []KV{{"is a", "Member ⊂ Orchestrator"}}},
				{ID: "coord-core", Name: "coord-core", Rank: "coord", Level: 1, Up: []GraphBond{{"orchestrator", "reports"}}, Knowledge: []KV{{"doc", ".agent-one/coord/core/coord-core.md"}, {"wears", "writing-for-agents"}}},
				{ID: "service-ci", Name: "service-ci", Rank: "service", Level: 1, Up: []GraphBond{{"orchestrator", "reports"}}},
				{ID: "domain-api", Name: "domain-api", Rank: "domain", Level: 2, Up: []GraphBond{{"coord-core", "verdict"}}, Knowledge: []KV{{"owns", "src/api/"}, {"sees", "1 fact by the flow rules"}, {"", "territory: tokens expire after 15 minutes"}}},
				{ID: "domain-db", Name: "domain-db", Rank: "domain", Level: 2, Up: []GraphBond{{"coord-core", "verdict"}}},
				{ID: "zone-auth", Name: "zone-auth", Rank: "zone", Level: 3, Up: []GraphBond{{"domain-api", "truth"}}, Knowledge: []KV{{"owns", "src/auth/"}, {"knows", "1 fact"}}},
				{ID: "zone-schema", Name: "zone-schema", Rank: "zone", Level: 3, Up: []GraphBond{{"domain-db", "truth"}}, Findings: 1, Knowledge: []KV{{"finding", "zone-schema has 0 truth edges (ZoneTruth)"}}},
			}},
		Offices: []OfficeRow{
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
			"agents":  {"orchestrator", "domain-api", "waiting on gate", "zone-auth", "CONTEXT"},
			"graph":   {"◆ orchestrator", "◆ coord-core", "◆ domain-api", "◆ zone-auth", "◇ zone-schema", "reasoned", "bonds"},
			"offices": {"analyst", "reads", "judge", "verdicts", "drafter", "drafts", "serves"},
			"usage":   {"168k tokens", "42 calls", "by agent", "by model", "by role"},
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
	m.Update(EvNotice{Text: "a court finished"})
	if len(m.prints) != 0 || len(m.held) != 1 {
		t.Fatalf("a block printed under the board: prints %d held %d", len(m.prints), len(m.held))
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.board != nil || len(m.prints) != 1 {
		t.Fatalf("closing the board prints what was held: board %v prints %d", m.board != nil, len(m.prints))
	}
}
