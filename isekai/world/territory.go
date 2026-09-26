package world

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/tool"
)

// TerritoryOptions is law.territory: an authoring body's write outside its territory is
// refused, not warned (Law 2). Which ranks are held to it is the table's Authors column; Ranks
// narrows it to a list when set.
type TerritoryOptions struct {
	Enabled bool
	Ranks   []string
}

func (o TerritoryOptions) held(w *World, c *Creature) bool {
	if len(o.Ranks) > 0 {
		return contains(o.Ranks, c.Rank)
	}
	r, ok := w.Ranks.Get(c.Rank)
	return ok && r.Authors
}

// Policy is the tool.Policy for a body: every path a write/edit touches must be inside its
// territory. A refusal carries the escalation hint — one hop up, never sideways.
func (w *World) Policy(body string, opt TerritoryOptions) tool.Policy {
	if !opt.Enabled {
		return nil
	}
	return func(a tool.Access) error {
		if a.Class < tool.Write || len(a.Paths) == 0 {
			return nil
		}
		c := w.Creature(body)
		if c == nil || !opt.held(w, c) || (len(c.Territory) == 0 && c.Dir == "") {
			return nil
		}
		for _, p := range a.Paths {
			rel := w.Rel(p)
			if c.InTerritory(rel) {
				continue
			}
			up := c.Parent
			if up == "" {
				up = "the dispatcher"
			}
			return fmt.Errorf("territory: %s is outside %s's territory (%s) — Law 2: do not write there; escalate with `@? %s` to %s, one hop up",
				rel, c.Name, strings.Join(append(c.Territory, c.Dir+"/"), ", "), oneLine("needs "+rel+" which "+c.Name+" does not own"), up)
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
