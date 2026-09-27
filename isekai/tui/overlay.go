package tui

import (
	"sort"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/x/ansi"
)

// Overlays drawn over the live area as Lip Gloss layers: the command palette (ctrl+k) and the
// toasts that slide in when a Court lands or the gate rules.

// --- the palette ---

type paletteState struct {
	query  string
	cursor int
	all    []MenuItem
}

type paletteHit struct {
	item  MenuItem
	score int
	idx   []int // matched rune positions in the name
}

// paletteActions are the UI's own entries beside the host's commands.
var paletteActions = []MenuItem{
	{Name: "board", Description: "the board, full screen"},
	{Name: "theme", Description: "switch between the dark and the light theme"},
	{Name: "help", Description: "commands and shortcuts"},
	{Name: "clear", Description: "clear the screen"},
	{Name: "quit", Description: "end the session"},
}

// noArgs are the commands the palette runs at once; the others land in the input to be finished.
var noArgs = map[string]bool{"board": true, "clear": true, "help": true, "quit": true, "agents": true, "usage": true,
	"status": true, "sessions": true, "compact": true, "theme": true}

func (m *Model) openPalette() {
	seen := map[string]bool{}
	var all []MenuItem
	for _, it := range append(append([]MenuItem(nil), paletteActions...), m.host.Commands()...) {
		if !seen[it.Name] {
			seen[it.Name] = true
			all = append(all, it)
		}
	}
	m.palette = &paletteState{all: all}
	m.menu, m.comp = nil, nil
}

// fuzzy scores name against q: every rune of q in order; consecutive and word-start matches score
// higher. ok false when q is not a subsequence.
func fuzzy(name, q string) (score int, idx []int, ok bool) {
	if q == "" {
		return 0, nil, true
	}
	n, qs := []rune(strings.ToLower(name)), []rune(strings.ToLower(q))
	j, last := 0, -2
	for i, r := range n {
		if j < len(qs) && r == qs[j] {
			idx = append(idx, i)
			score += 10
			if i == last+1 {
				score += 15
			}
			if i == 0 || !unicode.IsLetter(n[i-1]) {
				score += 20
			}
			last = i
			j++
		}
	}
	if j < len(qs) {
		return 0, nil, false
	}
	return score - len(n), idx, true
}

func (p *paletteState) hits() []paletteHit {
	var out []paletteHit
	for _, it := range p.all {
		s, idx, ok := fuzzy(it.Name, p.query)
		if !ok {
			continue
		}
		out = append(out, paletteHit{item: it, score: s, idx: idx})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

// paletteKey handles a key while the palette is open.
func (m *Model) paletteKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.palette
	hs := p.hits()
	switch k.String() {
	case "esc", "ctrl+k", "ctrl+c":
		m.palette = nil
	case "up", "ctrl+p":
		p.cursor = step(p.cursor, max(len(hs), 1), true)
	case "down", "ctrl+n":
		p.cursor = step(p.cursor, max(len(hs), 1), false)
	case "backspace":
		if r := []rune(p.query); len(r) > 0 {
			p.query, p.cursor = string(r[:len(r)-1]), 0
		}
	case "enter":
		m.palette = nil
		if len(hs) == 0 {
			return nil
		}
		name := hs[min(p.cursor, len(hs)-1)].item.Name
		if name == "theme" {
			m.setTheme(NewTheme(!m.theme.Dark))
			return m.reflow()
		}
		if noArgs[name] {
			m.setInput("/" + name)
			return m.submit()
		}
		m.setInput("/" + name + " ")
	default:
		if k.Text != "" {
			p.query, p.cursor = p.query+k.Text, 0
		}
	}
	return nil
}

// paletteView is the palette's box.
func (m *Model) paletteView() string {
	t, p := m.theme, m.palette
	w := min(64, m.width-4)
	hs := p.hits()
	lines := []string{t.accent.Render("❯ ") + p.query + t.accent.Render("▏")}
	if len(hs) == 0 {
		lines = append(lines, t.dim.Render("  nothing matches"))
	}
	if p.cursor >= len(hs) {
		p.cursor = max(0, len(hs)-1)
	}
	start := max(0, p.cursor-7)
	for i := start; i < len(hs) && i < start+8; i++ {
		h := hs[i]
		name := []rune(h.item.Name)
		var sb strings.Builder
		sb.WriteString("/")
		mi := 0
		for j, r := range name {
			if mi < len(h.idx) && h.idx[mi] == j {
				sb.WriteString(t.accent.Bold(true).Render(string(r)))
				mi++
			} else {
				sb.WriteString(string(r))
			}
		}
		nameStr := sb.String()
		desc := ansi.Truncate(h.item.Description, w-lipgloss.Width(nameStr)-8, "…")
		row := "  " + nameStr + "  " + t.dim.Render(desc)
		if i == p.cursor {
			row = t.accent.Render("› ") + nameStr + "  " + t.text.Render(desc)
		}
		lines = append(lines, row)
	}
	lines = append(lines, t.dim.Render("↑↓ choose · enter runs · esc closes"))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.accent.GetForeground()).Padding(0, 1).Width(w)
	return box.Render(strings.Join(lines, "\n"))
}

// --- the toasts ---

const (
	toastLife = 4 * time.Second
	toastFPS  = 60
)

type toast struct {
	text   string
	style  lipgloss.Style
	born   time.Time
	x, vel float64 // cells still to slide in from the right
	spring harmonica.Spring
}

type evToastFrame struct{}

func toastTick() tea.Cmd {
	return tea.Tick(time.Second/toastFPS, func(time.Time) tea.Msg { return evToastFrame{} })
}

// Toast shows a one-line notice at the live area's top right; it slides in and leaves after a
// few seconds. The block it echoes is already in the scrollback: a toast is only a glance.
func (m *Model) Toast(text string, st lipgloss.Style) tea.Cmd {
	anim := len(m.toasts) == 0 || m.toasts[len(m.toasts)-1].x < 0.5
	m.toasts = append(m.toasts, toast{text: text, style: st, born: time.Now(), x: float64(lipgloss.Width(text) + 6),
		spring: harmonica.NewSpring(harmonica.FPS(toastFPS), 9, 0.7)})
	if len(m.toasts) > 3 {
		m.toasts = m.toasts[len(m.toasts)-3:]
	}
	if anim {
		return toastTick()
	}
	return nil
}

// stepToasts slides the toasts in and drops the expired ones; true while one still moves.
func (m *Model) stepToasts() bool {
	moving := false
	kept := m.toasts[:0]
	for _, tt := range m.toasts {
		if time.Since(tt.born) > toastLife {
			continue
		}
		tt.x, tt.vel = tt.spring.Update(tt.x, tt.vel, 0)
		if tt.x > 0.3 || tt.vel > 0.3 || tt.vel < -0.3 {
			moving = true
		} else {
			tt.x, tt.vel = 0, 0
		}
		kept = append(kept, tt)
	}
	m.toasts = kept
	return moving
}

// overlay composes the live area with the palette and the toasts as layers.
func (m *Model) overlay(base string) string {
	if m.palette == nil && len(m.toasts) == 0 {
		return base
	}
	var layers []*lipgloss.Layer
	top := 0
	if len(m.toasts) > 0 {
		// a line per toast above the live area, the toasts right-aligned over them
		top = len(m.toasts)
		base = strings.Repeat("\n", top) + base
		for i, tt := range m.toasts {
			pill := tt.style.Padding(0, 1).Render(tt.text)
			x := m.width - lipgloss.Width(pill) - 1 + int(tt.x)
			if x >= m.width {
				continue
			}
			layers = append(layers, lipgloss.NewLayer(ansi.Truncate(pill, m.width-x, "")).X(x).Y(i).Z(2))
		}
	}
	if m.palette != nil {
		pv := m.paletteView()
		h := strings.Count(pv, "\n") + 1
		baseH := strings.Count(base, "\n") + 1
		// the input box and the footer are the live area's last four lines
		if need := h - (baseH - 4 - top); need > 0 {
			// room above the input box for the palette to float in
			base = strings.Repeat("\n", need) + base
			baseH += need
		}
		x := max(0, (m.width-lipgloss.Width(pv))/2)
		y := max(top, baseH-4-h) // it sits over the space above the input
		layers = append(layers, lipgloss.NewLayer(pv).X(x).Y(y).Z(3))
	}
	all := append([]*lipgloss.Layer{lipgloss.NewLayer(base)}, layers...)
	return lipgloss.NewCompositor(all...).Render()
}

// pill is a toast's style: dark text on the verdict's colour.
func (t Theme) pill(kind string) lipgloss.Style {
	bg := t.accent.GetForeground()
	switch kind {
	case "ok":
		bg = t.ok.GetForeground()
	case "bad":
		bg = t.bad.GetForeground()
	}
	return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#0d1117")).Bold(true)
}
