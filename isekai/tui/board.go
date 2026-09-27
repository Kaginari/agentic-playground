package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"
)

// BoardView is what /board draws: the live agents, the ontology graph, the offices and the
// consumption. The host builds it from the same feeds the web board reads.
type BoardView struct {
	Agents     []AgentRow
	AgentsNote string // why the list is empty or partial ("" when lit)
	Graph      GraphView
	Offices    []OfficeRow
	Usage      UsageView
	At         time.Time
}

// AgentRow is one live body.
type AgentRow struct {
	Name, Rank, Office, Model, Provider, State string
	Started                                    time.Time
	CtxTokens, CtxLimit                        int
	Input, Output, Cache                       int
	USD                                        *float64 // nil = unpriced
}

// GraphView is the reasoned ontology's creatures and the one-hop bonds between them.
type GraphView struct {
	Nodes   []GraphNode
	Note    string // why the graph is empty or partial
	Summary string // "6 creatures · 118 triples · 2 findings"
}

// GraphNode is one creature, its bonds up, and what the ontology knows about it.
type GraphNode struct {
	ID, Name, Rank string
	Level          int // 0 = the session, one more per hop down
	Up             []GraphBond
	Findings       int
	Knowledge      []KV // drawn in the card, in order
}

// GraphBond is a one-hop bond toward the session: truth · verdict · reports · above.
type GraphBond struct{ To, Bond string }

// KV is one line of a knowledge card; an empty Key continues the key above.
type KV struct{ Key, Value string }

// OfficeRow is one office of the triad: its role, its model and who serves in it.
type OfficeRow struct {
	Name, Role, Model, Fallback, Origin string
	Ranks                               []string
	Live                                int // bodies running in it now
}

// UsageView is the usage journal rolled up for a range.
type UsageView struct {
	Range, Note                      string
	Calls, Input, Output, Cache      int
	USD                              float64
	Unpriced                         int
	ByBody, ByModel, ByOffice, ByDay []UsageRow
}

// UsageRow is one rollup line.
type UsageRow struct {
	Key           string
	Calls, Tokens int
	USD           float64
	Unpriced      int
}

// BoardRanges are the usage ranges `r` cycles through.
var BoardRanges = []string{"24h", "7d", "30d", "all"}

var boardPages = []string{"Agents", "Graph", "Offices", "Usage"}

const (
	pageAgents = iota
	pageGraph
	pageOffices
	pageUsage
)

type boardState struct {
	page    int
	view    BoardView
	loaded  bool
	loading bool
	rng     string
	cursor  [4]int // the selected row per page (the scroll line on Usage)
	offset  [4]int
	sel     string // the selected graph node
	panX    int
	// what a click can hit, as the last frame drew it
	tabX    [][2]int // each tab's columns on the header row
	hits    []hit    // page lines: a row or a node, by line and columns
	lastOff int      // the page's first line on screen
	detail  bool
	ticks   int
}

type evBoard struct{ view BoardView }

// openBoard takes the whole screen for the board; the session keeps running below it.
func (m *Model) openBoard() tea.Cmd {
	m.board = &boardState{rng: BoardRanges[0]}
	return m.fetchBoard() // the view takes the alternate screen while m.board is set
}

// closeBoard gives the screen back and prints what finished while the board was open.
func (m *Model) closeBoard() tea.Cmd {
	m.board = nil
	held := m.held
	m.held = nil
	if m.width != m.lastWidth {
		seq := m.reflowSeq
		return func() tea.Msg { return evReflow{seq} }
	}
	for _, b := range held {
		m.enqueue(printItem{text: b})
	}
	return nil
}

func (m *Model) fetchBoard() tea.Cmd {
	if m.board == nil || m.board.loading {
		return nil
	}
	m.board.loading = true
	host, rng := m.host, m.board.rng
	return func() tea.Msg { return evBoard{host.Board(rng)} }
}

// boardUpdate handles what the board owns while it is open; ok false passes the message on.
func (m *Model) boardUpdate(msg tea.Msg) (tea.Cmd, bool) {
	b := m.board
	switch msg := msg.(type) {
	case evBoard:
		b.view, b.loaded, b.loading = msg.view, true, false
		return nil, true
	case evTick:
		b.ticks++
		if b.ticks%5 == 0 {
			return m.fetchBoard(), false
		}
		return nil, false
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m.closeBoard(), true
		case "tab":
			b.page, b.detail = (b.page+1)%len(boardPages), false
		case "shift+tab":
			b.page, b.detail = (b.page+len(boardPages)-1)%len(boardPages), false
		case "1", "2", "3", "4":
			b.page, b.detail = int(msg.String()[0]-'1'), false
		case "up", "k", "down", "j", "left", "h", "right", "l":
			if b.page == pageGraph {
				m.graphMove(msg.String())
				return nil, true
			}
			switch msg.String() {
			case "up", "k":
				if b.cursor[b.page] > 0 {
					b.cursor[b.page]--
				}
			case "down", "j":
				b.cursor[b.page]++ // clamped when drawn
			}
		case "home", "g":
			b.cursor[b.page] = 0
		case "end", "G":
			b.cursor[b.page] = 1 << 30
		case "enter", " ":
			b.detail = !b.detail
		case "r":
			if b.page == pageUsage {
				for i, r := range BoardRanges {
					if r == b.rng {
						b.rng = BoardRanges[(i+1)%len(BoardRanges)]
						break
					}
				}
				b.loading = false
				return m.fetchBoard(), true
			}
		}
		return nil, true
	case tea.MouseClickMsg:
		m.boardClick(msg.Mouse())
		return nil, true
	case tea.MouseWheelMsg:
		k := "down"
		if msg.Mouse().Button == tea.MouseWheelUp {
			k = "up"
		}
		if b.page == pageGraph {
			return nil, true
		}
		return m.boardUpdate(tea.KeyPressMsg{Code: map[string]rune{"up": tea.KeyUp, "down": tea.KeyDown}[k]})
	case tea.MouseMsg, tea.PasteMsg:
		return nil, true
	}
	return nil, false
}

// boardView draws the whole screen: tabs, the page, the key line.
func (m *Model) boardView() string {
	b, t, w, h := m.board, m.theme, m.width, m.height
	if h < 8 {
		h = 8
	}
	var tabs []string
	for i, p := range boardPages {
		label := fmt.Sprintf(" %d %s ", i+1, p)
		if i == b.page {
			tabs = append(tabs, t.accent.Bold(true).Underline(true).Render(label))
		} else {
			tabs = append(tabs, t.dim.Render(label))
		}
	}
	title := Title(m.words.Dist) + t.tag.Render(" board")
	status := t.dim.Render("loading…")
	if b.loaded {
		status = t.ok.Render("●") + t.dim.Render(" live · "+b.view.At.Format("15:04:05"))
	}
	head := title + "  " + strings.Join(tabs, t.border.Render("│"))
	b.tabX = b.tabX[:0]
	x := lipgloss.Width(title) + 2
	for i, p := range boardPages {
		w := lipgloss.Width(fmt.Sprintf(" %d %s ", i+1, p))
		b.tabX = append(b.tabX, [2]int{x, x + w})
		x += w + 1
	}
	head = padBetween(head, status, w)
	rule := t.border.Render(strings.Repeat("─", w))

	var lines []string
	cur := -1
	b.hits = b.hits[:0]
	switch b.page {
	case 0:
		lines, cur = m.boardAgents()
	case pageGraph:
		lines, cur = m.boardGraph()
	case pageOffices:
		lines, cur = m.boardOffices()
	case pageUsage:
		lines, cur = m.boardUsage()
	}
	body := h - 4
	// the cursor: clamped to the page, and kept in view
	if len(lines) == 0 {
		lines = []string{""}
	}
	sel := b.cursor[b.page]
	if b.page == pageUsage { // Usage scrolls; the cursor is the top line
		maxTop := len(lines) - body
		if maxTop < 0 {
			maxTop = 0
		}
		if sel > maxTop {
			sel = maxTop
		}
		b.cursor[pageUsage], b.offset[pageUsage] = sel, sel
	} else {
		if cur >= 0 {
			if b.offset[b.page] > cur {
				b.offset[b.page] = cur
			}
			if cur >= b.offset[b.page]+body {
				b.offset[b.page] = cur - body + 1
			}
		}
		if b.offset[b.page] > len(lines)-1 {
			b.offset[b.page] = 0
		}
	}
	off := b.offset[b.page]
	b.lastOff = off
	end := off + body
	if end > len(lines) {
		end = len(lines)
	}
	view := append([]string(nil), lines[off:end]...)
	for len(view) < body {
		view = append(view, "")
	}
	for i, l := range view {
		view[i] = ansi.Truncate(l, w, "…")
	}
	keys := "tab page · ↑↓ select · enter detail · esc back"
	if b.page == pageGraph {
		keys = "tab page · ←↑↓→ walk the graph · esc back"
	}
	if b.page == pageUsage {
		keys = "tab page · ↑↓ scroll · r range (" + b.rng + ") · esc back"
	}
	more := ""
	if len(lines) > body {
		more = fmt.Sprintf("%d–%d of %d", off+1, end, len(lines))
	}
	foot := padBetween(t.dim.Render(" "+keys), t.dim.Render(more+" "), w)
	return strings.Join(append(append([]string{head, rule}, view...), rule, foot), "\n")
}

// padBetween puts left and right on one line of width w.
func padBetween(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, w, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) boardAgents() ([]string, int) {
	b, t, w := m.board, m.theme, m.width
	rows := b.view.Agents
	if !b.loaded {
		return []string{"", t.dim.Render("  reading the court…")}, -1
	}
	if len(rows) == 0 {
		note := b.view.AgentsNote
		if note == "" {
			note = "no live " + m.words.Courts + " — the session is idle"
		}
		return []string{"", t.dim.Render("  " + note)}, -1
	}
	if b.cursor[0] >= len(rows) {
		b.cursor[0] = len(rows) - 1
	}
	sel := b.cursor[0]
	nameW := 6
	for _, r := range rows {
		if n := lipgloss.Width(r.Name); n > nameW {
			nameW = n
		}
	}
	if nameW > 24 {
		nameW = 24
	}
	// columns by priority: the narrow screen keeps state, context and spend
	showRank, showOffice, showAge, showModel := w >= 72, w >= 92, w >= 84, w >= 120
	stateW := 16
	if w < 80 {
		stateW = 12
	}
	now := b.view.At
	if now.IsZero() {
		now = time.Now()
	}
	hdr := []string{"", "NAME"}
	if showRank {
		hdr = append(hdr, "RANK")
	}
	if showOffice {
		hdr = append(hdr, "OFFICE")
	}
	hdr = append(hdr, "STATE")
	if showAge {
		hdr = append(hdr, "AGE")
	}
	hdr = append(hdr, "CONTEXT", "TOKENS", "COST")
	if showModel {
		hdr = append(hdr, "MODEL")
	}
	var cells [][]string
	for i, r := range rows {
		glyph, st := stateGlyph(t, r.State, stateW, m.spin.View())
		name := ansi.Truncate(r.Name, nameW, "…")
		mark := "  "
		if i == sel {
			mark, name = t.accent.Render("›")+" ", t.tag.Render(name)
		}
		row := []string{mark + glyph, name}
		if showRank {
			row = append(row, t.rankStyle(r.Rank).Render(r.Rank))
		}
		if showOffice {
			row = append(row, r.Office)
		}
		row = append(row, strings.TrimRight(st, " "))
		if showAge {
			age := "—"
			if !r.Started.IsZero() {
				age = shortDur(now.Sub(r.Started))
			}
			row = append(row, age)
		}
		row = append(row, strings.TrimRight(ctxBar(t, r.CtxTokens, r.CtxLimit, 8), " "), humanTokens(r.Input+r.Output+r.Cache), usd(r.USD))
		if showModel {
			row = append(row, t.dim.Render(r.Model))
		}
		cells = append(cells, row)
	}
	lines := []string{""}
	tbl := m.boardTable(hdr, cells, map[int]bool{len(hdr) - 3: true, len(hdr) - 2: true})
	first := len(lines) + 3 // the top border, the header, its rule
	lines = append(lines, tbl...)
	cur := first + sel
	for i := range rows {
		b.hits = append(b.hits, hit{line: first + i, x0: 0, x1: w, row: i})
	}
	if b.detail && sel >= 0 && sel < len(rows) {
		r := rows[sel]
		lines = append(lines, "", t.border.Render("  "+strings.Repeat("─", max(0, w-4))))
		kv := func(k, v string) {
			if v != "" {
				lines = append(lines, "  "+t.dim.Render(fmt.Sprintf("%-10s", k))+v)
			}
		}
		kv("body", t.tag.Render(r.Name))
		kv("rank", r.Rank)
		kv("office", r.Office)
		kv("model", r.Model)
		kv("provider", r.Provider)
		kv("state", r.State)
		if !r.Started.IsZero() {
			kv("started", r.Started.Local().Format("15:04:05")+" ("+shortDur(now.Sub(r.Started))+" ago)")
		}
		if r.CtxLimit > 0 {
			kv("context", fmt.Sprintf("%s of %s", humanTokens(r.CtxTokens), humanTokens(r.CtxLimit)))
		}
		kv("tokens", fmt.Sprintf("in %s · out %s · cache %s", humanTokens(r.Input), humanTokens(r.Output), humanTokens(r.Cache)))
		kv("cost", usd(r.USD))
	}
	return lines, cur
}

func (m *Model) boardOffices() ([]string, int) {
	b, t, w := m.board, m.theme, m.width
	if !b.loaded {
		return []string{"", t.dim.Render("  reading the offices…")}, -1
	}
	rows := b.view.Offices
	if len(rows) == 0 {
		return []string{"", t.dim.Render("  no office configured")}, -1
	}
	if b.cursor[pageOffices] >= len(rows) {
		b.cursor[pageOffices] = len(rows) - 1
	}
	sel := b.cursor[pageOffices]
	spent := map[string]UsageRow{}
	for _, u := range b.view.Usage.ByOffice {
		spent[u.Key] = u
	}
	nameW := 10
	for _, r := range rows {
		if n := lipgloss.Width(r.Name); n > nameW {
			nameW = n
		}
	}
	var cells [][]string
	for i, r := range rows {
		live := t.dim.Render("·")
		if r.Live > 0 {
			live = t.accent.Render(fmt.Sprintf("● %d", r.Live))
		}
		name := r.Name
		mark := "  "
		if i == sel {
			mark, name = t.accent.Render("›")+" ", t.tag.Render(name)
		}
		model := r.Model
		if r.Fallback != "" && w >= 120 {
			model += t.dim.Render("  ↳ " + r.Fallback)
		}
		cells = append(cells, []string{mark + t.accent.Render("◆"), name, r.Role, live, humanTokens(spent[r.Name].Tokens), model})
	}
	lines := []string{""}
	tbl := m.boardTable([]string{"", "OFFICE", "DOES", "LIVE", "TOKENS", "MODEL"}, cells, map[int]bool{4: true})
	first := len(lines) + 3
	lines = append(lines, tbl...)
	cur := first + sel
	for i := range rows {
		b.hits = append(b.hits, hit{line: first + i, x0: 0, x1: w, row: i})
	}
	r := rows[sel]
	lines = append(lines, "", t.border.Render("  "+strings.Repeat("─", max(0, w-4))))
	kv := func(k, v string) {
		if v != "" {
			lines = append(lines, "  "+t.dim.Render(fmt.Sprintf("%-10s", k))+v)
		}
	}
	kv("office", t.tag.Render(r.Name))
	kv("does", r.Role)
	kv("model", r.Model)
	kv("fallback", r.Fallback)
	kv("set in", r.Origin)
	var ranks []string
	for _, rk := range r.Ranks {
		ranks = append(ranks, t.rankStyle(rk).Render(rk))
	}
	kv("serves", strings.Join(ranks, ", "))
	if u, ok := spent[r.Name]; ok {
		kv("spent", fmt.Sprintf("%s tokens · %d calls · $%.4f", humanTokens(u.Tokens), u.Calls, u.USD))
	}
	return lines, cur
}

func (m *Model) boardUsage() ([]string, int) {
	b, t, w := m.board, m.theme, m.width
	u := b.view.Usage
	if !b.loaded {
		return []string{"", t.dim.Render("  reading the usage journal…")}, -1
	}
	var rs []string
	for _, r := range BoardRanges {
		if r == b.rng {
			rs = append(rs, t.accent.Bold(true).Render(r))
		} else {
			rs = append(rs, t.dim.Render(r))
		}
	}
	lines := []string{"", "  " + t.dim.Render("range ") + strings.Join(rs, t.dim.Render(" · "))}
	if u.Note != "" {
		lines = append(lines, "  "+t.dim.Render(u.Note))
	}
	if u.Calls == 0 {
		return append(lines, "", t.dim.Render("  no calls in this range")), -1
	}
	cost := fmt.Sprintf("$%.4f", u.USD)
	if u.Unpriced > 0 {
		cost += t.dim.Render(fmt.Sprintf(" (+%d unpriced)", u.Unpriced))
	}
	lines = append(lines, "",
		"  "+t.tag.Render(humanTokens(u.Input+u.Output+u.Cache))+t.dim.Render(" tokens · ")+t.tag.Render(itoa(u.Calls))+t.dim.Render(" calls · ")+t.tag.Render(cost),
		"  "+t.dim.Render(fmt.Sprintf("in %s · out %s · cache %s", humanTokens(u.Input), humanTokens(u.Output), humanTokens(u.Cache))))
	if len(u.ByDay) > 1 {
		lines = append(lines, "", "  "+t.dim.Render("by day   ")+sparkline(t, u.ByDay, w-14)+t.dim.Render("  "+u.ByDay[0].Key+" → "+u.ByDay[len(u.ByDay)-1].Key))
	}
	section := func(title string, rows []UsageRow) {
		if len(rows) == 0 {
			return
		}
		lines = append(lines, "", "  "+t.tag.Render(title))
		keyW := 8
		for _, r := range rows {
			if n := lipgloss.Width(r.Key); n > keyW {
				keyW = n
			}
		}
		if keyW > w/3 {
			keyW = w / 3
		}
		top := rows[0].Tokens
		for _, r := range rows {
			if r.Tokens > top {
				top = r.Tokens
			}
		}
		barW := w - keyW - 38
		if barW > 30 {
			barW = 30
		}
		if barW < 4 {
			barW = 4
		}
		for _, r := range rows {
			c := fmt.Sprintf("$%.4f", r.USD)
			if r.Unpriced > 0 && r.USD == 0 {
				c = "—"
			}
			lines = append(lines, fmt.Sprintf("  %-*s  %s %7s %5d calls %9s", keyW, ansi.Truncate(r.Key, keyW, "…"),
				bar(t, r.Tokens, top, barW), humanTokens(r.Tokens), r.Calls, c))
		}
	}
	section("by body", u.ByBody)
	section("by model", u.ByModel)
	section("by office", u.ByOffice)
	return lines, -1
}

func (t Theme) rankStyle(rank string) lipgloss.Style {
	if s, ok := t.ranks[rank]; ok {
		return s
	}
	return t.text
}

func bondStyle(t Theme, bond string) lipgloss.Style {
	switch bond {
	case "truth":
		return t.classes["read"]
	case "verdict":
		return t.accent
	case "reports":
		return t.classes["write"]
	}
	return t.dim
}

func stateGlyph(t Theme, state string, w int, spin string) (string, string) {
	s := fmt.Sprintf("%-*s", w, ansi.Truncate(state, w-1, "…"))
	switch {
	case state == "done":
		return t.ok.Render("✓"), t.ok.Render(s)
	case state == "failed" || strings.HasPrefix(state, "fail"):
		return t.bad.Render("✗"), t.bad.Render(s)
	case strings.Contains(state, "gate") || strings.Contains(state, "wait"):
		return t.warn.Render("◐"), t.warn.Render(s)
	case state == "" || state == "idle":
		return t.dim.Render("◌"), t.dim.Render(s)
	}
	g := []rune(strings.TrimSpace(ansi.Strip(spin)))
	if len(g) == 0 {
		g = []rune("●")
	}
	return t.accent.Render(string(g[0])), t.text.Render(s)
}

// ctxBar is the context window's fill as the footer's gradient meter and a percent.
func ctxBar(t Theme, used, limit, w int) string {
	if limit <= 0 {
		return t.Meter(-1, w) + strings.Repeat(" ", 6)
	}
	pct := 100 * used / limit
	return t.Meter(pct, w) + fmt.Sprintf(" %3d%% ", pct)
}

func bar(t Theme, v, top, w int) string {
	fill := 0
	if top > 0 {
		fill = (v*w + top - 1) / top
	}
	if fill > w {
		fill = w
	}
	return t.accent.Render(strings.Repeat("█", fill)) + t.border.Render(strings.Repeat("░", w-fill))
}

func sparkline(t Theme, rows []UsageRow, w int) string {
	levels := []rune("▁▂▃▄▅▆▇█")
	if len(rows) > w {
		rows = rows[len(rows)-w:]
	}
	top := 0
	for _, r := range rows {
		if r.Tokens > top {
			top = r.Tokens
		}
	}
	var sb strings.Builder
	for _, r := range rows {
		i := 0
		if top > 0 {
			i = r.Tokens * (len(levels) - 1) / top
		}
		sb.WriteRune(levels[i])
	}
	return t.accent.Render(sb.String())
}

func usd(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("$%.4f", *v)
}

func shortDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// hit is one clickable thing on a page: a list row (row) or a graph node (id).
type hit struct {
	line, x0, x1 int
	row          int
	id           string
}

// boardClick maps a click to what the last frame drew there: a tab, a row, a node. Clicking the
// selected row again opens its detail.
func (m *Model) boardClick(ms tea.Mouse) {
	b := m.board
	if ms.Button != tea.MouseLeft {
		return
	}
	if ms.Y == 0 {
		for i, r := range b.tabX {
			if ms.X >= r[0] && ms.X < r[1] {
				b.page, b.detail = i, false
			}
		}
		return
	}
	line := ms.Y - 2 + b.lastOff
	for _, h := range b.hits {
		if h.line != line || ms.X < h.x0 || ms.X >= h.x1 {
			continue
		}
		if h.id != "" {
			b.sel = h.id
			return
		}
		if b.cursor[b.page] == h.row {
			b.detail = !b.detail
		}
		b.cursor[b.page] = h.row
		return
	}
}

// boardTable draws rows as a Lip Gloss table: rounded, header dim and bold, no column rules;
// right names the columns aligned right (numbers). Its rows start 3 lines down.
func (m *Model) boardTable(headers []string, rows [][]string, right map[int]bool) []string {
	t := m.theme
	tb := table.New().Border(lipgloss.RoundedBorder()).BorderStyle(t.border).BorderColumn(false).
		Headers(headers...).Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := lipgloss.NewStyle().Padding(0, 1)
			if right[col] {
				st = st.Align(lipgloss.Right)
			}
			if row == table.HeaderRow {
				return st.Inherit(t.dim).Bold(true)
			}
			return st
		})
	return strings.Split(tb.Render(), "\n")
}
