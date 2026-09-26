package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/tool"
)

// Section is one heading-delimited block of the policy.
type Section struct {
	Title string
	Level int
	Text  string
}

// Policy is the loaded policy: the principles, read first and always; the code, consulted on demand.
type Policy struct {
	Path       string // relative to the root
	Text       string
	Principles string
	Sections   []Section
}

// LoadPolicy reads the policy file; principlesHeading names the principles section (else the first H2).
func LoadPolicy(path, rel, principlesHeading string) (*Policy, error) {
	text, ok := memory.Rd(path)
	if !ok {
		return nil, fmt.Errorf("no policy at %s", rel)
	}
	l := &Policy{Path: rel, Text: text}
	sm := memory.SectionMap(text)
	for _, s := range sm.Sections {
		agent := s.Text
		if i := strings.IndexByte(agent, '\n'); i >= 0 {
			agent = agent[i+1:] // the heading line is the title, not the text
		} else {
			agent = ""
		}
		l.Sections = append(l.Sections, Section{Title: s.T, Level: s.Depth, Text: strings.TrimSpace(agent)})
	}
	for _, s := range l.Sections {
		if principlesHeading != "" && strings.EqualFold(s.Title, principlesHeading) {
			l.Principles = s.Text
			break
		}
	}
	if l.Principles == "" {
		for _, s := range l.Sections {
			if s.Level == 2 {
				l.Principles = s.Text
				break
			}
		}
	}
	return l, nil
}

// Titles lists the section headings with their sizes, for the on-demand loader.
func (l *Policy) Titles() []string {
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
func (l *Policy) Section(title string) (Section, bool) {
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

// PolicyTool is the on-demand loader: `policy` with no section lists the headings; with one it
// returns that section's text. The principles never needs it — it is in every system prompt.
func (w *Workspace) PolicyTool() *tool.Tool {
	return &tool.Tool{
		Name:        "policy",
		Description: "Read one section of the policy by heading (the code, on demand). No section: list the headings.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"section":{"type":"string"}}}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var a struct{ Section string }
			_ = json.Unmarshal(in, &a)
			if w.Policy == nil {
				return tool.Result{Output: "no policy loaded (" + w.Lex.WorkspaceDir + "/" + w.Lex.Policy + " missing)", Err: true}
			}
			if strings.TrimSpace(a.Section) == "" {
				return tool.Result{Output: w.Policy.Path + "\n" + strings.Join(w.Policy.Titles(), "\n")}
			}
			s, ok := w.Policy.Section(a.Section)
			if !ok {
				return tool.Result{Output: fmt.Sprintf("no section %q in %s — headings:\n%s", a.Section, w.Policy.Path, strings.Join(w.Policy.Titles(), "\n")), Err: true}
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
