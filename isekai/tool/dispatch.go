package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Commission is what the dispatch tool hands its runner: a body to mint and the wire
// commission it carries (@ROOT @SCOPE @ASK @CAP).
type Commission struct {
	Body   string
	Ask    string
	Scope  string
	Root   string
	Unsaid bool
	Cap    int
	Depth  int    // the Court's nesting depth (1 for a body dispatched by the session)
	Office string // the office the ask names: findings → great-sage, verdict → raphael, draft → ciel
}

// OfficeOf reads the office off an @ASK word (the triad: perceive, judge, speak); "" if none.
func OfficeOf(ask string) string {
	for _, w := range strings.Fields(strings.ToLower(ask)) {
		switch w {
		case "findings":
			return "great-sage"
		case "verdict":
			return "raphael"
		case "draft":
			return "ciel"
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
	switch c.Office {
	case "raphael":
		ask = "verdict"
	case "ciel":
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

// DispatchReport is what a runner brings back: the Court's wire report (under @CAP) and the
// readings the dispatcher needs.
type DispatchReport struct {
	Text    string   // the report, ready for the model
	Status  string   // the run's status word
	Unsaid  int      // @U lines carried
	Failed  bool     // the run failed (the act failed)
	Holes   []string // the engine's own holes
	Wrote   []string // paths the Court wrote (gated on its own account)
	Journal string
}

// Dispatcher runs one commission as a Court Body: a fresh loop with its own context, tools cut
// to rank and territory, one wire report back. The world package implements it; this package
// cannot import the loop.
type Dispatcher func(ctx context.Context, env Env, c Commission) (DispatchReport, error)

// DispatchOptions is tools.dispatch.
type DispatchOptions struct {
	Enabled    bool
	Depth      int      // the depth of the body holding this tool (0 for the session)
	MaxDepth   int      // 0 = 1: a Court may not dispatch
	Cap        int      // default @CAP for a Court's report (0 = the wire's default)
	Unsaid     bool     // commission +unsaid by default
	Bodies     []string // allowed body names; nil = any
	Background bool     // `background: true` may run the Court asynchronously (tools.dispatch.background)
	// Wake receives a background Court's report when it lands; nil: background is refused.
	Wake func(body, report string, failed bool)
}

// DispatchTool is `dispatch`: mint a named body for one commission and return its report. A
// report with no @U line is flagged to the dispatcher (the unsaid).
func DispatchTool(opt DispatchOptions, run Dispatcher) *Tool {
	return &Tool{
		Name:        "dispatch",
		Description: "Dispatch a Court Body: a named body (slime-<zone>, orc-<domain>, elf-<name>) works one commission in a fresh context, tools cut to its rank and territory, and answers one wire report. Its context dies with the task.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"body":{"type":"string"},"ask":{"type":"string"},"scope":{"type":"string"},"office":{"type":"string","enum":["great-sage","raphael","ciel"],"description":"findings (great-sage), verdict (raphael) or draft (ciel); default by the ask"},"cap":{"type":"integer"},"unsaid":{"type":"boolean"},"background":{"type":"boolean","description":"run the Court in the background; its report wakes you when it lands"}},"required":["body","ask"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Body string }
			_ = decode(in, &a)
			return Classification{Class: Write, Why: "dispatches " + a.Body}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Body, Ask, Scope, Office string
				Cap                      int
				Unsaid                   *bool
				Background               bool
			}
			if err := decode(in, &a); err != nil {
				return fail("dispatch: %v", err)
			}
			if strings.TrimSpace(a.Body) == "" || strings.TrimSpace(a.Ask) == "" {
				return fail("dispatch: body and ask are required")
			}
			if !opt.Enabled {
				return fail("dispatch is off (tools.dispatch.enabled: false)")
			}
			max := opt.MaxDepth
			if max <= 0 {
				max = 1
			}
			if opt.Depth >= max {
				return fail("dispatch: nesting limit reached (depth %d of %d) — a Court at this depth may not dispatch; answer with @? one hop up instead", opt.Depth, max)
			}
			if opt.Bodies != nil && !inList(opt.Bodies, a.Body) {
				return fail("dispatch: %q is not a body this session may dispatch (%s)", a.Body, strings.Join(opt.Bodies, ", "))
			}
			if run == nil {
				return fail("dispatch: no runner wired")
			}
			c := Commission{Body: strings.TrimSpace(a.Body), Ask: a.Ask, Scope: a.Scope, Root: env.Root, Cap: a.Cap, Depth: opt.Depth + 1, Unsaid: opt.Unsaid, Office: a.Office}
			if c.Office == "" {
				c.Office = OfficeOf(a.Ask)
			}
			if c.Office == "" {
				c.Office = "great-sage"
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
					out += fmt.Sprintf("\n@? dispatch %s: report carries no @U line — nothing unsaid, or the duty failed; ask again with +unsaid", c.Body)
				}
				return out
			}
			if a.Background {
				if !opt.Background || opt.Wake == nil {
					return fail("dispatch: background Courts are off (tools.dispatch.background: false) — dispatch %s in the foreground", c.Body)
				}
				go func() {
					rep, err := run(context.Background(), env, c)
					if err != nil {
						opt.Wake(c.Body, fmt.Sprintf("@S FAIL\n@? dispatch %s: %v\n@E 0", c.Body, err), true)
						return
					}
					opt.Wake(c.Body, render(rep), rep.Failed)
				}()
				return Result{Output: fmt.Sprintf("court %s started in the background on `%s` — keep working; its report wakes you when it lands (or /send %s <text> to reach it)", c.Body, oneLineAsk(c.Ask), c.Body)}
			}
			rep, err := run(ctx, env, c)
			if err != nil {
				return fail("dispatch %s: %v", c.Body, err)
			}
			return Result{Output: render(rep), Err: rep.Failed}
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
