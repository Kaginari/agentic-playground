package workspace

import (
	"strings"

	"github.com/Kaginari/agent-one/compact"
)

// Homes is where the drain sends each piece of a draining context in this workspace: the principles,
// shared notes, ontology facts, the log entry (compact.Homes). The integrator merges
// compact.New(opt, w.Homes()).Hooks() over w.Hooks(...) with MergeHooks.
func (w *Workspace) Homes() compact.Homes {
	return compact.Homes{
		Root: w.Root, WorkspaceDir: w.Lex.WorkspaceDir,
		Principles: func() string {
			if w.Policy != nil {
				return w.Policy.Principles
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
		Episode: func(as string, wrote []string, workingNotes []string) error {
			return w.AppendLog(Entry{Author: as, Title: "drained: changes landed mid-session", Task: "compaction pass 5 — the episode recorded now, not at session end", Files: wrote, Gate: "pending — the turn's gate runs at its end", Result: "partial", Learned: workingNotes})
		},
	}
}

// ScopeOf is the rank the drain journals for a agent (its rank name, or orchestrator).
func (w *Workspace) ScopeOf(as string) string { return strings.ToLower(w.RankOf(as).Name) }
