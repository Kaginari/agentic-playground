package world

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/memory"
	"github.com/Kaginari/agentic-playground/isekai/tool"
)

// Section is one heading-delimited block of the law.
type Section struct {
	Title string
	Level int
	Text  string
}

// Law is the loaded law: the crest, read first and always; the code, consulted on demand.
type Law struct {
	Path     string // relative to the root
	Text     string
	Crest    string
	Sections []Section
}

// LoadLaw reads the law file; crestHeading names the crest section (else the first H2).
func LoadLaw(path, rel, crestHeading string) (*Law, error) {
	text, ok := memory.Rd(path)
	if !ok {
		return nil, fmt.Errorf("no law at %s", rel)
	}
	l := &Law{Path: rel, Text: text}
	sm := memory.SectionMap(text)
	for _, s := range sm.Sections {
		body := s.Text
		if i := strings.IndexByte(body, '\n'); i >= 0 {
			body = body[i+1:] // the heading line is the title, not the text
		} else {
			body = ""
		}
		l.Sections = append(l.Sections, Section{Title: s.T, Level: s.Depth, Text: strings.TrimSpace(body)})
	}
	for _, s := range l.Sections {
		if crestHeading != "" && strings.EqualFold(s.Title, crestHeading) {
			l.Crest = s.Text
			break
		}
	}
	if l.Crest == "" {
		for _, s := range l.Sections {
			if s.Level == 2 {
				l.Crest = s.Text
				break
			}
		}
	}
	return l, nil
}

// Titles lists the section headings with their sizes, for the on-demand loader.
func (l *Law) Titles() []string {
	if l == nil {
		return nil
	}
	var out []string
	for _, s := range l.Sections {
		out = append(out, fmt.Sprintf("%s%s (%d bytes)", strings.Repeat("#", s.Level)+" ", s.Title, len(s.Text)))
	}
	return out
}

// Section finds a section by title: exact (case-insensitive), then prefix, then contains.
func (l *Law) Section(title string) (Section, bool) {
	if l == nil {
		return Section{}, false
	}
	t := strings.ToLower(strings.TrimSpace(title))
	for _, match := range []func(a, b string) bool{func(a, b string) bool { return a == b }, strings.HasPrefix, strings.Contains} {
		for _, s := range l.Sections {
			if match(strings.ToLower(s.Title), t) {
				return s, true
			}
		}
	}
	return Section{}, false
}

// LawTool is the on-demand loader: `law` with no section lists the headings; with one it
// returns that section's text. The crest never needs it — it is in every system prompt.
func (w *World) LawTool() *tool.Tool {
	return &tool.Tool{
		Name:        "law",
		Description: "Read one section of the law by heading (the code, on demand). No section: list the headings.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"section":{"type":"string"}}}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var a struct{ Section string }
			_ = json.Unmarshal(in, &a)
			if w.Law == nil {
				return tool.Result{Output: "no law loaded (" + w.Lex.WorldDir + "/" + w.Lex.Law + " missing)", Err: true}
			}
			if strings.TrimSpace(a.Section) == "" {
				return tool.Result{Output: w.Law.Path + "\n" + strings.Join(w.Law.Titles(), "\n")}
			}
			s, ok := w.Law.Section(a.Section)
			if !ok {
				return tool.Result{Output: fmt.Sprintf("no section %q in %s — headings:\n%s", a.Section, w.Law.Path, strings.Join(w.Law.Titles(), "\n")), Err: true}
			}
			return tool.Result{Output: s.Text}
		},
	}
}

// Instruction is one project-instruction file (AGENTS.md, CLAUDE.md) as a harness reads it.
type Instruction struct {
	Path  string // absolute
	Scope string // project | global
	Text  string
}

// InstructionOptions is discovery.instructions.
type InstructionOptions struct {
	Enabled bool
	Files   []string // in priority order; default AGENTS.md, CLAUDE.md
	Global  []string // ~-prefixed paths tried after the project walk-up
	WalkUp  bool     // look above the root too; nearest wins, no stacking
}

// LoadInstructions finds the nearest project instruction file (root, then parents when WalkUp)
// and the first global one. Nearest wins; files never stack.
func LoadInstructions(root string, opt InstructionOptions) []Instruction {
	if !opt.Enabled {
		return nil
	}
	files := opt.Files
	if len(files) == 0 {
		files = []string{"AGENTS.md", "CLAUDE.md"}
	}
	var out []Instruction
	dir := root
	for {
		found := false
		for _, f := range files {
			p := dir + string(os.PathSeparator) + f
			if text, ok := memory.Rd(p); ok && strings.TrimSpace(text) != "" {
				out = append(out, Instruction{Path: p, Scope: "project", Text: text})
				found = true
				break
			}
		}
		if found || !opt.WalkUp {
			break
		}
		parent := parentDir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	home, _ := os.UserHomeDir()
	for _, g := range opt.Global {
		p := g
		if strings.HasPrefix(p, "~/") {
			p = home + p[1:]
		}
		if text, ok := memory.Rd(p); ok && strings.TrimSpace(text) != "" {
			out = append(out, Instruction{Path: p, Scope: "global", Text: text})
			break
		}
	}
	return out
}
