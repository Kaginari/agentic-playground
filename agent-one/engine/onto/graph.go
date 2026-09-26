package onto

import "sort"

// Graph is an in-memory triple store with S, P and O indexes. Triples added
// with Add are asserted (they came from a file); triples added by derivation
// or reasoning are marked derived and are never written back.
type Graph struct {
	triples []Triple
	set     map[Triple]int
	byS     map[Term][]int
	byP     map[Term][]int
	byO     map[Term][]int
	derived map[Triple]bool
}

// New makes an empty graph.
func New() *Graph {
	return &Graph{
		set: map[Triple]int{}, byS: map[Term][]int{}, byP: map[Term][]int{}, byO: map[Term][]int{},
		derived: map[Triple]bool{},
	}
}

// Add asserts a triple; false if it was already present.
func (g *Graph) Add(t Triple) bool { return g.add(t, false) }

// AddDerived adds a triple that is not on disk; false if already present.
func (g *Graph) AddDerived(t Triple) bool { return g.add(t, true) }

func (g *Graph) add(t Triple, derived bool) bool {
	if _, ok := g.set[t]; ok {
		if !derived {
			g.derived[t] = false
		}
		return false
	}
	i := len(g.triples)
	g.triples = append(g.triples, t)
	g.set[t] = i
	g.byS[t.S] = append(g.byS[t.S], i)
	g.byP[t.P] = append(g.byP[t.P], i)
	g.byO[t.O] = append(g.byO[t.O], i)
	g.derived[t] = derived
	return true
}

// Has reports whether the triple is present.
func (g *Graph) Has(t Triple) bool { _, ok := g.set[t]; return ok }

// Derived reports whether a present triple was derived rather than asserted.
func (g *Graph) Derived(t Triple) bool { return g.derived[t] }

// Len is the number of triples.
func (g *Graph) Len() int { return len(g.triples) }

// Match returns the triples matching the pattern (nil = any), sorted.
func (g *Graph) Match(s, p, o *Term) []Triple {
	var cand []int
	chosen := false
	pick := func(ix []int) {
		if !chosen || len(ix) < len(cand) {
			cand, chosen = ix, true
		}
	}
	if s != nil {
		pick(g.byS[*s])
	}
	if p != nil {
		pick(g.byP[*p])
	}
	if o != nil {
		pick(g.byO[*o])
	}
	var out []Triple
	if !chosen {
		out = append(out, g.triples...)
	} else {
		for _, i := range cand {
			t := g.triples[i]
			if (s == nil || t.S == *s) && (p == nil || t.P == *p) && (o == nil || t.O == *o) {
				out = append(out, t)
			}
		}
	}
	sortTriples(out)
	return out
}

// Objects lists the objects of (s, p, ?), sorted.
func (g *Graph) Objects(s, p Term) []Term {
	var out []Term
	for _, t := range g.Match(&s, &p, nil) {
		out = append(out, t.O)
	}
	return out
}

// Subjects lists the subjects of (?, p, o), sorted.
func (g *Graph) Subjects(p, o Term) []Term {
	var out []Term
	for _, t := range g.Match(nil, &p, &o) {
		out = append(out, t.S)
	}
	return out
}

// Instances lists the subjects typed with class c (after reasoning, this
// includes subclasses).
func (g *Graph) Instances(c Term) []Term { return g.Subjects(I(RDFType), c) }

// All returns every triple, sorted.
func (g *Graph) All() []Triple { return g.Match(nil, nil, nil) }

// Asserted returns the triples that came from files, sorted.
func (g *Graph) Asserted() []Triple {
	var out []Triple
	for _, t := range g.triples {
		if !g.derived[t] {
			out = append(out, t)
		}
	}
	sortTriples(out)
	return out
}

func sortTriples(ts []Triple) {
	sort.Slice(ts, func(i, j int) bool { return cmpTriple(ts[i], ts[j]) < 0 })
}

func sortTerms(ts []Term) {
	sort.Slice(ts, func(i, j int) bool { return cmpTerm(ts[i], ts[j]) < 0 })
}
