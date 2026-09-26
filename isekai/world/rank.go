package world

import (
	"fmt"
	"path"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/onto"
	"github.com/Kaginari/agentic-playground/isekai/tool"
)

// Rank is one row of the hierarchy — data, not code (binary.md §Ranks and bodies). The
// built-in table is the law's §The world; config may override a field, add a rank, or replace
// the whole table. Territory, the gate and dispatch read Authors, HoldsGate and Tools; nothing
// below names elf, orc or slime.
type Rank struct {
	Name      string   // canonical id: elf | orc | slime | kijin | high_elf | high_orc | dark_elf | anything from config
	ReportsTo string   // the rank one hop up; "rimuru" at the top
	Job       string   // one line
	Authors   bool     // authors changes in its territory (territory is enforced on it)
	HoldsGate bool     // gates the authoring ranks below it
	Sideways  bool     // may dispatch its own rank (orc ⇄ orc)
	Body      string   // court | keeper
	Office    string   // default office: great-sage | raphael | ciel
	Tools     []string // tool names its shelf is cut to
	Dir       string   // folder under the world dir; "" for a rank with no creatures on disk
	Prefix    string   // creature-id prefix, Dir + "-"
	Base      string   // an ascended rank keeps everything its base held
}

// Rimuru is the root of every table: the session, no dir, every tool.
const Rimuru = "rimuru"

// AllTools is the shelf a rank with no Tools listed gets (rimuru's).
var AllTools = []string{"read", "write", "edit", "bash", "glob", "grep", "law", "dispatch"}

var baseTools = []string{"read", "write", "edit", "bash", "glob", "grep", "law"}

// DefaultRanks is the law's table (isekai.md §The world), dirs and prefixes from the lexicon.
func DefaultRanks(lex Lexicon) []Rank {
	d := func(r string) string { return lex.Ranks[r] }
	p := func(r string) string { return lex.Prefixes[r] }
	return []Rank{
		{Name: "elf", ReportsTo: Rimuru, Job: "the shared mind and voice: thinks across domains, routes work to orcs, speaks back up", Body: "court", Office: "ciel", Tools: AllTools, Dir: d("elf"), Prefix: p("elf")},
		{Name: "orc", ReportsTo: "elf", Job: "rules its domain, commands its slimes, holds the gate", HoldsGate: true, Sideways: true, Body: "court", Office: "raphael", Tools: AllTools, Dir: d("orc"), Prefix: p("orc")},
		{Name: "slime", ReportsTo: "orc", Job: "the ground truth of its zone: authors changes there", Authors: true, Body: "court", Office: "great-sage", Tools: baseTools, Dir: d("slime"), Prefix: p("slime")},
		{Name: "kijin", ReportsTo: Rimuru, Job: "standing domain lead: owns one subsystem end-to-end across sessions", Authors: true, HoldsGate: true, Body: "keeper", Office: "raphael", Tools: AllTools, Dir: d("kijin"), Prefix: p("kijin")},
		{Name: "high_elf", ReportsTo: Rimuru, Job: "ascended elf: the cross-domain traits an elf distilled", Body: "court", Office: "ciel", Tools: AllTools, Dir: d("elf"), Prefix: p("elf"), Base: "elf"},
		{Name: "high_orc", ReportsTo: "elf", Job: "ascended orc: the domain traits an orc distilled", HoldsGate: true, Sideways: true, Body: "court", Office: "raphael", Tools: AllTools, Dir: d("orc"), Prefix: p("orc"), Base: "orc"},
		{Name: "dark_elf", ReportsTo: Rimuru, Job: "the auditor: reviews verdicts and the log, never authors", Body: "keeper", Office: "raphael", Tools: []string{"read", "bash", "glob", "grep", "law"}, Dir: d("dark_elf"), Prefix: p("dark_elf")},
	}
}

// Ranks is a table with lookups.
type Ranks []Rank

// Get finds a rank by name; "rimuru" is the synthetic root, whose shelf is everything. The
// throne holds no office: its calls run on the mount and are journaled with no office label —
// offices belong to dispatched Courts and to the binary's own routed calls.
func (rs Ranks) Get(name string) (Rank, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == Rimuru {
		return Rank{Name: Rimuru, Job: "the session: thinks, decides, orders", Tools: []string{"*"}}, true
	}
	for _, r := range rs {
		if r.Name == name {
			return r, true
		}
	}
	return Rank{}, false
}

// Of reads a creature's rank off its id prefix ("" for rimuru or an unknown name). The first
// rank on a prefix wins (an ascended rank shares its base's).
func (rs Ranks) Of(creature string) string {
	creature = strings.ToLower(creature)
	for _, r := range rs {
		if r.Prefix != "" && strings.HasPrefix(creature, r.Prefix) {
			return r.Name
		}
	}
	return ""
}

// ByDir is the base rank living in a folder.
func (rs Ranks) ByDir(dir string) (Rank, bool) {
	for _, r := range rs {
		if r.Dir == dir && r.Dir != "" {
			return r, true
		}
	}
	return Rank{}, false
}

// Below reports whether rank `lower` sits under `upper` in the tree (any number of hops).
func (rs Ranks) Below(lower, upper string) bool {
	if upper == Rimuru {
		return lower != Rimuru
	}
	seen := map[string]bool{}
	for cur := lower; cur != "" && cur != Rimuru && !seen[cur]; {
		seen[cur] = true
		r, ok := rs.Get(cur)
		if !ok {
			return false
		}
		if r.ReportsTo == upper {
			return true
		}
		cur = r.ReportsTo
	}
	return false
}

// GateAbove reports whether some rank at or above `name` holds a gate.
func (rs Ranks) GateAbove(name string) bool {
	seen := map[string]bool{}
	for cur := name; cur != "" && cur != Rimuru && !seen[cur]; {
		seen[cur] = true
		r, ok := rs.Get(cur)
		if !ok {
			return false
		}
		if r.HoldsGate {
			return true
		}
		cur = r.ReportsTo
	}
	return false
}

// Validate checks the law's shape: a tree rooted at rimuru, every rank with an escalation
// parent, every authoring rank with a gate holder at or above it (Law 3), ascended ranks with
// an existing base, dirs and prefixes consistent.
func (rs Ranks) Validate() error {
	seen := map[string]bool{}
	for _, r := range rs {
		if r.Name == "" || r.Name == Rimuru {
			return fmt.Errorf("rank %q: not a name", r.Name)
		}
		if seen[r.Name] {
			return fmt.Errorf("rank %s listed twice", r.Name)
		}
		seen[r.Name] = true
	}
	for _, r := range rs {
		if r.ReportsTo == "" {
			return fmt.Errorf("rank %s has no escalation parent (Absolute Rule III)", r.Name)
		}
		if _, ok := rs.Get(r.ReportsTo); !ok {
			return fmt.Errorf("rank %s reports to unknown rank %s", r.Name, r.ReportsTo)
		}
		if !rs.Below(r.Name, Rimuru) || !reachesRoot(rs, r.Name) {
			return fmt.Errorf("rank %s does not reach rimuru (the hierarchy is a tree rooted there)", r.Name)
		}
		if r.Authors && !rs.GateAbove(r.Name) {
			return fmt.Errorf("rank %s authors but no rank at or above it holds a gate (Law 3)", r.Name)
		}
		if r.Base != "" {
			if _, ok := rs.Get(r.Base); !ok {
				return fmt.Errorf("ascended rank %s has unknown base %s", r.Name, r.Base)
			}
		}
		if r.Dir != "" && r.Prefix != "" && r.Prefix != r.Dir+"-" {
			return fmt.Errorf("rank %s: prefix %q must be dir %q + \"-\"", r.Name, r.Prefix, r.Dir)
		}
	}
	return nil
}

func reachesRoot(rs Ranks, name string) bool {
	seen := map[string]bool{}
	for cur := name; ; {
		if cur == Rimuru {
			return true
		}
		if seen[cur] {
			return false
		}
		seen[cur] = true
		r, ok := rs.Get(cur)
		if !ok {
			return false
		}
		cur = r.ReportsTo
	}
}

// Layout is the onto layout of this table under a lexicon.
func (rs Ranks) Layout(lex Lexicon) onto.Layout {
	l := onto.Layout{WorldDir: lex.WorldDir, Law: lex.Law, Ranks: []onto.RankDir{}}
	for _, r := range rs {
		if r.Dir == "" {
			continue
		}
		parent := r.ReportsTo
		// an ascended rank's creatures live in the base's dir under the base's class
		if r.Base != "" {
			continue
		}
		l.Ranks = append(l.Ranks, onto.RankDir{Name: r.Name, Dir: r.Dir, Parent: parent})
	}
	return l
}

// ExpandTools resolves a rank's tool list against the names on offer: `*` is every tool, a
// glob (path.Match) picks by pattern, a plain name is itself (kept even when absent, so a
// later add still lands). Order follows the offer for globs.
func ExpandTools(patterns, offer []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, p := range patterns {
		switch {
		case p == "*":
			for _, n := range offer {
				add(n)
			}
		case strings.ContainsAny(p, "*?["):
			for _, n := range offer {
				if ok, _ := path.Match(p, n); ok {
					add(n)
				}
			}
		default:
			add(p)
		}
	}
	return out
}

// Tools is the shelf a rank is cut to: its own list, else its base's, else everything.
func (rs Ranks) Tools(name string) []string {
	r, ok := rs.Get(name)
	if !ok {
		return AllTools
	}
	if len(r.Tools) == 0 && r.Base != "" {
		if b, ok := rs.Get(r.Base); ok && len(b.Tools) > 0 {
			return b.Tools
		}
	}
	if len(r.Tools) == 0 {
		return AllTools
	}
	return r.Tools
}

// OfficeOf reads the office a commission's @ASK names (tool.OfficeOf); no word → great-sage,
// the office that perceives.
func OfficeOf(ask string) string {
	if o := tool.OfficeOf(ask); o != "" {
		return o
	}
	return "great-sage"
}
