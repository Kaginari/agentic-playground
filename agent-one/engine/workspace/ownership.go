package workspace

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/tool"
)

// OwnershipOptions is policy.ownership: an authoring agent's write outside its ownership is
// refused, not warned (Policy 2). Which ranks are held to it is the table's Authors column; Ranks
// narrows it to a list when set.
type OwnershipOptions struct {
	Enabled bool
	Ranks   []string
}

func (o OwnershipOptions) held(w *Workspace, c *Member) bool {
	if len(o.Ranks) > 0 {
		return contains(o.Ranks, c.Rank)
	}
	r, ok := w.Ranks.Get(c.Rank)
	return ok && r.Authors
}

// Policy is the tool.Policy for a agent: every path a write/edit touches must be inside its
// ownership. A refusal carries the escalation hint — one hop up, never sideways.
func (w *Workspace) ToolPolicy(agent string, opt OwnershipOptions) tool.Policy {
	if !opt.Enabled {
		return nil
	}
	return func(a tool.Access) error {
		if a.Class < tool.Write || len(a.Paths) == 0 {
			return nil
		}
		c := w.Member(agent)
		if c == nil || !opt.held(w, c) || (len(c.Ownership) == 0 && c.Dir == "") {
			return nil
		}
		for _, p := range a.Paths {
			rel := w.Rel(p)
			if c.InOwnership(rel) {
				continue
			}
			up := c.Parent
			if up == "" {
				up = "the dispatcher"
			}
			return fmt.Errorf("ownership: %s is outside %s's ownership (%s) — Policy 2: do not write there; escalate with `@? %s` to %s, one hop up",
				rel, c.Name, strings.Join(append(c.Ownership, c.Dir+"/"), ", "), oneLine("needs "+rel+" which "+c.Name+" does not own"), up)
		}
		return nil
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
