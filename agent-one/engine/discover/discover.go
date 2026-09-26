// Package discover reads what other harnesses wrote so a workspace set up under Claude Code or
// OpenCode keeps working (harness-parity.md §Compatibility promises): instruction files, skills
// (level 1: name + description; the agent on load), slash commands, agent definitions and MCP
// imports. Read-only translations into plain structs; each source behind its own switch.
package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Options is discovery.* from config. Home "" means the user's home.
type Options struct {
	Root         string
	Home         string
	Instructions bool
	Skills       bool
	Commands     bool
	Agents       bool
	MCP          bool
	Sources      Sources // nil-safe: a zero Sources enables every source
}

// Sources switches individual origins (each true by default via Default).
type Sources struct {
	Claude   *bool // .claude/, ~/.claude/, CLAUDE.md, .mcp.json
	OpenCode *bool // .opencode/, ~/.config/opencode/, AGENTS.md, opencode.json(c)
}

func on(b *bool) bool { return b == nil || *b }

// Instruction is one project-instruction file, nearest first.
type Instruction struct {
	Path    string
	Source  string // "claude" | "opencode" | "claude-global" | "opencode-global"
	Content string
}

// Skill is level 1 of a Skill: name and description; Load reads the agent.
type Skill struct {
	Name        string
	Description string
	Path        string // SKILL.md
	Dir         string
	Source      string
	Frontmatter map[string]string
}

// Load reads the skill's agent (frontmatter stripped).
func (s Skill) Load() (string, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return "", err
	}
	_, agent := Frontmatter(string(data))
	return agent, nil
}

// Command is a slash command template.
type Command struct {
	Name        string
	Description string
	Path        string
	Source      string
	Frontmatter map[string]string
	Template    string
}

var argN = regexp.MustCompile(`\$([1-9])`)

// Expand fills $ARGUMENTS and $1..$9 into the template.
func (c Command) Expand(args string) string {
	words := strings.Fields(args)
	out := strings.ReplaceAll(c.Template, "$ARGUMENTS", args)
	return argN.ReplaceAllStringFunc(out, func(m string) string {
		i := int(m[1] - '1')
		if i < len(words) {
			return words[i]
		}
		return ""
	})
}

// Agent is a sub-agent definition.
type Agent struct {
	Name        string
	Description string
	Model       string
	Mode        string // "subagent" default; "all" | "primary" from OpenCode
	Tools       []string
	Path        string
	Source      string
	Frontmatter map[string]string
	Prompt      string
}

// MCPServer is an imported server definition.
type MCPServer struct {
	Name      string
	Transport string // "stdio" | "http"
	Command   string
	Args      []string
	Env       map[string]string
	URL       string
	Headers   map[string]string
	Enabled   bool
	Source    string // "claude:.mcp.json" | "opencode:opencode.jsonc"
	Path      string
}

// Result is everything found.
type Result struct {
	Instructions []Instruction
	Skills       []Skill
	Commands     []Command
	Agents       []Agent
	MCP          []MCPServer
	Errors       []string
}

// All runs every enabled discovery.
func All(opt Options) Result {
	var r Result
	if opt.Instructions {
		r.Instructions = Instructions(opt)
	}
	if opt.Skills {
		r.Skills = Skills(opt)
	}
	if opt.Commands {
		r.Commands = Commands(opt)
	}
	if opt.Agents {
		r.Agents = Agents(opt)
	}
	if opt.MCP {
		var errs []error
		r.MCP, errs = MCP(opt)
		for _, e := range errs {
			r.Errors = append(r.Errors, e.Error())
		}
	}
	return r
}

func (o Options) home() string {
	if o.Home != "" {
		return o.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

func (o Options) root() string {
	if o.Root != "" {
		return o.Root
	}
	d, _ := os.Getwd()
	return d
}

// Instructions walks up from the root collecting AGENTS.md and CLAUDE.md (nearest first, both
// per directory, AGENTS.md before CLAUDE.md), then the global pair.
func Instructions(opt Options) []Instruction {
	var out []Instruction
	add := func(p, src string) {
		data, err := os.ReadFile(p)
		if err != nil {
			return
		}
		out = append(out, Instruction{Path: p, Source: src, Content: string(data)})
	}
	dir := filepath.Clean(opt.root())
	for {
		if on(opt.Sources.OpenCode) {
			add(filepath.Join(dir, "AGENTS.md"), "opencode")
		}
		if on(opt.Sources.Claude) {
			add(filepath.Join(dir, "CLAUDE.md"), "claude")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	home := opt.home()
	if home != "" {
		if on(opt.Sources.Claude) {
			add(filepath.Join(home, ".claude", "CLAUDE.md"), "claude-global")
		}
		if on(opt.Sources.OpenCode) {
			add(filepath.Join(home, ".config", "opencode", "AGENTS.md"), "opencode-global")
		}
	}
	return out
}

type dirSource struct {
	dir, source string
}

func (o Options) skillDirs() []dirSource {
	var ds []dirSource
	root, home := o.root(), o.home()
	if on(o.Sources.Claude) {
		ds = append(ds, dirSource{filepath.Join(root, ".claude", "skills"), "claude"})
	}
	if on(o.Sources.OpenCode) {
		ds = append(ds, dirSource{filepath.Join(root, ".opencode", "skill"), "opencode"}, dirSource{filepath.Join(root, ".opencode", "skills"), "opencode"}, dirSource{filepath.Join(root, ".agents", "skills"), "opencode"})
	}
	if home != "" {
		if on(o.Sources.Claude) {
			ds = append(ds, dirSource{filepath.Join(home, ".claude", "skills"), "claude-global"})
		}
		if on(o.Sources.OpenCode) {
			ds = append(ds, dirSource{filepath.Join(home, ".config", "opencode", "skill"), "opencode-global"}, dirSource{filepath.Join(home, ".config", "opencode", "skills"), "opencode-global"})
		}
	}
	return ds
}

// Skills lists every <dir>/<name>/SKILL.md; the first name found wins (project before global).
func Skills(opt Options) []Skill {
	seen := map[string]bool{}
	var out []Skill
	for _, ds := range opt.skillDirs() {
		entries, err := os.ReadDir(ds.dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
				continue
			}
			p := filepath.Join(ds.dir, e.Name(), "SKILL.md")
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			fm, _ := Frontmatter(string(data))
			name := fm["name"]
			if name == "" {
				name = e.Name()
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, Skill{Name: name, Description: fm["description"], Path: p, Dir: filepath.Join(ds.dir, e.Name()), Source: ds.source, Frontmatter: fm})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (o Options) commandDirs() []dirSource {
	var ds []dirSource
	root, home := o.root(), o.home()
	if on(o.Sources.Claude) {
		ds = append(ds, dirSource{filepath.Join(root, ".claude", "commands"), "claude"})
	}
	if on(o.Sources.OpenCode) {
		ds = append(ds, dirSource{filepath.Join(root, ".opencode", "command"), "opencode"}, dirSource{filepath.Join(root, ".opencode", "commands"), "opencode"})
	}
	if home != "" {
		if on(o.Sources.Claude) {
			ds = append(ds, dirSource{filepath.Join(home, ".claude", "commands"), "claude-global"})
		}
		if on(o.Sources.OpenCode) {
			ds = append(ds, dirSource{filepath.Join(home, ".config", "opencode", "command"), "opencode-global"}, dirSource{filepath.Join(home, ".config", "opencode", "commands"), "opencode-global"})
		}
	}
	return ds
}

// Commands lists <dir>/**/*.md as slash commands; a nested path becomes name:sub (Claude
// Code's convention). Project before global; first name wins.
func Commands(opt Options) []Command {
	seen := map[string]bool{}
	var out []Command
	for _, ds := range opt.commandDirs() {
		_ = filepath.Walk(ds.dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			rel, _ := filepath.Rel(ds.dir, p)
			name := strings.ReplaceAll(strings.TrimSuffix(filepath.ToSlash(rel), ".md"), "/", ":")
			if seen[name] {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			fm, agent := Frontmatter(string(data))
			seen[name] = true
			out = append(out, Command{Name: name, Description: fm["description"], Path: p, Source: ds.source, Frontmatter: fm, Template: agent})
			return nil
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (o Options) agentDirs() []dirSource {
	var ds []dirSource
	root, home := o.root(), o.home()
	if on(o.Sources.Claude) {
		ds = append(ds, dirSource{filepath.Join(root, ".claude", "agents"), "claude"})
	}
	if on(o.Sources.OpenCode) {
		ds = append(ds, dirSource{filepath.Join(root, ".opencode", "agent"), "opencode"}, dirSource{filepath.Join(root, ".opencode", "agents"), "opencode"})
	}
	if home != "" {
		if on(o.Sources.Claude) {
			ds = append(ds, dirSource{filepath.Join(home, ".claude", "agents"), "claude-global"})
		}
		if on(o.Sources.OpenCode) {
			ds = append(ds, dirSource{filepath.Join(home, ".config", "opencode", "agent"), "opencode-global"}, dirSource{filepath.Join(home, ".config", "opencode", "agents"), "opencode-global"})
		}
	}
	return ds
}

// Agents lists agent definitions: frontmatter name/description/model/tools/mode, agent prompt.
func Agents(opt Options) []Agent {
	seen := map[string]bool{}
	var out []Agent
	for _, ds := range opt.agentDirs() {
		_ = filepath.Walk(ds.dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			fm, agent := Frontmatter(string(data))
			name := fm["name"]
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(p), ".md")
			}
			if seen[name] {
				return nil
			}
			seen[name] = true
			mode := fm["mode"]
			if mode == "" {
				mode = "subagent"
			}
			out = append(out, Agent{Name: name, Description: fm["description"], Model: fm["model"], Mode: mode, Tools: List(fm["tools"]), Path: p, Source: ds.source, Frontmatter: fm, Prompt: strings.TrimSpace(agent)})
			return nil
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// MCP reads .mcp.json (Claude Code) and opencode.json(c)'s mcp block. Malformed files are
// returned as errors, never silently skipped.
func MCP(opt Options) ([]MCPServer, []error) {
	var out []MCPServer
	var errs []error
	root := opt.root()
	if on(opt.Sources.Claude) {
		p := filepath.Join(root, ".mcp.json")
		if data, err := os.ReadFile(p); err == nil {
			ss, err := ParseClaudeMCP(data)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %v", p, err))
			}
			for i := range ss {
				ss[i].Source, ss[i].Path = "claude:.mcp.json", p
			}
			out = append(out, ss...)
		}
	}
	if on(opt.Sources.OpenCode) {
		for _, name := range []string{"opencode.jsonc", "opencode.json"} {
			p := filepath.Join(root, name)
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			ss, err := ParseOpenCodeMCP(data)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %v", p, err))
			}
			for i := range ss {
				ss[i].Source, ss[i].Path = "opencode:"+name, p
			}
			out = append(out, ss...)
		}
	}
	return out, errs
}

// ParseClaudeMCP reads {"mcpServers": {name: {command, args, env} | {type: "http"|"sse", url, headers}}}.
func ParseClaudeMCP(data []byte) ([]MCPServer, error) {
	var f struct {
		Servers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(StripJSONC(data), &f); err != nil {
		return nil, err
	}
	var out []MCPServer
	for name, s := range f.Servers {
		m := MCPServer{Name: name, Command: s.Command, Args: s.Args, Env: s.Env, URL: s.URL, Headers: s.Headers, Enabled: true}
		switch {
		case s.Type == "http" || s.Type == "sse" || (s.Command == "" && s.URL != ""):
			m.Transport = "http"
		default:
			m.Transport = "stdio"
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ParseOpenCodeMCP reads {"mcp": {name: {type: "local", command: [...], environment, enabled} |
// {type: "remote", url, headers, enabled}}} from opencode.json(c).
func ParseOpenCodeMCP(data []byte) ([]MCPServer, error) {
	var f struct {
		MCP map[string]struct {
			Type        string            `json:"type"`
			Command     []string          `json:"command"`
			Environment map[string]string `json:"environment"`
			URL         string            `json:"url"`
			Headers     map[string]string `json:"headers"`
			Enabled     *bool             `json:"enabled"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal(StripJSONC(data), &f); err != nil {
		return nil, err
	}
	var out []MCPServer
	for name, s := range f.MCP {
		m := MCPServer{Name: name, Env: s.Environment, URL: s.URL, Headers: s.Headers, Enabled: s.Enabled == nil || *s.Enabled}
		if s.Type == "remote" || (len(s.Command) == 0 && s.URL != "") {
			m.Transport = "http"
		} else {
			m.Transport = "stdio"
			if len(s.Command) > 0 {
				m.Command, m.Args = s.Command[0], s.Command[1:]
			}
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// StripJSONC removes // and /* */ comments outside strings and trailing commas.
func StripJSONC(data []byte) []byte {
	var out []byte
	s := string(data)
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inStr:
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}
	return []byte(trailingComma.ReplaceAllString(string(out), "$1"))
}

var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)

// Frontmatter splits a `---` YAML header off a markdown file into a flat map (a small YAML
// subset: `key: value`, quoted values, `[a, b]` lists) and returns the agent.
func Frontmatter(text string) (map[string]string, string) {
	fm := map[string]string{}
	t := strings.TrimPrefix(text, "\xef\xbb\xbf")
	if !strings.HasPrefix(t, "---\n") && !strings.HasPrefix(t, "---\r\n") {
		return fm, text
	}
	rest := t[strings.IndexByte(t, '\n')+1:]
	end := -1
	lines := strings.Split(rest, "\n")
	for i, l := range lines {
		if strings.TrimRight(l, "\r") == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return fm, text
	}
	lastKey := ""
	for _, l := range lines[:end] {
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) == "" || strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		if (strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "\t")) && lastKey != "" {
			// a continuation or a nested `- item` list under the last key
			v := strings.TrimSpace(l)
			v = strings.TrimPrefix(v, "- ")
			if fm[lastKey] == "" {
				fm[lastKey] = v
			} else {
				fm[lastKey] += ", " + v
			}
			continue
		}
		i := strings.IndexByte(l, ':')
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(l[:i])
		v := strings.TrimSpace(l[i+1:])
		if v == "|" || v == ">" {
			v = ""
		}
		fm[k] = unquote(v)
		lastKey = k
	}
	agent := strings.Join(lines[end+1:], "\n")
	return fm, strings.TrimLeft(agent, "\r\n")
}

func unquote(v string) string {
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		inner := v[1 : len(v)-1]
		if v[0] == '"' {
			inner = strings.ReplaceAll(inner, `\"`, `"`)
		}
		return inner
	}
	return v
}

// List reads a frontmatter list value: `[a, b]`, `a, b` or `a b`.
func List(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	v = strings.TrimSuffix(strings.TrimPrefix(v, "["), "]")
	sep := ","
	if !strings.Contains(v, ",") {
		sep = " "
	}
	var out []string
	for _, p := range strings.Split(v, sep) {
		if p = unquote(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
