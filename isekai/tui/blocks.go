package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The view model: every block the session draws, as a pure function of a view and a width.
// Nothing here knows the loop or Bubble Tea; the golden tests render each block at 80 and 120
// columns with the escapes stripped.

const (
	collapseLines = 8  // tool output shown before "… +N lines"
	collapseDiff  = 20 // diff lines shown before collapsing
	indent        = "  "
)

// Welcome is the framed box at start.
type Welcome struct {
	Dist    string
	Version string
	World   string
	Model   string
	Offices []string // "great-sage → gemini/x"
	Board   string   // URL or ""
	Off     int      // features switched off
	Session string
	Holes   []string
	// WorldWord labels the root row ("world" | "workspace"); "" reads "world".
	WorldWord string
}

// Render the welcome box.
func (t Theme) Welcome(w Welcome, width int) string {
	if w.WorldWord == "" {
		w.WorldWord = "world"
	}
	inner := min(width-4, 76)
	if inner < 30 {
		inner = width - 4
	}
	var rows []string
	title := Title(w.Dist) + " " + t.dim.Render(w.Version)
	if inner >= mascotWidth+30 {
		// the mascot, with the title and the tagline beside it
		art, tag := Mascot(w.Dist)
		side := []string{"", title, t.dim.Render(ansi.Truncate(tag, inner-mascotWidth-3, "…")), ""}
		for i, a := range art {
			rows = append(rows, a+"   "+side[i])
		}
		rows = append(rows, "")
	} else {
		rows = append(rows, title, "")
	}
	labels := []string{w.WorldWord, "model", "offices", "board", "session", "off"}
	lw := 0
	for _, l := range labels {
		if len(l)+1 > lw {
			lw = len(l) + 1
		}
	}
	row := func(k, v string) {
		if v == "" {
			return
		}
		for i, l := range strings.Split(wrap(v, inner-lw-1), "\n") {
			k2 := k
			if i > 0 {
				k2 = ""
			}
			rows = append(rows, t.dim.Render(fmt.Sprintf("%-*s", lw, k2))+ansi.Truncate(l, inner-lw-1, "…"))
		}
	}
	row(w.WorldWord, w.World)
	row("model", w.Model)
	if len(w.Offices) > 0 {
		row("offices", strings.Join(w.Offices, " · "))
	}
	row("board", w.Board)
	row("session", w.Session)
	if w.Off > 0 {
		row("off", fmt.Sprintf("%d feature%s switched off by config (/status)", w.Off, plural(w.Off)))
	}
	for _, h := range w.Holes {
		for i, l := range strings.Split(wrap(h, inner-2), "\n") {
			p := "  "
			if i == 0 {
				p = t.warn.Render("? ")
			}
			rows = append(rows, p+ansi.Truncate(l, inner-2, "…"))
		}
	}
	rows = append(rows, "", t.dim.Render("/help for commands · ? for shortcuts · esc interrupts · ctrl+c twice exits"))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.border.GetForeground()).Padding(0, 1).Width(inner + 4) // lipgloss v2: the width includes padding and border
	return box.Render(strings.Join(rows, "\n"))
}

// User is the human's message as sent.
func (t Theme) User(text string, width int) string {
	text = strings.TrimSpace(text)
	lines := strings.Split(wrap(text, width-2), "\n")
	for i, l := range lines {
		p := "  "
		if i == 0 {
			p = "> "
		}
		lines[i] = t.user.Render(p + l)
	}
	return strings.Join(lines, "\n")
}

// Assistant is the model's answer, rendered as markdown.
func (t Theme) Assistant(md string, width int) string {
	return t.Markdown(md, width)
}

// ToolView is one tool step as the block shows it.
type ToolView struct {
	ID      string
	Name    string
	Summary string
	Class   string
	Status  string // running · done · failed · denied · refused · would-ask · would-run · pending
	Ms      int64
	Output  string
	Why     string // a denial or refusal reason
	Wrote   []string
	Diff    *Diff
	Expand  bool
	Body    string // the body running it, when not the session
}

// Tool renders a tool block: the header line, the status line, the output or the diff.
func (t Theme) Tool(v ToolView, width int) string {
	var b strings.Builder
	dot := t.dim.Render("●")
	switch v.Status {
	case "done":
		dot = t.ok.Render("●")
	case "failed", "denied", "refused":
		dot = t.bad.Render("●")
	case "would-ask", "would-run":
		dot = t.warn.Render("●")
	}
	tag := t.class(v.Class).Render(v.Class)
	head := dot + " " + t.text.Bold(true).Render(v.Name)
	if v.Body != "" {
		head += t.dim.Render(" (" + v.Body + ")")
	}
	room := width - ansi.StringWidth(head) - ansi.StringWidth(tag) - 4
	if s := oneLine(v.Summary); s != "" && room > 8 {
		head += "  " + ansi.Truncate(s, room, "…")
	}
	head += "  " + tag
	b.WriteString(head)
	b.WriteString("\n")
	// the status line
	st := ""
	switch v.Status {
	case "running", "pending":
		st = t.dim.Render("⎿ running…")
	case "done":
		parts := []string{t.ok.Render("ok"), elapsed(v.Ms)}
		if v.Diff != nil {
			parts = append(parts, v.Diff.Stat())
		} else if n := countLines(v.Output); n > 0 {
			parts = append(parts, fmt.Sprintf("%d line%s", n, plural(n)))
		}
		st = t.dim.Render("⎿ " + strings.Join(parts, " · "))
	case "failed":
		st = t.dim.Render("⎿ ") + t.bad.Render("failed") + t.dim.Render(" · "+elapsed(v.Ms))
	case "denied":
		st = t.dim.Render("⎿ ") + t.bad.Render("denied at the gate") + t.dim.Render(reason(v.Why))
	case "refused":
		st = t.dim.Render("⎿ ") + t.bad.Render("refused") + t.dim.Render(reason(v.Why))
	case "would-ask":
		st = t.dim.Render("⎿ ") + t.warn.Render("dry run: would ask") + t.dim.Render(reason(v.Why))
	case "would-run":
		st = t.dim.Render("⎿ ") + t.warn.Render("dry run: would run")
	default:
		st = t.dim.Render("⎿ " + v.Status)
	}
	b.WriteString(indent + ansi.Truncate(st, width-2, "…"))
	// the body: a diff, or the output collapsed
	if v.Diff != nil && v.Status == "done" {
		if d := t.diffLines(*v.Diff, width-4, v.Expand); d != "" {
			b.WriteString("\n" + d)
		}
		return b.String()
	}
	switch v.Status {
	case "running", "pending", "denied", "refused", "would-ask", "would-run":
		return b.String()
	}
	out := strings.TrimRight(v.Output, "\n")
	if out == "" || out == "(no output)" {
		return b.String()
	}
	lines := strings.Split(out, "\n")
	shown := lines
	hidden := 0
	if !v.Expand && len(lines) > collapseLines {
		shown = lines[:collapseLines]
		hidden = len(lines) - collapseLines
	}
	for _, l := range shown {
		b.WriteString("\n" + indent + indent + ansi.Truncate(expandTabs(l), width-4, "…"))
	}
	if hidden > 0 {
		b.WriteString("\n" + indent + indent + t.dim.Render(fmt.Sprintf("… +%d line%s (ctrl+o to expand)", hidden, plural(hidden))))
	}
	return b.String()
}

func (t Theme) diffLines(d Diff, width int, expand bool) string {
	if d.Truncated {
		return indent + indent + t.dim.Render("(too large to diff line by line)")
	}
	lines := d.Lines
	hidden := 0
	if !expand && len(lines) > collapseDiff {
		lines = lines[:collapseDiff]
		hidden = len(d.Lines) - collapseDiff
	}
	var out []string
	numw := 1
	for _, l := range d.Lines {
		if n := max(l.Old, l.New); len(itoa(n)) > numw {
			numw = len(itoa(n))
		}
	}
	for _, l := range lines {
		var s string
		switch l.Kind {
		case '~':
			s = t.dim.Render(strings.Repeat(" ", numw) + " ⋯")
		case '+':
			s = t.lineNo.Render(fmt.Sprintf("%*d", numw, l.New)) + " " + t.add.Render("+ "+ansi.Truncate(expandTabs(l.Text), width-numw-4, "…"))
		case '-':
			s = t.lineNo.Render(fmt.Sprintf("%*d", numw, l.Old)) + " " + t.del.Render("- "+ansi.Truncate(expandTabs(l.Text), width-numw-4, "…"))
		default:
			s = t.lineNo.Render(fmt.Sprintf("%*d", numw, l.New)) + " " + t.ctxLine.Render("  "+ansi.Truncate(expandTabs(l.Text), width-numw-4, "…"))
		}
		out = append(out, indent+indent+s)
	}
	if hidden > 0 {
		out = append(out, indent+indent+t.dim.Render(fmt.Sprintf("… +%d line%s (ctrl+o to expand)", hidden, plural(hidden))))
	}
	return strings.Join(out, "\n")
}

// CourtView is a dispatched Court: who, where, on what, its state, and its report when in.
type CourtView struct {
	Name     string
	Rank     string
	Office   string
	Model    string
	State    string // queued · thinking · tool · waiting on gate · done
	Elapsed  time.Duration
	Ask      string
	Report   *ReportView
	Failed   bool
	Word     string // "court" | "subagent"
	Expanded bool
	started  time.Time
}

// ReportView is a wire report rendered for the human.
type ReportView struct {
	Status   string
	Findings []string
	Verdicts []string
	Holes    []string
	Unsaid   []string // "kind: text"
	Other    []string
}

// Court renders a Court block.
func (t Theme) Court(v CourtView, width int) string {
	word := v.Word
	if word == "" {
		word = "court"
	}
	var b strings.Builder
	dot := t.rank(v.Rank).Render("◆")
	head := dot + " " + t.dim.Render(word) + " " + t.rank(v.Rank).Bold(true).Render(v.Name)
	meta := []string{}
	if v.Rank != "" {
		meta = append(meta, v.Rank)
	}
	if v.Office != "" {
		meta = append(meta, v.Office)
	}
	if v.Model != "" {
		meta = append(meta, v.Model)
	}
	if len(meta) > 0 {
		head += t.dim.Render(" · " + strings.Join(meta, " · "))
	}
	state := v.State
	if state == "" {
		state = "queued"
	}
	st := state
	if v.Elapsed > 0 {
		st += " · " + v.Elapsed.Round(time.Second).String()
	}
	stS := t.dim.Render(st)
	if v.State == "done" {
		if v.Failed {
			stS = t.bad.Render(st)
		} else {
			stS = t.ok.Render(st)
		}
	}
	gap := width - ansi.StringWidth(head) - ansi.StringWidth(stS) - 2
	if gap < 2 {
		b.WriteString(ansi.Truncate(head, width-ansi.StringWidth(stS)-3, "…") + "  " + stS)
	} else {
		b.WriteString(head + strings.Repeat(" ", gap) + stS)
	}
	if a := oneLine(v.Ask); a != "" {
		b.WriteString("\n" + indent + t.dim.Render("⎿ ") + ansi.Truncate(t.dim.Render(a), width-4, "…"))
	}
	if v.Report == nil {
		return b.String()
	}
	r := v.Report
	status := t.ok.Bold(true).Render(r.Status)
	if v.Failed || strings.HasPrefix(r.Status, "FAIL") || strings.HasPrefix(r.Status, "DENIED") || strings.HasPrefix(r.Status, "ESCALATE") {
		status = t.bad.Bold(true).Render(r.Status)
	}
	b.WriteString("\n" + indent + t.dim.Render("⎿ ") + status)
	lines := 0
	total := len(r.Findings) + len(r.Verdicts) + len(r.Holes) + len(r.Unsaid) + len(r.Other)
	limit := total
	if !v.Expanded && total > collapseLines {
		limit = collapseLines
	}
	add := func(mark lipgloss.Style, sym, text string) {
		if lines >= limit {
			lines++
			return
		}
		lines++
		b.WriteString("\n" + indent + indent + mark.Render(sym) + " " + ansi.Truncate(text, width-6, "…"))
	}
	for _, f := range r.Findings {
		add(t.ok, "●", f)
	}
	for _, v := range r.Verdicts {
		add(t.accent, "◆", v)
	}
	for _, h := range r.Holes {
		add(t.warn, "?", h)
	}
	for _, u := range r.Unsaid {
		add(t.dim, "∴", u)
	}
	for _, o := range r.Other {
		add(t.dim, "·", o)
	}
	if lines > limit {
		b.WriteString("\n" + indent + indent + t.dim.Render(fmt.Sprintf("… +%d line%s (ctrl+o to expand)", lines-limit, plural(lines-limit))))
	}
	return b.String()
}

// Gate is the one quiet line after a turn that wrote.
func (t Theme) Gate(verdict, logRel string, width int, word string) string {
	if word == "" {
		word = "gate"
	}
	mark := t.ok.Render("✓")
	if strings.HasPrefix(strings.ToLower(verdict), "fail") {
		mark = t.bad.Render("✗")
	}
	s := mark + " " + t.dim.Render(word+" · "+verdict)
	if logRel != "" {
		s += t.dim.Render(" · " + logRel)
	}
	return ansi.Truncate(s, width, "…")
}

// Holes are what the engine could not settle, one line each.
func (t Theme) Holes(holes []string, width int) string {
	var out []string
	for _, h := range holes {
		lines := strings.Split(wrap(h, width-2), "\n")
		for i, l := range lines {
			p := "  "
			if i == 0 {
				p = t.warn.Render("? ")
			}
			out = append(out, p+t.dim.Render(l))
		}
	}
	return strings.Join(out, "\n")
}

// Notice is one dim line (a court started, a line queued, config reloaded).
func (t Theme) Notice(text string, width int) string {
	return t.dim.Render(ansi.Truncate(text, width, "…"))
}

// Error is one line in the error colour.
func (t Theme) Error(text string, width int) string {
	return t.bad.Render(ansi.Truncate(text, width, "…"))
}

// ChoiceView is an inline choice: the human gate's approval, or a question the model asks.
type ChoiceView struct {
	Title    string // "Approval" | "Question"
	Class    string // the class tag for an approval ("" for a question)
	Tool     string
	Summary  string
	Why      string
	Body     string // the body that asks, when not the session
	Options  []string
	Notes    []string // one per option, shown under it ("" for none)
	Reasons  []string // one per option: a non-empty prompt asks for a line of text on pick
	Cursor   int
	Free     bool   // free text accepted (typed below)
	Typed    string // the free text so far
	Prompt   string // the free text prompt ("tell the model why")
	Typing   bool   // the human is typing the free text
	Answered bool   // printed after the answer: the picked option only, no hints
}

// Choice renders the choice block.
func (t Theme) Choice(v ChoiceView, width int) string {
	inner := min(width-4, 96)
	var rows []string
	title := t.accent.Bold(true).Render(v.Title)
	if v.Body != "" {
		title += t.dim.Render(" · " + v.Body)
	}
	rows = append(rows, title)
	if v.Tool != "" {
		head := ""
		if v.Class != "" {
			head = t.class(v.Class).Render(v.Class) + "  "
		}
		head += t.text.Bold(true).Render(v.Tool)
		if s := oneLine(v.Summary); s != "" {
			head += "  " + ansi.Truncate(s, inner-ansi.StringWidth(head)-2, "…")
		}
		rows = append(rows, head)
	}
	if v.Why != "" {
		rows = append(rows, t.dim.Render(wrap(v.Why, inner)))
	}
	rows = append(rows, "")
	for i, o := range v.Options {
		p := "  "
		s := t.text
		if v.Answered {
			rows = append(rows, t.ok.Render("✓ ")+ansi.Truncate(o, inner-3, "…"))
			continue
		}
		if i == v.Cursor && !v.Typing {
			p = t.accent.Render("❯ ")
			s = t.text.Bold(true)
		}
		rows = append(rows, p+s.Render(fmt.Sprintf("%d. %s", i+1, ansi.Truncate(o, inner-5, "…"))))
		if i < len(v.Notes) && v.Notes[i] != "" {
			rows = append(rows, "     "+t.dim.Render(ansi.Truncate(v.Notes[i], inner-5, "…")))
		}
	}
	if v.Free && !v.Answered {
		p := "  "
		s := t.dim
		if v.Cursor >= len(v.Options) && !v.Typing {
			p = t.accent.Render("❯ ")
			s = t.text.Bold(true)
		}
		rows = append(rows, p+s.Render("… or type an answer"))
	}
	switch {
	case v.Answered:
	case v.Typing:
		rows = append(rows, "", t.dim.Render(v.Prompt+" ")+v.Typed+t.accent.Render("▏"), t.dim.Render("enter sends · esc goes back"))
	default:
		rows = append(rows, "", t.dim.Render("↑↓ or a digit picks · enter confirms"))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.accent.GetForeground()).Padding(0, 1).Width(inner + 4)
	return box.Render(strings.Join(rows, "\n"))
}

// Spinner is the live line while a turn runs.
func (t Theme) Spinner(frame, verb string, elapsed time.Duration, tokens int, width int) string {
	parts := []string{elapsed.Round(time.Second).String()}
	if tokens > 0 {
		parts = append(parts, humanTokens(tokens)+" tokens")
	}
	parts = append(parts, "esc to interrupt")
	s := t.accent.Render(frame) + " " + verb + " " + t.dim.Render("("+strings.Join(parts, " · ")+")")
	return ansi.Truncate(s, width, "…")
}

// FooterView is what the footer says.
type FooterView struct {
	Model    string
	Ctx      string // "ctx 12%" or "ctx —"
	Cost     string // "$0.0123" | "no calls yet" | "unpriced"
	Live     int    // live courts
	World    string
	Courts   string // the plural word
	Hint     string // "? for shortcuts"
	Queued   int
	Message  string // a transient message shown in place of the hint
	Tokens   int    // the session's tokens so far (the spinner shows them)
	CtxPct   int    // the context window's fill in percent, drawn as a meter when CtxKnown
	CtxKnown bool
}

// Footer renders the line under the input.
func (t Theme) Footer(f FooterView, width int) string {
	parts := []string{f.Model, f.Ctx, f.Cost}
	parts = parts[:0:0]
	ctx := f.Ctx
	if f.CtxKnown && width >= 72 {
		ctx = "ctx " + t.Meter(f.CtxPct, 8) + fmt.Sprintf(" %d%%", f.CtxPct)
		if strings.Contains(f.Ctx, "stress") {
			ctx += " " + t.bad.Render("stress")
		}
	}
	parts = append(parts, f.Model, ctx, f.Cost)
	if f.Live > 0 {
		word := f.Courts
		if word == "" {
			word = "courts"
		}
		if f.Live == 1 {
			word = strings.TrimSuffix(word, "s")
		}
		parts = append(parts, fmt.Sprintf("%d live %s", f.Live, word))
	}
	if f.Queued > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", f.Queued))
	}
	right := f.Hint
	if f.Message != "" {
		right = f.Message
	}
	join := func(world string) string {
		ps := append([]string(nil), parts...)
		if world != "" {
			ps = append(ps, world)
		}
		return strings.Join(ps, " · ")
	}
	fits := func(left string) bool { return ansi.StringWidth(left)+ansi.StringWidth(right)+3 <= width }
	left := join(f.World)
	if !fits(left) && f.World != "" {
		// the world's last two path components, then none
		segs := strings.Split(strings.Trim(f.World, "/"), "/")
		if len(segs) > 2 {
			left = join("…/" + strings.Join(segs[len(segs)-2:], "/"))
		}
		if !fits(left) {
			left = join("")
		}
	}
	if right != "" && fits(left) {
		gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
		return t.dim.Render(left) + strings.Repeat(" ", gap) + t.dim.Render(right)
	}
	return t.dim.Render(ansi.Truncate(left, width, "…"))
}

// MenuItem is one slash command in the menu.
type MenuItem struct {
	Name        string // without the slash
	Description string
}

// Menu renders the slash menu under the input, filtered and with a cursor.
func (t Theme) Menu(items []MenuItem, cursor int, width int) string {
	if len(items) == 0 {
		return t.dim.Render("  no matching command")
	}
	namew := 0
	for _, it := range items {
		if l := len(it.Name) + 1; l > namew {
			namew = l
		}
	}
	var rows []string
	for i, it := range items {
		p := "  "
		name := t.text.Render(fmt.Sprintf("/%-*s", namew, it.Name))
		if i == cursor {
			p = t.accent.Render("❯ ")
			name = t.accent.Bold(true).Render(fmt.Sprintf("/%-*s", namew, it.Name))
		}
		rows = append(rows, ansi.Truncate(p+name+" "+t.dim.Render(it.Description), width, "…"))
	}
	return strings.Join(rows, "\n")
}

// Completions renders @path candidates in a row.
func (t Theme) Completions(paths []string, cursor int, width int) string {
	if len(paths) == 0 {
		return t.dim.Render("  no matching path")
	}
	var parts []string
	for i, p := range paths {
		if i == cursor {
			parts = append(parts, t.accent.Bold(true).Render(p))
		} else {
			parts = append(parts, t.dim.Render(p))
		}
	}
	return ansi.Truncate("  "+strings.Join(parts, "  "), width, "…")
}

// Shortcut is one key and what it does.
type Shortcut struct{ Key, Does string }

// Shortcuts renders the ? panel.
func (t Theme) Shortcuts(list []Shortcut, width int) string {
	var rows []string
	for _, s := range list {
		rows = append(rows, t.accent.Render(fmt.Sprintf("%-14s", s.Key))+t.dim.Render(s.Does))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.border.GetForeground()).Padding(0, 1)
	return box.Render(strings.Join(rows, "\n"))
}

// DefaultShortcuts is the list ? shows.
func DefaultShortcuts() []Shortcut {
	return []Shortcut{
		{"enter", "send"},
		{"\\ enter", "newline (also alt+enter)"},
		{"↑ ↓", "history (on the first / last line)"},
		{"/", "commands"},
		{"@path", "complete a file in the world (tab)"},
		{"esc", "interrupt the turn · clear the input"},
		{"ctrl+c ×2", "exit"},
		{"ctrl+o", "expand the last collapsed block"},
		{"ctrl+l", "redraw"},
		{"?", "this list"},
	}
}

// Queued is a line typed mid-turn, shown as queued.
func (t Theme) Queued(text string, width int) string {
	return t.dim.Render(ansi.Truncate("  ⏎ queued: "+oneLine(text), width, "…"))
}

// InputBox frames the textarea's view with the theme's border.
func (t Theme) InputBox(view string, width int, busy bool) string {
	col := t.border.GetForeground()
	if busy {
		col = t.dim.GetForeground()
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(col).Width(width).Render(view)
}

// helpers

func wrap(s string, width int) string {
	if width < 10 {
		width = 10
	}
	return ansi.Wrap(s, width, "")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func reason(why string) string {
	if why == "" {
		return ""
	}
	return " · " + oneLine(why)
}

func elapsed(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func countLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" || s == "(no output)" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

func humanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return itoa(n)
}
