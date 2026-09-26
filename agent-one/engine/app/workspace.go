package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/compact"
	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/discover"
	"github.com/Kaginari/agent-one/instrument"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/workspace"
)

// designRank folds config's rank spelling (principal-coordinator) to the workspace's (principal_coordinator).
func designRank(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "-", "_")
}

// ranksFor turns config's resolved rank table into the workspace's: builtin ranks take their dirs
// and prefixes from the lexicon (agent-one renames them), a custom rank its own dir (its name
// unless config says), tool lists stay globs for workspace.ExpandTools.
func ranksFor(cfg *config.Config, lex workspace.Lexicon) workspace.Ranks {
	def := map[string]workspace.Rank{}
	for _, d := range workspace.DefaultRanks(lex) {
		def[d.Name] = d
	}
	var out workspace.Ranks
	for _, r := range cfg.Ranks() {
		name := designRank(r.Name)
		wr := workspace.Rank{Name: name, ReportsTo: designRank(r.ReportsTo), Job: r.Job, Authors: r.Authors, HoldsGate: r.HoldsGate, Sideways: r.Sideways,
			Agent: r.Agent, Role: r.Role, Tools: append([]string(nil), r.Tools...), Base: designRank(r.PromotesFrom)}
		if d, ok := def[name]; ok && r.Builtin {
			wr.Dir, wr.Prefix = d.Dir, d.Prefix
			if wr.ReportsTo == "" {
				wr.ReportsTo = d.ReportsTo
			}
			if wr.Agent == "" {
				wr.Agent = d.Agent
			}
			if wr.Role == "" {
				wr.Role = d.Role
			}
			if len(wr.Tools) == 0 {
				wr.Tools = d.Tools
			}
			if wr.Base == "" {
				wr.Base = d.Base
			}
		} else {
			wr.Dir = r.Dir
			if wr.Dir == "" && wr.Base != "" {
				if b, ok := def[wr.Base]; ok {
					wr.Dir, wr.Prefix = b.Dir, b.Prefix
				}
			}
			if wr.Dir == "" {
				wr.Dir = strings.ReplaceAll(name, "_", "-")
			}
			wr.Prefix = r.Prefix
			if wr.Prefix == "" {
				wr.Prefix = wr.Dir + "-"
			}
		}
		if wr.Agent == "" {
			wr.Agent = "ephemeral"
		}
		if wr.Role == "" {
			wr.Role = "analyst"
		}
		if len(wr.Tools) == 0 {
			wr.Tools = []string{"*"}
		}
		out = append(out, wr)
	}
	return out
}

// lexiconFor is the vocabulary of a distribution.
func lexiconFor() workspace.Lexicon { return workspace.Default() }

// openWorkspace opens the workspace root under config's switches.
func openWorkspace(cfg *config.Config, root string, lex workspace.Lexicon) (*workspace.Workspace, error) {
	return workspace.Open(root, lex, workspace.Options{
		Instructions: workspace.InstructionOptions{Enabled: cfg.Discovery.Instructions.Enabled, Files: cfg.Discovery.Instructions.Files, Global: cfg.Discovery.Instructions.Global, WalkUp: cfg.Discovery.Instructions.WalkUp},
		Ontology:     cfg.Ontology.Enabled,
		Ranks:        ranksFor(cfg, lex),
	})
}

// foreignAgents turns discovered agent files into agents on the roster: the rank from the
// name's prefix when it has one, else the routing rank (coord), no ownership; the agent's own
// prompt reaches it as a rule scoped to it.
func foreignAgents(w *workspace.Workspace, agents []discover.Agent, lex workspace.Lexicon) []workspace.Rule {
	var rules []workspace.Rule
	for _, a := range agents {
		name := strings.ToLower(a.Name)
		if name == workspace.Orchestrator {
			continue // the session's own name is reserved: an agent file never becomes the throne
		}
		if w.Member(name) != nil {
			continue // one agent, two sources: the native doc wins
		}
		rank := w.Ranks.Of(name)
		if rank == "" {
			rank = "coord"
			if _, ok := w.Ranks.Get("coord"); !ok && len(w.Ranks) > 0 {
				rank = w.Ranks[0].Name
			}
		}
		w.AddForeign(workspace.Member{Name: name, Rank: rank, Doc: relOrAbs(w.Root, a.Path)})
		if strings.TrimSpace(a.Prompt) != "" {
			rules = append(rules, workspace.Rule{Text: a.Prompt, Scope: "member:" + name})
		}
	}
	return rules
}

func relOrAbs(root, p string) string {
	if r, err := relTo(root, p); err == nil {
		return r
	}
	return p
}

// hookOptions is config's word on the workspace's hooks.
func hookOptions(cfg *config.Config, rules []workspace.Rule) workspace.HookOptions {
	h := workspace.DefaultHooks()
	h.Recall = workspace.RecallOptions{Enabled: cfg.Memory.Long.Enabled || cfg.Toolbox.Enabled, Memory: cfg.Memory.Long.Enabled, Toolbox: cfg.Toolbox.Enabled, K: cfg.Memory.Recall.K, Budget: cfg.Toolbox.BudgetTokens}
	h.Record = workspace.RecordOptions{Enabled: cfg.Memory.Shared.Workspace.Enabled, Unsaid: cfg.Memory.Shared.Workspace.Enabled, Landed: false}
	h.Gate = workspace.GateOptions{Enabled: cfg.Policy.Gate.Enabled, RightAuthor: cfg.Policy.Gate.RightAuthor, InvariantsHold: cfg.Policy.Gate.InvariantsHold, DutiesDone: cfg.Policy.Gate.DutiesDone,
		DocTruthful: cfg.Policy.Gate.DocTruthful || cfg.Policy.DocsAsCode.Enabled, Log: cfg.Policy.Log.Enabled}
	for _, ck := range cfg.Checks("", "") {
		h.Gate.Checks = append(h.Gate.Checks, workspace.Check{Name: ck.ID, Command: ck.Command, Timeout: ck.Timeout, Scope: ck.Scope})
	}
	for _, r := range cfg.Rules {
		if ck := r.Check; ck != "" {
			found := false
			for _, c := range h.Gate.Checks {
				if c.Name == r.ID {
					found = true
				}
			}
			if !found {
				h.Gate.Checks = append(h.Gate.Checks, workspace.Check{Name: r.ID, Command: ck, Timeout: orDur(r.Timeout.D(), 60*time.Second), Scope: r.Scope})
			}
		}
	}
	h.Prompt = workspace.PromptOptions{Principles: cfg.Policy.Principles.Enabled, Identity: true, Ontology: workspace.OntologyOptions{Enabled: cfg.Ontology.Enabled, Budget: cfg.Ontology.ProjectBudgetTokens},
		Instructions: cfg.Discovery.Instructions.Enabled, Member: true, Memory: cfg.Memory.Long.Enabled || cfg.Memory.Shared.Workspace.Enabled}
	for _, r := range cfg.Rules {
		if r.Text != "" {
			h.Prompt.Rules = append(h.Prompt.Rules, workspace.Rule{Text: r.Text, Scope: r.Scope})
		}
	}
	h.Prompt.Rules = append(h.Prompt.Rules, rules...)
	return h
}

// compactOptions is config's compaction block as the drain reads it.
func compactOptions(cfg *config.Config, window int) compact.Options {
	c := cfg.Compaction
	sw := func(f config.Feature) compact.Switch { return compact.Switch{Enabled: f.Enabled} }
	return compact.Options{
		Enabled:   c.Enabled,
		Strategy:  c.Strategy,
		Trigger:   compact.Trigger{Tokens: c.Trigger.Tokens, Fraction: c.Trigger.Fraction, Window: window},
		Passes:    compact.Passes{Pointerize: sw(c.Passes.Pointerize), TrimSpent: sw(c.Passes.TrimSpent), Unsaid: sw(c.Passes.Unsaid), WorkingNotes: sw(c.Passes.WorkingNotes), Episode: sw(c.Passes.Episode), Verify: sw(c.Passes.Verify)},
		KeepTurns: c.KeepRecentTurns,
		Cap:       cfg.Policy.Wire.Cap * 2,
		Journal:   c.Journal,
	}
}

// policyBudget is policy.budget as the loop reads it.
func policyBudget(cfg *config.Config, maxTokens int) loop.Budget {
	return loop.Budget{Steps: cfg.Policy.Budget.Steps, Minutes: cfg.Policy.Budget.Minutes, Retries: cfg.Policy.Escalation.MaxRetries, MaxTokens: maxTokens,
		Context: instrument.Budget{Limit: cfg.Policy.Budget.ContextTokens, Stress: cfg.Policy.Budget.StressTokens}}
}

// router resolves and meters the provider for a workspace route (dispatch or session).
func (a *App) router() func(workspace.Route) provider.Provider {
	return func(r workspace.Route) provider.Provider {
		route := Route{Role: r.Role, Rank: r.Rank, Member: r.Member}
		if r.Rank == workspace.Orchestrator {
			route.Rank = ""
		}
		pr, m, err := a.Providers.Route(route)
		if err != nil {
			a.hole(fmt.Sprintf("route %s/%s/%s: %v — the session's model stands in", r.Member, r.Rank, r.Role, err))
			pr, m = a.mount, a.mountModel
			if pr == nil {
				return nil
			}
		}
		depth := 0
		if r.Task == "dispatch" {
			depth = 1
		}
		price, _ := a.Cfg.PriceFor(m.Ref.Model)
		meter := &Meter{Provider: pr, Journal: a.Journal, Agent: r.Member, Rank: r.Rank, Role: r.Role, Model: m.Ref.Model, Price: price,
			Session: a.Cfg.Budgets.Session, Subagent: a.Cfg.Budgets.Subagent, Depth: depth}
		if a.Subagent != nil {
			a.Subagent.Track(r.Member, r.Rank, r.Role, m.Ref.Model, nil, depth)
		}
		if a.OnState != nil {
			agent := r.Member
			meter.OnState = func(st string) { a.OnState(agent, st) }
		}
		return meter
	}
}

// taskProvider is the metered provider for one of the binary's own tasks (drain, gate, log).
func (a *App) taskProvider(task string) provider.Provider {
	pr, m, err := a.Providers.Route(Route{Task: task})
	if err != nil {
		a.hole(fmt.Sprintf("task %s: %v — the session's model stands in", task, err))
		pr, m = a.mount, a.mountModel
		if pr == nil {
			return nil
		}
	}
	price, _ := a.Cfg.PriceFor(m.Ref.Model)
	return &Meter{Provider: pr, Journal: a.Journal, Agent: task, Role: "", Model: m.Ref.Model, Price: price, Session: a.Cfg.Budgets.Session}
}

// budgetHook reads the agent's meter before a model call.
func budgetHook(s *loop.Session) string {
	if m, ok := s.Engine.Provider.(*Meter); ok {
		return m.Over()
	}
	return ""
}

// drainHooks are the compaction seams: the drain itself, guarded by the preCompact hooks.
func (a *App) drainHooks() loop.Hooks {
	if a.Drainer == nil {
		return loop.Hooks{}
	}
	h := a.Drainer.Hooks()
	inner := h.Drain
	h.Drain = func(ctx context.Context, s *loop.Session, c instrument.Context) (bool, error) {
		if a.Hooks != nil {
			if why := a.Hooks.PreCompact(ctx); why != "" {
				return false, fmt.Errorf("preCompact hook refused the drain: %s", why)
			}
		}
		a.record(s, "compaction", map[string]interface{}{"before": c.Tokens})
		ok, err := inner(ctx, s, c)
		if ok && a.Drainer.Last != nil {
			a.record(s, "compaction", map[string]interface{}{"after": a.Drainer.Last.After, "pointers": len(a.Drainer.Last.Pointers), "workingNotes": a.Drainer.WorkingNotesPath(s.RunID)})
		}
		return ok, err
	}
	return h
}

// workingNotesPath is where a session's workingNotes (working memory) lives.
func (a *App) workingNotesPath(run string) string {
	return filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "instruments", "working-notes", run+".md")
}
