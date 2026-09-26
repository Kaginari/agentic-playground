package workspace

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/loop"
)

// Rule is one injected rule text (config supplies them). Scope is `all`, `rank:<rank>` or
// `member:<name>`.
type Rule struct {
	Text  string
	Scope string
}

// Applies reports whether the rule reaches an agent.
func (r Rule) Applies(as, rank string) bool {
	s := strings.ToLower(strings.TrimSpace(r.Scope))
	switch {
	case s == "" || s == "all":
		return true
	case strings.HasPrefix(s, "rank:"):
		return strings.TrimPrefix(s, "rank:") == rank || (rank == "" && strings.TrimPrefix(s, "rank:") == "orchestrator")
	case strings.HasPrefix(s, "member:"):
		return strings.TrimPrefix(s, "member:") == strings.ToLower(as)
	}
	return false
}

// OntologyOptions is the projection's switch and budget (tokens).
type OntologyOptions struct {
	Enabled bool
	Budget  int
}

// PromptOptions is the system-prompt builder's switchboard.
type PromptOptions struct {
	Principles   bool // policy.principles: the nine breaths, first and always
	Identity     bool // who the agent is, the classes, the gate, the wire (the engine's default text)
	Rules        []Rule
	Ontology     OntologyOptions
	Instructions bool // project instruction files (AGENTS.md / CLAUDE.md)
	Member       bool // the agent's own card: ownership, parent, skills, verify
	Memory       bool // one line on how the tiers are reached (the manifest itself rides the Recall hook)
}

// DefaultPrompt is everything on.
func DefaultPrompt() PromptOptions {
	return PromptOptions{Principles: true, Identity: true, Ontology: OntologyOptions{Enabled: true, Budget: 800}, Instructions: true, Member: true, Memory: true}
}

// System is the loop's System hook: principles · identity · injected rules · the agent's card ·
// ontology projection · instructions · memory line. The recall manifest and the toolbox brief
// are appended by the loop from the Recall hook, so a provider call sees one prompt.
func (w *Workspace) System(opt PromptOptions) func(s *loop.Session) string {
	return func(s *loop.Session) string {
		as := s.Engine.As
		if as == "" {
			as = s.Engine.Lexicon.Agent
		}
		return w.Prompt(as, opt, s.Engine)
	}
}

// Prompt builds the system prompt for an agent outside a session (a dispatcher's preview, the
// drain's rebuilt context). engine may be nil.
func (w *Workspace) Prompt(as string, opt PromptOptions, engine *loop.Engine) string {
	var b strings.Builder
	rank := w.RankOf(as).Name
	if rank == Orchestrator {
		rank = ""
	}
	if opt.Principles {
		if w.Policy != nil && w.Policy.Principles != "" {
			fmt.Fprintf(&b, "%s — %s\n\n%s\n\n", w.Policy.Path, or(w.Lex.Principles, "the principles"), w.Policy.Principles)
		} else {
			fmt.Fprintf(&b, "@? no principles: %s/%s is missing or has no principles section\n\n", w.Lex.WorkspaceDir, w.Lex.Policy)
		}
	}
	if opt.Identity {
		if engine != nil {
			b.WriteString(strings.TrimSpace(engine.DefaultSystem(nil)) + "\n\n")
		} else {
			fmt.Fprintf(&b, "You are %s, an agent working inside the workspace rooted at %s.\n\n", as, w.Root)
		}
	}
	if len(opt.Rules) > 0 {
		var lines []string
		for _, r := range opt.Rules {
			if r.Applies(as, rank) && strings.TrimSpace(r.Text) != "" {
				lines = append(lines, "- "+oneLine(r.Text))
			}
		}
		if len(lines) > 0 {
			b.WriteString("Rules in force for you:\n" + strings.Join(lines, "\n") + "\n\n")
		}
	}
	if opt.Member {
		if c := w.Member(as); c != nil {
			r, _ := w.Ranks.Get(c.Rank)
			fmt.Fprintf(&b, "You are %s (%s%s). Doc: %s. Owns: %s.", c.Name, c.Rank, jobOf(r), or(c.Doc, "none"), or(strings.Join(c.Ownership, ", "), "none declared"))
			if c.Parent != "" {
				fmt.Fprintf(&b, " One hop up: %s.", c.Parent)
			}
			if len(c.Skills) > 0 {
				fmt.Fprintf(&b, " Skills held: %s.", strings.Join(c.Skills, ", "))
			}
			if len(c.Verify) > 0 {
				fmt.Fprintf(&b, " Verify: %s.", strings.Join(c.Verify, " · "))
			}
			b.WriteString(" A write outside your ownership is refused; a change under it changes its doc in the same turn (Docs-as-code).\n\n")
		}
	}
	if opt.Ontology.Enabled && w.Onto != nil {
		budget := opt.Ontology.Budget
		if budget == 0 {
			budget = 800
		}
		name := as
		if w.Member(as) == nil {
			name = "orchestrator"
		}
		if lines, err := w.Onto.Project(name, budget); err == nil && len(lines) > 0 {
			b.WriteString("@ONTO\n" + strings.Join(lines, "\n") + "\n\n")
		}
	}
	if opt.Instructions {
		for _, in := range w.Instructions {
			fmt.Fprintf(&b, "Instructions (%s, %s):\n%s\n\n", in.Scope, w.Rel(in.Path), strings.TrimSpace(in.Text))
		}
	}
	if opt.Memory {
		fmt.Fprintf(&b, "Memory: @RECALL below carries anchors (path#section) into the tiers and @TOOLS the level-1 manifest; load a section or a Skill only on your own decision, never pre-emptively. What you learned that nothing on disk says goes out as @U <%s|%s|%s>; the policy's code is read with the `policy` tool by heading.\n",
			w.Lex.Token("policy"), w.Lex.Token("team"), w.Lex.Token("domain"))
	}
	return strings.TrimSpace(b.String())
}

func jobOf(r Rank) string {
	var tags []string
	if r.Authors {
		tags = append(tags, "authors")
	}
	if r.HoldsGate {
		tags = append(tags, "holds the gate")
	}
	if len(tags) == 0 {
		return ""
	}
	return ": " + strings.Join(tags, ", ")
}
