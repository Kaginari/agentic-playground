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

// The cast: every body on screen has a face and a voice. A rank's icon is a two-frame half-block
// sprite (6 pixels wide, 4 high: two rows) drawn beside the spinner; its verbs are the rank's own —
// the orc at the gate weighs a verdict, the slime gathers ground truth. The model's thinking shows
// as it arrives and folds into a block when the answer starts. Every body's steps, text and
// thinking are kept, so the body view (ctrl+t) can watch the session and each Court.

// --- icons ---

type icon struct {
	frames  [2][]string
	palette map[byte]string
}

var isekaiIcons = map[string]icon{
	"slime": {frames: [2][]string{{"..BB..", ".BBBB.", "BEBBEB", "SSSSSS"}, {"......", ".BBBB.", "BEBBEB", "SSSSSS"}},
		palette: map[byte]string{'B': "#58b7f0", 'E': "#0d1b2a", 'S': "#2a78b8"}},
	"orc": {frames: [2][]string{{".DDDD.", "GRGGRG", "GGGGGG", "GWGGWG"}, {".DDDD.", "GGGGGG", "GGGGGG", "GWGGWG"}},
		palette: map[byte]string{'G': "#6fae5a", 'D': "#3f6f33", 'R': "#e06c5f", 'W': "#f2efe6"}},
	"elf": {frames: [2][]string{{".YYYY.", "YKKKKY", "PEKKEP", ".KKKK."}, {".YYYY.", "YKKKKY", "PKKKKP", ".KKKK."}},
		palette: map[byte]string{'Y': "#d4af37", 'K': "#f1d3b3", 'P': "#e8c29e", 'E': "#1a1033"}},
	"kijin": {frames: [2][]string{{"H....H", "RRRRRR", "RERRER", ".RRRR."}, {"H....H", "RRRRRR", "RRRRRR", ".RRRR."}},
		palette: map[byte]string{'R': "#c53d34", 'H': "#f2efe6", 'E': "#0d1b2a"}},
	"dark-elf": {frames: [2][]string{{".WWWW.", "WVVVVW", "PEVVEP", ".VVVV."}, {".WWWW.", "WVVVVW", "PVVVVP", ".VVVV."}},
		palette: map[byte]string{'W': "#e8e4ff", 'V': "#7c5cd6", 'P': "#9a86e8", 'E': "#f2efe6"}},
}

// robot is agent-one's face for every rank, tinted by the rank's colour.
var robot = [2][]string{{"..AA..", "FFFFFF", "FEFFEF", ".F..F."}, {"..AA..", "FFFFFF", "FFFFFF", ".F..F."}}

// Icon is a rank's two rows at a frame.
func (t Theme) Icon(dist, rank string, frame int) []string {
	f := frame % 2
	if dist == "agent-one" {
		col := "#eaf2ff"
		if c := t.rankStyle(rank).GetForeground(); c != nil {
			if rgba, ok := c.(interface{ RGBA() (r, g, b, a uint32) }); ok {
				r, g, b, _ := rgba.RGBA()
				col = fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
			}
		}
		return halfBlocks(robot[f], map[byte]string{'A': "#d4af37", 'F': col, 'E': "#1a1033"})
	}
	key := rank
	switch rank {
	case "rimuru", "":
		key = "slime"
	case "high-orc":
		key = "orc"
	case "high-elf":
		key = "elf"
	}
	ic, ok := isekaiIcons[key]
	if !ok {
		ic = isekaiIcons["slime"]
	}
	return halfBlocks(ic.frames[f], ic.palette)
}

// --- verbs ---

var isekaiVerbs = map[string][]string{
	"rimuru":   {"Pondering", "Consulting the Great Sage", "Absorbing the context", "Predating the problem", "Reincarnating the plan", "Slime-thinking"},
	"slime":    {"Gathering ground truth", "Oozing through the zone", "Reading the territory", "Tasting the facts"},
	"orc":      {"Weighing the verdict", "Guarding the gate", "Grunting at the diff", "Judging the landing", "Sharpening the tusks"},
	"elf":      {"Weaving the shared mind", "Routing to the orcs", "Braiding the lanes", "Singing the voice"},
	"kijin":    {"Tending the domain", "Forging", "Keeping the watch"},
	"dark-elf": {"Auditing the chronicle", "Reading between the verdicts"},
}

var agentOneVerbs = map[string][]string{
	"orchestrator": {"Planning", "Orchestrating", "Routing the work", "Thinking it through"},
	"zone":         {"Checking the zone", "Collecting the facts"},
	"domain":       {"Reviewing the change", "Checking the invariants", "Signing off"},
	"coord":        {"Coordinating", "Syncing the team"},
	"service":      {"Running the service", "Keeping it up"},
	"auditor":      {"Auditing", "Reading the log"},
}

// gateRank is the rank whose face the end-of-turn gate wears.
func gateRank(dist string) string {
	if dist == "agent-one" {
		return "domain"
	}
	return "orc"
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
	table := isekaiVerbs
	if dist == "agent-one" {
		table = agentOneVerbs
	}
	vs := table[rank]
	if len(vs) == 0 {
		if dist == "agent-one" {
			vs = table["orchestrator"]
		} else {
			vs = table["rimuru"]
		}
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

// --- every body's log ---

// EvStream is a body's streamed text or thinking (the session's text also arrives as EvDelta).
type EvStream struct {
	Body, Kind, Text string
}

// EvBodyStep is a Court's tool step, for its log.
type EvBodyStep struct {
	Body string
	Tool ToolView
	End  bool
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

func (m *Model) logOf(body string) *bodyLog {
	if m.logs == nil {
		m.logs = map[string]*bodyLog{}
	}
	l := m.logs[body]
	if l == nil {
		l = &bodyLog{started: time.Now()}
		m.logs[body] = l
		m.logOrder = append(m.logOrder, body)
	}
	return l
}

// appendStream adds streamed text to a body's log, joining a run of the same kind.
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

// --- the body view (ctrl+t) ---

type bodyView struct {
	sel    int
	scroll int // lines up from the bottom; 0 follows the tail
}

// bodies are the session and every Court seen, in order.
func (m *Model) bodies() []string {
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
	bs := m.bodies()
	// open on the first live Court, if one runs
	for i, b := range bs {
		if c := m.courts[b]; c != nil && c.State != "done" {
			m.bview.sel = i
			break
		}
	}
	return nil
}

func (m *Model) bodiesKey(k tea.KeyPressMsg) tea.Cmd {
	v, n := m.bview, len(m.bodies())
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

// bodiesView draws the whole screen: a tab per body, the selected body's log, the keys.
func (m *Model) bodiesView() string {
	t, w, h, v := m.theme, m.width, max(m.height, 8), m.bview
	bs := m.bodies()
	if v.sel >= len(bs) {
		v.sel = len(bs) - 1
	}
	var tabs []string
	for i, b := range bs {
		dot := t.dim.Render("·")
		if c := m.courts[b]; c != nil && c.State != "done" {
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
	head := padBetween(Title(m.words.Dist)+t.tag.Render(" bodies")+"  "+strings.Join(tabs, t.border.Render("│")), t.dim.Render("live "), w)
	sel := bs[v.sel]
	rank, state := m.rankOf(sel)
	ic := t.Icon(m.words.Dist, rank, m.shimmer/4)
	who := t.rankStyle(rank).Bold(true).Render(sel) + t.dim.Render("  "+rank+" · "+orStr(state, "idle"))
	card := []string{ic[0] + "  " + who, ic[1] + "  " + t.dim.Render(m.askOf(sel))}
	var body []string
	if v.sel == 0 {
		body = m.sessionLines(w)
	} else {
		body = m.logLines(sel, w)
	}
	room := h - 6
	end := len(body) - v.scroll
	if end < room {
		end = min(room, len(body))
		v.scroll = len(body) - end
	}
	start := max(0, end-room)
	view := append([]string(nil), body[start:end]...)
	for len(view) < room {
		view = append(view, "")
	}
	for i := range view {
		view[i] = ansi.Truncate(view[i], w, "…")
	}
	rule := t.border.Render(strings.Repeat("─", w))
	keys := " tab next body · ↑↓ scroll · end follow · esc back"
	more := ""
	if v.scroll > 0 {
		more = fmt.Sprintf("%d lines below ", v.scroll)
	}
	return strings.Join(append(append([]string{head, rule, card[0], card[1], rule}, view...), padBetween(t.dim.Render(keys), t.dim.Render(more), w)), "\n")
}

func (m *Model) rankOf(body string) (rank, state string) {
	if body == m.words.Session() {
		st := "idle"
		if m.busy {
			st = strings.ToLower(strings.TrimSuffix(m.verb, "…"))
		}
		return m.words.Session(), st
	}
	if c := m.courts[body]; c != nil {
		return c.Rank, c.State
	}
	if l := m.logs[body]; l != nil {
		return l.rank, orStr(l.state, "done")
	}
	return "", ""
}

func (m *Model) askOf(body string) string {
	if c := m.courts[body]; c != nil && c.Ask != "" {
		return oneLine(c.Ask)
	}
	if body == m.words.Session() {
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

// logLines is a Court's log: thinking dim, text as markdown, tool steps as cards.
func (m *Model) logLines(body string, w int) []string {
	l := m.logs[body]
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

// statusLines is the spinner as the cast draws it: the body's icon beside the verb, its name and
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
		word := m.words.Courts
		if n == 1 {
			word = m.words.Court
		}
		sub += fmt.Sprintf(" · %d %s running — ctrl+t to watch", n, word)
	}
	return ic[0] + " " + spinLine + "\n" + ic[1] + " " + m.theme.dim.Render(sub)
}

func (m *Model) liveCourts() int {
	n := 0
	for _, c := range m.courts {
		if c.State != "done" {
			n++
		}
	}
	return n
}

var _ = lipgloss.Width
