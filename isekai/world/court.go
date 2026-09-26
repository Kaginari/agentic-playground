package world

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/tool"
	"github.com/Kaginari/agentic-playground/isekai/wire"
)

// CourtOptions is tools.dispatch as the world applies it: who may dispatch, how deep, what a
// Court's report is capped at. Which ranks may dispatch, and whom, is the rank table's: a body
// dispatches ranks below its own (or its own when the rank is Sideways); the shelf is the
// rank's Tools.
type CourtOptions struct {
	Enabled    bool
	MaxDepth   int                                    // 0 = 1
	Cap        int                                    // 0 = the wire's default
	Unsaid     bool                                   // commission +unsaid by default (a report with no @U is flagged either way)
	Bodies     []string                               // names a session may dispatch; nil = any creature on the roster
	Trace      io.Writer                              // a Court's step trace; nil is quiet
	Background bool                                   // background Courts allowed (tools.dispatch.background)
	Wake       func(body, report string, failed bool) // a background Court's report lands here
}

// Route is what the integrator's model router sees for one engine: the office the ask names
// (findings → great-sage, verdict → raphael, draft → ciel), the rank and the creature.
type Route struct {
	Office   string
	Rank     string
	Creature string
	Task     string // dispatch | session
}

// Build is what an engine for a body is made of: the provider and gate it shares with its
// dispatcher, the base shelf, budgets, and every switch.
type Build struct {
	Provider  provider.Provider
	Models    func(Route) provider.Provider // nil = Provider for every route; the integrator routes models.offices/ranks/creatures here
	Gate      *gate.Gate
	Tools     *tool.Registry                                      // the base shelf; nil = tool.Builtins()
	Shelf     func(as string, depth int) (*tool.Registry, func()) // per-body shelf (its own shell); nil = Tools; the closer runs when the body's engine is closed
	Budget    loop.Budget
	Hooks     HookOptions
	Territory TerritoryOptions
	Court     CourtOptions
	Journal   string
	Trace     io.Writer
	Depth     int // 0 for the session; a dispatched body is Depth+1
	Extra     loop.Hooks
	Office    string // the office this engine serves (set by the dispatcher from @ASK; "" = the session's)
}

// providerFor resolves the provider for a route.
func (b Build) providerFor(r Route) provider.Provider {
	if b.Models != nil {
		if p := b.Models(r); p != nil {
			return p
		}
	}
	return b.Provider
}

// Engine builds a full engine for a body: tools cut to its rank and territory (plus dispatch
// when its rank and depth allow), the territory policy, the world's hooks, the lexicon.
func (w *World) Engine(as string, b Build) *loop.Engine {
	base := b.Tools
	var closer func()
	if b.Shelf != nil {
		base, closer = b.Shelf(as, b.Depth)
	}
	if base == nil {
		base = tool.Builtins()
	}
	shelf := tool.NewRegistry()
	for _, n := range base.Names() {
		if t, ok := base.Get(n); ok {
			shelf.Add(t)
		}
	}
	shelf.Add(w.LawTool())
	rank := w.RankOf(as)
	names := ExpandTools(w.Ranks.Tools(rank.Name), append(shelf.Names(), "dispatch"))
	office := b.Office
	if office == "" {
		office = rank.Office
	}
	task := "session"
	if b.Depth > 0 {
		task = "dispatch"
	}
	e := &loop.Engine{
		Provider: b.providerFor(Route{Office: office, Rank: rank.Name, Creature: as, Task: task}), Gate: b.Gate, Root: w.Root, As: as, Budget: b.Budget, Journal: b.Journal, Trace: b.Trace,
		Lexicon: w.Lex.Loop(), Policy: w.Policy(as, b.Territory), Cap: b.Court.Cap,
	}
	if e.Cap == 0 {
		e.Cap = wire.DefaultCap
	}
	max := b.Court.MaxDepth
	if max <= 0 {
		max = 1
	}
	if b.Court.Enabled && contains(names, "dispatch") && b.Depth < max {
		shelf.Add(tool.DispatchTool(tool.DispatchOptions{Enabled: true, Depth: b.Depth, MaxDepth: max, Cap: b.Court.Cap, Unsaid: b.Court.Unsaid, Bodies: b.Court.Bodies, Background: b.Court.Background && b.Depth == 0, Wake: b.Court.Wake}, w.Dispatcher(e, b)))
	}
	e.Tools = shelf.Only(names...)
	e.Hooks = MergeHooks(w.Hooks(b.Hooks), b.Extra)
	if closer != nil {
		w.mu.Lock()
		w.closers[e] = closer
		w.mu.Unlock()
	}
	return e
}

// Close ends what an engine's body held (its shell, its jobs). A dispatcher closes a Court's
// engine when the report is taken; the session closes its own at exit.
func (w *World) Close(e *loop.Engine) {
	w.mu.Lock()
	c := w.closers[e]
	delete(w.closers, e)
	delete(w.sessions, e)
	delete(w.courtWrote, e)
	w.mu.Unlock()
	if c != nil {
		c()
	}
}

// Dispatcher runs a commission as a Court Body under a parent engine: a fresh loop with its
// own context, the parent's provider and gate, budgets cut to what the parent has left, one
// wire report back. The Court's spend is added to the parent's.
func (w *World) Dispatcher(parent *loop.Engine, b Build) tool.Dispatcher {
	return func(ctx context.Context, env tool.Env, c tool.Commission) (tool.DispatchReport, error) {
		body := strings.ToLower(strings.TrimSpace(c.Body))
		cr := w.Creature(body)
		if cr == nil {
			return tool.DispatchReport{}, fmt.Errorf("no body named %q on the roster (%s/{%s}/<name>/) — a Court is minted for a rank, never generic", body, w.Lex.WorldDir, strings.Join(rankDirs(w.Ranks), ","))
		}
		me := w.RankOf(parent.As)
		if me.Name != Rimuru && !w.Ranks.Below(cr.Rank, me.Name) && !(me.Sideways && cr.Rank == me.Name) {
			return tool.DispatchReport{}, fmt.Errorf("%s (%s) may not dispatch %s (%s): work goes down the ranks, never up or sideways (Law 2) — escalate with @? instead", parent.As, me.Name, cr.Name, cr.Rank)
		}
		child := b
		child.Depth = b.Depth + 1
		child.Office = OfficeOf(c.Ask)
		if c.Office != "" {
			child.Office = c.Office
		}
		child.Trace = b.Court.Trace
		child.Budget = b.Budget
		if ps := w.SessionOf(parent); ps != nil {
			if steps := parent.Budget.Steps; steps >= 0 {
				if steps == 0 {
					steps = loop.DefaultSteps
				}
				left := steps - ps.Steps
				if left <= 0 {
					return tool.DispatchReport{}, fmt.Errorf("parent step budget spent (%d of %d) — nothing left for a Court", ps.Steps, steps)
				}
				child.Budget.Steps = left
			}
			if m := parent.Budget.Minutes; m >= 0 {
				if m == 0 {
					m = loop.DefaultMinutes
				}
				left := m - time.Since(ps.Started).Minutes()
				if left <= 0 {
					return tool.DispatchReport{}, fmt.Errorf("parent wall-clock budget spent (%g min) — nothing left for a Court", m)
				}
				child.Budget.Minutes = left
			}
		}
		e := w.Engine(cr.Name, child)
		defer w.Close(e) // the Court's context dies with the task
		e.Unsaid = c.Unsaid
		e.Wire = true
		if c.Cap > 0 {
			e.Cap = c.Cap
		}
		c.Cap = e.Cap // the ceiling in force rides the commission
		r, err := e.Run(ctx, c.String())
		if err != nil {
			return tool.DispatchReport{}, err
		}
		if ps := w.SessionOf(parent); ps != nil {
			ps.Spend = ps.Spend.Add(r.Usage)
		}
		rep := tool.DispatchReport{Text: r.Emit(e.Cap), Status: r.Status, Unsaid: len(r.Report.Unsaid), Failed: r.Status == loop.Fail, Holes: r.Holes, Wrote: wroteOf(r), Journal: r.Journal}
		w.courtGated(parent, rep.Wrote)
		return rep, nil
	}
}

// courtGated records what a Court of the parent engine wrote (and gated itself).
func (w *World) courtGated(parent *loop.Engine, wrote []string) {
	if len(wrote) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	m := w.courtWrote[parent]
	if m == nil {
		m = map[string]bool{}
		w.courtWrote[parent] = m
	}
	for _, p := range wrote {
		m[normRel(p)] = true
	}
}

// ownWrites is a session's writes less what its Courts wrote and gated on their own account.
func (w *World) ownWrites(s *loop.Session) []string {
	w.mu.Lock()
	m := w.courtWrote[s.Engine]
	w.mu.Unlock()
	if len(m) == 0 {
		return s.Wrote
	}
	var out []string
	for _, p := range s.Wrote {
		if !m[normRel(p)] {
			out = append(out, p)
		}
	}
	return out
}

func rankDirs(rs Ranks) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range rs {
		if r.Dir != "" && !seen[r.Dir] {
			seen[r.Dir] = true
			out = append(out, r.Dir)
		}
	}
	return out
}

func wroteOf(r *loop.Result) []string {
	if len(r.Wrote) > 0 {
		return r.Wrote // every write the Court's gate saw, whatever tool made it
	}
	var out []string
	for _, st := range r.Steps {
		out = append(out, st.Wrote...)
	}
	return out
}
