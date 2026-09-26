// Package workspace is the policy as a harness's home: discovery of the workspace root, the policy loader
// (principles always, code sections on demand), members from their docs (through onto's
// derivation), project instructions, the append-only log, ownership enforcement, the Domain owner's gate
// with Docs-as-code, the system prompt, the loop hooks that wire memory / toolbox / onto in, and the
// Subagent dispatcher. Every behaviour is an options struct with an Enabled switch; nothing global.
package workspace

import (
	"strings"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/onto"
)

// RankNames in canonical order (the disk order of onto and the toolbox).
var RankNames = []string{"coord", "domain", "zone", "service"}

// Lexicon is every user-facing word the engine speaks — one place, one vocabulary.
type Lexicon struct {
	Vocab        string            // "agent-one"
	Binary       string            // the binary name
	WorkspaceDir string            // ".agent-one"
	Machine      string            // "~/.agent-one"
	Policy       string            // "AGENT-ONE.md"
	Log          string            // "log.md"
	Ranks        map[string]string // canonical rank → dir
	Prefixes     map[string]string // canonical rank → name prefix
	Unsaid       map[string]string // canonical kind → wire token
	Tokens       map[string]string // every accepted token → canonical kind
	Principles   string            // the heading of the section read first
	GateNA       string            // gate line for a workspace without domain owners
	Checks       [4]string         // the four gate checks
	Human        string            // the human's title
	Session      string            // the session agent's title
	Subagent     string            // the disposable agent's title
	Notes        string            // the dated notes heading
	Owns         string            // the doc key naming owned paths
	Display      map[string]string // rank → display name
}

// Default is the engine's vocabulary.
func Default() Lexicon {
	return Lexicon{
		Vocab: "agent-one", Binary: "agent-one", WorkspaceDir: ".agent-one", Machine: "~/.agent-one", Policy: "AGENT-ONE.md", Log: "log.md",
		Ranks:      map[string]string{"coord": "coord", "domain": "domain", "zone": "zone", "service": "service", "auditor": "auditor"},
		Prefixes:   map[string]string{"coord": "coord-", "domain": "domain-", "zone": "zone-", "service": "service-", "auditor": "auditor-"},
		Unsaid:     map[string]string{"policy": "policy", "team": "team", "domain": "domain"},
		Tokens:     map[string]string{"policy": "policy", "team": "team", "domain": "domain"},
		Principles: "Core principles",
		GateNA:     "Gate: n/a (no domain owners)",
		Checks:     [4]string{"Right zone worker authored", "Invariants hold", "Duties done", "Doc truthful"},
		Human:      "Operator", Session: "Orchestrator", Subagent: "Ephemeral subagent",
		Notes: "## Working notes", Owns: "Owns:",
		Display: map[string]string{"coord": "Coordinator", "domain": "Domain owner", "zone": "Zone worker", "service": "Service owner", "auditor": "Auditor"},
	}
}

// Layout is the onto layout of this vocabulary with the policy's own ranks.
func (l Lexicon) Layout() onto.Layout { return Ranks(DefaultRanks(l)).Layout(l) }

// Loop is the loop engine's lexicon for this vocabulary.
func (l Lexicon) Loop() loop.Lexicon {
	agent := "ephemeral-subagent"
	if l.Subagent != "" {
		agent = strings.ToLower(strings.ReplaceAll(l.Subagent, " ", "-"))
	}
	human := "the human"
	if l.Human != "" {
		human = l.Human
	}
	return loop.Lexicon{Agent: agent, Human: human, WorkspaceDir: l.WorkspaceDir, Policy: "The policy of this workspace is " + l.WorkspaceDir + "/" + l.Policy + "; its principles is read first, always."}
}

// Rank reads the policy's rank off a member id by its prefix ("" for orchestrator or unknown).
func (l Lexicon) Rank(name string) string { return Ranks(DefaultRanks(l)).Of(name) }

// Canonical maps a wire kind token (any accepted spelling) to the on-disk kind.
func (l Lexicon) Canonical(kind string) (string, bool) {
	k, ok := l.Tokens[strings.ToLower(strings.TrimSpace(kind))]
	return k, ok
}

// Token is the wire spelling of a canonical kind.
func (l Lexicon) Token(kind string) string {
	if t := l.Unsaid[kind]; t != "" {
		return t
	}
	return kind
}
