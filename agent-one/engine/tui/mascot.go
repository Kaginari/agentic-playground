package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The mascot at the top of the welcome: a pixel sprite drawn with half blocks, two pixels per
// cell (the upper one the foreground of ▀, the lower one its background): the agent. 16 pixels
// wide, 8 high: 4 rows.

type sprite struct {
	pixels  []string          // one string per pixel row; '.' is empty
	palette map[byte]string   // pixel → hex colour
	tagline string
}

var mascots = map[string]sprite{
	"agent-one": {
		pixels: []string{
			".......AA.......",
			".......FF.......",
			"...FFFFFFFFFF...",
			"...FWWWWWWWWF...",
			"...FWEEWWEEWF...",
			"...FWWWWWWWWF...",
			"...FFFFFFFFFF...",
			".....F....F.....",
		},
		palette: map[byte]string{'A': "#d4af37", 'F': "#7c5cd6", 'W': "#e8e4ff", 'E': "#1a1033"},
		tagline: "one agent, a whole team · the workspace remembers",
	},
}

// mascotWidth is the sprite's width in cells.
const mascotWidth = 16

// Mascot draws the distribution's sprite as half-block rows, and its tagline.
func Mascot(dist string) (rows []string, tagline string) {
	sp, ok := mascots[dist]
	if !ok {
		sp = mascots["agent-one"]
	}
	px := sp.pixels
	for y := 0; y+1 < len(px); y += 2 {
		var sb strings.Builder
		for x := 0; x < len(px[y]); x++ {
			top, bot := px[y][x], px[y+1][x]
			switch {
			case top == '.' && bot == '.':
				sb.WriteByte(' ')
			case top == '.':
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(sp.palette[bot])).Render("▄"))
			case bot == '.':
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(sp.palette[top])).Render("▀"))
			case top == bot:
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(sp.palette[top])).Render("█"))
			default:
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(sp.palette[top])).Background(lipgloss.Color(sp.palette[bot])).Render("▀"))
			}
		}
		rows = append(rows, sb.String())
	}
	return rows, sp.tagline
}
