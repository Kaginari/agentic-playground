// Package tool holds the tools an agent may run and the class each act reaches: read · write
// (inside the workspace) · outward (Principle 7) · destructive (Policy 6). A tool declares its class; a
// per-call classifier may tighten it; nothing may loosen it.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Class is how far an act reaches. The order is the policy's: a declaration may only move right.
type Class int

const (
	Read Class = iota
	Write
	Outward
	Destructive
)

var classNames = []string{"read", "write", "outward", "destructive"}

func (c Class) String() string {
	if c < 0 || int(c) >= len(classNames) {
		return "unknown"
	}
	return classNames[c]
}

// ParseClass reads a class name; ok is false for anything else.
func ParseClass(s string) (Class, bool) {
	for i, n := range classNames {
		if n == strings.ToLower(strings.TrimSpace(s)) {
			return Class(i), true
		}
	}
	return Read, false
}

// Max returns the tighter of two classes.
func Max(a, b Class) Class {
	if b > a {
		return b
	}
	return a
}

// Result is what a tool hands back: text for the model, whether the act failed (the verify
// beat's reading), and the paths it wrote (for the gate's Docs-as-code check).
type Result struct {
	Output string
	Err    bool
	Wrote  []string
}

// Access is what the policy hook sees before an act: the tool, the settled class, the paths it
// touches (absolute). The workspace package sets Policy to enforce ownership; a refusal is an error
// the model reads, never a warning.
type Access struct {
	Tool  string
	Class Class
	Paths []string
	Input json.RawMessage
}

// Policy decides whether an act may proceed. nil allows everything inside the workspace.
type Policy func(a Access) error

// Env is the workspace a tool runs in. Everything a distribution may rename is a field: the workspace
// directory name (".agent-one" by default; the machine-shared tier is ~/<WorkspaceDir>) and the
// record test (files never overwritten, Policy 4).
type Env struct {
	Root         string // the workspace root; every relative path resolves against it
	Cwd          string // defaults to Root
	WorkspaceDir string // defaults to DefaultWorkspaceDir
	Policy       Policy
	IsRecord     func(rel string) bool // defaults to IsRecord; nil-safe
	Timeout      int                   // seconds, for bash; 0 means the tool's default
}

// DefaultWorkspaceDir is the workspace directory name when Env.WorkspaceDir is empty.
const DefaultWorkspaceDir = ".agent-one"

func (e Env) workspaceDir() string {
	if e.WorkspaceDir != "" {
		return e.WorkspaceDir
	}
	return DefaultWorkspaceDir
}

func (e Env) isRecord(abs string) bool {
	if e.IsRecord != nil {
		return e.IsRecord(e.Rel(abs))
	}
	return IsRecord(abs)
}

// Classification is a per-call reading: the class, the reason, and the paths involved.
type Classification struct {
	Class Class
	Why   string
	Paths []string
}

// Tool is one capability. Class is its declared floor; Classify, when set, reads the call and
// may raise it. Run performs the act.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Class       Class
	Classify    func(env Env, input json.RawMessage) Classification
	Run         func(ctx context.Context, env Env, input json.RawMessage) Result
	// Declare carries a provider-specific declaration keyed by provider type ("anthropic"):
	// e.g. {"type":"bash_20250124","name":"bash"} — a provider that finds its key sends that
	// object instead of Name/Description/Schema; every other provider sees the custom schema.
	Declare map[string]json.RawMessage
}

// Settle reads a call and returns its effective class with the reason: the tool's declared
// floor, the classifier's reading, and the caller's own declaration (`class` in the input),
// each only able to tighten.
func (t *Tool) Settle(env Env, input json.RawMessage) (Classification, []string) {
	c := Classification{Class: t.Class, Why: "declared by the tool"}
	if t.Classify != nil {
		r := t.Classify(env, input)
		c.Paths = r.Paths
		if r.Class > c.Class {
			c.Class, c.Why = r.Class, r.Why
		} else if r.Why != "" && r.Class == c.Class {
			c.Why = r.Why
		}
	}
	var holes []string
	var decl struct {
		Class string `json:"class"`
	}
	if json.Unmarshal(input, &decl) == nil && decl.Class != "" {
		if d, ok := ParseClass(decl.Class); !ok {
			holes = append(holes, fmt.Sprintf("declared class %q is not read|write|outward|destructive — ignored", decl.Class))
		} else if d > c.Class {
			c.Class, c.Why = d, "declared "+d.String()
		} else if d < c.Class {
			holes = append(holes, fmt.Sprintf("declared %s but the call reads as %s (%s) — a declaration only tightens; %s kept", d, c.Class, c.Why, c.Class))
		}
	}
	return c, holes
}

// Def is the tool as the provider sees it.
func (t *Tool) Def() Def {
	s := t.Schema
	if len(s) == 0 {
		s = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return Def{Name: t.Name, Description: t.Description, Schema: s, Declare: t.Declare}
}

// Def mirrors provider.ToolDef without importing it; the loop converts.
type Def struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Declare     map[string]json.RawMessage // provider-specific declarations, by provider type
}

// DeclareFor returns the provider-specific declaration for a provider type, if the tool has one.
func (d Def) DeclareFor(provider string) (json.RawMessage, bool) {
	raw, ok := d.Declare[provider]
	return raw, ok && len(raw) > 0
}

// Registry is an ordered set of tools. Later packages (workspace: dispatch) add to it.
type Registry struct {
	tools map[string]*Tool
	order []string
}

func NewRegistry(tools ...*Tool) *Registry {
	r := &Registry{tools: map[string]*Tool{}}
	for _, t := range tools {
		r.Add(t)
	}
	return r
}

// Add registers or replaces a tool by name.
func (r *Registry) Add(t *Tool) {
	if _, ok := r.tools[t.Name]; !ok {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

// Remove drops a tool (a rank cut to fewer tools).
func (r *Registry) Remove(name string) {
	if _, ok := r.tools[name]; !ok {
		return
	}
	delete(r.tools, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

func (r *Registry) Get(name string) (*Tool, bool) { t, ok := r.tools[name]; return t, ok }

// Names lists the tools in registration order.
func (r *Registry) Names() []string { return append([]string(nil), r.order...) }

// Defs lists the tools for a provider request.
func (r *Registry) Defs() []Def {
	out := make([]Def, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n].Def())
	}
	return out
}

// Only returns a new registry cut to the named tools (a Ephemeral subagent's rank).
func (r *Registry) Only(names ...string) *Registry {
	n := NewRegistry()
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			n.Add(t)
		}
	}
	return n
}

// Builtins is the standard shelf: read, write, edit, bash (on a persistent shell, started
// lazily on its first call and living until the process ends — an agent's shelf closes its own),
// glob, grep.
func Builtins() *Registry {
	bash, _ := NewBashTool(BashOptions{Enabled: true})
	return NewRegistry(ReadTool(), WriteTool(), EditTool(), bash, GlobTool(), GrepTool())
}

// Resolve makes p absolute against the env's cwd (or root) and cleans it.
func (e Env) Resolve(p string) string {
	if p == "" {
		return e.Dir()
	}
	if strings.HasPrefix(p, "~/") || p == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(e.Dir(), p)
	}
	return filepath.Clean(p)
}

// Dir is the working directory: Cwd, else Root.
func (e Env) Dir() string {
	if e.Cwd != "" {
		return e.Cwd
	}
	return e.Root
}

// Inside reports whether an absolute path is inside the workspace (the root, the system
// prefixes, or ~/.agent-one — the machine-shared tier is inward). A symlink is read for where it
// points: the longest existing prefix of the path is resolved, so a link out of the workspace, and
// a file yet to be created under one, are outside.
func (e Env) Inside(abs string) bool {
	return e.insideLiteral(abs) && e.insideLiteral(realOf(abs))
}

// realOf resolves the longest existing prefix of abs through symlinks and re-attaches the rest.
func realOf(abs string) string {
	probe, rest := abs, ""
	for {
		if real, err := filepath.EvalSymlinks(probe); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return abs
		}
		rest = filepath.Join(filepath.Base(probe), rest)
		probe = parent
	}
}

func (e Env) insideLiteral(abs string) bool {
	root := filepath.Clean(e.Root)
	if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return true
	}
	if real, err := filepath.EvalSymlinks(root); err == nil && real != root && (abs == real || strings.HasPrefix(abs, real+string(filepath.Separator))) {
		return true
	}
	for _, s := range systemPrefixes {
		if strings.HasPrefix(abs, s) {
			return true
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		m := filepath.Join(h, e.workspaceDir())
		if abs == m || strings.HasPrefix(abs, m+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Rel renders an absolute path relative to the root when it is inside it.
func (e Env) Rel(abs string) string {
	if r, err := filepath.Rel(e.Root, abs); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return abs
}

// sorted is a small helper for deterministic output.
func sorted(ss []string) []string {
	out := append([]string(nil), ss...)
	sort.Strings(out)
	return out
}
