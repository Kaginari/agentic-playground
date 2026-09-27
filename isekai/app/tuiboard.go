package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/board"
	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/onto"
	"github.com/Kaginari/agentic-playground/isekai/tui"
)

// officeRoles says what each office of the triad does (isekai.md §Minds & Bodies: the triad).
var officeRoles = map[string]string{"great-sage": "reads", "raphael": "verdicts", "ciel": "drafts"}

// Board is what /board draws, read from the same feeds as the web board: the live court, the
// reasoned ontology, the offices as config resolves them, the usage journal.
func (h *tuiHost) Board(rng string) tui.BoardView { return h.a.BoardView(rng) }

// BoardView is the board as /board and the SSH board draw it.
func (a *App) BoardView(rng string) tui.BoardView {
	now := time.Now()
	opt := a.boardOptions()
	src := board.FileSources(opt.WorldRoot, opt.WorldDir, opt.Layout, time.Now)
	v := tui.BoardView{At: now}

	live := a.liveBodies()
	for _, b := range live {
		v.Agents = append(v.Agents, tui.AgentRow{Name: b.Name, Rank: a.Cfg.Dist.Word(b.Rank), Office: a.Cfg.Dist.Word(b.Office), Model: b.Model, Provider: b.Provider,
			State: b.State, Started: b.Started, CtxTokens: b.ContextTokens, CtxLimit: b.ContextLimit,
			Input: b.Input, Output: b.Output, Cache: b.CacheRead + b.CacheWrite, USD: b.USD})
	}

	v.Graph = a.boardGraph(opt.Layout)

	liveIn := map[string]int{}
	for _, b := range live {
		if b.State != "done" && b.Office != "" {
			liveIn[b.Office]++
		}
	}
	for _, off := range config.Offices {
		m, origin := a.Cfg.ResolveModel("", "", off, "")
		row := tui.OfficeRow{Name: a.Cfg.Dist.Word(off), Role: officeRoles[off], Model: m.Ref.Model, Fallback: m.Ref.Fallback, Origin: origin.String(), Live: liveIn[off]}
		for _, r := range a.World.Ranks {
			if r.Office == off {
				row.Ranks = append(row.Ranks, a.Cfg.Dist.Word(r.Name))
			}
		}
		v.Offices = append(v.Offices, row)
	}

	recs, rd := src.Usage()
	since, name := board.RangeSince(rng, now)
	rep := board.Rollup(recs, since, name)
	u := tui.UsageView{Range: name, Calls: rep.Total.Calls, Input: rep.Total.Input, Output: rep.Total.Output, Cache: rep.Total.CacheRead + rep.Total.CacheWrite,
		USD: rep.Total.USD, Unpriced: rep.Total.Unpriced}
	if !rd.Lit {
		u.Note = rd.String()
	}
	rows := func(ss []board.UsageSum, word bool) []tui.UsageRow {
		var out []tui.UsageRow
		for _, s := range ss {
			k := s.Key
			if word {
				k = a.Cfg.Dist.Word(k)
			}
			out = append(out, tui.UsageRow{Key: k, Calls: s.Calls, Tokens: s.Tokens(), USD: s.USD, Unpriced: s.Unpriced})
		}
		return out
	}
	u.ByBody, u.ByModel, u.ByOffice, u.ByDay = rows(rep.ByBody, false), rows(rep.ByModel, false), rows(rep.ByOffice, true), rows(rep.ByDay, false)
	v.Usage = u
	return v
}

// boardGraph projects the reasoned ontology: every creature, its one-hop bonds up, and what the
// graph knows about it — its classes, derived chain, territory, minds, facts it can see (the
// flow rules), and the shape findings against it.
func (a *App) boardGraph(layout onto.Layout) tui.GraphView {
	w, err := onto.LoadLayout(a.Root, layout)
	if err != nil {
		return tui.GraphView{Note: "the ontology did not load: " + err.Error()}
	}
	g := w.Graph
	findings := map[string][]string{}
	all := w.Validate()
	for _, f := range all {
		findings[f.Subject.Local()] = append(findings[f.Subject.Local()], f.Msg+" ("+f.Shape+")")
	}
	isP := func(n string) onto.Term { return onto.Is(n) }
	creatures := g.Instances(isP("Creature"))
	ids := map[string]bool{}
	for _, c := range creatures {
		if c.IsIRI() {
			ids[c.Local()] = true
		}
	}
	names := func(ts []onto.Term) []string {
		var out []string
		for _, t := range ts {
			out = append(out, t.Local())
		}
		sort.Strings(out)
		return out
	}
	var nodes []tui.GraphNode
	// the one-hop bonds up, as the web board draws them (an inferred `above` is not a bond)
	col := board.BuildColony(w)
	up := map[string][]tui.GraphBond{}
	rankOf := map[string]string{}
	for _, n := range col.Nodes {
		rankOf[n.ID] = n.Rank
	}
	for _, e := range col.Edges {
		if e.Bond != "wears" && e.Bond != "holds" && ids[e.From] && ids[e.To] {
			up[e.From] = append(up[e.From], tui.GraphBond{To: e.To, Bond: e.Bond})
		}
	}
	// level = the longest bond chain up to a root
	level := map[string]int{}
	var depth func(id string, guard int) int
	depth = func(id string, guard int) int {
		if l, ok := level[id]; ok {
			return l
		}
		if guard > len(ids) {
			return 0
		}
		l := 0
		for _, u := range up[id] {
			if d := depth(u.To, guard+1) + 1; d > l {
				l = d
			}
		}
		level[id] = l
		return l
	}
	triples := g.Len()
	facts := len(g.Instances(isP("Fact")))
	minds := len(g.Instances(isP("Mind")))
	for id := range ids {
		t := isP(id)
		n := tui.GraphNode{ID: id, Name: id, Rank: a.Cfg.Dist.Word(rankOf[id]), Level: depth(id, 0), Up: up[id], Findings: len(findings[id])}
		kv := func(k, v string) {
			if v != "" {
				n.Knowledge = append(n.Knowledge, tui.KV{Key: k, Value: v})
			}
		}
		list := func(k string, vs []string, limit int) {
			for i, v := range vs {
				if i == limit {
					kv("", fmt.Sprintf("… %d more", len(vs)-limit))
					break
				}
				if i == 0 {
					kv(k, v)
				} else {
					kv("", v)
				}
			}
		}
		var classes []string
		for _, c := range g.Objects(t, onto.I(onto.RDFType)) {
			classes = append(classes, c.Local())
		}
		sort.Strings(classes)
		kv("is a", strings.Join(classes, " ⊂ "))
		// the derived chain: every creature above, by the transitive `above`
		var chain []string
		for _, o := range g.Objects(t, isP("above")) {
			chain = append(chain, o.Local())
		}
		sort.Slice(chain, func(i, j int) bool { return depth(chain[i], 0) > depth(chain[j], 0) })
		if len(chain) > 0 {
			kv("chain", strings.Join(chain, " → ")+" (inferred)")
		}
		below := names(g.Subjects(isP("above"), t))
		if len(below) > 0 {
			kv("under it", fmt.Sprintf("%d creature%s", len(below), plural(len(below))))
		}
		for _, d := range g.Objects(t, isP("doc")) {
			for _, p := range g.Objects(d, isP("path")) {
				kv("doc", p.Value)
			}
		}
		var owns []string
		for _, o := range g.Objects(t, isP("owns")) {
			owns = append(owns, o.Value)
		}
		sort.Strings(owns)
		list("owns", owns, 4)
		list("wears", names(g.Objects(t, isP("wears"))), 4)
		known := g.Objects(t, isP("knows"))
		if len(known) > 0 {
			kv("knows", fmt.Sprintf("%d fact%s", len(known), plural(len(known))))
		}
		seen := w.Visible(t)
		if len(seen) > 0 {
			kv("sees", fmt.Sprintf("%d fact%s by the flow rules", len(seen), plural(len(seen))))
			var ls []string
			for _, s := range seen {
				ls = append(ls, s.Kind+": "+oneLineOf(s.Text))
			}
			list("", ls, 3)
		}
		list("finding", findings[id], 3)
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	gv := tui.GraphView{Nodes: nodes,
		Summary: fmt.Sprintf("%d creatures · %d minds · %d facts · %d triples (reasoned) · %d finding%s", len(ids), minds, facts, triples, len(all), plural(len(all)))}
	if len(w.Notes) > 0 {
		gv.Note = fmt.Sprintf("%d hole%s: %s", len(w.Notes), plural(len(w.Notes)), w.Notes[0])
	}
	return gv
}

func oneLineOf(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:79] + "…"
	}
	return s
}
