package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The Graph page draws the reasoned ontology as a layered graph: one row per level (the session
// on top, a hop down per row), each one-hop bond a line from the child up to its parent, colored
// by bond. The selected creature's knowledge card sits beside it (below on a narrow terminal).

const (
	dirU = 1 << iota
	dirD
	dirL
	dirR
)

var boxGlyph = map[int]rune{
	dirU: '│', dirD: '│', dirU | dirD: '│',
	dirL: '─', dirR: '─', dirL | dirR: '─',
	dirD | dirR: '┌', dirD | dirL: '┐', dirU | dirR: '└', dirU | dirL: '┘',
	dirU | dirD | dirR: '├', dirU | dirD | dirL: '┤', dirL | dirR | dirD: '┬', dirL | dirR | dirU: '┴',
	dirU | dirD | dirL | dirR: '┼',
}

type placed struct {
	n      GraphNode
	x, w   int // label start and width on the canvas
	cx, y  int
	lvl, i int // level and index in it
}

type graphLayout struct {
	nodes  map[string]*placed
	levels [][]*placed
	width  int
	height int
}

// layoutGraph orders each level by its parents' mean position (fewer crossings) and spreads
// the levels over a canvas; labels are "◆ name".
func layoutGraph(nodes []GraphNode) *graphLayout {
	g := &graphLayout{nodes: map[string]*placed{}}
	maxLvl := 0
	for _, n := range nodes {
		if n.Level > maxLvl {
			maxLvl = n.Level
		}
	}
	g.levels = make([][]*placed, maxLvl+1)
	for _, n := range nodes {
		p := &placed{n: n, lvl: n.Level, w: lipgloss.Width(n.Name) + 2}
		g.nodes[n.ID] = p
		g.levels[n.Level] = append(g.levels[n.Level], p)
	}
	for l, row := range g.levels {
		bary := func(p *placed) float64 {
			if l == 0 || len(p.n.Up) == 0 {
				return 0
			}
			sum, k := 0.0, 0
			for _, u := range p.n.Up {
				if q, ok := g.nodes[u.To]; ok {
					sum += float64(q.i)
					k++
				}
			}
			if k == 0 {
				return 0
			}
			return sum / float64(k)
		}
		sort.SliceStable(row, func(i, j int) bool {
			bi, bj := bary(row[i]), bary(row[j])
			if bi != bj {
				return bi < bj
			}
			return row[i].n.Name < row[j].n.Name
		})
		for i, p := range row {
			p.i = i
		}
	}
	// place: the roots side by side; every lower level under its parents (the mean of their
	// centers), siblings as one block centered there, pushed right only to avoid overlap
	const gap = 3
	for l, row := range g.levels {
		if l == 0 {
			x := 0
			for _, p := range row {
				p.x, p.y, p.cx = x, 0, x+1
				x += p.w + gap
			}
			continue
		}
		want := func(p *placed) int {
			sum, k := 0, 0
			for _, u := range p.n.Up {
				if q, ok := g.nodes[u.To]; ok && q.lvl < l {
					sum += q.cx
					k++
				}
			}
			if k == 0 {
				return -1
			}
			return sum / k
		}
		end := -gap
		for i := 0; i < len(row); {
			j, bw := i, 0
			d := want(row[i])
			for j < len(row) && want(row[j]) == d {
				bw += row[j].w + gap
				j++
			}
			bw -= gap
			start := end + gap
			if d >= 0 && d-bw/2 > start {
				start = d - bw/2
			}
			x := start
			for k := i; k < j; k++ {
				row[k].x, row[k].y, row[k].cx = x, l*4, x+1
				x += row[k].w + gap
			}
			end = x - gap
			i = j
		}
	}
	minX := 0
	for _, row := range g.levels {
		for _, p := range row {
			if p.x < minX {
				minX = p.x
			}
			if p.x+p.w > g.width {
				g.width = p.x + p.w
			}
		}
	}
	if minX < 0 {
		for _, row := range g.levels {
			for _, p := range row {
				p.x -= minX
				p.cx -= minX
			}
		}
		g.width -= minX
	}
	g.height = maxLvl*4 + 1
	return g
}

type cell struct {
	r    rune
	dirs int
	bond string
}

// render draws the canvas; sel is the selected node's id.
func (g *graphLayout) render(t Theme, sel string) []string {
	grid := make([][]cell, g.height)
	for y := range grid {
		grid[y] = make([]cell, g.width)
	}
	set := func(x, y, d int, bond string) {
		if y < 0 || y >= g.height || x < 0 || x >= g.width {
			return
		}
		c := &grid[y][x]
		if c.r != 0 {
			return // a label wins
		}
		c.dirs |= d
		if c.bond == "" || bond == "truth" || bond == "verdict" {
			c.bond = bond
		}
	}
	for _, row := range g.levels {
		for _, c := range row {
			for _, u := range c.n.Up {
				p, ok := g.nodes[u.To]
				if !ok {
					continue
				}
				// down from the parent, across on the row above the child, down into the child
				mid := c.y - 2
				if mid <= p.y {
					mid = p.y + 1
				}
				for y := p.y + 1; y < mid; y++ {
					set(p.cx, y, dirU|dirD, u.Bond)
				}
				if p.cx == c.cx {
					set(p.cx, mid, dirU|dirD, u.Bond)
				} else {
					toward := dirR
					back := dirL
					if c.cx < p.cx {
						toward, back = dirL, dirR
					}
					set(p.cx, mid, dirU|toward, u.Bond)
					lo, hi := p.cx, c.cx
					if lo > hi {
						lo, hi = hi, lo
					}
					for x := lo + 1; x < hi; x++ {
						set(x, mid, dirL|dirR, u.Bond)
					}
					set(c.cx, mid, dirD|back, u.Bond)
				}
				for y := mid + 1; y < c.y; y++ {
					set(c.cx, y, dirU|dirD, u.Bond)
				}
			}
		}
	}
	// labels last: they sit on top of any line that crosses a level
	labels := map[int][]*placed{}
	for _, row := range g.levels {
		for _, p := range row {
			labels[p.y] = append(labels[p.y], p)
			for i := 0; i < p.w && p.x+i < g.width; i++ {
				grid[p.y][p.x+i].r = ' '
			}
		}
	}
	out := make([]string, g.height)
	for y, row := range grid {
		var sb strings.Builder
		ls := labels[y]
		sort.Slice(ls, func(i, j int) bool { return ls[i].x < ls[j].x })
		li := 0
		for x := 0; x < len(row); {
			if li < len(ls) && ls[li].x == x {
				p := ls[li]
				st := t.rankStyle(p.n.Rank)
				mark := "◆"
				if p.n.Findings > 0 {
					mark = "◇"
				}
				label := mark + " " + p.n.Name
				if p.n.ID == sel {
					label = st.Reverse(true).Bold(true).Render(label)
				} else {
					label = st.Render(label)
				}
				sb.WriteString(label)
				x += p.w
				li++
				continue
			}
			c := row[x]
			if c.dirs == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteString(bondStyle(t, c.bond).Render(string(boxGlyph[c.dirs])))
			}
			x++
		}
		out[y] = sb.String()
	}
	return out
}

// graphMove walks the selection: ←→ along a level, ↑↓ to the nearest node one level over.
func (m *Model) graphMove(key string) {
	b := m.board
	g := layoutGraph(b.view.Graph.Nodes)
	cur, ok := g.nodes[b.sel]
	if !ok {
		return
	}
	switch key {
	case "left", "h":
		if cur.i > 0 {
			b.sel = g.levels[cur.lvl][cur.i-1].n.ID
		}
	case "right", "l":
		if cur.i < len(g.levels[cur.lvl])-1 {
			b.sel = g.levels[cur.lvl][cur.i+1].n.ID
		}
	case "up", "k", "down", "j":
		d := 1
		if key == "up" || key == "k" {
			d = -1
		}
		for l := cur.lvl + d; l >= 0 && l < len(g.levels); l += d {
			if len(g.levels[l]) == 0 {
				continue
			}
			best := g.levels[l][0]
			for _, p := range g.levels[l] {
				if abs(p.cx-cur.cx) < abs(best.cx-cur.cx) {
					best = p
				}
			}
			b.sel = best.n.ID
			return
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (m *Model) boardGraph() ([]string, int) {
	b, t, w := m.board, m.theme, m.width
	gv := b.view.Graph
	if !b.loaded {
		return []string{"", t.dim.Render("  reasoning over the ontology…")}, -1
	}
	lines := []string{""}
	if gv.Summary != "" {
		lines = append(lines, "  "+t.dim.Render(gv.Summary))
	}
	if gv.Note != "" {
		lines = append(lines, "  "+t.warn.Render(gv.Note))
	}
	legend := "  " + t.dim.Render("bonds ")
	for i, bd := range []string{"truth", "verdict", "reports", "above"} {
		if i > 0 {
			legend += t.dim.Render(" · ")
		}
		legend += bondStyle(t, bd).Render("── " + bd)
	}
	legend += t.dim.Render("   ◇ has findings")
	lines = append(lines, legend)

	if len(gv.Nodes) == 0 {
		return append(lines, "", t.dim.Render("  no creature in the ontology yet")), -1
	}
	g := layoutGraph(gv.Nodes)
	if _, ok := g.nodes[b.sel]; !ok {
		b.sel = g.levels[0][0].n.ID
	}
	sel := g.nodes[b.sel]
	lines = append(lines, "")
	cardW := 0
	if w >= 100 {
		cardW = min(46, w/3)
	}
	viewW := w - 2
	if cardW > 0 {
		viewW = w - cardW - 5
	}
	canvas := g.render(t, b.sel)
	// pan so the selection stays in view on a graph wider than the screen
	if g.width > viewW {
		if sel.x < b.panX {
			b.panX = sel.x
		}
		if sel.x+sel.w > b.panX+viewW {
			b.panX = sel.x + sel.w - viewW
		}
	} else {
		b.panX = 0
	}
	lead := ""
	if g.width < viewW {
		lead = strings.Repeat(" ", (viewW-g.width)/2)
	}
	for i, l := range canvas {
		if b.panX > 0 {
			l = ansi.Cut(l, b.panX, b.panX+viewW)
		} else {
			l = ansi.Truncate(l, viewW, "")
		}
		canvas[i] = "  " + lead + l
	}
	card := knowledgeCard(t, g, sel, max(20, cardW))
	top := len(lines)
	if cardW > 0 {
		for i := 0; i < len(canvas) || i < len(card); i++ {
			left, right := "", ""
			if i < len(canvas) {
				left = canvas[i]
			}
			if i < len(card) {
				right = card[i]
			}
			pad := viewW + 2 - lipgloss.Width(left)
			lines = append(lines, left+strings.Repeat(" ", max(0, pad))+t.border.Render(" │ ")+right)
		}
	} else {
		lines = append(lines, canvas...)
		lines = append(lines, "", t.border.Render("  "+strings.Repeat("─", max(0, w-4))))
		for _, c := range knowledgeCard(t, g, sel, w-4) {
			lines = append(lines, "  "+c)
		}
	}
	return lines, top + sel.y
}

// knowledgeCard is what the ontology knows about one creature, as the host projected it.
func knowledgeCard(t Theme, g *graphLayout, p *placed, w int) []string {
	n := p.n
	head := t.rankStyle(n.Rank).Bold(true).Render(n.Name)
	if n.Rank != "" && n.Rank != n.Name {
		head += t.dim.Render("  " + n.Rank)
	}
	out := []string{head}
	line := func(k, v string) {
		out = append(out, ansi.Truncate(t.dim.Render(fmt.Sprintf("%-9s", k))+v, w, "…"))
	}
	var ups []string
	for _, u := range n.Up {
		name := u.To
		if q, ok := g.nodes[u.To]; ok {
			name = q.n.Name
		}
		ups = append(ups, name+" "+bondStyle(t, u.Bond).Render("‹"+u.Bond+"›"))
	}
	if len(ups) > 0 {
		line("answers", strings.Join(ups, ", "))
	}
	var downs []string
	for _, row := range g.levels {
		for _, c := range row {
			for _, u := range c.n.Up {
				if u.To == n.ID {
					downs = append(downs, c.n.Name)
				}
			}
		}
	}
	if len(downs) > 0 {
		line("rules", strings.Join(downs, ", "))
	}
	for _, kv := range n.Knowledge {
		line(kv.Key, kv.Value)
	}
	return out
}
