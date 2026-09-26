package onto

import (
	"fmt"
	"sort"
	"strings"
)

// Finding is one shape violation, pointed at the doc that owns it.
type Finding struct {
	Shape   string
	Subject Term
	Doc     string // file:line when known
	Msg     string
}

func (f Finding) String() string {
	if f.Doc != "" {
		return f.Doc + ": " + f.Msg + " (" + f.Shape + ")"
	}
	return f.Msg + " (" + f.Shape + ")"
}

// Validate applies every is:Shape in the graph and returns the findings, sorted.
func (w *World) Validate() []Finding {
	g := w.Graph
	var out []Finding
	for _, shape := range g.Instances(cShape) {
		on := first(g.Objects(shape, pOn))
		prop := first(g.Objects(shape, pProperty))
		if on == nil || prop == nil {
			continue
		}
		name := shape.Local()
		min, hasMin := intOf(g, shape, pMinCount)
		max, hasMax := intOf(g, shape, pMaxCount)
		disjoint := boolOf(g, shape, pDisjoint)
		valuesHave := first(g.Objects(shape, pValuesHave))
		insts := g.Instances(*on)
		var owners []Term
		var paths []string
		for _, x := range insts {
			vals := g.Objects(x, *prop)
			n := len(vals)
			switch {
			case hasMin && hasMax && min == max && n != min:
				out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s has %d %s edge%s, wants exactly %d", x.Local(), n, prop.Local(), plural(n), min)))
			case hasMin && n < min:
				out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s has %d %s edge%s, wants at least %d", x.Local(), n, prop.Local(), plural(n), min)))
			case hasMax && n > max:
				out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s has %d %s edge%s, wants at most %d", x.Local(), n, prop.Local(), plural(n), max)))
			}
			if valuesHave != nil {
				for _, v := range vals {
					if v.Kind == Literal || len(g.Objects(v, *valuesHave)) == 0 {
						out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s %s %s, which has no %s", x.Local(), prop.Local(), v.Local(), valuesHave.Local())))
					}
				}
			}
			if disjoint {
				for _, v := range vals {
					owners = append(owners, x)
					paths = append(paths, v.Value)
				}
			}
		}
		if disjoint {
			for i := range paths {
				for j := i + 1; j < len(paths); j++ {
					if owners[i] == owners[j] || !overlap(paths[i], paths[j]) {
						continue
					}
					out = append(out, w.finding(name, owners[i], "territory", fmt.Sprintf("%s owns %s, overlapping %s of %s", owners[i].Local(), paths[i], paths[j], owners[j].Local())))
				}
			}
		}
	}
	sortFindings(out)
	return out
}

func (w *World) finding(shape string, x Term, field, msg string) Finding {
	f := Finding{Shape: shape, Subject: x, Msg: msg}
	if d, ok := w.Docs[x]; ok {
		line := 1
		switch field {
		case "truth", "verdict", "reports", "above":
			if l, ok := d.Lines["parent"]; ok {
				line = l
			}
		case "territory", "owns":
			if l, ok := d.Lines["territory"]; ok {
				line = l
			}
		}
		f.Doc = fmt.Sprintf("%s:%d", d.Path, line)
	} else if docs := w.Graph.Objects(x, pDoc); len(docs) > 0 {
		if p := w.Graph.Objects(docs[0], pPath); len(p) > 0 {
			f.Doc = p[0].Value + ":1"
		}
	}
	return f
}

func overlap(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func first(ts []Term) *Term {
	if len(ts) == 0 {
		return nil
	}
	return &ts[0]
}

func intOf(g *Graph, s, p Term) (int, bool) {
	if t := first(g.Objects(s, p)); t != nil {
		return t.AsInt()
	}
	return 0, false
}

func boolOf(g *Graph, s, p Term) bool {
	t := first(g.Objects(s, p))
	return t != nil && t.Value == "true"
}

func sortFindings(fs []Finding) {
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].Shape != fs[j].Shape {
			return fs[i].Shape < fs[j].Shape
		}
		if fs[i].Doc != fs[j].Doc {
			return fs[i].Doc < fs[j].Doc
		}
		return fs[i].Msg < fs[j].Msg
	})
}
