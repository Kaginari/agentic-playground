package workspace

import (
	"fmt"
	"path"
	"strings"

	"github.com/Kaginari/agent-one/onto"
	"github.com/Kaginari/agent-one/tool"
)

// Rank is one row of the hierarchy — data, not code (binary.md §Ranks and agents). The
// built-in table is the policy's §The workspace; config may override a field, add a rank, or replace
// the whole table. Ownership, the gate and dispatch read Authors, HoldsGate and Tools; nothing
// below names coord, domain or zone worker.
type Rank struct {
	Name      string   // canonical id: coord | domain | zone | service | principal_coordinator | principal_domain_owner | auditor | anything from config
	ReportsTo string   // the rank one hop up; "orchestrator" at the top
	Job       string   // one line
	Authors   bool     // authors changes in its ownership (ownership is enforced on it)
	HoldsGate bool     // gates the authoring ranks below it
	Sideways  bool     // may dispatch its own rank (domain ⇄ domain)
	Agent     string   // ephemeral | persistent
	Role      string   // default role: analyst | judge | drafter
	Tools     []string // tool names its shelf is cut to
	Dir       string   // folder under the workspace dir; "" for a rank with no members on disk
	Prefix    string   // member-id prefix, Dir + "-"
	Base      string   // an promoted rank keeps everything its base held
}

// Orchestrator is the root of every table: the session, no dir, every tool.
const Orchestrator = "orchestrator"

// AllTools is the shelf a rank with no Tools listed gets (orchestrator's).
var AllTools = []string{"read", "write", "edit", "bash", "glob", "grep", "policy", "dispatch"}

var baseTools = []string{"read", "write", "edit", "bash", "glob", "grep", "policy"}

// DefaultRanks is the policy's table (AGENT-ONE.md §The workspace), dirs and prefixes from the lexicon.
func DefaultRanks(lex Lexicon) []Rank {
	d := func(r string) string { return lex.Ranks[r] }
	p := func(r string) string { return lex.Prefixes[r] }
	return []Rank{
		{Name: "coord", ReportsTo: Orchestrator, Job: "the shared skill and voice: thinks across domains, routes work to domain owners, speaks back up", Agent: "ephemeral", Role: "drafter", Tools: AllTools, Dir: d("coord"), Prefix: p("coord")},
		{Name: "domain", ReportsTo: "coord", Job: "rules its domain, commands its zone workers, holds the gate", HoldsGate: true, Sideways: true, Agent: "ephemeral", Role: "judge", Tools: AllTools, Dir: d("domain"), Prefix: p("domain")},
		{Name: "zone", ReportsTo: "domain", Job: "the ground truth of its zone: authors changes there", Authors: true, Agent: "ephemeral", Role: "analyst", Tools: baseTools, Dir: d("zone"), Prefix: p("zone")},
		{Name: "service", ReportsTo: Orchestrator, Job: "standing domain lead: owns one subsystem end-to-end across sessions", Authors: true, HoldsGate: true, Agent: "persistent", Role: "judge", Tools: AllTools, Dir: d("service"), Prefix: p("service")},
		{Name: "principal_coordinator", ReportsTo: Orchestrator, Job: "promoted coord: the cross-domain invariants an coordinator consolidated", Agent: "ephemeral", Role: "drafter", Tools: AllTools, Dir: d("coord"), Prefix: p("coord"), Base: "coord"},
		{Name: "principal_domain_owner", ReportsTo: "coord", Job: "promoted domain: the domain invariants an domain owner consolidated", HoldsGate: true, Sideways: true, Agent: "ephemeral", Role: "judge", Tools: AllTools, Dir: d("domain"), Prefix: p("domain"), Base: "domain"},
		{Name: "auditor", ReportsTo: Orchestrator, Job: "the auditor: reviews verdicts and the log, never authors", Agent: "persistent", Role: "judge", Tools: []string{"read", "bash", "glob", "grep", "policy"}, Dir: d("auditor"), Prefix: p("auditor")},
	}
}

// Ranks is a table with lookups.
type Ranks []Rank

// Get finds a rank by name; "orchestrator" is the synthetic root, whose shelf is everything. The
// orchestrator holds no role: its calls run on the mount and are journaled with no role label —
// roles belong to dispatched Subagents and to the binary's own routed calls.
func (rs Ranks) Get(name string) (Rank, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == Orchestrator {
		return Rank{Name: Orchestrator, Job: "the session: thinks, decides, orders", Tools: []string{"*"}}, true
	}
	for _, r := range rs {
		if r.Name == name {
			return r, true
		}
	}
	return Rank{}, false
}

// Of reads a member's rank off its id prefix ("" for orchestrator or an unknown name). The first
// rank on a prefix wins (an promoted rank shares its base's).
func (rs Ranks) Of(member string) string {
	member = strings.ToLower(member)
	for _, r := range rs {
		if r.Prefix != "" && strings.HasPrefix(member, r.Prefix) {
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
	if upper == Orchestrator {
		return lower != Orchestrator
	}
	seen := map[string]bool{}
	for cur := lower; cur != "" && cur != Orchestrator && !seen[cur]; {
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
	for cur := name; cur != "" && cur != Orchestrator && !seen[cur]; {
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

// Validate checks the policy's shape: a tree rooted at orchestrator, every rank with an escalation
// parent, every authoring rank with a gate holder at or above it (Policy 3), promoted ranks with
// an existing base, dirs and prefixes consistent.
func (rs Ranks) Validate() error {
	seen := map[string]bool{}
	for _, r := range rs {
		if r.Name == "" || r.Name == Orchestrator {
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
		if !rs.Below(r.Name, Orchestrator) || !reachesRoot(rs, r.Name) {
			return fmt.Errorf("rank %s does not reach orchestrator (the hierarchy is a tree rooted there)", r.Name)
		}
		if r.Authors && !rs.GateAbove(r.Name) {
			return fmt.Errorf("rank %s authors but no rank at or above it holds a gate (Policy 3)", r.Name)
		}
		if r.Base != "" {
			if _, ok := rs.Get(r.Base); !ok {
				return fmt.Errorf("promoted rank %s has unknown base %s", r.Name, r.Base)
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
		if cur == Orchestrator {
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
	l := onto.Layout{WorkspaceDir: lex.WorkspaceDir, Policy: lex.Policy, Ranks: []onto.RankDir{}}
	for _, r := range rs {
		if r.Dir == "" {
			continue
		}
		parent := r.ReportsTo
		// an promoted rank's members live in the base's dir under the base's class
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

// RoleOf reads the role a commission's @ASK names (tool.RoleOf); no word → analyst,
// the role that perceives.
func RoleOf(ask string) string {
	if o := tool.RoleOf(ask); o != "" {
		return o
	}
	return "analyst"
}
