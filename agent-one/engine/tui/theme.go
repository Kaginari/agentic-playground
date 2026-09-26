// Package tui is the live session's terminal UI (canon/tui.md): the conversation flows into the
// terminal's own scrollback as readable blocks, a bordered input sits at the bottom, and every
// event of the loop — a streamed answer, a tool step, a diff, a Subagent, the gate, an approval —
// is drawn, not printed raw. Built on the Charm libraries; the one place the binary takes
// third-party code.
//
// The package is layered by the ladder: blocks.go is the view model (pure functions from views
// to styled text), model.go the Bubble Tea program over it, and the app wires the loop's
// events into both.
package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Theme is the palette: the board's rank and lane tokens (board.css) carried to the terminal,
// a dark and a light variant, and nothing at all when the terminal has no colour (NO_COLOR,
// a pipe): lipgloss drops every escape on an ASCII profile, so a plain run is the same text.
type Theme struct {
	Dark  bool
	Color bool

	dim, text, accent, user, ok, bad, warn, border, tag lipgloss.Style
	classes                                             map[string]lipgloss.Style
	ranks                                               map[string]lipgloss.Style
	add, del, ctxLine, lineNo                           lipgloss.Style
}

// Words are the labels a distribution renames (agent-one says Subagent, Operator, Workspace).
type Words struct {
	Subagent  string // "subagent" | "subagent"
	Subagents string // plural
	Workspace string // "workspace" | "workspace"
	Human     string // "the operator" | "Operator"
	Gate      string // "gate"
	Dist      string // binary name
	ConfigRel string // where a don't-ask-again rule lands, workspace-relative
	// SessionName is the session agent's name ("orchestrator" | "orchestrator"): its events drive
	// the spinner, a Subagent's its block.
	SessionName string
}

// DefaultWords is the agent-one vocabulary.
func DefaultWords() Words {
	return Words{Subagent: "subagent", Subagents: "subagents", Workspace: "workspace", Human: "the operator", Gate: "gate", Dist: "agent-one", ConfigRel: ".agent-one/config.local.yaml", SessionName: "orchestrator"}
}

// NewTheme builds the palette for a dark or light background.
func NewTheme(dark bool) Theme {
	t := Theme{Dark: dark, Color: lipgloss.ColorProfile() != termenv.Ascii}
	c := func(dark, light string) lipgloss.Color {
		if t.Dark {
			return lipgloss.Color(dark)
		}
		return lipgloss.Color(light)
	}
	t.dim = lipgloss.NewStyle().Foreground(c("#7f8a99", "#5c6672"))
	t.text = lipgloss.NewStyle()
	t.accent = lipgloss.NewStyle().Foreground(c("#d4af37", "#8a6600")) // the global lane
	t.user = lipgloss.NewStyle().Foreground(c("#9fb0c3", "#4b5568"))
	t.ok = lipgloss.NewStyle().Foreground(c("#5fbf7a", "#1f7a3a"))
	t.bad = lipgloss.NewStyle().Foreground(c("#e06c5f", "#b8341f"))
	t.warn = lipgloss.NewStyle().Foreground(c("#e0b04f", "#8a6600"))
	t.border = lipgloss.NewStyle().Foreground(c("#4a5566", "#b3bcc8"))
	t.tag = lipgloss.NewStyle().Bold(true)
	t.add = lipgloss.NewStyle().Foreground(c("#5fbf7a", "#1f7a3a"))
	t.del = lipgloss.NewStyle().Foreground(c("#e06c5f", "#b8341f"))
	t.ctxLine = lipgloss.NewStyle().Foreground(c("#8a94a3", "#5c6672"))
	t.lineNo = lipgloss.NewStyle().Foreground(c("#5b6675", "#8a94a3"))
	t.classes = map[string]lipgloss.Style{
		"read":        lipgloss.NewStyle().Foreground(c("#4db8dd", "#1a7fa3")),
		"write":       lipgloss.NewStyle().Foreground(c("#9a86e8", "#5c3fc9")),
		"outward":     lipgloss.NewStyle().Foreground(c("#e0b04f", "#8a6600")).Bold(true),
		"destructive": lipgloss.NewStyle().Foreground(c("#e06c5f", "#b8341f")).Bold(true),
	}
	t.ranks = map[string]lipgloss.Style{
		"zone":                   lipgloss.NewStyle().Foreground(c("#2695bd", "#1a7fa3")),
		"domain":                 lipgloss.NewStyle().Foreground(c("#7c5cd6", "#5c3fc9")),
		"coord":                  lipgloss.NewStyle().Foreground(c("#bd8c24", "#8a6600")),
		"service":                lipgloss.NewStyle().Foreground(c("#c53d34", "#a12e26")),
		"auditor":                lipgloss.NewStyle().Foreground(c("#d6402a", "#b8341f")),
		"principal-domain-owner": lipgloss.NewStyle().Foreground(c("#8a7300", "#6b7400")),
		"principal-coordinator":  lipgloss.NewStyle().Foreground(c("#bd8c24", "#8a6600")),
		"orchestrator":           lipgloss.NewStyle().Foreground(c("#eaf2ff", "#0b1424")),
	}
	return t
}

// DetectTheme picks dark unless the environment says the background is light (COLORFGBG's
// background code, or <PREFIX>_THEME=light); the terminal is never queried, so a start is never
// held by an unanswered escape.
func DetectTheme(env func(string) string, prefix string) Theme {
	if env == nil {
		env = os.Getenv
	}
	if v := strings.ToLower(env(prefix + "THEME")); v == "light" {
		return NewTheme(false)
	} else if v == "dark" {
		return NewTheme(true)
	}
	if v := env("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		bg := parts[len(parts)-1]
		if bg == "7" || bg == "15" {
			return NewTheme(false)
		}
	}
	return NewTheme(true)
}

func (t Theme) class(name string) lipgloss.Style {
	if s, ok := t.classes[name]; ok {
		return s
	}
	return t.dim
}

func (t Theme) rank(name string) lipgloss.Style {
	if s, ok := t.ranks[strings.ToLower(name)]; ok {
		return s
	}
	return t.dim
}
