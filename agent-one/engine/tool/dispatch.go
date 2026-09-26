package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Commission is what the dispatch tool hands its runner: a agent to register and the wire
// commission it carries (@ROOT @SCOPE @ASK @CAP).
type Commission struct {
	Agent  string
	Ask    string
	Scope  string
	Root   string
	Unsaid bool
	Cap    int
	Depth  int    // the Subagent's nesting depth (1 for a agent dispatched by the session)
	Role   string // the role the ask names: findings → analyst, verdict → judge, draft → drafter
}

// RoleOf reads the role off an @ASK word (the triad: perceive, judge, speak); "" if none.
func RoleOf(ask string) string {
	for _, w := range strings.Fields(strings.ToLower(ask)) {
		switch w {
		case "findings":
			return "analyst"
		case "verdict":
			return "judge"
		case "draft":
			return "drafter"
		}
	}
	return ""
}

// String renders the commission on the wire.
func (c Commission) String() string {
	var b strings.Builder
	if c.Root != "" {
		fmt.Fprintf(&b, "@ROOT %s\n", c.Root)
	}
	if c.Scope != "" {
		fmt.Fprintf(&b, "@SCOPE %s\n", c.Scope)
	}
	ask := "findings"
	switch c.Role {
	case "judge":
		ask = "verdict"
	case "drafter":
		ask = "draft"
	}
	if c.Unsaid {
		ask += " +unsaid"
	}
	fmt.Fprintf(&b, "@ASK %s\n", ask)
	if c.Cap > 0 {
		fmt.Fprintf(&b, "@CAP %d\n", c.Cap)
	}
	b.WriteString(strings.TrimSpace(c.Ask) + "\n")
	return b.String()
}

// DispatchReport is what a runner brings back: the Subagent's wire report (under @CAP) and the
// readings the dispatcher needs.
type DispatchReport struct {
	Text    string   // the report, ready for the model
	Status  string   // the run's status word
	Unsaid  int      // @U lines carried
	Failed  bool     // the run failed (the act failed)
	Holes   []string // the engine's own holes
	Wrote   []string // paths the Subagent wrote (gated on its own account)
	Journal string
}

// Dispatcher runs one commission as a Ephemeral subagent: a fresh loop with its own context, tools cut
// to rank and ownership, one wire report back. The workspace package implements it; this package
// cannot import the loop.
type Dispatcher func(ctx context.Context, env Env, c Commission) (DispatchReport, error)

// DispatchOptions is tools.dispatch.
type DispatchOptions struct {
	Enabled    bool
	Depth      int      // the depth of the agent holding this tool (0 for the session)
	MaxDepth   int      // 0 = 1: a Subagent may not dispatch
	Cap        int      // default @CAP for a Subagent's report (0 = the wire's default)
	Unsaid     bool     // commission +unsaid by default
	Agents     []string // allowed agent names; nil = any
	Background bool     // `background: true` may run the Subagent asynchronously (tools.dispatch.background)
	// Wake receives a background Subagent's report when it lands; nil: background is refused.
	Wake func(agent, report string, failed bool)
}

// DispatchTool is `dispatch`: register a named agent for one commission and return its report. A
// report with no @U line is flagged to the dispatcher (the unsaid).
func DispatchTool(opt DispatchOptions, run Dispatcher) *Tool {
	return &Tool{
		Name:        "dispatch",
		Description: "Dispatch a Ephemeral subagent: a named agent (zone-<zone>, domain-<domain>, coord-<name>) works one commission in a fresh context, tools cut to its rank and ownership, and answers one wire report. Its context dies with the task.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"agent":{"type":"string"},"ask":{"type":"string"},"scope":{"type":"string"},"role":{"type":"string","enum":["analyst","judge","drafter"],"description":"findings (analyst), verdict (judge) or draft (drafter); default by the ask"},"cap":{"type":"integer"},"unsaid":{"type":"boolean"},"background":{"type":"boolean","description":"run the Subagent in the background; its report wakes you when it lands"}},"required":["agent","ask"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Agent string }
			_ = decode(in, &a)
			return Classification{Class: Write, Why: "dispatches " + a.Agent}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Agent, Ask, Scope, Role string
				Cap                     int
				Unsaid                  *bool
				Background              bool
			}
			if err := decode(in, &a); err != nil {
				return fail("dispatch: %v", err)
			}
			if strings.TrimSpace(a.Agent) == "" || strings.TrimSpace(a.Ask) == "" {
				return fail("dispatch: agent and ask are required")
			}
			if !opt.Enabled {
				return fail("dispatch is off (tools.dispatch.enabled: false)")
			}
			max := opt.MaxDepth
			if max <= 0 {
				max = 1
			}
			if opt.Depth >= max {
				return fail("dispatch: nesting limit reached (depth %d of %d) — a Subagent at this depth may not dispatch; answer with @? one hop up instead", opt.Depth, max)
			}
			if opt.Agents != nil && !inList(opt.Agents, a.Agent) {
				return fail("dispatch: %q is not a agent this session may dispatch (%s)", a.Agent, strings.Join(opt.Agents, ", "))
			}
			if run == nil {
				return fail("dispatch: no runner wired")
			}
			c := Commission{Agent: strings.TrimSpace(a.Agent), Ask: a.Ask, Scope: a.Scope, Root: env.Root, Cap: a.Cap, Depth: opt.Depth + 1, Unsaid: opt.Unsaid, Role: a.Role}
			if c.Role == "" {
				c.Role = RoleOf(a.Ask)
			}
			if c.Role == "" {
				c.Role = "analyst"
			}
			if c.Cap == 0 {
				c.Cap = opt.Cap
			}
			if a.Unsaid != nil {
				c.Unsaid = *a.Unsaid
			}
			render := func(rep DispatchReport) string {
				out := strings.TrimSpace(rep.Text)
				if rep.Unsaid == 0 {
					out += fmt.Sprintf("\n@? dispatch %s: report carries no @U line — nothing unsaid, or the duty failed; ask again with +unsaid", c.Agent)
				}
				return out
			}
			if a.Background {
				if !opt.Background || opt.Wake == nil {
					return fail("dispatch: background Subagents are off (tools.dispatch.background: false) — dispatch %s in the foreground", c.Agent)
				}
				go func() {
					rep, err := run(context.Background(), env, c)
					if err != nil {
						opt.Wake(c.Agent, fmt.Sprintf("@S FAIL\n@? dispatch %s: %v\n@E 0", c.Agent, err), true)
						return
					}
					opt.Wake(c.Agent, render(rep), rep.Failed)
				}()
				return Result{Output: fmt.Sprintf("subagent %s started in the background on `%s` — keep working; its report wakes you when it lands (or /send %s <text> to reach it)", c.Agent, oneLineAsk(c.Ask), c.Agent)}
			}
			rep, err := run(ctx, env, c)
			if err != nil {
				return fail("dispatch %s: %v", c.Agent, err)
			}
			// the Subagent's writes are named so the dispatcher's record is honest; they were gated
			// on the Subagent's own account and its dispatcher's gate leaves them alone
			return Result{Output: render(rep), Err: rep.Failed, Wrote: rep.Wrote}
		},
	}
}

func oneLineAsk(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

func inList(xs []string, x string) bool {
	for _, y := range xs {
		if strings.EqualFold(y, x) {
			return true
		}
	}
	return false
}
