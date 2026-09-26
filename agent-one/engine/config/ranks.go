package config

import (
	"fmt"
	"sort"
	"strings"
)

// RankDef is a rank as config writes it; every field is optional and overrides the built-in
// table's field of the same name (rankSet: extend) or defines a new rank outright.
type RankDef struct {
	ReportsTo    string   `json:"reportsTo"`
	Job          string   `json:"job"`
	Authors      *bool    `json:"authors"`
	HoldsGate    *bool    `json:"holdsGate"`
	Agent        string   `json:"agent"` // ephemeral | persistent
	Role         string   `json:"role"`
	Tools        []string `json:"tools"` // globs over tool names
	Model        ModelRef `json:"model"`
	Dir          string   `json:"dir"`
	Prefix       string   `json:"prefix"`
	PromotesFrom string   `json:"promotesFrom"`
	Sideways     *bool    `json:"sideways"` // may speak to its own rank (domain ⇄ domain)
}

// Rank is one resolved rank with the origin of each field ("policy" for the built-in table).
type Rank struct {
	Name         string
	ReportsTo    string
	Job          string
	Authors      bool
	HoldsGate    bool
	Agent        string
	Role         string
	Tools        []string
	Model        ModelRef
	Dir          string
	Prefix       string
	PromotesFrom string
	Sideways     bool
	Origins      map[string]string // field → origin
	Builtin      bool
}

// RootRank is the session's rank: the tree's root, never configured.
const RootRank = "orchestrator"

// builtinRanks is AGENT-ONE.md §The workspace as data.
var builtinRanks = []Rank{
	{Name: "coord", ReportsTo: "orchestrator", Job: "the shared skill and voice: thinks across domains, routes work to domain owners, speaks back up", Agent: "ephemeral", Role: "drafter", Tools: []string{"*"}, Dir: "coord", Prefix: "coord-"},
	{Name: "domain", ReportsTo: "coord", Job: "rules its domain, holds the gate, commands and validates its zone workers", HoldsGate: true, Sideways: true, Agent: "ephemeral", Role: "judge", Tools: []string{"*"}, Dir: "domain", Prefix: "domain-"},
	{Name: "zone", ReportsTo: "domain", Job: "the ground truth of its zone; authors changes there", Authors: true, Agent: "ephemeral", Role: "analyst", Tools: []string{"*"}, Dir: "zone", Prefix: "zone-"},
	{Name: "service", ReportsTo: "orchestrator", Job: "a standing domain lead owning one subsystem end-to-end across sessions", Authors: true, HoldsGate: true, Agent: "persistent", Role: "judge", Tools: []string{"*"}, Dir: "service", Prefix: "service-"},
	{Name: "principal-coordinator", PromotesFrom: "coord", Job: "promoted coord: holds the accumulated cross-domain invariants"},
	{Name: "principal-domain-owner", PromotesFrom: "domain", Job: "promoted domain: holds the accumulated domain invariants"},
	{Name: "auditor", ReportsTo: "orchestrator", Job: "the auditor: reviews gate verdicts and log.md for policy violations; never authors", Agent: "ephemeral", Role: "judge", Tools: []string{"read", "glob", "grep", "ls", "recall", "onto", "toolbox", "git", "ask"}, Dir: "auditor", Prefix: "auditor-"},
}

// RankNames lists the built-in ranks in policy order.
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
				rr.Origins[f] = "policy"
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
		set("agent", func() { r.Agent = def.Agent })
		set("role", func() { r.Role = def.Role })
		set("tools", func() { r.Tools = def.Tools })
		set("model", func() { r.Model = def.Model })
		set("dir", func() { r.Dir = def.Dir })
		set("prefix", func() { r.Prefix = def.Prefix })
		set("promotesFrom", func() { r.PromotesFrom = def.PromotesFrom })
		set("sideways", func() { r.Sideways = *def.Sideways })
		if def.Agent != "" && def.Agent != "ephemeral" && def.Agent != "persistent" {
			return fmt.Errorf("%s: ranks.%s.agent: %q is not ephemeral or persistent", c.Where("ranks."+name+".agent"), name, def.Agent)
		}
		// models.ranks.<r> and ranks.<r>.model are one slot.
		if mr, ok := c.Models.Ranks[name]; ok && def.Model.Model != "" && mr.Model != def.Model.Model {
			return fmt.Errorf("%s: ranks.%s.model %q conflicts with models.ranks.%s %q (%s) — one slot, one value",
				c.Where("ranks."+name+".model"), name, def.Model.Model, name, mr.Model, c.Where("models.ranks."+name))
		}
	}
	// Promoted ranks inherit every base field they did not set; dropping one is refused.
	for _, name := range order {
		r := byName[name]
		if r.PromotesFrom == "" {
			continue
		}
		base, ok := byName[r.PromotesFrom]
		if !ok {
			return fmt.Errorf("ranks.%s.promotesFrom: no rank %q (%s)", name, r.PromotesFrom, c.rankWhere(name, "promotesFrom"))
		}
		if base.PromotesFrom != "" {
			return fmt.Errorf("ranks.%s.promotesFrom: %q is itself promoted", name, r.PromotesFrom)
		}
		// A field config did not set is inherited; a held capability config drops is refused.
		inherit := func(field string, apply func()) {
			if o := r.Origins[field]; o == "" || o == "policy" {
				apply()
				r.Origins[field] = "promotes:" + base.Name
			}
		}
		inherit("reportsTo", func() { r.ReportsTo = base.ReportsTo })
		inherit("authors", func() { r.Authors = base.Authors })
		inherit("holdsGate", func() { r.HoldsGate = base.HoldsGate })
		inherit("sideways", func() { r.Sideways = base.Sideways })
		inherit("agent", func() { r.Agent = base.Agent })
		inherit("role", func() { r.Role = base.Role })
		inherit("tools", func() { r.Tools = base.Tools })
		inherit("dir", func() { r.Dir = base.Dir })
		inherit("prefix", func() { r.Prefix = base.Prefix })
		inherit("model", func() { r.Model = base.Model })
		for f, dropped := range map[string]bool{"authors": base.Authors && !r.Authors, "holdsGate": base.HoldsGate && !r.HoldsGate, "sideways": base.Sideways && !r.Sideways} {
			if dropped {
				return fmt.Errorf("ranks.%s.%s: an promoted rank keeps everything %s held (%s)", name, f, base.Name, r.Origins[f])
			}
		}
	}
	// Defaults for new ranks.
	for _, name := range order {
		r := byName[name]
		if r.Agent == "" {
			r.Agent = "ephemeral"
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
	// The policy's shape.
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
			return fmt.Errorf("ranks.%s authors changes but no rank above it holds a gate (Policy 3: nothing lands without the gate)", name)
		}
	}
	c.ranks = make([]Rank, 0, len(order))
	for _, name := range order {
		c.ranks = append(c.ranks, *byName[name])
	}
	return nil
}

var rankFields = []string{"reportsTo", "job", "authors", "holdsGate", "agent", "role", "tools", "model", "dir", "prefix", "promotesFrom", "sideways"}

func (c *Config) rankWhere(name, field string) string { return c.Where("ranks." + name + "." + field) }

// Ranks returns the resolved hierarchy in policy order then config order; Orchestrator is implicit.
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

// Deviations lists every rank field whose value did not come from the policy's table, and every
// rank the policy does not have; empty when the workspace runs on the policy as written.
func (c *Config) Deviations() []string {
	var out []string
	if c.RankSet == "replace" {
		out = append(out, "rankSet: replace — the policy's table is not loaded ("+c.Where("rankSet")+")")
	}
	for _, r := range c.ranks {
		if !r.Builtin {
			out = append(out, fmt.Sprintf("rank %s: not in the policy (%s)", r.Name, c.Where("ranks."+r.Name)))
			continue
		}
		fields := sortedKeys(r.Origins)
		for _, f := range fields {
			o := r.Origins[f]
			if o != "policy" && !strings.HasPrefix(o, "promotes:") {
				out = append(out, fmt.Sprintf("rank %s.%s: %s", r.Name, f, o))
			}
		}
	}
	return out
}

// RankTree renders the hierarchy as an indented tree under the orchestrator.
func (c *Config) RankTree() string {
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
			if r.PromotesFrom != "" {
				marks = append(marks, "promotes "+r.PromotesFrom)
			}
			marks = append(marks, r.Agent)
			if r.Role != "" {
				marks = append(marks, "role "+r.Role)
			}
			fmt.Fprintf(&b, "%s%s (%s)\n", pad, ch, strings.Join(marks, ", "))
			walk(ch, pad+"  ")
		}
	}
	fmt.Fprintf(&b, "%s [rankSet: %s]\n", RootRank, c.RankSet)
	walk(RootRank, "  ")
	return b.String()
}
