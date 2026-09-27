package tui

import (
	"image/color"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/x/ansi"
)

// Motion and colour: the intro (the mascot drops in on a spring, lands where the welcome box
// holds it, blinks), gradients along a line of text, the shimmer that sweeps the spinner's verb.

const (
	introFPS  = 60
	introDrop = 6 // pixels the mascot falls (3 rows)
	introMax  = 1400 * time.Millisecond
	introHold = 380 * time.Millisecond // after landing: settle, blink, then the welcome
)

type introState struct {
	spring  harmonica.Spring
	pos     float64 // pixels above its resting place
	vel     float64
	started time.Time
	landed  time.Time
}

type evIntro struct{}

func introTick() tea.Cmd {
	return tea.Tick(time.Second/introFPS, func(time.Time) tea.Msg { return evIntro{} })
}

// SetIntro turns the startup animation on; Start does it for a real terminal.
func (m *Model) SetIntro(on bool) {
	if !on {
		m.intro = nil
		return
	}
	m.intro = &introState{spring: harmonica.NewSpring(harmonica.FPS(introFPS), 7.5, 0.28), pos: introDrop}
}

// stepIntro advances the spring; false when the intro is over.
func (m *Model) stepIntro() bool {
	in := m.intro
	now := time.Now()
	if in.started.IsZero() {
		in.started = now
	}
	in.pos, in.vel = in.spring.Update(in.pos, in.vel, 0)
	if in.landed.IsZero() && math.Abs(in.pos) < 0.3 && math.Abs(in.vel) < 0.8 {
		in.landed = now
	}
	done := (!in.landed.IsZero() && now.Sub(in.landed) > introHold) || now.Sub(in.started) > introMax
	return !done
}

// introView draws the mascot at its spring position, two pixels per row, where the welcome box
// will hold it (the box's border and padding: two columns in).
func (m *Model) introView() string {
	in := m.intro
	off := int(math.Round(introDrop - in.pos))
	if off < 0 {
		off = 0
	}
	sp := mascotFor(m.words.Dist)
	pixels := sp.pixels
	if off > introDrop {
		// past its resting place the mascot squashes instead of sinking: a row shorter, the base
		// a pixel wider each side
		off = introDrop + 1
		pixels = squash(pixels)
	}
	px := make([]string, 0, introDrop+len(sp.pixels)+2)
	blank := strings.Repeat(".", len(pixels[0]))
	for i := 0; i < off; i++ {
		px = append(px, blank)
	}
	blink := !in.landed.IsZero() && time.Since(in.landed) > 150*time.Millisecond && time.Since(in.landed) < 260*time.Millisecond
	for _, row := range pixels {
		if blink {
			row = strings.Map(func(r rune) rune {
				if r == 'E' {
					return rune(sp.body)
				}
				return r
			}, row)
		}
		px = append(px, row)
	}
	for len(px) < introDrop+len(sp.pixels) {
		px = append(px, blank)
	}
	rows := halfBlocks(px, sp.palette)
	for i := range rows {
		rows[i] = "  " + rows[i]
	}
	return strings.Join(rows, "\n")
}

// squash drops the sprite's top row and widens its bottom row by a pixel each side.
func squash(px []string) []string {
	out := append([]string(nil), px[1:]...)
	last := []byte(out[len(out)-1])
	for i := range last {
		if last[i] != '.' {
			if i > 0 {
				last[i-1] = last[i]
			}
			break
		}
	}
	for i := len(last) - 1; i >= 0; i-- {
		if last[i] != '.' {
			if i < len(last)-1 {
				last[i+1] = last[i]
			}
			break
		}
	}
	out[len(out)-1] = string(last)
	return out
}

// Gradient colours s rune by rune along the stops.
func Gradient(s string, bold bool, stops ...color.Color) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	cols := lipgloss.Blend1D(max(len(runes), 2), stops...)
	var sb strings.Builder
	for i, r := range runes {
		st := lipgloss.NewStyle().Foreground(cols[i])
		if bold {
			st = st.Bold(true)
		}
		sb.WriteString(st.Render(string(r)))
	}
	return sb.String()
}

// Shimmer draws s with a soft highlight centred on phase (a rune index that sweeps past both
// ends), the base colour elsewhere.
func (t Theme) Shimmer(s string, phase int) string {
	runes := []rune(s)
	base, hi := t.text.GetForeground(), t.accent.GetForeground()
	if base == nil || base == (lipgloss.NoColor{}) {
		base = lipgloss.Color("#c9d1d9")
		if !t.Dark {
			base = lipgloss.Color("#24292f")
		}
	}
	ramp := lipgloss.Blend1D(5, base, hi)
	var sb strings.Builder
	for i, r := range runes {
		d := i - phase
		if d < 0 {
			d = -d
		}
		c := base
		if d < 4 {
			c = ramp[4-d]
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(r)))
	}
	return sb.String()
}

// shimmerPhase maps a tick count onto a sweep that runs a few runes past each end.
func shimmerPhase(tick, n int) int {
	span := n + 8
	return tick%span - 4
}

// Meter is a context meter: cells filled to pct, each coloured along green → amber → red.
func (t Theme) Meter(pct, cells int) string {
	if pct < 0 {
		return t.dim.Render(strings.Repeat("▱", cells))
	}
	fill := (pct*cells + 99) / 100
	if fill > cells {
		fill = cells
	}
	cols := lipgloss.Blend1D(cells, t.ok.GetForeground(), t.warn.GetForeground(), t.bad.GetForeground())
	var sb strings.Builder
	for i := 0; i < cells; i++ {
		if i < fill {
			sb.WriteString(lipgloss.NewStyle().Foreground(cols[i]).Render("▰"))
		} else {
			sb.WriteString(t.border.Render("▱"))
		}
	}
	return sb.String()
}

func visibleLen(s string) int { return ansi.StringWidth(s) }
