package config

import (
	"fmt"
	"sort"
	"strings"
)

// RankDef is a rank as config writes it; every field is optional and overrides the built-in
// table's field of the same name (rankSet: extend) or defines a new rank outright.
type RankDef struct {
	ReportsTo   string   `json:"reportsTo"`
	Job         string   `json:"job"`
	Authors     *bool    `json:"authors"`
	HoldsGate   *bool    `json:"holdsGate"`
	Body        string   `json:"body"` // court | keeper
	Office      string   `json:"office"`
	Tools       []string `json:"tools"` // globs over tool names
	Model       ModelRef `json:"model"`
	Dir         string   `json:"dir"`
	Prefix      string   `json:"prefix"`
	AscendsFrom string   `json:"ascendsFrom"`
	Sideways    *bool    `json:"sideways"` // may speak to its own rank (orc ⇄ orc)
}

// Rank is one resolved rank with the origin of each field ("law" for the built-in table).
type Rank struct {
	Name        string
	ReportsTo   string
	Job         string
	Authors     bool
	HoldsGate   bool
	Body        string
	Office      string
	Tools       []string
	Model       ModelRef
	Dir         string
	Prefix      string
	AscendsFrom string
	Sideways    bool
	Origins     map[string]string // field → origin
	Builtin     bool
}

// RootRank is the session's rank: the tree's root, never configured.
const RootRank = "rimuru"

// builtinRanks is isekai.md §The world as data.
var builtinRanks = []Rank{
	{Name: "elf", ReportsTo: "rimuru", Job: "the shared mind and voice: thinks across domains, routes work to orcs, speaks back up", Body: "court", Office: "ciel", Tools: []string{"*"}, Dir: "elf", Prefix: "elf-"},
	{Name: "orc", ReportsTo: "elf", Job: "rules its domain, holds the gate, commands and validates its slimes", HoldsGate: true, Sideways: true, Body: "court", Office: "raphael", Tools: []string{"*"}, Dir: "orc", Prefix: "orc-"},
	{Name: "slime", ReportsTo: "orc", Job: "the ground truth of its zone; authors changes there", Authors: true, Body: "court", Office: "great-sage", Tools: []string{"*"}, Dir: "slime", Prefix: "slime-"},
	{Name: "kijin", ReportsTo: "rimuru", Job: "a standing domain lead owning one subsystem end-to-end across sessions", Authors: true, HoldsGate: true, Body: "keeper", Office: "raphael", Tools: []string{"*"}, Dir: "kijin", Prefix: "kijin-"},
	{Name: "high-elf", AscendsFrom: "elf", Job: "ascended elf: holds the accumulated cross-domain traits"},
	{Name: "high-orc", AscendsFrom: "orc", Job: "ascended orc: holds the accumulated domain traits"},
	{Name: "dark-elf", ReportsTo: "rimuru", Job: "the auditor: reviews gate verdicts and log.md for law violations; never authors", Body: "court", Office: "raphael", Tools: []string{"read", "glob", "grep", "ls", "recall", "onto", "toolbox", "git", "ask"}, Dir: "dark-elf", Prefix: "dark-elf-"},
}

// RankNames lists the built-in ranks in law order.
func RankNames() []string {
	var out []string
	for _, r := range builtinRanks {
		out = append(out, r.Name)
	}
	return out
}

func (c *Config) buildRanks() error {
	byName := map[string]*Rank{}
	var order []string
	if c.RankSet == "" {
		c.RankSet = "extend"
	}
	if c.RankSet != "extend" && c.RankSet != "replace" {
		return fmt.Errorf("%s: rankSet: %q is not extend or replace", c.Where("rankSet"), c.RankSet)
	}
	if c.RankSet == "extend" {
		for _, r := range builtinRanks {
			rr := r
			rr.Builtin = true
			rr.Origins = map[string]string{}
			for _, f := range rankFields {
				rr.Origins[f] = "law"
			}
			byName[rr.Name] = &rr
			order = append(order, rr.Name)
		}
	}
	for _, name := range sortedKeys(c.RankDefs) {
		if strings.EqualFold(name, RootRank) {
			return fmt.Errorf("%s: ranks.%s: the root rank is the session and is not configured", c.Where("ranks."+name), name)
		}
		def := c.RankDefs[name]
		r, ok := byName[name]
		if !ok {
			r = &Rank{Name: name, Origins: map[string]string{}}
			byName[name] = r
			order = append(order, name)
		}
		set := func(field string, apply func()) {
			path := "ranks." + name + "." + field
			if _, ok := c.Origins[path]; ok {
				apply()
				r.Origins[field] = c.Where(path)
			}
		}
		set("reportsTo", func() { r.ReportsTo = def.ReportsTo })
		set("job", func() { r.Job = def.Job })
		set("authors", func() { r.Authors = *def.Authors })
		set("holdsGate", func() { r.HoldsGate = *def.HoldsGate })
		set("body", func() { r.Body = def.Body })
		set("office", func() { r.Office = def.Office })
		set("tools", func() { r.Tools = def.Tools })
		set("model", func() { r.Model = def.Model })
		set("dir", func() { r.Dir = def.Dir })
		set("prefix", func() { r.Prefix = def.Prefix })
		set("ascendsFrom", func() { r.AscendsFrom = def.AscendsFrom })
		set("sideways", func() { r.Sideways = *def.Sideways })
		if def.Body != "" && def.Body != "court" && def.Body != "keeper" {
			return fmt.Errorf("%s: ranks.%s.body: %q is not court or keeper", c.Where("ranks."+name+".body"), name, def.Body)
		}
		// models.ranks.<r> and ranks.<r>.model are one slot.
		if mr, ok := c.Models.Ranks[name]; ok && def.Model.Model != "" && mr.Model != def.Model.Model {
			return fmt.Errorf("%s: ranks.%s.model %q conflicts with models.ranks.%s %q (%s) — one slot, one value",
				c.Where("ranks."+name+".model"), name, def.Model.Model, name, mr.Model, c.Where("models.ranks."+name))
		}
	}
	// Ascended ranks inherit every base field they did not set; dropping one is refused.
	for _, name := range order {
		r := byName[name]
		if r.AscendsFrom == "" {
			continue
		}
		base, ok := byName[r.AscendsFrom]
		if !ok {
			return fmt.Errorf("ranks.%s.ascendsFrom: no rank %q (%s)", name, r.AscendsFrom, c.rankWhere(name, "ascendsFrom"))
		}
		if base.AscendsFrom != "" {
			return fmt.Errorf("ranks.%s.ascendsFrom: %q is itself ascended", name, r.AscendsFrom)
		}
		// A field config did not set is inherited; a held capability config drops is refused.
		inherit := func(field string, apply func()) {
			if o := r.Origins[field]; o == "" || o == "law" {
				apply()
				r.Origins[field] = "ascends:" + base.Name
			}
		}
		inherit("reportsTo", func() { r.ReportsTo = base.ReportsTo })
		inherit("authors", func() { r.Authors = base.Authors })
		inherit("holdsGate", func() { r.HoldsGate = base.HoldsGate })
		inherit("sideways", func() { r.Sideways = base.Sideways })
		inherit("body", func() { r.Body = base.Body })
		inherit("office", func() { r.Office = base.Office })
		inherit("tools", func() { r.Tools = base.Tools })
		inherit("dir", func() { r.Dir = base.Dir })
		inherit("prefix", func() { r.Prefix = base.Prefix })
		inherit("model", func() { r.Model = base.Model })
		for f, dropped := range map[string]bool{"authors": base.Authors && !r.Authors, "holdsGate": base.HoldsGate && !r.HoldsGate, "sideways": base.Sideways && !r.Sideways} {
			if dropped {
				return fmt.Errorf("ranks.%s.%s: an ascended rank keeps everything %s held (%s)", name, f, base.Name, r.Origins[f])
			}
		}
	}
	// Defaults for new ranks.
	for _, name := range order {
		r := byName[name]
		if r.Body == "" {
			r.Body = "court"
		}
		if len(r.Tools) == 0 {
			r.Tools = []string{"*"}
		}
		if r.Dir == "" {
			r.Dir = name
		}
		if r.Prefix == "" {
			r.Prefix = name + "-"
		}
	}
	// The law's shape.
	for _, name := range order {
		r := byName[name]
		if r.ReportsTo == "" {
			return fmt.Errorf("ranks.%s.reportsTo: every rank has an escalation parent (Absolute Rule III)", name)
		}
		if r.ReportsTo != RootRank {
			if _, ok := byName[r.ReportsTo]; !ok {
				return fmt.Errorf("ranks.%s.reportsTo: no rank %q (%s)", name, r.ReportsTo, c.rankWhere(name, "reportsTo"))
			}
		}
	}
	for _, name := range order {
		seen := map[string]bool{name: true}
		gateAbove := false
		for cur := byName[name]; cur.ReportsTo != RootRank; {
			next := byName[cur.ReportsTo]
			if seen[next.Name] {
				return fmt.Errorf("ranks: a cycle through %s — the hierarchy is a tree rooted at %s", next.Name, RootRank)
			}
			seen[next.Name] = true
			if next.HoldsGate {
				gateAbove = true
			}
			cur = next
		}
		r := byName[name]
		if r.Authors && !gateAbove && !r.HoldsGate {
			return fmt.Errorf("ranks.%s authors changes but no rank above it holds a gate (Law 3: nothing lands without the gate)", name)
		}
	}
	c.ranks = make([]Rank, 0, len(order))
	for _, name := range order {
		c.ranks = append(c.ranks, *byName[name])
	}
	return nil
}

var rankFields = []string{"reportsTo", "job", "authors", "holdsGate", "body", "office", "tools", "model", "dir", "prefix", "ascendsFrom", "sideways"}

func (c *Config) rankWhere(name, field string) string { return c.Where("ranks." + name + "." + field) }

// Ranks returns the resolved hierarchy in law order then config order; Rimuru is implicit.
func (c *Config) Ranks() []Rank { return append([]Rank(nil), c.ranks...) }

// Rank finds one rank by name.
func (c *Config) Rank(name string) (Rank, bool) {
	for _, r := range c.ranks {
		if r.Name == name {
			return r, true
		}
	}
	return Rank{}, false
}

// Deviations lists every rank field whose value did not come from the law's table, and every
// rank the law does not have; empty when the world runs on the law as written.
func (c *Config) Deviations() []string {
	var out []string
	if c.RankSet == "replace" {
		out = append(out, "rankSet: replace — the law's table is not loaded ("+c.Where("rankSet")+")")
	}
	for _, r := range c.ranks {
		if !r.Builtin {
			out = append(out, fmt.Sprintf("rank %s: not in the law (%s)", c.Dist.Word(r.Name), c.Where("ranks."+r.Name)))
			continue
		}
		fields := sortedKeys(r.Origins)
		for _, f := range fields {
			o := r.Origins[f]
			if o != "law" && !strings.HasPrefix(o, "ascends:") {
				out = append(out, fmt.Sprintf("rank %s.%s: %s", c.Dist.Word(r.Name), f, o))
			}
		}
	}
	return out
}

// RankTree renders the hierarchy as an indented tree under rimuru, in the distribution's words.
func (c *Config) RankTree() string {
	word := c.Dist.Word
	children := map[string][]string{}
	for _, r := range c.ranks {
		children[r.ReportsTo] = append(children[r.ReportsTo], r.Name)
	}
	for k := range children {
		sort.Strings(children[k])
	}
	var b strings.Builder
	var walk func(name, pad string)
	walk = func(name, pad string) {
		for _, ch := range children[name] {
			r, _ := c.Rank(ch)
			var marks []string
			if r.Authors {
				marks = append(marks, "authors")
			}
			if r.HoldsGate {
				marks = append(marks, "gate")
			}
			if r.Sideways {
				marks = append(marks, "sideways")
			}
			if r.AscendsFrom != "" {
				marks = append(marks, "ascends "+word(r.AscendsFrom))
			}
			marks = append(marks, r.Body)
			if r.Office != "" {
				marks = append(marks, "office "+word(r.Office))
			}
			fmt.Fprintf(&b, "%s%s (%s)\n", pad, word(ch), strings.Join(marks, ", "))
			walk(ch, pad+"  ")
		}
	}
	fmt.Fprintf(&b, "%s [rankSet: %s]\n", word(RootRank), c.RankSet)
	walk(RootRank, "  ")
	return b.String()
}
