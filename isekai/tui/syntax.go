package tui

import (
	"image/color"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Syntax colour for code in diffs: chroma's tokens, drawn with Lip Gloss, one line at a time (a
// diff is lines, not a file; a construct that spans lines is coloured per line).

var (
	lexMu    sync.Mutex
	lexCache = map[string]chroma.Lexer{}
)

func lexerFor(path string) chroma.Lexer {
	lexMu.Lock()
	defer lexMu.Unlock()
	if l, ok := lexCache[path]; ok {
		return l
	}
	l := lexers.Match(path)
	if l != nil {
		l = chroma.Coalesce(l)
	}
	lexCache[path] = l
	return l
}

func (t Theme) chromaStyle() *chroma.Style {
	if t.Dark {
		return styles.Get("catppuccin-mocha")
	}
	return styles.Get("github")
}

// Highlight colours one line of code for path's language, over bg when bg is not nil; the line
// comes back plain (in fg) when the language is unknown or the terminal has no colour.
func (t Theme) Highlight(path, line string, fg lipgloss.Style, bg color.Color) string {
	base := fg
	if bg != nil {
		base = base.Background(bg)
	}
	lx := lexerFor(path)
	if lx == nil || !t.Color {
		return base.Render(line)
	}
	it, err := lx.Tokenise(nil, line)
	if err != nil {
		return base.Render(line)
	}
	st := t.chromaStyle()
	var sb strings.Builder
	for tok := it(); tok != chroma.EOF; tok = it() {
		s := base
		e := st.Get(tok.Type)
		if e.Colour.IsSet() {
			s = s.Foreground(lipgloss.Color(e.Colour.String()))
		}
		if e.Bold == chroma.Yes {
			s = s.Bold(true)
		}
		if e.Italic == chroma.Yes {
			s = s.Italic(true)
		}
		sb.WriteString(s.Render(strings.TrimRight(tok.Value, "\n")))
	}
	return sb.String()
}

// diffBg is the tint behind an added or a removed line.
func (t Theme) diffBg(kind byte) color.Color {
	switch {
	case kind == '+' && t.Dark:
		return lipgloss.Color("#16301f")
	case kind == '+':
		return lipgloss.Color("#dafbe1")
	case kind == '-' && t.Dark:
		return lipgloss.Color("#3a1a1d")
	case kind == '-':
		return lipgloss.Color("#ffebe9")
	}
	return nil
}
