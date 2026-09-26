package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/config/yaml"
)

// Show renders the effective config as JSON (keys sorted) or YAML, in the distribution's
// words: the canonical role and rank keys the loader folded to are spelled back.
func (c *Config) Show(asYAML bool) string {
	if c.tree == nil {
		return "{}"
	}
	tree := c.tree
	if asYAML {
		return yaml.Emit(tree)
	}
	return tree.Pretty() + "\n"
}

// Explain renders every live value with its origin: one `key: value  # origin` line each,
// sorted by key, followed by the layers, the rank tree and the honesty-rule findings.
func (c *Config) Explain() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s · root %s · %d layers\n", c.Dist.Name, c.Root, len(c.Layers))
	for _, l := range c.Paths() {
		fmt.Fprintf(&b, "# %s\n", l)
	}
	b.WriteString("\n")
	type line struct{ key, val, origin string }
	var lines []line
	var walk func(n *yaml.Node, path string)
	walk = func(n *yaml.Node, path string) {
		switch {
		case n == nil:
		case n.Kind == yaml.Map && len(n.Keys) > 0:
			for i, k := range n.Keys {
				walk(n.Vals[i], join(path, k))
			}
		case n.Kind == yaml.List && len(n.Items) > 0:
			for i, it := range n.Items {
				walk(it, fmt.Sprintf("%s[%d]", path, i))
			}
		default:
			o := c.Origins[path]
			val := n.JSON()
			if n.Kind == yaml.String {
				val = fmt.Sprintf("%q", n.Str)
			}
			lines = append(lines, line{path, val, o.String()})
		}
	}
	walk(c.tree, "")
	sort.Slice(lines, func(i, j int) bool { return lines[i].key < lines[j].key })
	width := 0
	for _, l := range lines {
		if n := len(l.key) + len(l.val) + 2; n > width && n < 90 {
			width = n
		}
	}
	for _, l := range lines {
		s := l.key + ": " + l.val
		fmt.Fprintf(&b, "%-*s  # %s\n", width, s, l.origin)
	}
	b.WriteString("\n# ranks\n")
	for _, l := range strings.Split(strings.TrimSpace(c.RankTree()), "\n") {
		fmt.Fprintf(&b, "# %s\n", l)
	}
	if dev := c.Deviations(); len(dev) > 0 {
		b.WriteString("# deviations from the policy:\n")
		for _, d := range dev {
			fmt.Fprintf(&b, "#   %s\n", d)
		}
	}
	b.WriteString("\n# models\n")
	for _, l := range c.ModelTable() {
		fmt.Fprintf(&b, "# %s\n", l)
	}
	if off := c.OffLines(); len(off) > 0 {
		b.WriteString("\n")
		for _, l := range off {
			b.WriteString(l + "\n")
		}
	}
	if lo := c.Loosenings(); len(lo) > 0 {
		b.WriteString("\n# loosening:\n")
		for _, l := range lo {
			fmt.Fprintf(&b, "#   %s\n", l)
		}
	}
	for _, h := range c.Holes {
		fmt.Fprintf(&b, "@? %s\n", h)
	}
	return b.String()
}

// StatusLines is the block `status` prints before the instrument board: the off-list
// (the honesty rule), the loosening rules, tools off, the rank set and any holes.
func (c *Config) StatusLines() []string {
	var out []string
	out = append(out, c.OffLines()...)
	for _, l := range c.Loosenings() {
		out = append(out, "loosening: "+l)
	}
	if off := c.ToolsOff(); len(off) > 0 {
		out = append(out, "tools off: "+strings.Join(off, "; "))
	}
	out = append(out, "ranks: "+strings.TrimSpace(strings.ReplaceAll(c.RankTree(), "\n", " · ")))
	for _, d := range c.Deviations() {
		out = append(out, "rank deviation: "+d)
	}
	for _, h := range c.Holes {
		out = append(out, "@? "+h)
	}
	return out
}
