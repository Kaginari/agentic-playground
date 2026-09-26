package onto

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Now is the clock AssertUnsaid stamps facts with; tests pin it.
var Now = time.Now

// bonds, in the order a walk tries them: the specific ones before the generic.
var bonds = []Term{pTruth, pVerdict, pReports, pAbove}

// Seen is one fact a member may see, with the chain it travelled.
type Seen struct {
	Fact   Term
	Holder Term
	Hops   int
	Kind   string // policy | team | ownership | fact
	Text   string
	Line   string // the plain projection line
}

type hop struct {
	from, bond, to Term
}

// Visible walks the flow rules for a viewer: analysis up (any fact known
// below it, up its truth/verdict/reports chain) and wisdom down (any Policy known
// above it). Nearest first, then by line.
func (w *Workspace) Visible(viewer Term) []Seen {
	g := w.Graph
	type node struct {
		t    Term
		hops int
		path []hop // from holder toward viewer
	}
	var out []Seen
	emit := func(n node, lawsOnly bool) {
		for _, f := range g.Objects(n.t, pKnows) {
			kind := factKind(g, f)
			if lawsOnly && kind != "policy" {
				continue
			}
			text := ""
			if t := first(g.Objects(f, pText)); t != nil {
				text = t.Value
			}
			line := renderChain(n.t, n.path) + " · " + text + " (" + kind + ")"
			out = append(out, Seen{Fact: f, Holder: n.t, Hops: n.hops, Kind: kind, Text: text, Line: line})
		}
	}
	// self
	emit(node{t: viewer}, false)
	// down: everything that is below the viewer, one bond at a time
	visited := map[Term]bool{viewer: true}
	queue := []node{{t: viewer}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, bond := range []Term{pTruth, pVerdict, pReports, pAbove} {
			for _, child := range g.Subjects(bond, n.t) {
				if visited[child] || !baseBond(g, child, bond, n.t) {
					continue
				}
				visited[child] = true
				c := node{t: child, hops: n.hops + 1, path: append([]hop{{child, bond, n.t}}, n.path...)}
				emit(c, false)
				queue = append(queue, c)
			}
		}
	}
	// up: policies known by anything above
	visited = map[Term]bool{viewer: true}
	queue = []node{{t: viewer}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, bond := range []Term{pTruth, pVerdict, pReports, pAbove} {
			for _, parent := range g.Objects(n.t, bond) {
				if visited[parent] || !baseBond(g, n.t, bond, parent) {
					continue
				}
				visited[parent] = true
				c := node{t: parent, hops: n.hops + 1, path: append(append([]hop(nil), n.path...), hop{n.t, bond, parent})}
				emit(c, true)
				queue = append(queue, c)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hops != out[j].Hops {
			return out[i].Hops < out[j].Hops
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// baseBond is true when (s bond o) is a one-hop bond, not the transitive
// closure: `above` counts only when no more specific bond carries it.
func baseBond(g *Graph, s, bond, o Term) bool {
	if bond != pAbove {
		return true
	}
	for _, b := range []Term{pTruth, pVerdict, pReports} {
		if g.Has(Triple{s, b, o}) {
			return false
		}
	}
	for _, mid := range g.Objects(s, pAbove) {
		if mid != o && g.Has(Triple{mid, pAbove, o}) {
			return false
		}
	}
	return true
}

func factKind(g *Graph, f Term) string {
	for k, c := range kindClass {
		if g.Has(Triple{f, rdfType, c}) {
			return k
		}
	}
	return "fact"
}

// renderChain draws holder → viewer; each arrow points along its bond.
func renderChain(holder Term, path []hop) string {
	if len(path) == 0 {
		return holder.Local()
	}
	var b strings.Builder
	if path[0].from == holder {
		b.WriteString(holder.Local())
		for _, h := range path {
			b.WriteString(" ⇒" + h.bond.Local() + " " + h.to.Local())
		}
		return b.String()
	}
	// downward: the holder is the last hop's target; walk back toward the viewer
	b.WriteString(holder.Local())
	for i := len(path) - 1; i >= 0; i-- {
		b.WriteString(" ⇐" + path[i].bond.Local() + " " + path[i].from.Local())
	}
	return b.String()
}

// Tokens estimates a line's cost: about four characters per token.
func Tokens(s string) int { return (utf8.RuneCountInString(s) + 3) / 4 }

// Project emits what a member may see as plain lines, nearest first, under
// a token budget: its own bonds and ownership, then facts by distance.
func (w *Workspace) Project(member string, budget int) ([]string, error) {
	viewer, err := w.Member(member)
	if err != nil {
		return nil, err
	}
	g := w.Graph
	me := viewer.Local()
	type ranked struct {
		hops int
		line string
	}
	var lines []ranked
	for _, bond := range bonds {
		for _, o := range g.Objects(viewer, bond) {
			if baseBond(g, viewer, bond, o) {
				lines = append(lines, ranked{0, me + " ⇒" + bond.Local() + " " + o.Local()})
			}
		}
	}
	for _, m := range g.Objects(viewer, pHolds) {
		lines = append(lines, ranked{0, me + " ⇌holds " + nameOf(g, m)})
	}
	for _, p := range g.Objects(viewer, pOwns) {
		lines = append(lines, ranked{0, me + " owns " + p.Value})
	}
	for _, bond := range bonds {
		for _, s := range g.Subjects(bond, viewer) {
			if baseBond(g, s, bond, viewer) {
				lines = append(lines, ranked{1, s.Local() + " ⇒" + bond.Local() + " " + me})
			}
		}
	}
	for _, s := range w.Visible(viewer) {
		lines = append(lines, ranked{s.Hops, s.Line})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].hops < lines[j].hops })
	var out []string
	used := 0
	for _, l := range lines {
		c := Tokens(l.line)
		if budget > 0 && used+c > budget {
			break
		}
		used += c
		out = append(out, l.line)
	}
	return out, nil
}

func nameOf(g *Graph, t Term) string {
	if n := first(g.Objects(t, pName)); n != nil {
		return n.Value
	}
	return t.Local()
}

// AssertUnsaid turns a Subagent's `@U <kind> <text>` into an asserted Fact,
// appended to .agent-one/ontology/graph/unsaid.ttl (append-only) and to the
// loaded graph. It returns the fact's term.
func (w *Workspace) AssertUnsaid(agent, kind, text string) (Term, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	class, ok := kindClass[kind]
	if !ok {
		return Term{}, fmt.Errorf("kind must be policy, team or ownership, not %q", kind)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Term{}, fmt.Errorf("the unsaid needs text")
	}
	who, err := w.Member(agent)
	if err != nil {
		return Term{}, err
	}
	when := Now().UTC().Format(time.RFC3339)
	h := sha1.Sum([]byte(who.Value + "|" + kind + "|" + text + "|" + when))
	fact := Ao("fact-" + hex.EncodeToString(h[:6]))
	triples := []Triple{
		{fact, rdfType, class},
		{fact, pText, L(text)},
		{fact, pSaid, L(when)},
		{fact, pAbout, who},
		{who, pKnows, fact},
	}
	dir := filepath.Join(OntologyDirIn(w.Root, w.Layout), "graph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Term{}, err
	}
	path := filepath.Join(dir, "unsaid.ttl")
	var sb strings.Builder
	if _, err := os.Stat(path); err != nil {
		sb.WriteString("# The unsaid, asserted: one Fact per @U line a Subagent reported. Append-only.\n")
		sb.WriteString("@prefix ao: <" + NS + "> .\n")
	}
	sb.WriteString("\n")
	if err := WriteTriples(&sb, triples, DefaultPrefixes()); err != nil {
		return Term{}, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Term{}, err
	}
	if _, err := f.WriteString(sb.String()); err != nil {
		f.Close()
		return Term{}, err
	}
	if err := f.Close(); err != nil {
		return Term{}, err
	}
	for _, t := range triples {
		w.Graph.Add(t)
	}
	Infer(w.Graph)
	return fact, nil
}
