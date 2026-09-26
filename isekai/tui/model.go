package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Host is what the program asks of the app. Every call is made from the program's goroutine;
// a slow one (a slash command) runs as a tea.Cmd.
type Host interface {
	Welcome() Welcome
	Footer() FooterView
	// Submit starts a turn; a non-empty string refuses it (a userPrompt hook) and is shown.
	Submit(text string) string
	// Queue hands a line typed mid-turn to the running body; the string is the notice shown.
	Queue(text string) string
	// Interrupt cancels the running turn.
	Interrupt()
	// Slash runs a command the UI does not handle itself; lines are printed, quit ends the session.
	Slash(line string) (lines []string, quit bool)
	// Commands lists the slash commands for the menu (built-in and discovered).
	Commands() []MenuItem
	// Complete lists world paths for an @prefix.
	Complete(prefix string) []string
}

// Model is the Bubble Tea model of the session: the live area at the bottom (stream, running
// tools, courts, a choice, the spinner, the input, the menu, the footer); everything finished
// is printed above it with tea.Println and stays in the terminal's scrollback.
type Model struct {
	host  Host
	theme Theme
	words Words

	width, height int
	ready         bool

	// the turn
	busy      bool
	turnStart time.Time
	verb      string
	stream    strings.Builder
	streamed  bool
	tools     []*ToolView // running tools, in order
	courts    map[string]*CourtView
	courtSeq  []string
	lastBlock *collapsed // the last collapsed block, for ctrl+o

	// the choice
	choice *EvChoice
	view   ChoiceView

	// the input
	input     textarea.Model
	history   []string
	histIdx   int
	draft     string
	pasted    map[string]string // placeholder → text
	menu      *menuState
	comp      *compState
	shortcuts bool
	queued    int
	lastCtrlC time.Time
	message   string
	msgUntil  time.Time

	spin   spinner.Model
	footer FooterView
	quit   bool
	err    error

	// prints is the FIFO of finished blocks; one goroutine hands them to the program in order
	// (a tea.Println per Update would race the next Update's).
	prints   chan string
	sender   func(tea.Msg)
	attached chan struct{}
}

type collapsed struct {
	tool  *ToolView
	court *CourtView
}

type menuState struct {
	items  []MenuItem
	cursor int
}

type compState struct {
	prefix string // the @token being completed, without the @
	items  []string
	cursor int
}

// New builds the model.
func New(host Host, theme Theme, words Words) *Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetPromptFunc(2, func(i int) string {
		if i == 0 {
			return "> "
		}
		return "  "
	})
	ta.Placeholder = "ask, or / for commands"
	ta.CharLimit = 0
	ta.MaxHeight = 8
	ta.SetHeight(1)
	ta.KeyMap.InsertNewline.SetEnabled(false)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = theme.dim
	ta.FocusedStyle.Placeholder = theme.dim
	ta.BlurredStyle = ta.FocusedStyle
	ta.Focus()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.accent))
	m := &Model{host: host, theme: theme, words: words, input: ta, spin: sp, courts: map[string]*CourtView{}, pasted: map[string]string{}, width: 80, height: 24,
		prints: make(chan string, 4096), attached: make(chan struct{})}
	m.footer = host.Footer()
	m.histIdx = -1
	return m
}

// Attach gives the model the program's Send, which the printer needs; Start and the test
// harness call it once the program exists.
func (m *Model) Attach(send func(tea.Msg)) {
	m.sender = send
	close(m.attached)
}

// Init prints the welcome and starts the ticks and the printer.
func (m *Model) Init() tea.Cmd {
	w := m.host.Welcome()
	m.print(m.theme.Welcome(w, m.width))
	return tea.Batch(textarea.Blink, tick(), m.printer)
}

// printer is a long-lived Cmd: it takes blocks off the FIFO and prints each above the live
// area, in order. It ends with the program.
func (m *Model) printer() tea.Msg {
	<-m.attached
	for s := range m.prints {
		m.sender(tea.Printf("%s\n", s)())
	}
	return nil
}

func tick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return evTick{at: t} })
}

// Update is the event loop.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.width < 20 {
			m.width = 20
		}
		m.input.SetWidth(m.width - 4)
		m.ready = true
		return m, nil
	case evTick:
		if m.busy || m.choice != nil {
			m.footer = m.host.Footer()
		}
		if m.message != "" && time.Now().After(m.msgUntil) {
			m.message = ""
		}
		return m, tick()
	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		return m.key(msg)
	case EvDelta:
		m.stream.WriteString(msg.Text)
		m.streamed = true
		return m, m.flushStream(false)
	case EvState:
		if msg.Body == "" || m.isSession(msg.Body) {
			if msg.State == "tool" && msg.Tool == "" && strings.HasPrefix(m.verb, "Running ") {
				return m, nil
			}
			if msg.State != "done" {
				m.verb = verbFor(msg.State, msg.Tool)
			}
			return m, nil
		}
		if c := m.courts[msg.Body]; c != nil && c.State != "done" {
			c.State = msg.State
		}
		return m, nil
	case EvToolStart:
		cmd := m.flushStream(true)
		t := msg.Tool
		t.Status = "running"
		m.tools = append(m.tools, &t)
		m.verb = verbFor("tool", t.Name)
		return m, cmd
	case EvToolEnd:
		t := msg.Tool
		for i, r := range m.tools {
			if r.ID == t.ID {
				m.tools = append(m.tools[:i], m.tools[i+1:]...)
				break
			}
		}
		m.verb = "Thinking…"
		if m.collapses(t) {
			m.lastBlock = &collapsed{tool: &t}
		}
		return m, m.print(m.theme.Tool(t, m.width))
	case EvCourt:
		c := msg.Court
		if c.Word == "" {
			c.Word = m.words.Court
		}
		cur := m.courts[c.Name]
		if cur == nil {
			m.courtSeq = append(m.courtSeq, c.Name)
			cur = &CourtView{}
			m.courts[c.Name] = cur
			// the model's text before the dispatch stays above the block
			cmd := m.flushStream(true)
			*cur = c
			if c.Report == nil && c.State != "done" {
				return m, tea.Sequence(cmd, m.print(m.theme.Notice("→ "+c.Word+" "+c.Name+" started", m.width)))
			}
			return m, tea.Sequence(cmd, m.landCourt(cur))
		}
		if c.Rank != "" {
			cur.Rank, cur.Office, cur.Model = c.Rank, c.Office, c.Model
		}
		if c.Ask != "" {
			cur.Ask = c.Ask
		}
		cur.State, cur.Elapsed, cur.Failed = c.State, c.Elapsed, c.Failed
		if c.Report != nil {
			cur.Report = c.Report
		}
		if cur.State == "done" && cur.Report != nil {
			return m, m.landCourt(cur)
		}
		return m, nil
	case EvTurnStart:
		m.begin()
		if msg.Auto {
			return m, m.print(m.theme.Notice("↻ continuing with what arrived: "+oneLine(msg.Text), m.width))
		}
		return m, m.print(m.theme.User(msg.Text, m.width))
	case EvTurnDone:
		return m, m.finish(msg)
	case EvNotice:
		return m, m.print(m.theme.Notice(msg.Text, m.width))
	case EvError:
		return m, m.print(m.theme.Error(msg.Text, m.width))
	case EvLines:
		return m, m.print(strings.Join(msg.Lines, "\n"))
	case EvChoice:
		m.choice = &msg
		m.view = msg.View
		m.verb = "Waiting for you…"
		return m, m.flushStream(true)
	case evSlashDone:
		var cmds []tea.Cmd
		if len(msg.lines) > 0 {
			cmds = append(cmds, m.print(strings.Join(msg.lines, "\n")))
		}
		if msg.quit {
			m.quit = true
			cmds = append(cmds, tea.Quit)
		}
		return m, tea.Sequence(cmds...)
	case EvQuit:
		m.quit = true
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) isSession(body string) bool {
	return body == "" || body == m.words.Session()
}

// Session is the session body's name (the one that is not a Court).
func (w Words) Session() string {
	if w.SessionName != "" {
		return w.SessionName
	}
	return "rimuru"
}

func verbFor(state, tool string) string {
	switch state {
	case "thinking":
		return "Thinking…"
	case "tool":
		if tool != "" {
			return "Running " + tool + "…"
		}
		return "Running a tool…"
	case "waiting on gate":
		return "Waiting for approval…"
	case "done":
		return "Finishing…"
	}
	return "Working…"
}

func (m *Model) begin() {
	m.busy = true
	m.turnStart = time.Now()
	m.verb = "Thinking…"
	m.stream.Reset()
	m.streamed = false
	m.tools = nil
	m.queued = 0
}

func (m *Model) finish(ev EvTurnDone) tea.Cmd {
	var cmds []tea.Cmd
	if ev.Streamed || m.streamed {
		cmds = append(cmds, m.flushStream(true))
	} else if t := strings.TrimSpace(ev.Text); t != "" {
		cmds = append(cmds, m.print(m.theme.Assistant(t, m.width)))
	}
	for _, t := range m.tools {
		t.Status = "failed"
		t.Why = "interrupted"
		cmds = append(cmds, m.print(m.theme.Tool(*t, m.width)))
	}
	m.tools = nil
	if ev.Interrupted {
		cmds = append(cmds, m.print(m.theme.Notice("■ interrupted", m.width)))
	} else if len(ev.Holes) > 0 {
		cmds = append(cmds, m.print(m.theme.Holes(ev.Holes, m.width)))
	}
	if ev.Verdict != "" {
		cmds = append(cmds, m.print(m.theme.Gate(ev.Verdict, ev.LogRel, m.width, m.words.Gate)))
	}
	if ev.Hint != "" {
		cmds = append(cmds, m.print(m.theme.Notice(ev.Hint, m.width)))
	}
	m.busy = false
	m.verb = ""
	m.footer = m.host.Footer()
	return tea.Sequence(cmds...)
}

// print queues a finished block for the scrollback, followed by a blank line. It returns nil
// so call sites read the same whether or not they chain a Cmd after it.
func (m *Model) print(block string) tea.Cmd {
	if block == "" {
		return nil
	}
	select {
	case m.prints <- block:
	default:
		// a full FIFO (the program is gone or stuck): the block is dropped rather than the loop
	}
	return nil
}

func (m *Model) collapses(t ToolView) bool {
	if t.Diff != nil {
		return len(t.Diff.Lines) > collapseDiff
	}
	return countLines(t.Output) > collapseLines
}

// landCourt prints a Court's finished block and drops it from the live area.
func (m *Model) landCourt(c *CourtView) tea.Cmd {
	delete(m.courts, c.Name)
	for i, n := range m.courtSeq {
		if n == c.Name {
			m.courtSeq = append(m.courtSeq[:i], m.courtSeq[i+1:]...)
			break
		}
	}
	if c.Report != nil && len(c.Report.Findings)+len(c.Report.Holes)+len(c.Report.Unsaid)+len(c.Report.Verdicts)+len(c.Report.Other) > collapseLines {
		cp := *c
		m.lastBlock = &collapsed{court: &cp}
	}
	return m.print(m.theme.Court(*c, m.width))
}

// flushStream moves complete paragraphs of the streamed answer into the scrollback (all of it
// when final), keeping only the paragraph still being written live. A code fence is never
// split.
func (m *Model) flushStream(final bool) tea.Cmd {
	s := m.stream.String()
	if strings.TrimSpace(s) == "" {
		if final {
			m.stream.Reset()
		}
		return nil
	}
	if final {
		m.stream.Reset()
		return m.print(m.theme.Assistant(s, m.width))
	}
	cut := safeCut(s)
	if cut <= 0 {
		return nil
	}
	head, tail := s[:cut], s[cut:]
	m.stream.Reset()
	m.stream.WriteString(tail)
	return m.print(m.theme.Assistant(head, m.width))
}

// safeCut finds the last blank line outside a code fence such that the paragraph after it is
// not a continuation of a list or an indented block; 0 when there is none.
func safeCut(s string) int {
	lines := strings.Split(s, "\n")
	fence := false
	best := 0
	off := 0
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if !fence && l == "" && i > 0 && i+1 < len(lines) {
			next := lines[i+1]
			if next == "" || isListLine(next) || strings.HasPrefix(next, "  ") {
				off += len(l) + 1
				continue
			}
			// the previous paragraph must not be a list that the next line continues
			best = off + 1
		}
		off += len(l) + 1
	}
	return best
}

func isListLine(l string) bool {
	t := strings.TrimLeft(l, " ")
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ") {
		return true
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' '
}

// View draws the live area.
func (m *Model) View() string {
	if m.quit {
		return ""
	}
	var parts []string
	if s := m.stream.String(); m.busy && strings.TrimSpace(s) != "" {
		parts = append(parts, m.theme.Assistant(s, m.width), "")
	}
	for _, t := range m.tools {
		parts = append(parts, m.theme.Tool(*t, m.width), "")
	}
	for _, n := range m.courtSeq {
		if c := m.courts[n]; c != nil {
			c.Elapsed = time.Since(m.courtStart(c))
			parts = append(parts, m.theme.Court(*c, m.width), "")
		}
	}
	if m.choice != nil {
		parts = append(parts, m.theme.Choice(m.view, m.width), "")
	} else if m.busy {
		tokens := m.footer.Tokens
		parts = append(parts, m.theme.Spinner(m.spin.View(), m.verb, time.Since(m.turnStart), tokens, m.width))
	}
	if m.shortcuts {
		parts = append(parts, m.theme.Shortcuts(DefaultShortcuts(), m.width))
	}
	parts = append(parts, m.theme.InputBox(m.input.View(), m.width, m.choice != nil))
	switch {
	case m.menu != nil:
		parts = append(parts, m.theme.Menu(m.menu.items, m.menu.cursor, m.width))
	case m.comp != nil:
		parts = append(parts, m.theme.Completions(m.comp.items, m.comp.cursor, m.width))
	}
	f := m.footer
	f.Queued = m.queued
	f.Hint = "? for shortcuts"
	f.Message = m.message
	if f.Courts == "" {
		f.Courts = m.words.Courts
	}
	parts = append(parts, m.theme.Footer(f, m.width))
	return strings.Join(parts, "\n")
}

func (m *Model) courtStart(c *CourtView) time.Time {
	if c.started.IsZero() {
		c.started = time.Now()
	}
	return c.started
}

// say shows a transient message in the footer.
func (m *Model) say(s string) {
	m.message = s
	m.msgUntil = time.Now().Add(4 * time.Second)
}

// answer replies to the pending choice.
func (m *Model) answer(a ChoiceAnswer) tea.Cmd {
	if m.choice == nil {
		return nil
	}
	ch := m.choice
	m.choice = nil
	block := m.theme.Choice(chosen(m.view, a), m.width)
	select {
	case ch.Reply <- a:
	default:
	}
	m.verb = "Thinking…"
	return m.print(block)
}

// chosen is the choice as printed once answered: only the picked option, no cursor hints.
func chosen(v ChoiceView, a ChoiceAnswer) ChoiceView {
	out := v
	out.Typing = false
	switch {
	case a.Aborted:
		out.Options = []string{"(no answer — the session ended)"}
	case a.Index >= 0 && a.Index < len(v.Options):
		out.Options = []string{v.Options[a.Index]}
		if a.Text != "" {
			out.Options = []string{v.Options[a.Index] + ": " + a.Text}
		}
	default:
		out.Options = []string{a.Text}
	}
	out.Notes = nil
	out.Cursor = 0
	out.Answered = true
	return out
}

// Abort answers a pending choice when the program goes away.
func (m *Model) Abort() {
	if m.choice != nil {
		ch := m.choice
		m.choice = nil
		select {
		case ch.Reply <- ChoiceAnswer{Index: -1, Aborted: true}:
		default:
		}
	}
}

func (m *Model) statusText() string {
	return fmt.Sprintf("busy=%v tools=%d courts=%d", m.busy, len(m.tools), len(m.courts))
}
