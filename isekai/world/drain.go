package world

import (
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/compact"
)

// Homes is where the drain sends each piece of a draining context in this world: the crest,
// shared notes, ontology facts, the log entry (compact.Homes). The integrator merges
// compact.New(opt, w.Homes()).Hooks() over w.Hooks(...) with MergeHooks.
func (w *World) Homes() compact.Homes {
	return compact.Homes{
		Root: w.Root, WorldDir: w.Lex.WorldDir,
		Crest: func() string {
			if w.Law != nil {
				return w.Law.Crest
			}
			return ""
		},
		Remember: func(as, kind, text string) error { return w.Remember(as, kind, text, "drain") },
		Assert: func(as, kind, text string) error {
			if w.Onto == nil {
				return nil
			}
			_, err := w.Onto.AssertUnsaid(as, kind, text)
			return err
		},
		Episode: func(as string, wrote []string, desk []string) error {
			return w.AppendLog(Entry{Author: as, Title: "drained: changes landed mid-session", Task: "compaction pass 5 — the episode recorded now, not at session end", Files: wrote, Gate: "pending — the turn's gate runs at its end", Result: "partial", Learned: desk})
		},
	}
}

// ScopeOf is the rank the drain journals for a body (its rank name, or rimuru).
func (w *World) ScopeOf(as string) string { return strings.ToLower(w.RankOf(as).Name) }
