package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/compact"
	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/discover"
	"github.com/Kaginari/agentic-playground/isekai/instrument"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/world"
)

// canonRank folds config's rank spelling (high-elf) to the world's (high_elf).
func canonRank(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "-", "_")
}

// ranksFor turns config's resolved rank table into the world's: builtin ranks take their dirs
// and prefixes from the lexicon (agent-one renames them), a custom rank its own dir (its name
// unless config says), tool lists stay globs for world.ExpandTools.
func ranksFor(cfg *config.Config, lex world.Lexicon) world.Ranks {
	def := map[string]world.Rank{}
	for _, d := range world.DefaultRanks(lex) {
		def[d.Name] = d
	}
	var out world.Ranks
	for _, r := range cfg.Ranks() {
		name := canonRank(r.Name)
		wr := world.Rank{Name: name, ReportsTo: canonRank(r.ReportsTo), Job: r.Job, Authors: r.Authors, HoldsGate: r.HoldsGate, Sideways: r.Sideways,
			Body: r.Body, Office: r.Office, Tools: append([]string(nil), r.Tools...), Base: canonRank(r.AscendsFrom)}
		if d, ok := def[name]; ok && r.Builtin {
			wr.Dir, wr.Prefix = d.Dir, d.Prefix
			if wr.ReportsTo == "" {
				wr.ReportsTo = d.ReportsTo
			}
			if wr.Body == "" {
				wr.Body = d.Body
			}
			if wr.Office == "" {
				wr.Office = d.Office
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
		if wr.Body == "" {
			wr.Body = "court"
		}
		if wr.Office == "" {
			wr.Office = "great-sage"
		}
		if len(wr.Tools) == 0 {
			wr.Tools = []string{"*"}
		}
		out = append(out, wr)
	}
	return out
}

// lexiconFor is the vocabulary of a distribution.
func lexiconFor(dist string) world.Lexicon {
	if dist == "agent-one" {
		return world.AgentOne()
	}
	return world.Isekai()
}

// openWorld opens the world root under config's switches.
func openWorld(cfg *config.Config, root string, lex world.Lexicon) (*world.World, error) {
	return world.Open(root, lex, world.Options{
		Instructions: world.InstructionOptions{Enabled: cfg.Discovery.Instructions.Enabled, Files: cfg.Discovery.Instructions.Files, Global: cfg.Discovery.Instructions.Global, WalkUp: cfg.Discovery.Instructions.WalkUp},
		Ontology:     cfg.Ontology.Enabled,
		Ranks:        ranksFor(cfg, lex),
	})
}

// foreignBodies turns discovered agent files into bodies on the roster: the rank from the
// name's prefix when it has one, else the routing rank (elf), no territory; the agent's own
// prompt reaches it as a rule scoped to it.
func foreignBodies(w *world.World, agents []discover.Agent, lex world.Lexicon) []world.Rule {
	var rules []world.Rule
	for _, a := range agents {
		name := strings.ToLower(a.Name)
		if w.Creature(name) != nil {
			continue // one body, two sources: the native doc wins
		}
		rank := w.Ranks.Of(name)
		if rank == "" {
			rank = "elf"
			if _, ok := w.Ranks.Get("elf"); !ok && len(w.Ranks) > 0 {
				rank = w.Ranks[0].Name
			}
		}
		w.AddForeign(world.Creature{Name: name, Rank: rank, Doc: relOrAbs(w.Root, a.Path)})
		if strings.TrimSpace(a.Prompt) != "" {
			rules = append(rules, world.Rule{Text: a.Prompt, Scope: "creature:" + name})
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

// hookOptions is config's word on the world's hooks.
func hookOptions(cfg *config.Config, rules []world.Rule) world.HookOptions {
	h := world.DefaultHooks()
	h.Recall = world.RecallOptions{Enabled: cfg.Memory.Long.Enabled || cfg.Toolbox.Enabled, Memory: cfg.Memory.Long.Enabled, Toolbox: cfg.Toolbox.Enabled, K: cfg.Memory.Recall.K, Budget: cfg.Toolbox.BudgetTokens}
	h.Record = world.RecordOptions{Enabled: cfg.Memory.Shared.World.Enabled, Unsaid: cfg.Memory.Shared.World.Enabled, Landed: false}
	h.Gate = world.GateOptions{Enabled: cfg.Law.Gate.Enabled, RightAuthor: cfg.Law.Gate.RightAuthor, TraitsHold: cfg.Law.Gate.TraitsHold, DutiesDone: cfg.Law.Gate.DutiesDone,
		DocTruthful: cfg.Law.Gate.DocTruthful || cfg.Law.Vitality.Enabled, Log: cfg.Law.Log.Enabled}
	for _, ck := range cfg.Checks("", "") {
		h.Gate.Checks = append(h.Gate.Checks, world.Check{Name: ck.ID, Command: ck.Command, Timeout: ck.Timeout, Scope: ck.Scope})
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
				h.Gate.Checks = append(h.Gate.Checks, world.Check{Name: r.ID, Command: ck, Timeout: orDur(r.Timeout.D(), 60*time.Second), Scope: r.Scope})
			}
		}
	}
	h.Prompt = world.PromptOptions{Crest: cfg.Law.Crest.Enabled, Identity: true, Ontology: world.OntologyOptions{Enabled: cfg.Ontology.Enabled, Budget: cfg.Ontology.ProjectBudgetTokens},
		Instructions: cfg.Discovery.Instructions.Enabled, Creature: true, Memory: cfg.Memory.Long.Enabled || cfg.Memory.Shared.World.Enabled}
	for _, r := range cfg.Rules {
		if r.Text != "" {
			h.Prompt.Rules = append(h.Prompt.Rules, world.Rule{Text: r.Text, Scope: r.Scope})
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
		Passes:    compact.Passes{Pointerize: sw(c.Passes.Pointerize), TrimSpent: sw(c.Passes.TrimSpent), Unsaid: sw(c.Passes.Unsaid), Desk: sw(c.Passes.Desk), Episode: sw(c.Passes.Episode), Verify: sw(c.Passes.Verify)},
		KeepTurns: c.KeepRecentTurns,
		Cap:       cfg.Law.Wire.Cap * 2,
		Journal:   c.Journal,
	}
}

// lawBudget is law.budget as the loop reads it.
func lawBudget(cfg *config.Config, maxTokens int) loop.Budget {
	return loop.Budget{Steps: cfg.Law.Budget.Steps, Minutes: cfg.Law.Budget.Minutes, Retries: cfg.Law.Escalation.MaxRetries, MaxTokens: maxTokens,
		Context: instrument.Budget{Limit: cfg.Law.Budget.ContextTokens, Stress: cfg.Law.Budget.StressTokens}}
}

// router resolves and meters the provider for a world route (dispatch or session).
func (a *App) router() func(world.Route) provider.Provider {
	return func(r world.Route) provider.Provider {
		route := Route{Office: r.Office, Rank: r.Rank, Creature: r.Creature}
		if r.Rank == world.Rimuru {
			route.Rank = ""
		}
		pr, m, err := a.Providers.Route(route)
		if err != nil {
			a.hole(fmt.Sprintf("route %s/%s/%s: %v — the session's model stands in", r.Creature, r.Rank, r.Office, err))
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
		meter := &Meter{Provider: pr, Journal: a.Journal, Body: r.Creature, Rank: r.Rank, Office: r.Office, Model: m.Ref.Model, Price: price,
			Session: a.Cfg.Budgets.Session, Court: a.Cfg.Budgets.Court, Depth: depth}
		if a.Court != nil {
			a.Court.Track(r.Creature, r.Rank, r.Office, m.Ref.Model, nil, depth)
		}
		if a.OnState != nil {
			body := r.Creature
			meter.OnState = func(st string) { a.OnState(body, st) }
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
	return &Meter{Provider: pr, Journal: a.Journal, Body: task, Office: "", Model: m.Ref.Model, Price: price, Session: a.Cfg.Budgets.Session}
}

// budgetHook reads the body's meter before a model call.
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
			a.record(s, "compaction", map[string]interface{}{"after": a.Drainer.Last.After, "pointers": len(a.Drainer.Last.Pointers), "desk": a.Drainer.DeskPath(s.RunID)})
		}
		return ok, err
	}
	return h
}

// deskPath is where a session's desk (working memory) lives.
func (a *App) deskPath(run string) string {
	return filepath.Join(a.Root, a.Cfg.Dist.WorldDir, "instruments", "desk", run+".md")
}
