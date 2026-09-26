package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

var (
	mdMu    sync.Mutex
	mdCache = map[string]*glamour.TermRenderer{}
)

// Markdown renders an assistant answer at a width: Glamour with the theme's style (dark,
// light, or notty when the terminal has no colour), no document margin so the text sits
// flush with the blocks around it. A render failure falls back to the plain text.
func (t Theme) Markdown(md string, width int) string {
	md = strings.ReplaceAll(strings.TrimSpace(md), "\t", "    ")
	if md == "" {
		return ""
	}
	if width < 20 {
		width = 20
	}
	r, err := t.renderer(width)
	if err != nil {
		return wrap(md, width)
	}
	mdMu.Lock()
	out, err := r.Render(md)
	mdMu.Unlock()
	if err != nil {
		return wrap(md, width)
	}
	// glamour pads every line to the width and wraps the document in blank lines
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

func (t Theme) renderer(width int) (*glamour.TermRenderer, error) {
	key := t.styleName() + ":" + itoa(width)
	mdMu.Lock()
	r, ok := mdCache[key]
	mdMu.Unlock()
	if ok {
		return r, nil
	}
	sc := t.styleConfig()
	r, err := glamour.NewTermRenderer(glamour.WithStyles(sc), glamour.WithWordWrap(width))
	if err != nil {
		return nil, err
	}
	mdMu.Lock()
	mdCache[key] = r
	mdMu.Unlock()
	return r, nil
}

func (t Theme) styleName() string {
	switch {
	case !t.Color:
		return "notty"
	case t.Dark:
		return "dark"
	}
	return "light"
}

func (t Theme) styleConfig() ansi.StyleConfig {
	var sc ansi.StyleConfig
	switch t.styleName() {
	case "notty":
		sc = styles.NoTTYStyleConfig
	case "light":
		sc = styles.LightStyleConfig
	default:
		sc = styles.DarkStyleConfig
	}
	zero := uint(0)
	sc.Document.Margin = &zero
	sc.Document.BlockPrefix = ""
	sc.Document.BlockSuffix = ""
	sc.CodeBlock.Margin = &zero
	return sc
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
