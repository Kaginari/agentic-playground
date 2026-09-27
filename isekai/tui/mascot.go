package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// The mascot at the top of the welcome: a pixel sprite drawn with half blocks, two pixels per
// cell (the upper one the foreground of ▀, the lower one its background). isekai shows the slime
// the world is reincarnated as; agent-one its own agent. Each is 16 pixels wide, 8 high: 4 rows.

type sprite struct {
	pixels  []string        // one string per pixel row; '.' is empty
	palette map[byte]string // pixel → hex colour
	body    byte            // the pixel an eye becomes when it blinks
	title   []string        // the title's gradient stops
	tagline string
}

var mascots = map[string]sprite{
	"isekai": {
		pixels: []string{
			"......BBBB......",
			"....BBBBBBBB....",
			"...BLLBBBBBBB...",
			"..BLLBBBBBBBBB..",
			".BBBBBEBBBEBBBB.",
			".BBBBBEBBBEBBBB.",
			"BBBBBBBBBBBBBBBB",
			".SSSSSSSSSSSSSS.",
		},
		palette: map[byte]string{'B': "#58b7f0", 'L': "#d2f0ff", 'E': "#0d1b2a", 'S': "#2a78b8"},
		body:    'B',
		title:   []string{"#2a78b8", "#58b7f0", "#a6e3ff"},
		tagline: "reincarnated · the world remembers in documents",
	},
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
		body:    'W',
		title:   []string{"#7c5cd6", "#b69cff", "#d4af37"},
		tagline: "one agent, a whole team · the workspace remembers",
	},
}

// mascotWidth is the sprite's width in cells.
const mascotWidth = 16

func mascotFor(dist string) sprite {
	if sp, ok := mascots[dist]; ok {
		return sp
	}
	for _, sp := range mascots {
		return sp
	}
	return sprite{}
}

// Mascot draws the distribution's sprite as half-block rows, and its tagline.
func Mascot(dist string) (rows []string, tagline string) {
	sp := mascotFor(dist)
	return halfBlocks(sp.pixels, sp.palette), sp.tagline
}

// Title is the distribution's name on its gradient.
func Title(dist string) string {
	sp := mascotFor(dist)
	var stops []color.Color
	for _, h := range sp.title {
		stops = append(stops, lipgloss.Color(h))
	}
	if len(stops) < 2 {
		return "✦ " + dist
	}
	return Gradient("✦ "+dist, true, stops...)
}

// halfBlocks draws pixel rows two to a cell: the upper pixel the foreground of ▀, the lower its
// background.
func halfBlocks(px []string, palette map[byte]string) []string {
	var rows []string
	for y := 0; y+1 < len(px); y += 2 {
		var sb strings.Builder
		for x := 0; x < len(px[y]); x++ {
			top, bot := px[y][x], px[y+1][x]
			switch {
			case top == '.' && bot == '.':
				sb.WriteByte(' ')
			case top == '.':
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(palette[bot])).Render("▄"))
			case bot == '.':
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(palette[top])).Render("▀"))
			case top == bot:
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(palette[top])).Render("█"))
			default:
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(palette[top])).Background(lipgloss.Color(palette[bot])).Render("▀"))
			}
		}
		rows = append(rows, sb.String())
	}
	return rows
}
