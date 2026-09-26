package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	historyMax   = 200
	pasteCollaps = 3 // a paste of this many lines or more is collapsed to a placeholder
	menuMax      = 8
	compMax      = 8
)

// key handles a keypress: the choice first, then the menus, then the session keys, then the
// textarea.
func (m *Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.choice != nil {
		return m, m.choiceKey(k)
	}
	if m.shortcuts && k.Type != tea.KeyCtrlC {
		m.shortcuts = false
		if k.String() == "?" {
			return m, nil
		}
	}
	switch k.Type {
	case tea.KeyCtrlC:
		now := time.Now()
		if now.Sub(m.lastCtrlC) < 2*time.Second {
			m.quit = true
			return m, tea.Quit
		}
		m.lastCtrlC = now
		if m.busy {
			m.host.Interrupt()
			m.say("interrupting · ctrl+c again to exit")
			return m, nil
		}
		m.say("ctrl+c again to exit")
		return m, nil
	case tea.KeyCtrlL:
		return m, tea.ClearScreen
	case tea.KeyCtrlO:
		return m, m.expandLast()
	case tea.KeyEsc:
		switch {
		case m.menu != nil || m.comp != nil:
			m.menu, m.comp = nil, nil
		case m.busy:
			m.host.Interrupt()
			m.say("interrupting…")
		default:
			m.input.Reset()
			m.input.SetHeight(1)
		}
		return m, nil
	case tea.KeyUp, tea.KeyDown:
		up := k.Type == tea.KeyUp
		switch {
		case m.menu != nil:
			m.menu.cursor = step(m.menu.cursor, len(m.menu.items), up)
			return m, nil
		case m.comp != nil:
			m.comp.cursor = step(m.comp.cursor, len(m.comp.items), up)
			return m, nil
		case up && m.input.Line() == 0:
			return m, m.historyMove(-1)
		case !up && m.input.Line() == m.input.LineCount()-1:
			return m, m.historyMove(1)
		}
	case tea.KeyTab:
		switch {
		case m.menu != nil && len(m.menu.items) > 0:
			m.setInput("/" + m.menu.items[m.menu.cursor].Name + " ")
			m.menu = nil
			return m, nil
		case m.comp != nil && len(m.comp.items) > 0:
			m.acceptCompletion()
			return m, nil
		default:
			if m.openCompletion() {
				return m, nil
			}
		}
		return m, nil
	case tea.KeyEnter:
		if k.Alt {
			m.input.InsertString("\n")
			m.grow()
			return m, nil
		}
		switch {
		case m.menu != nil && len(m.menu.items) > 0:
			line := m.input.Value()
			_, args, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
			name := m.menu_name(line)
			m.menu = nil
			m.setInput(strings.TrimSpace("/" + name + " " + args))
			return m, m.submit()
		case m.comp != nil && len(m.comp.items) > 0:
			m.acceptCompletion()
			return m, nil
		}
		if v := m.input.Value(); strings.HasSuffix(v, "\\") {
			m.setInput(strings.TrimSuffix(v, "\\") + "\n")
			m.grow()
			return m, nil
		}
		return m, m.submit()
	case tea.KeyCtrlJ:
		m.input.InsertString("\n")
		m.grow()
		return m, nil
	case tea.KeyRunes:
		if k.Paste {
			return m, m.paste(string(k.Runes))
		}
		if string(k.Runes) == "?" && strings.TrimSpace(m.input.Value()) == "" {
			m.shortcuts = true
			return m, nil
		}
		if len(k.Runes) == 1 && k.Runes[0] >= '1' && k.Runes[0] <= '9' && m.menu != nil {
			// a digit does nothing special in the menu; fall through to typing
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	m.grow()
	m.refreshMenus()
	return m, cmd
}

func (m *Model) menu_name(line string) string {
	if m.menu != nil && len(m.menu.items) > 0 {
		return m.menu.items[m.menu.cursor].Name
	}
	cmd, _, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	return cmd
}

func step(cur, n int, up bool) int {
	if n == 0 {
		return 0
	}
	if up {
		return (cur - 1 + n) % n
	}
	return (cur + 1) % n
}

// grow sizes the textarea to its content, up to its maximum.
func (m *Model) grow() {
	h := m.input.LineCount()
	if h < 1 {
		h = 1
	}
	if h > m.input.MaxHeight {
		h = m.input.MaxHeight
	}
	m.input.SetHeight(h)
}

func (m *Model) setInput(s string) {
	m.input.SetValue(s)
	m.input.CursorEnd()
	m.grow()
}

// refreshMenus opens, filters or closes the slash menu and the @completion after a keystroke.
func (m *Model) refreshMenus() {
	v := m.input.Value()
	// the slash menu: the input is one line starting with "/" and no space yet
	if strings.HasPrefix(v, "/") && !strings.ContainsAny(v, " \n") {
		items := filterMenu(m.host.Commands(), strings.TrimPrefix(v, "/"))
		if m.menu == nil {
			m.menu = &menuState{}
		}
		if m.menu.cursor >= len(items) {
			m.menu.cursor = 0
		}
		m.menu.items = items
		m.comp = nil
		return
	}
	m.menu = nil
	// the @completion follows the token under the cursor while it is being typed
	if m.comp != nil {
		tok := currentToken(v)
		if !strings.HasPrefix(tok, "@") {
			m.comp = nil
			return
		}
		m.comp.prefix = tok[1:]
		m.comp.items = limit(m.host.Complete(m.comp.prefix), compMax)
		if m.comp.cursor >= len(m.comp.items) {
			m.comp.cursor = 0
		}
	}
}

func filterMenu(all []MenuItem, q string) []MenuItem {
	q = strings.ToLower(q)
	var pre, sub []MenuItem
	for _, it := range all {
		n := strings.ToLower(it.Name)
		switch {
		case q == "" || strings.HasPrefix(n, q):
			pre = append(pre, it)
		case strings.Contains(n, q) || strings.Contains(strings.ToLower(it.Description), q):
			sub = append(sub, it)
		}
	}
	return limitItems(append(pre, sub...), menuMax)
}

func limit(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func limitItems(xs []MenuItem, n int) []MenuItem {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

// currentToken is the whitespace-delimited token the input ends with.
func currentToken(v string) string {
	i := strings.LastIndexAny(v, " \n\t")
	return v[i+1:]
}

// openCompletion opens @path completion for the token being typed; false when there is none.
func (m *Model) openCompletion() bool {
	tok := currentToken(m.input.Value())
	if !strings.HasPrefix(tok, "@") {
		return false
	}
	m.comp = &compState{prefix: tok[1:]}
	m.comp.items = limit(m.host.Complete(m.comp.prefix), compMax)
	return true
}

func (m *Model) acceptCompletion() {
	if m.comp == nil || len(m.comp.items) == 0 {
		return
	}
	v := m.input.Value()
	tok := currentToken(v)
	pick := m.comp.items[m.comp.cursor]
	m.setInput(v[:len(v)-len(tok)] + "@" + pick + " ")
	m.comp = nil
}

// paste collapses a multi-line paste to a placeholder, restored on submit.
func (m *Model) paste(text string) tea.Cmd {
	n := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
	if n < pasteCollaps {
		m.input.InsertString(text)
		m.grow()
		m.refreshMenus()
		return nil
	}
	ph := fmt.Sprintf("[pasted %d lines #%d]", n, len(m.pasted)+1)
	m.pasted[ph] = text
	m.input.InsertString(ph)
	m.grow()
	return nil
}

func (m *Model) expandPastes(s string) string {
	for ph, text := range m.pasted {
		s = strings.ReplaceAll(s, ph, text)
	}
	return s
}

// submit sends the input: a slash command to the host or the UI, a line to the running turn
// (queued), or a new turn.
func (m *Model) submit() tea.Cmd {
	raw := m.input.Value()
	text := strings.TrimSpace(m.expandPastes(raw))
	if text == "" {
		return nil
	}
	m.input.Reset()
	m.input.SetHeight(1)
	m.menu, m.comp = nil, nil
	m.pasted = map[string]string{}
	m.remember(text)
	if strings.HasPrefix(text, "/") {
		return m.slash(text)
	}
	if m.busy {
		notice := m.host.Queue(text)
		m.queued++
		return m.print(m.theme.Queued(text, m.width) + "\n" + m.theme.Notice("  "+notice, m.width))
	}
	if why := m.host.Submit(text); why != "" {
		return m.print(m.theme.Error(why, m.width))
	}
	m.begin()
	return tea.Sequence(m.print(m.theme.User(text, m.width)), m.spin.Tick)
}

// slash runs a command: the UI's own (help, quit) here, the rest through the host as a Cmd.
func (m *Model) slash(line string) tea.Cmd {
	cmd, rest, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	switch cmd {
	case "help", "?":
		items := m.host.Commands()
		sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		var out []string
		out = append(out, m.theme.Menu(items, -1, m.width))
		return m.print(strings.Join(out, "\n") + "\n" + m.theme.Shortcuts(DefaultShortcuts(), m.width))
	case "quit", "exit", "q":
		m.quit = true
		return tea.Quit
	case "clear":
		return tea.ClearScreen
	}
	_ = rest
	echo := m.print(m.theme.User(line, m.width))
	host := m.host
	return tea.Sequence(echo, func() tea.Msg {
		lines, quit := host.Slash(line)
		return evSlashDone{lines: lines, quit: quit}
	})
}

// history

func (m *Model) remember(text string) {
	if n := len(m.history); n > 0 && m.history[n-1] == text {
		m.histIdx = -1
		return
	}
	m.history = append(m.history, text)
	if len(m.history) > historyMax {
		m.history = m.history[len(m.history)-historyMax:]
	}
	m.histIdx = -1
}

func (m *Model) historyMove(delta int) tea.Cmd {
	if len(m.history) == 0 {
		return nil
	}
	if m.histIdx == -1 {
		if delta > 0 {
			return nil
		}
		m.draft = m.input.Value()
		m.histIdx = len(m.history)
	}
	next := m.histIdx + delta
	switch {
	case next < 0:
		return nil
	case next >= len(m.history):
		m.histIdx = -1
		m.setInput(m.draft)
		return nil
	}
	m.histIdx = next
	m.setInput(m.history[next])
	return nil
}

// expandLast re-prints the last collapsed block in full.
func (m *Model) expandLast() tea.Cmd {
	if m.lastBlock == nil {
		m.say("nothing collapsed to expand")
		return nil
	}
	b := m.lastBlock
	m.lastBlock = nil
	switch {
	case b.tool != nil:
		t := *b.tool
		t.Expand = true
		return m.print(m.theme.Tool(t, m.width))
	case b.court != nil:
		c := *b.court
		c.Expanded = true
		return m.print(m.theme.Court(c, m.width))
	}
	return nil
}

// the choice block's keys

func (m *Model) choiceKey(k tea.KeyMsg) tea.Cmd {
	v := &m.view
	if v.Typing {
		switch k.Type {
		case tea.KeyEsc:
			v.Typing = false
			v.Typed = ""
			return nil
		case tea.KeyEnter:
			text := strings.TrimSpace(v.Typed)
			if text == "" && !v.Free {
				return nil
			}
			if v.Free && v.Cursor >= len(v.Options) {
				return m.answer(ChoiceAnswer{Index: -1, Text: text})
			}
			return m.answer(ChoiceAnswer{Index: v.Cursor, Text: text})
		case tea.KeyBackspace:
			if len(v.Typed) > 0 {
				r := []rune(v.Typed)
				v.Typed = string(r[:len(r)-1])
			}
			return nil
		case tea.KeyRunes:
			v.Typed += string(k.Runes)
			return nil
		case tea.KeySpace:
			v.Typed += " "
			return nil
		case tea.KeyCtrlC:
			return m.answer(ChoiceAnswer{Index: -1, Aborted: true})
		}
		return nil
	}
	n := len(v.Options)
	extra := 0
	if v.Free {
		extra = 1 // the last row is "type an answer"
	}
	switch k.Type {
	case tea.KeyUp:
		v.Cursor = step(v.Cursor, n+extra, true)
	case tea.KeyDown:
		v.Cursor = step(v.Cursor, n+extra, false)
	case tea.KeyEnter:
		if v.Cursor >= n {
			v.Typing, v.Prompt = true, "your answer:"
			return nil
		}
		if v.Reasons != nil && v.Reasons[v.Cursor] != "" {
			v.Typing, v.Prompt = true, v.Reasons[v.Cursor]
			return nil
		}
		return m.answer(ChoiceAnswer{Index: v.Cursor})
	case tea.KeyEsc, tea.KeyCtrlC:
		// esc on an approval is a plain no: the act does not run
		if v.Title != "Question" {
			return m.answer(ChoiceAnswer{Index: -1, Text: ""})
		}
	case tea.KeyRunes:
		if len(k.Runes) == 1 && k.Runes[0] >= '1' && k.Runes[0] <= '9' {
			i := int(k.Runes[0] - '1')
			if i < n {
				v.Cursor = i
				if v.Reasons != nil && v.Reasons[i] != "" {
					v.Typing, v.Prompt = true, v.Reasons[i]
					return nil
				}
				return m.answer(ChoiceAnswer{Index: i})
			}
		}
		if v.Free {
			v.Typing, v.Prompt, v.Typed = true, "your answer:", string(k.Runes)
			v.Cursor = n
		}
	}
	return nil
}
