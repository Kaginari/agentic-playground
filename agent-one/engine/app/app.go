// Package app is the one engine behind the two binaries (agent-one, agent-one): it reads the
// switchboard (config), opens the workspace under its lexicon, builds the providers, the sandbox,
// the tool shelf, the MCP servers and the discoveries, wires the policy's hooks into the loop,
// and runs sessions — a REPL, one-shot runs, Subagent dispatch, the drain. The two mains differ
// only by distribution: lexicon, workspace dir, env prefix, defaults.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/compact"
	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/sandbox"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
	"github.com/Kaginari/agent-one/workspace"
)

// Version is what the binary was built as (GoReleaser's ldflags fill it).
type Version struct {
	Version, Commit, Date string
}

// Options is what a main hands the engine.
type Options struct {
	Dist    string // the binary name; "" means agent-one
	Root    string // the workspace root; "" discovers from Cwd
	Cwd     string
	Home    string
	Env     func(string) string
	Flags   []config.Override // the config flags (--model, --approve, --set, --no-<feature>…)
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Version Version
	// NoWorkspace opens the config only (status/config commands work without a workspace).
	NoWorkspace bool
	// Session is the session id to use (resume); "" makes a new one.
	Session string
	// Quiet silences the step trace.
	Quiet bool
	// IsTTY overrides the gate's TTY detection (tests).
	IsTTY func() bool
	// NoBoard keeps the board down whatever ui.board.autostart says (one-shot runs, tests).
	NoBoard bool
}

// App is one opened engine.
type App struct {
	Opt       Options
	Cfg       *config.Config
	Root      string
	Lex       workspace.Lexicon
	Workspace *workspace.Workspace
	Providers *Providers
	Sandbox   *sandbox.Sandbox
	Shelf     *Shelf
	MCP       *MCPSet
	Found     Discovered
	Journal   *UsageJournal
	Hooks     *ShellHooks
	Missing   *MissingPolicy
	Drainer   *compact.Drainer
	Gate      *gate.Gate
	SessionID string
	Sessions  *SessionStore
	Subagent  *Subagent

	// OnState receives an agent's state changes (the live subagent); nil is quiet.
	OnState func(agent, state string)
	// OnDelta receives streamed text of the session agent; nil is quiet.
	OnDelta func(text string)
	// Inbox hands lines typed mid-turn to the running agent.
	Inbox func(s *loop.Session) []string

	mount      provider.Provider
	mountModel config.Model
	boardAddr  string
	rules      []workspace.Rule
	engines    []*loop.Engine
	mu         sync.Mutex
	holes      []string
	started    time.Time
	closed     bool
}

// New opens the engine: config, workspace, providers, sandbox, shelf, MCP, discoveries, drain.
func New(opt Options) (*App, error) {
	if opt.Env == nil {
		opt.Env = os.Getenv
	}
	if opt.In == nil {
		opt.In = os.Stdin
	}
	if opt.Out == nil {
		opt.Out = os.Stdout
	}
	if opt.Err == nil {
		opt.Err = os.Stderr
	}
	if opt.Cwd == "" {
		opt.Cwd, _ = os.Getwd()
	}
	if opt.Home == "" {
		opt.Home = opt.Env("HOME")
	}
	if opt.Home == "" {
		opt.Home, _ = os.UserHomeDir()
	}
	cfg, err := config.LoadWith(config.Options{Dist: opt.Dist, Root: opt.Root, Cwd: opt.Cwd, Home: opt.Home, Env: opt.Env, Flags: opt.Flags})
	if err != nil {
		return nil, err
	}
	a := &App{Opt: opt, Cfg: cfg, Root: cfg.Root, Lex: lexiconFor(), started: time.Now()}
	a.Providers = NewProviders(cfg, opt.Env)
	if opt.Session == "" {
		opt.Session = newSessionID()
		a.Opt.Session = opt.Session
	}
	a.SessionID = opt.Session
	if opt.NoWorkspace {
		return a, nil
	}
	if st, err := os.Stat(filepath.Join(a.Root, cfg.Dist.WorkspaceDir)); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("no %s/ at or above %s — found a workspace by `%s init`, or pass --root", cfg.Dist.WorkspaceDir, opt.Cwd, cfg.Dist.Name)
	}
	// the mount: the human's model, never routed
	m, _ := cfg.Mount()
	a.mountModel = m
	if pr, err := a.Providers.For(m); err == nil {
		a.mount = pr
	} else {
		a.hole("mount " + m.Ref.Model + ": " + err.Error())
	}
	a.Journal = NewUsageJournal(filepath.Join(a.Root, cfg.Dist.WorkspaceDir, "instruments", "usage"), a.SessionID)
	// the workspace under its lexicon and config's ranks
	w, err := openWorkspace(cfg, a.Root, a.Lex)
	if err != nil {
		return nil, err
	}
	a.Workspace = w
	for _, n := range w.Notes {
		a.hole(n)
	}
	for _, h := range cfg.Holes {
		a.hole(h)
	}
	// the sandbox, the human gate, the shelf, mcp, discoveries
	a.Sandbox = newSandbox(cfg, a.Root)
	a.Gate = a.newGate()
	asker := tool.TTYAsker(opt.In, opt.Err)
	if opt.IsTTY != nil && !opt.IsTTY() || opt.IsTTY == nil && !gate.StdinIsTTY() {
		asker = nil
	}
	a.Shelf = NewShelf(cfg, a.Root, a.Sandbox, asker, opt.Env)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	a.MCP = ConnectMCP(ctx, cfg, a.Root, a.Sandbox, opt.Env)
	cancel()
	for _, h := range a.MCP.Holes() {
		a.hole(h)
	}
	a.Shelf.Resource = a.MCP.ReadResource
	a.Shelf.Inward = a.MCP.IsInward
	a.Shelf.Extra = append(a.Shelf.Extra, func(string) []*tool.Tool { return a.MCP.Tools() }, a.workspaceTools)
	a.Found = Discover(cfg, a.Root, opt.Home)
	a.rules = foreignAgents(w, a.Found.Agents, a.Lex)
	// hooks, the missing-tool policy, the drain, sessions
	a.Hooks = &ShellHooks{Cfg: cfg, Root: a.Root, Session: a.SessionID, Journal: func(ev string, f map[string]interface{}) {
		a.record(nil, "hook", merge(f, map[string]interface{}{"event": ev}))
	}}
	a.Missing = &MissingPolicy{Cfg: cfg, Disabled: a.Shelf.Disabled, Ask: asker,
		Note:   func(as, text string) error { return w.Remember(as, "team", text, "missing") },
		Reload: func(next *config.Config, changes []config.Change) { a.reload(next, changes) }}
	if cfg.Compaction.Enabled {
		homes := w.Homes()
		homes.Provider = a.taskProvider("drain")
		a.Drainer = compact.New(compactOptions(cfg, a.Providers.Window(context.Background(), m)), homes)
	}
	if cfg.Sessions.Enabled {
		a.Sessions = NewSessionStore(cfg.Sessions.Dir, a.Root)
	}
	a.Subagent = newSubagent(a)
	return a, nil
}

func newSessionID() string {
	return time.Now().UTC().Format("20060102-150405") + "-" + randHex(3)
}

func (a *App) hole(h string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, x := range a.holes {
		if x == h {
			return
		}
	}
	a.holes = append(a.holes, h)
}

// Holes lists everything the engine could not settle while opening.
func (a *App) Holes() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := append([]string(nil), a.holes...)
	if a.Providers != nil {
		out = append(out, a.Providers.Holes()...)
	}
	if a.Shelf != nil {
		out = append(out, a.Shelf.Holes()...)
	}
	return out
}

// newGate builds the human gate from config: pre-approvals from the file layers or the
// --approve flag, strict, dry run, the TTY.
func (a *App) newGate() *gate.Gate {
	g := gate.New()
	for _, c := range a.Cfg.Policy.HumanGate.Approve {
		if cl, ok := tool.ParseClass(c); ok {
			g.Approve[cl] = true
		}
	}
	g.Strict, g.DryRun = a.Cfg.Policy.HumanGate.Strict, a.Cfg.Policy.HumanGate.DryRun
	g.In, g.Out, g.IsTTY = a.Opt.In, a.Opt.Err, a.Opt.IsTTY
	g.Flag = "--approve"
	return g
}

// reload swaps the config in after an accepted patch: the shelf, the policies and the
// providers read the new one; running engines keep their shelf until their next build.
func (a *App) reload(next *config.Config, changes []config.Change) {
	a.mu.Lock()
	a.Cfg = next
	a.Shelf.Cfg = next
	a.Providers.Cfg = next
	a.Hooks.Cfg = next
	a.mu.Unlock()
	for _, c := range changes {
		fmt.Fprintln(a.Opt.Err, "config reloaded: "+c.String())
	}
}

// Build is the workspace's build for this engine: routed models, the gate, the per-agent shelf,
// budgets, every switch, the extra hooks (rules, missing tools, shell hooks, budgets, drain).
func (a *App) Build() workspace.Build {
	cfg := a.Cfg
	var maxOut int
	if a.mountModel.Entry != nil {
		maxOut = a.mountModel.Entry.MaxOutputTokens
	}
	b := workspace.Build{
		Provider:  a.sessionProvider(),
		Models:    a.router(),
		Gate:      a.Gate,
		Shelf:     func(as string, depth int) (*tool.Registry, func()) { return a.Shelf.Build(as) },
		Budget:    policyBudget(cfg, maxOut),
		Hooks:     hookOptions(cfg, a.rules),
		Ownership: workspace.OwnershipOptions{Enabled: cfg.Policy.Ownership.Enabled},
		Subagent: workspace.SubagentOptions{Enabled: a.on("dispatch"), MaxDepth: cfg.Tools.Dispatch.MaxDepth, Cap: cfg.Policy.Wire.Cap, Unsaid: cfg.Policy.Wire.RequireUnsaid,
			Background: cfg.Tools.Dispatch.Background, Wake: func(agent, report string, failed bool) {
				// a background report never passes the dispatcher's Record hook: its @U goes home here
				if rep, ok := wire.ParseReport(report); ok && len(rep.Unsaid) > 0 {
					for _, h := range a.Workspace.Land(agent, rep) {
						a.hole(h)
					}
				}
				a.Subagent.Finish(agent, report)
				a.Subagent.Wake(fmt.Sprintf("[subagent %s reported]\n%s", agent, report))
			}},
		Journal: a.journalDir(),
	}
	if !a.Opt.Quiet {
		b.Trace = a.Opt.Err
		b.Subagent.Trace = a.Opt.Err
	}
	env := func() tool.Env { return tool.Env{Root: a.Root, WorkspaceDir: cfg.Dist.WorkspaceDir} }
	mine := loop.Hooks{
		Decide:   decideHook(cfg, env),
		Missing:  a.Missing.Hook(),
		PreTool:  a.Hooks.PreTool,
		PostTool: a.Hooks.PostTool,
		Budget:   budgetHook,
		State: func(s *loop.Session, st string) {
			if a.Subagent != nil {
				a.Subagent.observe(s, st)
			}
			if a.OnState != nil {
				a.OnState(agentName(s), st)
			}
		},
		Inbox: func(s *loop.Session) []string {
			if a.Inbox != nil {
				return a.Inbox(s)
			}
			return nil
		},
	}
	b.Extra = workspace.MergeHooks(a.drainHooks(), mine)
	return b
}

func (a *App) on(tool string) bool {
	ok, _ := a.Cfg.ToolEnabled(tool)
	return ok
}

func (a *App) journalDir() string {
	if !a.Cfg.Instruments.Loop.Enabled {
		return "-"
	}
	if d := a.Cfg.Instruments.Loop.JournalDir; d != "" {
		if filepath.IsAbs(d) {
			return d
		}
		return filepath.Join(a.Root, d)
	}
	return ""
}

// sessionProvider is the mount, metered for the session agent.
func (a *App) sessionProvider() provider.Provider {
	if a.mount == nil {
		return nil
	}
	price, _ := a.Cfg.PriceFor(a.mountModel.Ref.Model)
	m := &Meter{Provider: a.mount, Journal: a.Journal, Agent: workspace.Orchestrator, Rank: "", Role: "", Model: a.mountModel.Ref.Model, Price: price, Session: a.Cfg.Budgets.Session}
	if a.OnState != nil {
		m.OnState = func(st string) { a.OnState(workspace.Orchestrator, st) }
	}
	return m
}

// Engine builds the session's engine (Orchestrator's, on the mount).
func (a *App) Engine() (*loop.Engine, error) {
	if a.mount == nil {
		return nil, fmt.Errorf("no model: %s", strings.Join(a.Providers.Holes(), "; "))
	}
	e := a.Workspace.Engine(workspace.Orchestrator, a.Build())
	e.OnDelta = a.OnDelta
	e.Cap = 0 // the human's answer is never capped
	if a.Subagent != nil {
		a.Subagent.Track(workspace.Orchestrator, "", "", a.mountModel.Ref.Model, nil, 0)
	}
	a.mu.Lock()
	a.engines = append(a.engines, e)
	a.mu.Unlock()
	return e, nil
}

// Close ends every engine's shell, the MCP servers and the session hooks.
func (a *App) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	engines := a.engines
	a.mu.Unlock()
	if a.Subagent != nil {
		a.Subagent.Close()
	}
	for _, e := range engines {
		if a.Workspace != nil {
			a.Workspace.Close(e)
		}
	}
	if a.MCP != nil {
		a.MCP.Close()
	}
	if a.Hooks != nil {
		a.Hooks.Stop(context.Background())
	}
}

func agentName(s *loop.Session) string {
	if s.Engine.As != "" {
		return s.Engine.As
	}
	return s.Engine.Lexicon.Agent
}

func merge(a, b map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// StatusLines is the honesty rule and the instrument board, in the order status prints them:
// the off-list, loosenings, tools off, ranks, models, sandbox, mcp, providers, holes.
func (a *App) StatusLines() []string {
	var out []string
	if a.Cfg.Instruments.Status.ShowOff {
		out = append(out, a.Cfg.OffLines()...)
	} else {
		out = append(out, "@? off instruments.status.showOff — "+a.Cfg.Where("instruments.status.showOff"))
	}
	for _, l := range a.Cfg.Loosenings() {
		out = append(out, "loosening: "+l)
	}
	if off := a.Cfg.ToolsOff(); len(off) > 0 {
		out = append(out, "tools off: "+strings.Join(off, "; "))
	}
	out = append(out, "ranks: "+strings.TrimSpace(strings.ReplaceAll(a.Cfg.RankTree(), "\n", " · ")))
	for _, d := range a.Cfg.Deviations() {
		out = append(out, "rank deviation: "+d)
	}
	for _, l := range a.Cfg.ModelTable() {
		out = append(out, "model "+l)
	}
	if a.Sandbox != nil {
		out = append(out, sandboxLine(a.Sandbox))
	}
	if a.MCP != nil {
		out = append(out, a.MCP.Status()...)
	}
	if a.Cfg.Compaction.Enabled {
		out = append(out, fmt.Sprintf("compaction: %s · passes %s", a.Cfg.Compaction.Strategy, passesOn(a.Cfg)))
	}
	out = append(out, a.Cfg.BudgetLines()...)
	for _, h := range a.Holes() {
		out = append(out, "@? "+h)
	}
	return out
}

func passesOn(cfg *config.Config) string {
	p := cfg.Compaction.Passes
	var on []string
	for _, x := range []struct {
		n string
		f config.Feature
	}{{"pointerize", p.Pointerize}, {"trimSpent", p.TrimSpent}, {"unsaid", p.Unsaid}, {"workingNotes", p.WorkingNotes}, {"episode", p.Episode}, {"verify", p.Verify}} {
		if x.f.Enabled {
			on = append(on, x.n)
		}
	}
	return strings.Join(on, ",")
}

// OffProse is the honesty rule in the human tongue, once at REPL start.
func (a *App) OffProse() string {
	off := a.Cfg.Off()
	if len(off) == 0 {
		return ""
	}
	var parts []string
	for _, f := range off {
		parts = append(parts, f.Key+" ("+f.Origin.String()+")")
	}
	return "Switched off by config, so this session runs without: " + strings.Join(parts, ", ") + "."
}
