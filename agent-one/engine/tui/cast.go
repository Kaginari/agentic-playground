package tui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The cast: every agent on screen has a face and a voice. A rank's icon is a two-frame half-block
// sprite (6 pixels wide, 4 high: two rows) drawn beside the spinner; its verbs are the rank's own —
// the domain at the gate weighs a verdict, the zone gathers ground truth. The model's thinking shows
// as it arrives and folds into a block when the answer starts. Every agent's steps, text and
// thinking are kept, so the agent view (ctrl+t) can watch the session and each Subagent.

// --- icons ---

type icon struct {
	frames  [2][]string
	palette map[byte]string
}

// robot is agent-one's face for every rank, tinted by the rank's colour.
var robot = [2][]string{{"..AA..", "FFFFFF", "FEFFEF", ".F..F."}, {"..AA..", "FFFFFF", "FFFFFF", ".F..F."}}

// Icon is a rank's two rows at a frame.
func (t Theme) Icon(dist, rank string, frame int) []string {
	f := frame % 2
	if dist == "agent-one" {
		col := "#eaf2ff"
		if c := t.rankStyle(rank).GetForeground(); c != nil && c != (lipgloss.NoColor{}) {
			if rgba, ok := c.(interface{ RGBA() (r, g, b, a uint32) }); ok {
				r, g, b, _ := rgba.RGBA()
				col = fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
			}
		}
		return halfBlocks(robot[f], map[byte]string{'A': "#d4af37", 'F': col, 'E': "#1a1033"})
	}
	return halfBlocks(robot[f], map[byte]string{'A': "#d4af37", 'F': "#eaf2ff", 'E': "#1a1033"})
}

// --- verbs ---

var agentOneVerbs = map[string][]string{
	"orchestrator": {"Planning", "Orchestrating", "Routing the work", "Thinking it through"},
	"zone":         {"Checking the zone", "Collecting the facts"},
	"domain":       {"Reviewing the change", "Checking the invariants", "Signing off"},
	"coord":        {"Coordinating", "Syncing the team"},
	"service":      {"Running the service", "Keeping it up"},
	"auditor":      {"Auditing", "Reading the log"},
}

// gateRank is the rank whose face the end-of-turn gate holds.
func gateRank(dist string) string {
	if dist == "agent-one" {
		return "domain"
	}
	return "domain"
}

// Verb is a rank's word for a state: the rank's own for thinking and the gate, the plain one for
// a tool or an approval. seed picks among them, so a state keeps its word while it lasts.
func Verb(dist, rank, state, tool string, seed int) string {
	switch state {
	case "tool":
		if tool != "" {
			return "Running " + tool + "…"
		}
		return "Running a tool…"
	case "waiting on gate":
		return "Waiting for approval…"
	case "done":
		return "Finishing…"
	case "gating":
		rank = gateRank(dist)
	}
	table := agentOneVerbs
	vs := table[rank]
	if len(vs) == 0 {
		vs = table["orchestrator"]
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%s|%s|%d", rank, state, seed)
	return vs[int(h.Sum32())%len(vs)] + "…"
}

// --- thinking ---

// thought is a finished stretch of reasoning, folded into the scrollback.
type thought struct {
	text   string
	took   time.Duration
	expand bool
}

// Thought renders a folded thought: one line, or all of it when expanded.
func (t Theme) Thought(v thought, width int) string {
	head := t.dim.Italic(true).Render(fmt.Sprintf("∴ Thought for %s", v.took.Round(time.Second)))
	if !v.expand {
		return head + t.dim.Render("  (ctrl+o to expand)")
	}
	var b strings.Builder
	b.WriteString(head)
	for _, l := range strings.Split(wrap(strings.TrimSpace(v.text), width-4), "\n") {
		b.WriteString("\n" + indent + t.dim.Italic(true).Render(l))
	}
	return b.String()
}

// liveThinking is the thought as it arrives: its last lines, dim.
func (t Theme) liveThinking(text string, width, lines int) string {
	ls := strings.Split(wrap(strings.TrimSpace(text), width-4), "\n")
	if len(ls) > lines {
		ls = ls[len(ls)-lines:]
	}
	for i, l := range ls {
		ls[i] = indent + t.dim.Italic(true).Render(l)
	}
	return t.dim.Italic(true).Render("∴ thinking") + "\n" + strings.Join(ls, "\n")
}

// --- every agent's log ---

// EvStream is a agent's streamed text or thinking (the session's text also arrives as EvDelta).
type EvStream struct {
	Agent, Kind, Text string
}

// EvBodyStep is a Subagent's tool step, for its log.
type EvBodyStep struct {
	Agent string
	Tool  ToolView
	End   bool
}

type logEntry struct {
	kind string // text · thinking · tool
	text string
	tool ToolView
}

type bodyLog struct {
	rank, state string
	started     time.Time
	entries     []logEntry
}

func (m *Model) logOf(agent string) *bodyLog {
	if m.logs == nil {
		m.logs = map[string]*bodyLog{}
	}
	l := m.logs[agent]
	if l == nil {
		l = &bodyLog{started: time.Now()}
		m.logs[agent] = l
		m.logOrder = append(m.logOrder, agent)
	}
	return l
}

// appendStream adds streamed text to a agent's log, joining a run of the same kind.
func (l *bodyLog) appendStream(kind, text string) {
	if n := len(l.entries); n > 0 && l.entries[n-1].kind == kind {
		l.entries[n-1].text += text
		return
	}
	l.entries = append(l.entries, logEntry{kind: kind, text: text})
}

func (l *bodyLog) step(t ToolView, end bool) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if e := &l.entries[i]; e.kind == "tool" && e.tool.ID == t.ID {
			e.tool = t
			return
		}
	}
	l.entries = append(l.entries, logEntry{kind: "tool", tool: t})
}

// --- the agent view (ctrl+t) ---

type bodyView struct {
	sel    int
	scroll int // lines up from the bottom; 0 follows the tail
}

// agents are the session and every Subagent seen, in order.
func (m *Model) agents() []string {
	out := []string{m.words.Session()}
	for _, b := range m.logOrder {
		if b != m.words.Session() {
			out = append(out, b)
		}
	}
	return out
}

func (m *Model) openBodies() tea.Cmd {
	m.bview = &bodyView{}
	bs := m.agents()
	// open on the first live Subagent, if one runs
	for i, b := range bs {
		if c := m.subagents[b]; c != nil && c.State != "done" {
			m.bview.sel = i
			break
		}
	}
	return nil
}

func (m *Model) bodiesKey(k tea.KeyPressMsg) tea.Cmd {
	v, n := m.bview, len(m.agents())
	switch k.String() {
	case "esc", "ctrl+t", "q", "ctrl+c":
		m.bview = nil
		if m.width != m.lastWidth {
			seq := m.reflowSeq
			return func() tea.Msg { return evReflow{seq} }
		}
		for _, b := range m.held {
			m.enqueue(printItem{text: b})
		}
		m.held = nil
	case "tab", "right", "l":
		v.sel, v.scroll = (v.sel+1)%n, 0
	case "shift+tab", "left", "h":
		v.sel, v.scroll = (v.sel+n-1)%n, 0
	case "up", "k":
		v.scroll++
	case "down", "j":
		if v.scroll > 0 {
			v.scroll--
		}
	case "pgup":
		v.scroll += m.height / 2
	case "pgdown":
		v.scroll = max(0, v.scroll-m.height/2)
	case "end", "G":
		v.scroll = 0
	}
	return nil
}

// bodiesView draws the whole screen: a tab per agent, the selected agent's log, the keys.
func (m *Model) bodiesView() string {
	t, w, h, v := m.theme, m.width, max(m.height, 8), m.bview
	bs := m.agents()
	if v.sel >= len(bs) {
		v.sel = len(bs) - 1
	}
	var tabs []string
	for i, b := range bs {
		dot := t.dim.Render("·")
		if c := m.subagents[b]; c != nil && c.State != "done" {
			dot = t.accent.Render("●")
		} else if i == 0 && m.busy {
			dot = t.accent.Render("●")
		}
		label := " " + b + " "
		if i == v.sel {
			label = t.accent.Bold(true).Underline(true).Render(label)
		} else {
			label = t.dim.Render(label)
		}
		tabs = append(tabs, dot+label)
	}
	head := padBetween(Title(m.words.Dist)+t.tag.Render(" agents")+"  "+strings.Join(tabs, t.border.Render("│")), t.dim.Render("live "), w)
	sel := bs[v.sel]
	rank, state := m.rankOf(sel)
	ic := t.Icon(m.words.Dist, rank, m.shimmer/4)
	who := t.rankStyle(rank).Bold(true).Render(sel) + t.dim.Render("  "+rank+" · "+orStr(state, "idle"))
	card := []string{ic[0] + "  " + who, ic[1] + "  " + t.dim.Render(m.askOf(sel))}
	var agent []string
	if v.sel == 0 {
		agent = m.sessionLines(w)
	} else {
		agent = m.logLines(sel, w)
	}
	room := h - 6
	end := len(agent) - v.scroll
	if end < room {
		end = min(room, len(agent))
		v.scroll = len(agent) - end
	}
	start := max(0, end-room)
	view := append([]string(nil), agent[start:end]...)
	for len(view) < room {
		view = append(view, "")
	}
	for i := range view {
		view[i] = ansi.Truncate(view[i], w, "…")
	}
	rule := t.border.Render(strings.Repeat("─", w))
	keys := " tab next agent · ↑↓ scroll · end follow · esc back"
	more := ""
	if v.scroll > 0 {
		more = fmt.Sprintf("%d lines below ", v.scroll)
	}
	return strings.Join(append(append([]string{head, rule, card[0], card[1], rule}, view...), padBetween(t.dim.Render(keys), t.dim.Render(more), w)), "\n")
}

func (m *Model) rankOf(agent string) (rank, state string) {
	if agent == m.words.Session() {
		st := "idle"
		if m.busy {
			st = strings.ToLower(strings.TrimSuffix(m.verb, "…"))
		}
		return m.words.Session(), st
	}
	if c := m.subagents[agent]; c != nil {
		return c.Rank, c.State
	}
	if l := m.logs[agent]; l != nil {
		return l.rank, orStr(l.state, "done")
	}
	return "", ""
}

func (m *Model) askOf(agent string) string {
	if c := m.subagents[agent]; c != nil && c.Ask != "" {
		return oneLine(c.Ask)
	}
	if agent == m.words.Session() {
		return "the session"
	}
	return ""
}

// sessionLines is the session's transcript: its blocks at the width, the live stream after.
func (m *Model) sessionLines(w int) []string {
	var out []string
	start := max(0, len(m.blocks)-60)
	for _, r := range m.blocks[start:] {
		if b := r(w); b != "" {
			out = append(out, strings.Split(b, "\n")...)
			out = append(out, "")
		}
	}
	if m.think.Len() > 0 {
		out = append(out, strings.Split(m.theme.liveThinking(m.think.String(), w, 8), "\n")...)
	}
	if s := m.stream.String(); strings.TrimSpace(s) != "" {
		out = append(out, strings.Split(m.theme.Assistant(s, w), "\n")...)
	}
	return out
}

// logLines is a Subagent's log: thinking dim, text as markdown, tool steps as cards.
func (m *Model) logLines(agent string, w int) []string {
	l := m.logs[agent]
	if l == nil || len(l.entries) == 0 {
		return []string{"", m.theme.dim.Render("  nothing yet — its steps, text and thinking appear here as they happen")}
	}
	var out []string
	for _, e := range l.entries {
		var b string
		switch e.kind {
		case "thinking":
			b = m.theme.Thought(thought{text: e.text, expand: true}, w)
			b = strings.Replace(b, "∴ Thought for 0s", "∴ thinking", 1)
		case "text":
			b = m.theme.Assistant(e.text, w)
		case "tool":
			b = m.theme.Tool(e.tool, w)
		}
		if b != "" {
			out = append(out, strings.Split(b, "\n")...)
			out = append(out, "")
		}
	}
	return out
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// statusLines is the spinner as the cast draws it: the agent's icon beside the verb, its name and
// state under it.
func (m *Model) statusLines(spinLine string) string {
	rank := m.words.Session()
	if strings.HasPrefix(m.state, "gating") {
		rank = gateRank(m.words.Dist)
	}
	ic := m.theme.Icon(m.words.Dist, rank, m.shimmer/4)
	sub := m.words.Session() + " · " + orStr(m.state, "thinking")
	if m.state == "gating" {
		sub = rank + " · the " + m.words.Gate + " weighs the turn's writes"
	}
	if n := m.liveCourts(); n > 0 {
		word := m.words.Subagents
		if n == 1 {
			word = m.words.Subagent
		}
		sub += fmt.Sprintf(" · %d %s running — ctrl+t to watch", n, word)
	}
	return ic[0] + " " + spinLine + "\n" + ic[1] + " " + m.theme.dim.Render(sub)
}

func (m *Model) liveCourts() int {
	n := 0
	for _, c := range m.subagents {
		if c.State != "done" {
			n++
		}
	}
	return n
}

var _ = lipgloss.Width
