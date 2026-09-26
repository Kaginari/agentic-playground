// Package toolbox is the Go port of .agent-one/tools/toolbox.js — a big registry, a small prompt
// (AGENT-ONE.md §Skills & Agents, the toolbox). Same registry file, same journal, same @T/@TOOLS lines.
package toolbox

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/memory"
)

const (
	RegV          = 1
	BudgetDefault = 1500
	MinDefault    = 0.0
	FitWords      = 2
	PipeBuf       = 4096
)

var Kinds = []string{"skill", "command", "tool", "agent", "external"}

var laneOfRank = map[string]string{"zone": "zone", "domain": "review", "coord": "global", "service": "global"}
var rankOrder = []string{"zone", "domain", "coord", "service"}
var triggerW = map[string]float64{"frontmatter": 0.8, "quoted": 0.8, "name": 0.5, "sentence": 0.4}

// TOK is ~4 bytes/token: an estimate, not a billing figure.
func TOK(s string) int { return int(math.Ceil(float64(len(s)) / 4)) }

type Fail = memory.Fail

func failf(format string, a ...any) error { return &Fail{Holes: []string{fmt.Sprintf(format, a...)}} }

type Workspace struct {
	Root string
	Dir  string // the workspace directory name (".agent-one"; a distribution renames it)
	Now  func() time.Time
	// Path is the PATH an external is looked up on; empty means the process environment's.
	Path string
}

// Open opens a workspace under the default directory name.
func Open(root string) (*Workspace, error) { return OpenIn(root, memory.DefaultWorkspaceDir) }

// OpenIn opens a workspace whose workspace directory is named dir.
func OpenIn(root, dir string) (*Workspace, error) {
	if dir == "" {
		dir = memory.DefaultWorkspaceDir
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	if !memory.Exists(filepath.Join(abs, dir)) {
		return nil, failf("no %s/ under %s", dir, abs)
	}
	return &Workspace{Root: abs, Dir: dir, Now: time.Now}, nil
}

func (w *Workspace) dir() string {
	if w.Dir == "" {
		return memory.DefaultWorkspaceDir
	}
	return w.Dir
}

func (w *Workspace) Base() string         { return filepath.Join(w.Root, w.dir()) }
func (w *Workspace) RegistryPath() string { return filepath.Join(w.Base(), "toolbox", "registry.json") }
func (w *Workspace) ExtraPath() string    { return filepath.Join(w.Base(), "toolbox", "extra.jsonl") }
func (w *Workspace) LoadsPath() string {
	return filepath.Join(w.Base(), "instruments", "toolbox", "loads.jsonl")
}
func (w *Workspace) now() string { return memory.NowISO(w.Now()) }
func (w *Workspace) WorkspaceName() string {
	if s, ok := memory.Rd(filepath.Join(w.Base(), "name")); ok {
		return memory.JSTrim(s)
	}
	return filepath.Base(w.Root)
}

// rel is toolbox.js's: relative only for an absolute path under root; anything else as given.
func (w *Workspace) rel(p string) string {
	if filepath.IsAbs(p) && strings.HasPrefix(p, w.Root+string(filepath.Separator)) {
		r, err := filepath.Rel(w.Root, p)
		if err == nil {
			return r
		}
	}
	return p
}

// ------------------------------------------------------------------ frontmatter

func unquote(s string) string {
	s = memory.JSTrim(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' && !strings.Contains(s, "\n") {
		return strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`)
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' && !strings.Contains(s, "\n") {
		return s[1 : len(s)-1]
	}
	return s
}

var (
	reKV     = regexp.MustCompile(`^([A-Za-z_][\w-]*):\s*(.*)$`)
	reLi     = memory.JSRe(`^\s*-\s+(.+)$`)
	reCont   = memory.JSRe(`^\s+\S`)
	reFold   = memory.JSRe(`^[>|]\s*`)
	reTitle  = memory.JSRe(`(?m)^#\s+(.+)$`)
	reQuoted = regexp.MustCompile(`"([^"\n]{3,80})"|“([^”\n]{3,80})”`)
)

// Frontmatter parses `key: value` lines, `- item` lists and indented continuations; nothing more.
// A value is a string or a []string.
func Frontmatter(text string) (fm map[string]any, agent string) {
	fm = map[string]any{}
	if !strings.HasPrefix(text, "---\n") {
		return fm, text
	}
	// the first "\n---" followed by "\n" or the end
	end, after := -1, 0
	for from := 4; ; {
		i := strings.Index(text[from:], "\n---")
		if i < 0 {
			return fm, text
		}
		i += from
		if i+4 == len(text) {
			end, after = i, len(text)
			break
		}
		if text[i+4] == '\n' {
			end, after = i, i+5
			break
		}
		from = i + 1
	}
	key := ""
	for _, l := range strings.Split(text[4:end], "\n") {
		if kv := reKV.FindStringSubmatch(l); kv != nil {
			key = strings.ToLower(kv[1])
			fm[key] = memory.JSTrim(kv[2])
			continue
		}
		if key == "" {
			continue
		}
		if li := reLi.FindStringSubmatch(l); li != nil {
			var list []string
			switch v := fm[key].(type) {
			case []string:
				list = v
			case string:
				if v != "" {
					list = []string{v}
				}
			}
			fm[key] = append(list, unquote(li[1]))
			continue
		}
		if s, ok := fm[key].(string); ok && reCont.MatchString(l) {
			fm[key] = reFold.ReplaceAllString(s+" "+memory.JSTrim(l), "")
		}
	}
	return fm, text[after:]
}

func asList(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, len(x))
		for i, s := range x {
			out[i] = unquote(s)
		}
		return out
	case []any:
		out := make([]string, len(x))
		for i, s := range x {
			out[i] = unquote(memory.JSString(s, true))
		}
		return out
	}
	s := memory.JSString(v, true)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	var out []string
	for _, p := range strings.Split(s, ",") {
		if u := unquote(p); u != "" {
			out = append(out, u)
		}
	}
	return out
}

func fmStr(fm map[string]any, k string) string {
	v, ok := fm[k]
	if !ok {
		return ""
	}
	return memory.JSString(v, true)
}

func oneLine(s string) string {
	t := memory.FirstSentence(s)
	if memory.UTF16Len(t) > 120 {
		return memory.JSTrimEnd(memory.UTF16Slice(t, 0, 119)) + "…"
	}
	return t
}

// ------------------------------------------------------------------ harvest

type home struct {
	P    string
	Srcs []string
}

// dirsOf: first base wins; later bases add to srcs (one thing with two homes, not two things).
func (w *Workspace) dirsOf(bases []string, pick func(d string, e os.DirEntry) (string, string)) ([]string, map[string]*home) {
	out := map[string]*home{}
	var order []string
	for _, base := range bases {
		d := filepath.Join(w.Root, base)
		es, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		sort.SliceStable(es, func(i, j int) bool { return memory.LocaleCompare(es[i].Name(), es[j].Name()) < 0 })
		for _, e := range es {
			name, p := pick(d, e)
			if name == "" {
				continue
			}
			if h, ok := out[name]; ok {
				h.Srcs = append(h.Srcs, w.rel(p))
			} else {
				out[name] = &home{p, []string{w.rel(p)}}
				order = append(order, name)
			}
		}
	}
	return order, out
}

func (w *Workspace) skills() ([]string, map[string]*home) {
	return w.dirsOf(memory.SkillBases, func(d string, e os.DirEntry) (string, string) {
		p := filepath.Join(d, e.Name(), "SKILL.md")
		if e.IsDir() && memory.Exists(p) {
			return e.Name(), p
		}
		return "", ""
	})
}

func mdFile(d string, e os.DirEntry) (string, string) {
	if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".md") {
		return strings.TrimSuffix(e.Name(), ".md"), filepath.Join(d, e.Name())
	}
	return "", ""
}

func (w *Workspace) commands() ([]string, map[string]*home) {
	return w.dirsOf(memory.CommandBases, mdFile)
}
func (w *Workspace) agents() ([]string, map[string]*home) {
	return w.dirsOf([]string{".claude/agents", ".opencode/agents", ".opencode/agent"}, mdFile)
}
func (w *Workspace) tools() ([]string, map[string]*home) {
	return w.dirsOf([]string{w.dir() + "/tools"}, func(d string, e os.DirEntry) (string, string) {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			return e.Name(), filepath.Join(d, e.Name())
		}
		return "", ""
	})
}

var (
	reUseStrict  = memory.JSRe(`^\s*'use strict';?\s*$`)
	reBlockStart = memory.JSRe(`^\s*/\*\*?`)
	reBlockLead  = memory.JSRe(`^\s*/\*\*?\s?`)
	reStarLead   = memory.JSRe(`^\s*\*\s?`)
	reBlockEnd   = memory.JSRe(`\s*\*/.*$`)
	reLineCmt    = memory.JSRe(`^\s*(?://|#)\s?(.*)$`)
	reBlankLine  = memory.JSRe(`\n\s*\n`)
	reToolLead   = memory.JSRe(`^[\w.\-]+\s+[—-]+\s+`)
)

// HeaderOf is a tool's leading comment block (//, #, or /** … */), shebang skipped, markers stripped.
func HeaderOf(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	i := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		i = 1
	}
	block := false
	for ; i < len(lines); i++ {
		l := lines[i]
		if reUseStrict.MatchString(l) && len(out) == 0 {
			continue
		}
		if block {
			out = append(out, reStarLead.ReplaceAllString(l, ""))
			if strings.Contains(l, "*/") {
				out[len(out)-1] = reBlockEnd.ReplaceAllString(out[len(out)-1], "")
				block = false
			}
			continue
		}
		if reBlockStart.MatchString(l) {
			block = true
			out = append(out, reBlockLead.ReplaceAllString(l, ""))
			if strings.Contains(l, "*/") {
				out[len(out)-1] = reBlockEnd.ReplaceAllString(out[len(out)-1], "")
				block = false
			}
			continue
		}
		if m := reLineCmt.FindStringSubmatch(l); m != nil {
			out = append(out, m[1])
		} else if len(out) > 0 || memory.JSTrim(l) != "" {
			break
		}
	}
	return memory.JSTrim(strings.Join(out, "\n"))
}

type Trigger struct {
	T   string `json:"t"`
	Src string `json:"src"`
}

var reExt = regexp.MustCompile(`(?i)\.[a-z0-9]+$`)
var reNameSep = regexp.MustCompile(`[-_.]+`)

func triggersOf(name string, fm map[string]any, desc string) []Trigger {
	var out []Trigger
	add := func(t, src string) {
		t = memory.JSTrim(memory.CollapseWS(t))
		if t == "" || len(memory.Tokens(t)) == 0 {
			return
		}
		for _, x := range out {
			if strings.ToLower(x.T) == strings.ToLower(t) {
				return
			}
		}
		out = append(out, Trigger{t, src})
	}
	allName := func() bool {
		for _, x := range out {
			if x.Src != "name" {
				return false
			}
		}
		return true
	}
	add(name, "name")
	var words []string
	for _, p := range reNameSep.Split(reExt.ReplaceAllString(name, ""), -1) {
		if p != "" {
			words = append(words, p)
		}
	}
	add(strings.Join(words, " "), "name")
	for _, k := range []string{"triggers", "trigger", "when", "use-when", "use_when"} {
		for _, t := range asList(fm[k]) {
			add(t, "frontmatter")
		}
	}
	if allName() {
		for _, m := range reQuoted.FindAllStringSubmatch(desc, -1) {
			q := m[1]
			if q == "" {
				q = m[2]
			}
			add(q, "quoted")
		}
	}
	if allName() {
		add(memory.FirstSentence(desc), "sentence")
	}
	if out == nil {
		out = []Trigger{}
	}
	return out
}

type external struct {
	name, description string
	path              *string
	triggers          []string
	cost              any
	serves            string
	usage             *string
	installed         bool
	claimed           *bool
}

func (w *Workspace) externalsOf() []external {
	var out []external
	pathEnv := w.Path
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	onPath := func(n string) bool {
		for _, d := range strings.Split(pathEnv, string(os.PathListSeparator)) {
			if d != "" && memory.Exists(filepath.Join(d, n)) {
				return true
			}
		}
		return false
	}
	for _, raw := range memory.ReadJSONL(w.ExtraPath()) {
		var x map[string]any
		if json.Unmarshal(raw, &x) != nil {
			continue
		}
		name, ok := x["name"].(string)
		if !ok || name == "" {
			continue
		}
		var p *string
		if s, ok := x["path"].(string); ok && s != "" {
			p = &s
		}
		installed := false
		if p != nil {
			installed = memory.Exists(*p)
		} else {
			installed = onPath(name)
		}
		if p == nil && installed {
			s := "$PATH/" + name
			p = &s
		}
		desc := ""
		if memory.Truthy(x["description"]) {
			desc = memory.JSString(x["description"], true)
		}
		serves := "shared"
		if memory.Truthy(x["serves"]) {
			serves = memory.JSString(x["serves"], true)
		}
		var usage *string
		if s, ok := x["usage"].(string); ok {
			usage = &s
		}
		var claimed *bool
		if b, ok := x["installed"].(bool); ok {
			claimed = &b
		}
		out = append(out, external{name, desc, p, asList(x["triggers"]), x["cost"], serves, usage, installed, claimed})
	}
	return out
}

func (w *Workspace) sources() []string {
	var out []string
	for _, f := range []func() ([]string, map[string]*home){w.skills, w.commands, w.agents, w.tools} {
		order, m := f()
		for _, n := range order {
			out = append(out, m[n].P)
		}
	}
	if memory.Exists(w.ExtraPath()) {
		out = append(out, w.ExtraPath())
	}
	return out
}

func (w *Workspace) sourceStamps() memory.Stamps {
	var o memory.Stamps
	for _, p := range w.sources() {
		if ms, ok := memory.MtimeMs(p); ok {
			o.Add(w.rel(p), ms)
		}
	}
	return o
}

// Entry is one registry entry: a pointer with known costs, never a agent.
type Entry struct {
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	Path        *string        `json:"path"`
	Srcs        []string       `json:"srcs"`
	Description string         `json:"description"`
	Line        string         `json:"line"`
	Triggers    []Trigger      `json:"triggers"`
	TF          map[string]int `json:"tf"`
	DL          int            `json:"dl"`
	Serves      string         `json:"serves"`
	Holders     []string       `json:"holders"`
	Cost        Cost           `json:"cost"`
	Sections    int            `json:"sections"`
	FileTokens  int            `json:"fileTokens"`
	Mode        *string        `json:"mode"`
	Model       *string        `json:"model"`
	Installed   *bool          `json:"installed"`
	Claimed     *bool          `json:"claimed"`
	Usage       *string        `json:"usage"`
}

type Cost struct {
	Resident float64 `json:"resident"`
	Full     float64 `json:"full"`
}

func (e *Entry) Terms() (map[string]int, int) { return e.TF, e.DL }

func (e *Entry) PathStr() string {
	if e.Path == nil {
		return ""
	}
	return *e.Path
}

// oj lays the entry out with JSON.stringify's key order and per-kind key presence.
func (e *Entry) oj(withTerms bool) memory.OJ {
	var path any
	if e.Path != nil {
		path = *e.Path
	}
	o := memory.OJ{memory.P("kind", e.Kind), memory.P("name", e.Name), memory.P("path", path), memory.P("srcs", memory.Nz(e.Srcs)), memory.P("description", e.Description), memory.P("line", e.Line), memory.P("triggers", e.Triggers)}
	if withTerms {
		o = append(o, memory.P("tf", e.TF), memory.P("dl", e.DL))
	}
	o = append(o, memory.P("serves", e.Serves), memory.P("holders", memory.Nz(e.Holders)))
	switch e.Kind {
	case "agent":
		o = append(o, memory.P("mode", e.Mode), memory.P("model", e.Model))
	case "external":
		o = append(o, memory.P("installed", e.Installed), memory.P("claimed", e.Claimed), memory.P("usage", e.Usage))
	}
	o = append(o, memory.P("cost", e.Cost))
	if e.Kind == "tool" {
		o = append(o, memory.P("fileTokens", e.FileTokens))
	}
	return append(o, memory.P("sections", e.Sections))
}

func (e *Entry) MarshalJSON() ([]byte, error) { return memory.MarshalJS(e.oj(true)) }

func rankOf(n string) string {
	for _, r := range rankOrder {
		if strings.HasPrefix(n, r+"-") {
			return r
		}
	}
	return ""
}

func (w *Workspace) Harvest() []*Entry {
	var entries []*Entry
	push := func(e *Entry, p string, srcs []string, triggers []Trigger) *Entry {
		e.Path = strPtr(w.rel(p))
		e.Srcs = srcs
		if e.Srcs == nil {
			e.Srcs = []string{w.rel(p)}
		}
		e.Line, e.Triggers = oneLine(e.Description), triggers
		var ts []string
		for _, t := range triggers {
			ts = append(ts, t.T)
		}
		tf := memory.TermCounts(e.Name + " " + reNameSep.ReplaceAllString(e.Name, " ") + "\n" + e.Description + "\n" + strings.Join(ts, "\n"))
		e.TF, e.DL = tf.Map, tf.DL
		if e.Holders == nil {
			e.Holders = []string{}
		}
		entries = append(entries, e)
		return e
	}
	type doc struct {
		memory.Member
		text string
	}
	var docs []doc
	for _, c := range memory.Members(w.Base()) {
		if c.Doc != "" {
			docs = append(docs, doc{c, memory.RdOr(c.Doc)})
		}
	}
	titleOf := func(agent string) string {
		if m := reTitle.FindStringSubmatch(agent); m != nil {
			return memory.JSTrim(m[1])
		}
		return ""
	}
	order, skills := w.skills()
	for _, name := range order {
		v := skills[name]
		text := memory.RdOr(v.P)
		fm, agent := Frontmatter(text)
		description := unquote(fmStr(fm, "description"))
		if description == "" {
			description = titleOf(agent)
		}
		if description == "" {
			description = memory.FirstSentence(agent)
		}
		var holders []string
		ranks := map[string]bool{}
		var rankList []string
		for _, d := range docs {
			if memory.NameIn(d.text, name) {
				holders = append(holders, d.Name)
				r := strings.SplitN(d.Name, "-", 2)[0]
				if !ranks[r] {
					ranks[r] = true
					rankList = append(rankList, r)
				}
			}
		}
		serves := "shared"
		if r := rankOf(name); r != "" {
			serves = laneOfRank[r]
		} else if len(rankList) == 1 {
			serves = laneOfRank[rankList[0]]
		}
		push(&Entry{Kind: "skill", Name: name, Description: description, Serves: serves, Holders: holders,
			Cost: Cost{float64(TOK(name + " " + description)), float64(TOK(text))}, Sections: len(memory.SectionMap(text).Sections)}, v.P, v.Srcs, triggersOf(name, fm, description))
	}
	order, cmds := w.commands()
	for _, name := range order {
		v := cmds[name]
		text := memory.RdOr(v.P)
		fm, agent := Frontmatter(text)
		description := unquote(fmStr(fm, "description"))
		if description == "" {
			description = titleOf(agent)
		}
		push(&Entry{Kind: "command", Name: name, Description: description, Serves: "shared",
			Cost: Cost{float64(TOK(name + " " + description)), float64(TOK(text))}, Sections: len(memory.SectionMap(text).Sections)}, v.P, v.Srcs, triggersOf(name, fm, description))
	}
	order, tools := w.tools()
	for _, name := range order {
		v := tools[name]
		text := memory.RdOr(v.P)
		head := HeaderOf(text)
		first := head
		if loc := reBlankLine.FindStringIndex(head); loc != nil {
			first = head[:loc[0]]
		}
		description := memory.UTF16Slice(memory.JSTrim(memory.CollapseWS(reToolLead.ReplaceAllString(first, ""))), 0, 500)
		push(&Entry{Kind: "tool", Name: name, Description: description, Serves: "shared",
			Cost: Cost{float64(TOK(name + " " + description)), float64(TOK(head))}, FileTokens: TOK(text)}, v.P, v.Srcs, triggersOf(name, map[string]any{}, description))
	}
	order, bods := w.agents()
	for _, name := range order {
		v := bods[name]
		text := memory.RdOr(v.P)
		fm, agent := Frontmatter(text)
		description := unquote(fmStr(fm, "description"))
		if description == "" {
			description = titleOf(agent)
		}
		if description == "" {
			description = memory.FirstSentence(agent)
		}
		serves := "shared"
		if r := rankOf(name); r != "" {
			serves = r
		}
		e := &Entry{Kind: "agent", Name: name, Description: description, Serves: serves,
			Cost: Cost{float64(TOK(name + " " + description)), float64(TOK(text))}, Sections: len(memory.SectionMap(text).Sections)}
		if memory.Truthy(fm["mode"]) {
			e.Mode = strPtr(fmStr(fm, "mode"))
		}
		if memory.Truthy(fm["model"]) {
			e.Model = strPtr(fmStr(fm, "model"))
		}
		push(e, v.P, v.Srcs, triggersOf(name, fm, description))
	}
	for _, x := range w.externalsOf() {
		c, _ := x.cost.(map[string]any)
		var resident, full float64
		if n, ok := x.cost.(float64); ok {
			resident = n
		} else if n, ok := c["resident"].(float64); ok {
			resident = n
		} else {
			resident = float64(TOK(x.name + " " + x.description))
		}
		if n, ok := c["full"].(float64); ok {
			full = n
		} else if x.usage != nil && *x.usage != "" {
			full = float64(TOK(*x.usage))
		} else {
			full = float64(TOK(x.description))
		}
		p := ""
		if x.path != nil {
			p = *x.path
		}
		inst := x.installed
		e := push(&Entry{Kind: "external", Name: x.name, Description: x.description, Serves: x.serves, Installed: &inst, Claimed: x.claimed, Usage: x.usage,
			Cost: Cost{resident, full}}, p, []string{w.rel(w.ExtraPath())}, triggersOf(x.name, map[string]any{"triggers": x.triggers}, x.description))
		e.Path = x.path
	}
	return entries
}

func strPtr(s string) *string { return &s }

// ------------------------------------------------------------------ registry

type Registry struct {
	V         int                `json:"v"`
	Workspace string             `json:"workspace"`
	BuiltAt   string             `json:"builtAt"`
	Head      *string            `json:"head"`
	Sources   memory.Stamps      `json:"sources"`
	N         int                `json:"n"`
	AvgDL     float64            `json:"avgdl"`
	IDF       map[string]float64 `json:"idf"`
	Entries   []*Entry           `json:"entries"`
}

func (r *Registry) stats() memory.Stats { return memory.Stats{N: r.N, AvgDL: r.AvgDL, IDF: r.IDF} }

func (w *Workspace) BuildRegistry(write bool) (*Registry, error) {
	entries := w.Harvest()
	docs := make([]memory.Doc, len(entries))
	for i, e := range entries {
		docs[i] = e
	}
	st := memory.StatsOf(docs)
	reg := &Registry{V: RegV, Workspace: w.WorkspaceName(), BuiltAt: w.now(), Head: memory.GitHead(w.Root), Sources: w.sourceStamps(), N: st.N, AvgDL: st.AvgDL, IDF: st.IDF, Entries: entries}
	if reg.Entries == nil {
		reg.Entries = []*Entry{}
	}
	if write {
		data, err := memory.MarshalJS(reg)
		if err != nil {
			return nil, err
		}
		if err := memory.WriteAtomic(w.RegistryPath(), data); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// Loaded is a registry plus how it was obtained: live (a harvest, named as a hole) or from disk.
type Loaded struct {
	Reg  *Registry
	Live bool
	Why  string
}

func (w *Workspace) LoadRegistry() Loaded {
	live := func(why string) Loaded {
		reg, _ := w.BuildRegistry(false)
		return Loaded{reg, true, why}
	}
	if !memory.Exists(w.RegistryPath()) {
		return live("no registry on disk — answered from a live harvest; run `toolbox.js index`")
	}
	raw := memory.RdOr(w.RegistryPath())
	var probe struct {
		V       *float64        `json:"v"`
		Entries json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return live(fmt.Sprintf("registry unreadable (%s) — answered live; rerun `toolbox.js index`", w.rel(w.RegistryPath())))
	}
	if probe.V == nil || *probe.V != RegV || !strings.HasPrefix(strings.TrimSpace(string(probe.Entries)), "[") {
		return live("registry built by an older toolbox.js — answered live; rerun `toolbox.js index`")
	}
	var reg Registry
	if err := json.Unmarshal([]byte(raw), &reg); err != nil {
		return live(fmt.Sprintf("registry unreadable (%s) — answered live; rerun `toolbox.js index`", w.rel(w.RegistryPath())))
	}
	return Loaded{&reg, false, ""}
}

func (w *Workspace) RegistryHoles(R Loaded) []string {
	h := []string{}
	if R.Why != "" {
		h = append(h, R.Why)
	}
	if len(R.Reg.Entries) == 0 {
		h = append(h, "registry empty — nothing under .claude/skills, .opencode/skill(s), .claude/commands, .opencode/command(s), "+w.Dir+"/tools, .claude/agents, .opencode/agent(s) or "+w.rel(w.ExtraPath()))
	}
	if R.Live {
		return h
	}
	if parts := memory.StaleParts(w.sourceStamps(), R.Reg.Sources); len(parts) > 0 {
		h = append(h, "registry older than its sources — "+strings.Join(parts, "; ")+" — rerun `toolbox.js index`")
	}
	return h
}

func (w *Workspace) IsStale(R Loaded) bool {
	if R.Live {
		return false
	}
	for _, h := range w.RegistryHoles(R) {
		if strings.HasPrefix(h, "registry older") {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ relations and the pick

type Relations struct {
	memory.Relations
	Lane string
}

func (w *Workspace) RelationsOf(name string) Relations {
	R := Relations{Relations: memory.RelationsOf(w.Root, w.Base(), name)}
	R.Lane = laneOfRank[R.Rank]
	return R
}

func relationBoost(e *Entry, R Relations) (float64, string) {
	var parts []string
	sum := 0.0
	add := func(why string, v float64) { parts = append(parts, why); sum += v }
	if R.Self != "orchestrator" {
		held := false
		for _, x := range e.Holders {
			if x == R.Self {
				held = true
			}
		}
		for _, m := range R.Skills {
			if m == e.Name {
				held = true
			}
		}
		if held {
			add("held", 0.15)
		} else if memory.Has(e.TF, R.Self) {
			add("names-asker", 0.10)
		}
		if R.Lane != "" && e.Serves == R.Lane {
			add("lane", 0.08)
		}
		isBond := false
		if e.Kind == "agent" {
			if e.Name == R.Parent {
				isBond = true
			}
			for _, c := range R.Children {
				if c == e.Name {
					isBond = true
				}
			}
		}
		if isBond {
			add("bond", 0.10)
		} else {
			names := R.Parent != "" && memory.Has(e.TF, R.Parent)
			for _, c := range R.Children {
				if memory.Has(e.TF, c) {
					names = true
				}
			}
			if names {
				add("bond", 0.05)
			}
		}
		for _, z := range R.Zone {
			if memory.Has(e.TF, z) {
				add("zone", 0.08)
				break
			}
		}
	}
	return math.Min(0.3, sum), strings.Join(parts, "+")
}

// stem: a light stem for trigger matching only, so "render" meets "rendering".
func stem(t string) string {
	s := t
	for _, suf := range []string{"ing", "ed", "es", "s"} {
		if strings.HasSuffix(t, suf) {
			s = t[:len(t)-len(suf)]
			break
		}
	}
	if len(s) >= 3 {
		return s
	}
	return t
}

func trigHits(askSet map[string]bool, triggers []Trigger) []Trigger {
	var hits []Trigger
	for _, x := range triggers {
		ts := memory.Tokens(x.T)
		if len(ts) == 0 {
			continue
		}
		all := true
		for _, t := range ts {
			if !askSet[stem(t)] {
				all = false
				break
			}
		}
		if all {
			hits = append(hits, x)
		}
	}
	return hits
}

func triggerBoost(hits []Trigger) float64 {
	if len(hits) == 0 {
		return 0
	}
	mx := 0.0
	for _, h := range hits {
		wgt, ok := triggerW[h.Src]
		if !ok {
			wgt = 0.4
		}
		mx = math.Max(mx, wgt)
	}
	return mx + math.Min(0.15, 0.05*float64(len(hits)-1))
}

// Pick is one ranked entry: a priced pointer and why it was picked.
type Pick struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Path      *string  `json:"path"`
	Srcs      []string `json:"srcs"`
	Serves    string   `json:"serves"`
	Installed *bool    `json:"installed,omitempty"`
	Resident  float64  `json:"resident"`
	Full      float64  `json:"full"`
	Score     float64  `json:"score"`
	Sim       float64  `json:"sim"`
	Matched   int      `json:"matched"`
	Trig      float64  `json:"trig"`
	Rel       float64  `json:"rel"`
	Why       string   `json:"why"`
	Hits      []string `json:"hits"`
	Line      string   `json:"line"`
	Triggers  []string `json:"triggers"`
}

func (p *Pick) PathStr() string {
	if p.Path == nil {
		return ""
	}
	return *p.Path
}

func (w *Workspace) Rank(reg *Registry, q string, R Relations, kinds []string, min float64) []Pick {
	st := reg.stats()
	qtf := memory.TermCounts(q)
	if len(qtf.Order) == 0 {
		return nil
	}
	askSet := map[string]bool{}
	for _, t := range memory.Tokens(q) {
		askSet[stem(t)] = true
	}
	need := FitWords
	if len(qtf.Order) < 3 {
		need = 1
	}
	var out []Pick
	for _, e := range reg.Entries {
		if kinds != nil && !contains(kinds, e.Kind) {
			continue
		}
		sim := memory.BM25(qtf, e.TF, e.DL, st.AvgDL, st.IDFQ)
		hits := trigHits(askSet, e.Triggers)
		trig := triggerBoost(hits)
		rb, rwhy := relationBoost(e, R)
		matched := 0
		for _, t := range qtf.Order {
			if _, ok := e.TF[t]; ok {
				matched++
			}
		}
		why := "meaning"
		if trig != 0 {
			why = `trigger "` + hits[0].T + `"`
		} else if rb != 0 && rb >= sim {
			why = "relation " + rwhy
		}
		p := Pick{Kind: e.Kind, Name: e.Name, Path: e.Path, Srcs: memory.Nz(e.Srcs), Serves: e.Serves, Installed: e.Installed, Resident: e.Cost.Resident, Full: e.Cost.Full,
			Score: memory.ToFixed(sim+trig+rb, 4), Sim: memory.ToFixed(sim, 4), Matched: matched, Trig: memory.ToFixed(trig, 2), Rel: memory.ToFixed(rb, 2), Why: why, Hits: []string{}, Line: e.Line, Triggers: []string{}}
		for _, h := range hits {
			p.Hits = append(p.Hits, h.T)
		}
		for _, t := range e.Triggers {
			if t.Src == "frontmatter" || t.Src == "quoted" {
				p.Triggers = append(p.Triggers, t.T)
			}
		}
		if (p.Trig > 0 || p.Matched >= need) && p.Score >= min {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Resident != b.Resident {
			return a.Resident < b.Resident
		}
		return memory.LocaleCompare(a.Name, b.Name) < 0
	})
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

type PickOpts struct {
	As     string
	Budget int     // 0 = 1500
	K      int     // 0 = 5
	Min    float64 // score floor
	Kinds  []string
	Cmd    string // "pick" | "brief" — what the journal records
}

type PickResult struct {
	As     string
	Q      string
	K      int
	Budget int
	Min    float64
	Picks  []Pick
	Over   []Pick
	Cost   float64
	Holes  []string
}

// DoPick ranks, fills the budget greedily by score and journals the offer.
func (w *Workspace) DoPick(q string, o PickOpts) (*PickResult, error) {
	cmd := o.Cmd
	if cmd == "" {
		cmd = "pick"
	}
	q = memory.JSTrim(q)
	if q == "" {
		return nil, failf("%s needs the turn's ask", cmd)
	}
	K, budget := o.K, o.Budget
	if K == 0 {
		K = 5
	}
	if K < 0 {
		return nil, failf("-k needs a positive integer, got %d", o.K)
	}
	if budget == 0 {
		budget = BudgetDefault
	}
	if budget < 0 {
		return nil, failf("--budget needs a positive integer, got %d", o.Budget)
	}
	if !(o.Min >= 0) {
		return nil, failf("--min needs a score ≥ 0, got %v", o.Min)
	}
	for _, k := range o.Kinds {
		if !contains(Kinds, k) {
			return nil, failf("--kind must be %s, got %s", strings.Join(Kinds, "|"), k)
		}
	}
	as := memory.NormAs(o.As)
	Rg := w.LoadRegistry()
	R := w.RelationsOf(as)
	ranked := w.Rank(Rg.Reg, q, R, o.Kinds, o.Min)
	picks, over := []Pick{}, []Pick{}
	cost := 0.0
	for _, r := range ranked {
		if len(picks) >= K {
			break
		}
		if cost+r.Resident <= float64(budget) {
			picks = append(picks, r)
			cost += r.Resident
		} else {
			over = append(over, r)
		}
	}
	holes := []string{}
	if len(memory.Tokens(q)) == 0 {
		holes = append(holes, "ask has no content words after stop-word removal — nothing to rank")
	}
	holes = append(holes, w.RegistryHoles(Rg)...)
	if len(over) > 0 {
		var names []string
		for _, r := range over[:min(3, len(over))] {
			names = append(names, r.Name+" "+memory.JSNum(r.Resident)+"tok")
		}
		more := ""
		if len(over) > 3 {
			more = ", …"
		}
		holes = append(holes, fmt.Sprintf("%d fit the turn but not the budget: %s%s — raise --budget or load by name", len(over), strings.Join(names, ", "), more))
	}
	for _, r := range picks {
		if r.Kind == "external" && r.Installed != nil && !*r.Installed {
			holes = append(holes, r.Name+" fits the turn but is not installed on this machine")
		}
	}
	if len(picks) > 0 {
		names := make([]string, len(picks))
		for i, r := range picks {
			names[i] = r.Name
		}
		memory.AppendLine(w.LoadsPath(), memory.OJ{memory.P("at", w.now()), memory.P("by", as), memory.P("ev", "offer"), memory.P("cmd", cmd), memory.P("ask", memory.UTF16Slice(q, 0, 200)), memory.P("names", names), memory.P("tokens", cost), memory.P("budget", budget)})
	}
	return &PickResult{As: as, Q: q, K: K, Budget: budget, Min: o.Min, Picks: picks, Over: over, Cost: cost, Holes: holes}, nil
}

func (p *PickResult) overOJ() []memory.OJ {
	out := []memory.OJ{}
	for _, r := range p.Over {
		out = append(out, memory.OJ{memory.P("name", r.Name), memory.P("resident", r.Resident), memory.P("score", r.Score)})
	}
	return out
}

// PickJSON is the `pick --json` document.
func (p *PickResult) PickJSON() memory.OJ {
	return memory.OJ{memory.P("@S", "PICK"), memory.P("as", p.As), memory.P("ask", p.Q), memory.P("k", len(p.Picks)), memory.P("cost", p.Cost), memory.P("budget", p.Budget), memory.P("min", p.Min), memory.P("picks", p.Picks), memory.P("over", p.overOJ()), memory.P("@?", p.Holes)}
}

// PickLines are the `@S PICK` head and one `@T` per pick.
func (p *PickResult) PickLines() []string {
	lines := []string{fmt.Sprintf("@S PICK k=%d cost=%s/%d as=%s", len(p.Picks), memory.JSNum(p.Cost), p.Budget, p.As)}
	for _, r := range p.Picks {
		lines = append(lines, fmt.Sprintf("@T %s %s — %s — %stok (load≈%s) — %s %s", r.Kind, r.Name, pathOr(r.PathStr()), memory.JSNum(r.Resident), memory.JSNum(r.Full), r.Why, memory.ToFixedStr(r.Score, 2)))
	}
	return lines
}

// PickBytes is the @E figure of a pick.
func (p *PickResult) PickBytes() int {
	rows := [][]any{}
	for _, r := range p.Picks {
		var path any
		if r.Path != nil {
			path = *r.Path
		}
		rows = append(rows, []any{r.Kind, r.Name, path, r.Resident, r.Full, r.Line})
	}
	b, _ := memory.MarshalJS(rows)
	return len(b)
}

func pathOr(p string) string {
	if p == "" {
		return "not installed"
	}
	return p
}

// Brief is level 1 on the wire: the @TOOLS manifest a Subagent brief carries — never a agent.
type Brief struct {
	*PickResult
	Head  string
	Lines []string
}

func (w *Workspace) Brief(q string, o PickOpts) (*Brief, error) {
	o.Cmd = "brief"
	P, err := w.DoPick(q, o)
	if err != nil {
		return nil, err
	}
	tool := w.rel(filepath.Join(w.Base(), "tools", "toolbox.js"))
	b := &Brief{PickResult: P, Lines: []string{}}
	b.Head = fmt.Sprintf("@TOOLS as=%s k=%d cost=%s/%d — level 2 on decision only: node %s load <name> [--map|--sec N] · Claude Code: Skill <name> / ToolSearch \"select:<name>\" / Agent <body> · OpenCode: load skill <name>", P.As, len(P.Picks), memory.JSNum(P.Cost), P.Budget, tool)
	for _, r := range P.Picks {
		l := fmt.Sprintf("@T %s %s — %s — load≈%stok — %s", r.Kind, r.Name, pathOr(r.PathStr()), memory.JSNum(r.Full), r.Line)
		if len(r.Triggers) > 0 {
			l += " — ⟨" + strings.Join(r.Triggers[:min(4, len(r.Triggers))], " · ") + "⟩"
		}
		b.Lines = append(b.Lines, l)
	}
	return b, nil
}

func (b *Brief) JSON() memory.OJ {
	picks := []memory.OJ{}
	for _, r := range b.Picks {
		var path any
		if r.Path != nil {
			path = *r.Path
		}
		picks = append(picks, memory.OJ{memory.P("kind", r.Kind), memory.P("name", r.Name), memory.P("path", path), memory.P("resident", r.Resident), memory.P("full", r.Full), memory.P("why", r.Why), memory.P("score", r.Score)})
	}
	return memory.OJ{memory.P("@S", "TOOLS"), memory.P("as", b.As), memory.P("ask", b.Q), memory.P("k", len(b.Picks)), memory.P("cost", b.Cost), memory.P("budget", b.Budget), memory.P("head", b.Head), memory.P("lines", b.Lines), memory.P("picks", picks), memory.P("over", b.overOJ()), memory.P("@?", b.Holes)}
}

// ------------------------------------------------------------------ level 2: load

func (w *Workspace) FindEntry(reg *Registry, name, kind string) (*Entry, error) {
	n := strings.TrimPrefix(strings.ToLower(name), "/")
	var c []*Entry
	for _, e := range reg.Entries {
		l := strings.ToLower(e.Name)
		if (l == n || l == n+".js" || l == n+".sh") && (kind == "" || e.Kind == kind) {
			c = append(c, e)
		}
	}
	if len(c) > 1 {
		var ks []string
		for _, e := range c {
			ks = append(ks, e.Kind)
		}
		return nil, failf("%s names %d entries (%s) — add --kind", name, len(c), strings.Join(ks, ", "))
	}
	if len(c) == 0 {
		return nil, nil
	}
	return c[0], nil
}

func (w *Workspace) agentOf(e *Entry) (string, string) {
	if e.Kind == "external" {
		if e.Usage != nil {
			return *e.Usage, ""
		}
		return e.Description, fmt.Sprintf("%s has no usage notes in %s — description only", e.Name, w.rel(w.ExtraPath()))
	}
	p := e.PathStr()
	if !filepath.IsAbs(p) {
		p = filepath.Join(w.Root, p)
	}
	text, ok := memory.Rd(p)
	if !ok {
		return "", fmt.Sprintf("%s unreadable — registry older than the workspace? rerun `toolbox.js index`", e.PathStr())
	}
	if e.Kind == "tool" {
		return HeaderOf(text), ""
	}
	return text, ""
}

type LoadOpts struct {
	As   string
	Kind string
	Map  bool
	Sec  *int // nil = whole; 0 = preamble
}

type LoadResult struct {
	Entry  *Entry
	Map    memory.SecMap
	IsMap  bool
	Sec    any // "all" or a number
	Text   string
	Tokens int
	Holes  []string
}

func (w *Workspace) Load(name string, o LoadOpts) (*LoadResult, error) {
	if name == "" {
		return nil, failf("load needs a name — `toolbox.js pick \"<ask>\"` lists them")
	}
	Rg := w.LoadRegistry()
	e, err := w.FindEntry(Rg.Reg, name, o.Kind)
	if err != nil {
		return nil, err
	}
	if e == nil {
		with := ""
		if o.Kind != "" {
			with = " with kind " + o.Kind
		}
		return nil, failf("no entry named %s in the registry%s — `toolbox.js index` rebuilds it", name, with)
	}
	text, why := w.agentOf(e)
	holes := w.RegistryHoles(Rg)
	if why != "" {
		holes = append(holes, why)
	}
	sm := memory.SectionMap(text)
	if o.Map {
		if len(sm.Sections) == 0 {
			holes = append(holes, e.Name+" has no sections — load it whole")
		}
		return &LoadResult{Entry: e, Map: sm, IsMap: true, Holes: holes}, nil
	}
	out, sec := text, any("all")
	if o.Sec != nil {
		n := *o.Sec
		if n < 0 {
			return nil, failf("--sec needs a section number (0 = preamble), got %d", n)
		}
		if n == 0 {
			out = sm.Preamble
		} else {
			if n > len(sm.Sections) {
				return nil, failf("%s has %d section%s, no #%d — `load %s --map`", e.Name, len(sm.Sections), plural(len(sm.Sections)), n, e.Name)
			}
			out = sm.Sections[n-1].Text
		}
		sec = n
	}
	tk := TOK(out)
	memory.AppendLine(w.LoadsPath(), memory.OJ{memory.P("at", w.now()), memory.P("by", memory.NormAs(o.As)), memory.P("ev", "load"), memory.P("name", e.Name), memory.P("kind", e.Kind), memory.P("tokens", tk), memory.P("sec", sec)})
	return &LoadResult{Entry: e, Map: sm, Sec: sec, Text: out, Tokens: tk, Holes: holes}, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ------------------------------------------------------------------ index · status · explain

type IndexResult struct {
	Reg      *Registry
	ByKind   map[string]int
	Resident float64
	Holes    []string
}

func (w *Workspace) Index() (*IndexResult, error) {
	reg, err := w.BuildRegistry(true)
	if err != nil {
		return nil, failf("index crashed: %v", err)
	}
	r := &IndexResult{Reg: reg, ByKind: byKind(reg), Holes: []string{}}
	for _, e := range reg.Entries {
		r.Resident += e.Cost.Resident
	}
	if reg.N == 0 {
		r.Holes = append(r.Holes, fmt.Sprintf("nothing to index under %s — no Skills, commands, tools, Agents or %s entries", w.Root, w.rel(w.ExtraPath())))
	}
	for _, e := range reg.Entries {
		if e.Kind == "external" && (e.Installed == nil || !*e.Installed) {
			h := "external " + e.Name + " is not installed on this machine"
			if e.Claimed != nil && *e.Claimed {
				h += " (extra.jsonl claims it is)"
			}
			r.Holes = append(r.Holes, h)
		}
	}
	return r, nil
}

func (r *IndexResult) JSON(w *Workspace) memory.OJ {
	o := memory.OJ{memory.P("@S", "INDEXED"), memory.P("workspace", r.Reg.Workspace), memory.P("n", r.Reg.N)}
	for _, k := range Kinds {
		o = append(o, memory.P(k, r.ByKind[k]))
	}
	return append(o, memory.P("resident", r.Resident), memory.P("head", r.Reg.Head), memory.P("src", w.rel(w.RegistryPath())), memory.P("@?", r.Holes))
}

func byKind(reg *Registry) map[string]int {
	m := map[string]int{}
	for _, e := range reg.Entries {
		m[e.Kind]++
	}
	return m
}

func kindsLine(by map[string]int) string {
	var parts []string
	for _, k := range Kinds {
		parts = append(parts, fmt.Sprintf("%s %d", k, by[k]))
	}
	return strings.Join(parts, " · ")
}

// Instrument is offered against loaded, read back from the loads journal.
type Instrument struct {
	Offers          int     `json:"offers"`
	Offered         int     `json:"offered"`
	OfferedDistinct int     `json:"offeredDistinct"`
	ResidentTokens  float64 `json:"residentTokens"`
	Loads           int     `json:"loads"`
	LoadedDistinct  int     `json:"loadedDistinct"`
	LoadedTokens    float64 `json:"loadedTokens"`
	Src             string  `json:"src"`
}

func (w *Workspace) instrument() Instrument {
	ins := Instrument{Src: w.rel(w.LoadsPath())}
	offered, loaded := map[string]bool{}, map[string]bool{}
	for _, raw := range memory.ReadJSONL(w.LoadsPath()) {
		var x struct {
			Ev     string  `json:"ev"`
			Names  []any   `json:"names"`
			Tokens float64 `json:"tokens"`
			Name   any     `json:"name"`
		}
		if json.Unmarshal(raw, &x) != nil {
			continue
		}
		switch x.Ev {
		case "offer":
			ins.Offers++
			ins.Offered += len(x.Names)
			ins.ResidentTokens += x.Tokens
			for _, n := range x.Names {
				offered[memory.JSString(n, true)] = true
			}
		case "load":
			ins.Loads++
			ins.LoadedTokens += x.Tokens
			loaded[memory.JSString(x.Name, x.Name != nil)] = true
		}
	}
	ins.OfferedDistinct, ins.LoadedDistinct = len(offered), len(loaded)
	return ins
}

type StatusReport struct {
	Workspace string
	At        string
	Loaded    Loaded
	Stale     bool
	ByKind    map[string]int
	Resident  float64
	Full      float64
	Budget    float64
	Ext       int
	Missing   []string
	Ins       Instrument
	Holes     []string
}

// Status reads the registry, extra.jsonl and the loads journal. Budget is `--budget` parsed the
// JS way (NaN allowed — it is never validated there); math.NaN() keeps that shape.
func (w *Workspace) Status(budget float64) *StatusReport {
	Rg := w.LoadRegistry()
	reg := Rg.Reg
	r := &StatusReport{Workspace: w.WorkspaceName(), At: w.now(), Loaded: Rg, Stale: w.IsStale(Rg), ByKind: byKind(reg), Budget: budget, Missing: []string{}, Holes: w.RegistryHoles(Rg)}
	for _, e := range reg.Entries {
		r.Resident += e.Cost.Resident
		r.Full += e.Cost.Full
		if e.Kind == "external" {
			r.Ext++
			if e.Installed == nil || !*e.Installed {
				r.Missing = append(r.Missing, e.Name)
			}
		}
	}
	r.Ins = w.instrument()
	if !memory.Exists(w.LoadsPath()) {
		r.Holes = append(r.Holes, fmt.Sprintf("no loads journal yet (%s) — nothing offered or loaded through the toolbox so far", w.rel(w.LoadsPath())))
	}
	return r
}

func (r *StatusReport) JSON(w *Workspace) memory.OJ {
	reg := r.Loaded.Reg
	var share any
	if r.Budget != 0 && r.Budget == r.Budget {
		s := memory.ToFixed(r.Resident/r.Budget, 2)
		share = s
	} else if r.Budget != r.Budget {
		share = nil
	}
	var budget any = r.Budget
	if r.Budget != r.Budget {
		budget = nil
	}
	by := memory.OJ{}
	for _, k := range Kinds {
		by = append(by, memory.P(k, r.ByKind[k]))
	}
	return memory.OJ{memory.P("workspace", r.Workspace), memory.P("at", r.At),
		memory.P("registry", memory.OJ{memory.P("present", !r.Loaded.Live), memory.P("builtAt", reg.BuiltAt), memory.P("head", reg.Head), memory.P("stale", r.Stale), memory.P("n", reg.N), memory.P("byKind", by), memory.P("src", w.rel(w.RegistryPath()))}),
		memory.P("cost", memory.OJ{memory.P("residentIfAllInjected", r.Resident), memory.P("budget", budget), memory.P("budgetShare", share), memory.P("fullIfAllLoaded", r.Full)}),
		memory.P("externals", memory.OJ{memory.P("installed", r.Ext-len(r.Missing)), memory.P("missing", r.Missing)}),
		memory.P("instrument", r.Ins), memory.P("@?", r.Holes)}
}

// Explain is one entry's full card.
func (w *Workspace) Explain(name, kind string) (*Entry, []string, error) {
	if name == "" {
		return nil, nil, failf("explain needs a name")
	}
	Rg := w.LoadRegistry()
	e, err := w.FindEntry(Rg.Reg, name, kind)
	if err != nil {
		return nil, nil, err
	}
	if e == nil {
		return nil, nil, failf("no entry named %s in the registry — `toolbox.js index` rebuilds it", name)
	}
	return e, w.RegistryHoles(Rg), nil
}

// ExplainJSON is `explain --json`: the entry without tf/dl, then the holes.
func ExplainJSON(e *Entry, holes []string) memory.OJ {
	return append(append(memory.OJ{memory.P("@S", "EXPLAIN")}, e.oj(false)...), memory.P("@?", holes))
}
