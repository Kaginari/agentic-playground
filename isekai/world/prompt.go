package world

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/loop"
)

// Rule is one injected rule text (config supplies them). Scope is `all`, `rank:<race>` or
// `creature:<name>`.
type Rule struct {
	Text  string
	Scope string
}

// Applies reports whether the rule reaches a body.
func (r Rule) Applies(as, race string) bool {
	s := strings.ToLower(strings.TrimSpace(r.Scope))
	switch {
	case s == "" || s == "all":
		return true
	case strings.HasPrefix(s, "rank:"):
		return strings.TrimPrefix(s, "rank:") == race || (race == "" && strings.TrimPrefix(s, "rank:") == "rimuru")
	case strings.HasPrefix(s, "creature:"):
		return strings.TrimPrefix(s, "creature:") == strings.ToLower(as)
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
	Crest        bool // law.crest: the nine breaths, first and always
	Identity     bool // who the body is, the classes, the gate, the wire (the engine's default text)
	Rules        []Rule
	Ontology     OntologyOptions
	Instructions bool // project instruction files (AGENTS.md / CLAUDE.md)
	Creature     bool // the body's own card: territory, parent, minds, verify
	Memory       bool // one line on how the tiers are reached (the manifest itself rides the Recall hook)
}

// DefaultPrompt is everything on.
func DefaultPrompt() PromptOptions {
	return PromptOptions{Crest: true, Identity: true, Ontology: OntologyOptions{Enabled: true, Budget: 800}, Instructions: true, Creature: true, Memory: true}
}

// System is the loop's System hook: crest · identity · injected rules · the body's card ·
// ontology projection · instructions · memory line. The recall manifest and the toolbox brief
// are appended by the loop from the Recall hook, so a provider call sees one prompt.
func (w *World) System(opt PromptOptions) func(s *loop.Session) string {
	return func(s *loop.Session) string {
		as := s.Engine.As
		if as == "" {
			as = s.Engine.Lexicon.Body
		}
		return w.Prompt(as, opt, s.Engine)
	}
}

// Prompt builds the system prompt for a body outside a session (a dispatcher's preview, the
// drain's rebuilt context). engine may be nil.
func (w *World) Prompt(as string, opt PromptOptions, engine *loop.Engine) string {
	var b strings.Builder
	race := w.RankOf(as).Name
	if race == Rimuru {
		race = ""
	}
	if opt.Crest {
		if w.Law != nil && w.Law.Crest != "" {
			fmt.Fprintf(&b, "%s — %s\n\n%s\n\n", w.Law.Path, or(w.Lex.Crest, "the crest"), w.Law.Crest)
		} else {
			fmt.Fprintf(&b, "@? no crest: %s/%s is missing or has no crest section\n\n", w.Lex.WorldDir, w.Lex.Law)
		}
	}
	if opt.Identity {
		if engine != nil {
			b.WriteString(strings.TrimSpace(engine.DefaultSystem(nil)) + "\n\n")
		} else {
			fmt.Fprintf(&b, "You are %s, a body working inside the world rooted at %s.\n\n", as, w.Root)
		}
	}
	if len(opt.Rules) > 0 {
		var lines []string
		for _, r := range opt.Rules {
			if r.Applies(as, race) && strings.TrimSpace(r.Text) != "" {
				lines = append(lines, "- "+oneLine(r.Text))
			}
		}
		if len(lines) > 0 {
			b.WriteString("Rules in force for you:\n" + strings.Join(lines, "\n") + "\n\n")
		}
	}
	if opt.Creature {
		if c := w.Creature(as); c != nil {
			r, _ := w.Ranks.Get(c.Rank)
			fmt.Fprintf(&b, "You are %s (%s%s). Doc: %s. Territory: %s.", c.Name, c.Rank, jobOf(r), or(c.Doc, "none"), or(strings.Join(c.Territory, ", "), "none declared"))
			if c.Parent != "" {
				fmt.Fprintf(&b, " One hop up: %s.", c.Parent)
			}
			if len(c.Minds) > 0 {
				fmt.Fprintf(&b, " Minds worn: %s.", strings.Join(c.Minds, ", "))
			}
			if len(c.Verify) > 0 {
				fmt.Fprintf(&b, " Verify: %s.", strings.Join(c.Verify, " · "))
			}
			b.WriteString(" A write outside your territory is refused; a change under it changes its doc in the same turn (Vitality).\n\n")
		}
	}
	if opt.Ontology.Enabled && w.Onto != nil {
		budget := opt.Ontology.Budget
		if budget == 0 {
			budget = 800
		}
		name := as
		if w.Creature(as) == nil {
			name = "rimuru"
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
		fmt.Fprintf(&b, "Memory: @RECALL below carries anchors (path#section) into the tiers and @TOOLS the level-1 manifest; load a section or a Mind only on your own decision, never pre-emptively. What you learned that nothing on disk says goes out as @U <%s|%s|%s>; the law's code is read with the `law` tool by heading.\n",
			w.Lex.Token("law"), w.Lex.Token("colony"), w.Lex.Token("territory"))
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
