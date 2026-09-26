package config

import (
	"fmt"
	"sort"
	"strings"
)

// Action is a permission rule's verdict.
type Action string

const (
	Allow Action = "allow"
	Ask   Action = "ask"
	Deny  Action = "deny"
)

// pathTools match their subject as a slash path: `*` stops at `/`, `**` crosses it.
var pathTools = map[string]bool{"read": true, "write": true, "edit": true, "multiedit": true, "patch": true, "glob": true, "grep": true, "ls": true}

// Specificity orders matching rules: an exact tool beats `*`; more literal characters beat
// fewer; fewer `*`/`**` runs beat more; fewer `?` beat more. Ties fall to deny > ask >
// allow, then to the later rule.
type Specificity struct {
	ToolExact int // 1 when the rule names the tool, 0 for `*`
	Literal   int // non-wildcard runes in the pattern
	Wildcards int // `*` and `**` runs in the pattern
	Singles   int // `?` tokens in the pattern
}

func (s Specificity) beats(o Specificity) bool {
	if s.ToolExact != o.ToolExact {
		return s.ToolExact > o.ToolExact
	}
	if s.Literal != o.Literal {
		return s.Literal > o.Literal
	}
	if s.Wildcards != o.Wildcards {
		return s.Wildcards < o.Wildcards
	}
	return s.Singles < o.Singles
}

// Specificity computes a rule's rank.
func (r *PermRule) Specificity() Specificity {
	s := Specificity{}
	if r.Tool != "*" {
		s.ToolExact = 1
	}
	p := r.Pattern
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '*':
			s.Wildcards++
			if i+1 < len(p) && p[i+1] == '*' {
				i++
			}
		case '?':
			s.Singles++
		default:
			s.Literal++
		}
	}
	return s
}

// wildcardOnly is true for `*`, `**`, `*?*` and every pattern with no literal character.
func wildcardOnly(p string) bool {
	return p != "" && strings.Trim(p, "*?") == ""
}

var actionRank = map[Action]int{Deny: 3, Ask: 2, Allow: 1}

// Decide answers the gate for one act: the most specific matching rule wins, deny beats
// approval on a tie, and with no match the class default holds — read and write allow (the
// gate applies `strict` itself when rule is nil), outward and destructive ask. A full gate
// off (law.humanGate.enabled: false) allows everything but a matching deny.
func (c *Config) Decide(tool, input, class string) (Action, *PermRule) {
	var best *PermRule
	var bestSpec Specificity
	if c.Permissions.Enabled {
		for _, r := range c.Permissions.Rules {
			if r.Tool != "*" && r.Tool != tool {
				continue
			}
			if !Match(r.Pattern, input, pathTools[tool]) {
				continue
			}
			spec := r.Specificity()
			switch {
			case best == nil, spec.beats(bestSpec):
				best, bestSpec = r, spec
			case bestSpec.beats(spec):
			case actionRank[Action(r.Action)] >= actionRank[Action(best.Action)]:
				best, bestSpec = r, spec
			}
		}
	}
	if best != nil {
		return Action(best.Action), best
	}
	if !c.Law.HumanGate.Enabled {
		return Allow, nil
	}
	return ClassDefault(class), nil
}

// ClassDefault is the gate's answer with no rule: read/write allow, outward/destructive ask.
func ClassDefault(class string) Action {
	switch class {
	case "outward", "destructive":
		return Ask
	}
	return Allow
}

// Match is the rule glob: `*` any run (not `/` in path mode), `**` any run, `?` one rune.
// A `**/` prefix also matches at the root, so `**/*.lock` matches `go.lock`.
func Match(pattern, s string, pathMode bool) bool {
	if pathMode && strings.HasPrefix(pattern, "**/") && match(pattern[3:], s, pathMode) {
		return true
	}
	return match(pattern, s, pathMode)
}

func match(p, s string, pathMode bool) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			deep := strings.HasPrefix(p, "**")
			rest := strings.TrimLeft(p, "*")
			if rest == "" {
				return deep || !pathMode || !strings.Contains(s, "/")
			}
			for i := 0; i <= len(s); i++ {
				if match(rest, s[i:], pathMode) {
					return true
				}
				if i < len(s) && pathMode && !deep && s[i] == '/' {
					return false
				}
			}
			return false
		case '?':
			if s == "" || (pathMode && s[0] == '/') {
				return false
			}
			p, s = p[1:], s[1:]
		default:
			if s == "" || s[0] != p[0] {
				return false
			}
			p, s = p[1:], s[1:]
		}
	}
	return s == ""
}

// checkPermissions splits `match`, validates actions and applies the wildcard refusal.
func (c *Config) checkPermissions() error {
	for i, r := range c.Permissions.Rules {
		path := fmt.Sprintf("permissions.rules[%d]", i)
		r.Origin, _ = c.Origins[path]
		tool, pat, ok := strings.Cut(r.Match, ":")
		if !ok || tool == "" || pat == "" {
			return fmt.Errorf("%s: %s.match: %q is not <tool>:<glob>", c.Where(path), path, r.Match)
		}
		r.Tool, r.Pattern = strings.TrimSpace(tool), strings.TrimSpace(pat)
		if r.Tool == "question" {
			r.Tool = "ask"
		}
		switch Action(r.Action) {
		case Allow, Ask, Deny:
		default:
			return fmt.Errorf("%s: %s.action: %q is not allow, ask or deny", c.Where(path), path, r.Action)
		}
		if _, known := Builtins[r.Tool]; !known && r.Tool != "*" && c.Tools.Custom[r.Tool] == nil && !strings.HasPrefix(r.Tool, "mcp__") {
			c.Holes = append(c.Holes, fmt.Sprintf("%s names no tool %q — %s", path, r.Tool, c.Where(path)))
		}
		floor := Builtins[r.Tool]
		if Action(r.Action) == Allow && wildcardOnly(r.Pattern) && (OutwardCapable[r.Tool] || floor == "outward" || floor == "destructive") {
			return fmt.Errorf("%s: %s: allow %q on %s is an auto-approve mode — pre-approve a pattern, never everything", c.Where(path), path, r.Pattern, r.Tool)
		}
	}
	return nil
}

// Loosenings lists every rule and setting that silences a question the gate would ask:
// `allow` on an outward-capable tool, standing class approvals, and a gate switched off.
func (c *Config) Loosenings() []string {
	var out []string
	if !c.Law.HumanGate.Enabled {
		out = append(out, "law.humanGate.enabled: false — "+c.Where("law.humanGate.enabled"))
	}
	for i, a := range c.Law.HumanGate.Approve {
		out = append(out, fmt.Sprintf("law.humanGate.approve: %s — %s", a, c.Where(fmt.Sprintf("law.humanGate.approve[%d]", i))))
	}
	for _, r := range c.Permissions.Rules {
		if Action(r.Action) == Allow && (OutwardCapable[r.Tool] || Builtins[r.Tool] == "outward") {
			out = append(out, fmt.Sprintf("allow %s:%s — %s", r.Tool, r.Pattern, r.Origin))
		}
	}
	sort.Strings(out[boolInt(!c.Law.HumanGate.Enabled):])
	return out
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
