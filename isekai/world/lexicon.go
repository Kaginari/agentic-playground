// Package world is the law as a harness's home: discovery of the world root, the law loader
// (crest always, code sections on demand), creatures from their docs (through onto's
// derivation), project instructions, the append-only log, territory enforcement, the Orc's gate
// with Vitality, the system prompt, the loop hooks that wire memory / toolbox / onto in, and the
// Court dispatcher. Every behaviour is an options struct with an Enabled switch; nothing global.
package world

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/onto"
)

// lexiconJSON is the binary's copy of agent-one/lexicon.json; a test keeps the two equal.
//
//go:embed lexicon.json
var lexiconJSON []byte

// Races in canonical order (the disk order of onto and the toolbox).
var Races = []string{"elf", "orc", "slime", "kijin"}

// Lexicon is every word a distribution renames, read from lexicon.json for one vocabulary.
type Lexicon struct {
	Vocab     string            // "isekai" | "agent-one"
	Binary    string            // tool.binary
	WorldDir  string            // dir.root: ".isekai"
	Machine   string            // dir.machine: "~/.isekai"
	Law       string            // file.law: "isekai.md"
	Log       string            // file.log
	Ranks     map[string]string // canonical race → dir (rank.<race>.dir)
	Prefixes  map[string]string // canonical race → name prefix (rank.<race>.prefix)
	Unsaid    map[string]string // canonical kind → wire token (kind.unsaid.<kind>.token)
	Tokens    map[string]string // every vocabulary's token → canonical kind (either spelling is accepted on input)
	Crest     string            // section.crest heading text
	GateNA    string            // gate.na line for a world without orcs
	Checks    [4]string         // gate.check1..4
	Human     string            // rank.veldora
	Session   string            // rank.rimuru
	Court     string            // unit.court_body
	Thoughts  string            // heading.thoughts
	Territory string            // field.territory
	Display   map[string]string // race → display name (rank.<race>)
}

// Isekai is the canonical vocabulary.
func Isekai() Lexicon { l, _ := ParseLexicon(lexiconJSON, "isekai"); return l }

// AgentOne is the IT vocabulary.
func AgentOne() Lexicon { l, _ := ParseLexicon(lexiconJSON, "agent-one"); return l }

// Lexicons lists both, isekai first — the order Discover tries them in.
func Lexicons() []Lexicon { return []Lexicon{Isekai(), AgentOne()} }

type lexFile struct {
	Schema       int                                   `json:"schema"`
	Vocabularies []string                              `json:"vocabularies"`
	Canonical    string                                `json:"canonical"`
	Entries      map[string]map[string]json.RawMessage `json:"entries"`
}

// ParseLexicon reads a lexicon.json for one vocabulary.
func ParseLexicon(data []byte, vocab string) (Lexicon, error) {
	var f lexFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Lexicon{}, fmt.Errorf("lexicon: %v", err)
	}
	if f.Schema != 1 {
		return Lexicon{}, fmt.Errorf("lexicon: schema %d, want 1", f.Schema)
	}
	known := false
	for _, v := range f.Vocabularies {
		known = known || v == vocab
	}
	if !known {
		return Lexicon{}, fmt.Errorf("lexicon: unknown vocabulary %q (%s)", vocab, strings.Join(f.Vocabularies, ", "))
	}
	str := func(key, facet string) string {
		e, ok := f.Entries[key]
		if !ok {
			return ""
		}
		raw, ok := e[facet]
		if !ok {
			return ""
		}
		if facet == vocab || facet == "canonical" {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				return s
			}
			return ""
		}
		var m map[string]*string
		if json.Unmarshal(raw, &m) == nil {
			if v := m[vocab]; v != nil {
				return *v
			}
		}
		return ""
	}
	l := Lexicon{Vocab: vocab, Ranks: map[string]string{}, Prefixes: map[string]string{}, Unsaid: map[string]string{}, Tokens: map[string]string{}, Display: map[string]string{}}
	l.Binary = str("tool.binary", vocab)
	l.WorldDir = str("dir.root", vocab)
	l.Machine = str("dir.machine", vocab)
	l.Law = str("file.law", vocab)
	l.Log = str("file.log", vocab)
	l.Crest = str("section.crest", vocab)
	l.GateNA = str("gate.na", vocab)
	for i := range l.Checks {
		l.Checks[i] = str(fmt.Sprintf("gate.check%d", i+1), vocab)
	}
	l.Human = str("rank.veldora", vocab)
	l.Session = str("rank.rimuru", vocab)
	l.Court = str("unit.court_body", vocab)
	l.Thoughts = str("heading.thoughts", vocab)
	l.Territory = str("field.territory", vocab)
	for _, r := range append(append([]string(nil), Races...), "dark_elf") {
		l.Ranks[r] = str("rank."+r, "dir")
		l.Prefixes[r] = str("rank."+r, "prefix")
		l.Display[r] = str("rank."+r, vocab)
	}
	for _, k := range []string{"law", "colony", "territory"} {
		l.Unsaid[k] = str("kind.unsaid."+k, "token")
		l.Tokens[k] = k
		var toks map[string]string
		if e, ok := f.Entries["kind.unsaid."+k]; ok {
			_ = json.Unmarshal(e["token"], &toks)
		}
		for _, t := range toks {
			l.Tokens[t] = k
		}
	}
	if l.WorldDir == "" || l.Law == "" || l.Ranks["slime"] == "" {
		return Lexicon{}, fmt.Errorf("lexicon: vocabulary %q lacks dir.root, file.law or rank dirs", vocab)
	}
	if l.Log == "" {
		l.Log = "log.md"
	}
	return l, nil
}

// Layout is the onto layout of this vocabulary with the law's own ranks.
func (l Lexicon) Layout() onto.Layout { return Ranks(DefaultRanks(l)).Layout(l) }

// Loop is the loop engine's lexicon for this vocabulary.
func (l Lexicon) Loop() loop.Lexicon {
	body := "court-body"
	if l.Court != "" {
		body = strings.ToLower(strings.ReplaceAll(l.Court, " ", "-"))
	}
	human := "the human"
	if l.Human != "" {
		human = l.Human
	}
	return loop.Lexicon{Body: body, Human: human, WorldDir: l.WorldDir, Law: "The law of this world is " + l.WorldDir + "/" + l.Law + "; its crest is read first, always."}
}

// Race reads the law's rank off a creature id by its prefix ("" for rimuru or unknown).
func (l Lexicon) Race(name string) string { return Ranks(DefaultRanks(l)).Of(name) }

// Canonical maps a wire kind token (any vocabulary's, or the canonical) to the on-disk kind.
func (l Lexicon) Canonical(kind string) (string, bool) {
	k, ok := l.Tokens[strings.ToLower(strings.TrimSpace(kind))]
	return k, ok
}

// Token is the wire spelling of a canonical kind in this vocabulary.
func (l Lexicon) Token(kind string) string {
	if t := l.Unsaid[kind]; t != "" {
		return t
	}
	return kind
}
