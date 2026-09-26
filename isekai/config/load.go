package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/config/yaml"
)

// Options is what Load needs beyond the process: tests fill every field.
type Options struct {
	Dist  string              // isekai | agent-one; "" detects from the world dir, else isekai
	Root  string              // the world root; "" walks up from Cwd to the nearest <dist-dir>
	Cwd   string              // "" is the process cwd
	Home  string              // "" is $HOME
	Env   func(string) string // nil is os.Getenv
	Flags []Override          // layer 7, from ParseFlags
	// Overlay maps a file path to contents read instead of the disk (Patch trials, tests).
	Overlay map[string][]byte
	// DefaultsOnly skips every layer but the built-in defaults.
	DefaultsOnly bool
}

// Override is one CLI-layer assignment: a dotted path and its raw text value.
type Override struct {
	Path  string
	Value string
	Flag  string // the flag it came from, for origins
}

// Layer is one source in precedence order.
type Layer struct {
	Name    string // default | global | project | local | env-file | env | flag
	Path    string // file path, "env", "flag" or "default"
	Present bool
	Node    *yaml.Node
}

// Load reads the effective config for a distribution from the process environment.
func Load(dist string) (*Config, error) {
	return LoadWith(Options{Dist: dist})
}

// Defaults is layer 1 alone: the built-in table for a distribution, no files, env or flags.
func Defaults(dist string) (*Config, error) {
	return LoadWith(Options{Dist: dist, Root: string(os.PathSeparator), Home: "/nonexistent", Env: func(string) string { return "" }, DefaultsOnly: true})
}

var configNames = []string{"config.yaml", "config.yml", "config.json"}
var localNames = []string{"config.local.yaml", "config.local.yml", "config.local.json"}

// LoadWith reads every layer, merges, decodes, validates. A load error names file:line.
func LoadWith(o Options) (*Config, error) {
	if o.Env == nil {
		o.Env = os.Getenv
	}
	if o.Home == "" {
		o.Home = o.Env("HOME")
	}
	if o.Cwd == "" {
		o.Cwd, _ = os.Getwd()
	}
	root := o.Root
	distName := o.Dist
	if root == "" {
		if distName == "" {
			root, distName = findRoot(o.Cwd)
		} else if d, ok := dists[distName]; ok {
			root = walkUp(o.Cwd, d.WorldDir)
		}
		if root == "" {
			root = o.Cwd
		}
	}
	if distName == "" {
		distName = Detect(root, "isekai")
	}
	dist, err := ParseDist(distName, o.Home, o.Env)
	if err != nil {
		return nil, err
	}
	c := &Config{Dist: dist, Root: root, Origins: map[string]Origin{}, opts: o}
	rd := &reader{o: o}

	layers := []Layer{{Name: "default", Path: "default"}}
	if n, err := yaml.ParseJSON([]byte(defaultsJSON), "default"); err != nil {
		return nil, fmt.Errorf("built-in defaults: %w", err)
	} else {
		layers[0].Node, layers[0].Present = n, true
	}
	if o.DefaultsOnly {
		return finishLoad(c, layers, o)
	}
	disableProject := o.Env(dist.EnvPrefix+"DISABLE_PROJECT_CONFIG") == "1"
	global, err := rd.dirLayer("global", dist.GlobalDir, configNames)
	if err != nil {
		return nil, err
	}
	layers = append(layers, global)
	project, err := rd.dirLayer("project", filepath.Join(root, dist.WorldDir), configNames)
	if err != nil {
		return nil, err
	}
	local, err := rd.dirLayer("local", filepath.Join(root, dist.WorldDir), localNames)
	if err != nil {
		return nil, err
	}
	if disableProject {
		project.Present, project.Node = false, nil
		local.Present, local.Node = false, nil
		c.Holes = append(c.Holes, fmt.Sprintf("project and local config skipped — env:%sDISABLE_PROJECT_CONFIG", dist.EnvPrefix))
	}
	layers = append(layers, project, local)

	envFile := Layer{Name: "env-file", Path: "env"}
	if p := o.Env(dist.EnvPrefix + "CONFIG"); p != "" {
		n, err := rd.file(p)
		if err != nil {
			return nil, fmt.Errorf("env:%sCONFIG: %w", dist.EnvPrefix, err)
		}
		envFile.Node, envFile.Present, envFile.Path = n, true, p
	}
	if s := o.Env(dist.EnvPrefix + "CONFIG_CONTENT"); s != "" {
		name := "env:" + dist.EnvPrefix + "CONFIG_CONTENT"
		var n *yaml.Node
		if strings.HasPrefix(strings.TrimSpace(s), "{") {
			n, err = yaml.ParseJSON([]byte(s), name)
		} else {
			n, err = yaml.Parse([]byte(s), name)
		}
		if err != nil {
			return nil, err
		}
		if envFile.Present {
			envFile.Node = merge(envFile.Node, normalize(n))
		} else {
			envFile.Node, envFile.Present = n, true
		}
	}
	layers = append(layers, envFile, envKeys(dist, o.Env), flagLayer(o.Flags))
	return finishLoad(c, layers, o)
}

// finishLoad merges the layers, decodes and validates.
func finishLoad(c *Config, layers []Layer, o Options) (*Config, error) {
	dist := c.Dist
	var tree *yaml.Node
	for i := range layers {
		l := &layers[i]
		if !l.Present || l.Node == nil {
			continue
		}
		if l.Node.Kind == yaml.Null {
			continue
		}
		if l.Node.Kind != yaml.Map {
			return nil, fmt.Errorf("%s: a config document must be a map, got %s", l.Node.Where(), l.Node.Kind)
		}
		if err := foldTimeouts(l.Node); err != nil {
			return nil, err
		}
		l.Node = normalize(l.Node)
		tree = merge(tree, l.Node)
	}
	c.envProviderKeys(tree, o.Env)
	c.Layers = layers
	c.tree = tree
	layerOf := map[string]string{}
	for _, l := range layers {
		layerOf[l.Path] = l.Name
	}
	collectOrigins(tree, "", c.Origins, layerOf)

	d := &decoder{dist: dist, home: o.Home, seen: map[string]bool{}}
	d.decode(tree, reflect.ValueOf(c).Elem(), "")
	for _, u := range d.unknown {
		if err := secretKey(u); err != nil {
			d.errs = append(d.errs, err)
			continue
		}
		c.Holes = append(c.Holes, fmt.Sprintf("unknown key %s — %s", u.path, u.node.Where()))
	}
	if len(d.errs) > 0 {
		return nil, errors.Join(d.errs...)
	}
	c.implicitEnabled(d.seen)
	if err := c.finish(); err != nil {
		return nil, err
	}
	return c, nil
}

// reader reads files through the overlay first.
type reader struct{ o Options }

func (r *reader) read(p string) ([]byte, bool, error) {
	if b, ok := r.o.Overlay[p]; ok {
		return b, true, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return b, true, nil
}

func (r *reader) file(p string) (*yaml.Node, error) {
	b, ok, err := r.read(p)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s: no such file", p)
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".json":
		return yaml.ParseJSON(b, p)
	case ".yaml", ".yml":
		return yaml.Parse(b, p)
	}
	return nil, fmt.Errorf("%s: config files are .yaml, .yml or .json", p)
}

// dirLayer finds the one config file of a layer; two formats side by side is an error.
func (r *reader) dirLayer(name, dir string, names []string) (Layer, error) {
	l := Layer{Name: name, Path: filepath.Join(dir, names[0])}
	var found []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, ok := r.o.Overlay[p]; ok {
			found = append(found, p)
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			found = append(found, p)
		}
	}
	if len(found) > 1 {
		return l, fmt.Errorf("%s: one config file per layer — found %s", dir, strings.Join(found, " and "))
	}
	if len(found) == 0 {
		return l, nil
	}
	n, err := r.file(found[0])
	if err != nil {
		return l, err
	}
	l.Path, l.Node, l.Present = found[0], n, true
	return l, nil
}

// envKeys is layer 6: the single-key env variables.
func envKeys(dist Dist, env func(string) string) Layer {
	l := Layer{Name: "env", Path: "env", Node: yaml.MapNode("env", 0)}
	set := func(path, val, varName string) {
		setPath(l.Node, strings.Split(path, "."), coerceValue(val, "env:"+varName))
		l.Present = true
	}
	p := dist.EnvPrefix
	for _, kv := range [][2]string{{"MODEL", "models.default"}, {"SMALL_MODEL", "smallModel"}, {"LOG_LEVEL", "logLevel"}, {"MODE", "mode"}} {
		if v := env(p + kv[0]); v != "" {
			set(kv[1], v, p+kv[0])
		}
	}
	return l
}

// coerceValue types an env or flag value: a one-line flow `[...]`/`{...}` is parsed as such,
// anything else as a plain scalar.
func coerceValue(val, src string) *yaml.Node {
	t := strings.TrimSpace(val)
	if strings.HasPrefix(t, "[") || strings.HasPrefix(t, "{") {
		if n, err := yaml.Parse([]byte(t), src); err == nil {
			return n
		}
	}
	return yaml.Coerce(val, src, 0)
}

// envProviderKeys applies <PREFIX>PROVIDER_<NAME>_API_KEY after the merge: the variable's
// name becomes providers.<name>.apiKeyEnv (the value never enters the config); a name that
// matches no provider is a hole.
func (c *Config) envProviderKeys(tree *yaml.Node, env func(string) string) {
	providers := tree.Get("providers")
	for _, name := range providerNamesFromEnv(c.Dist, env) {
		varName := c.Dist.EnvPrefix + "PROVIDER_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_API_KEY"
		if providers == nil || providers.Get(name) == nil {
			c.Holes = append(c.Holes, fmt.Sprintf("env:%s names no provider %q", varName, name))
			continue
		}
		setPath(tree, []string{"providers", name, "apiKeyEnv"}, yaml.StringNode(varName, "env:"+varName, 0))
	}
}

// providerNamesFromEnv lists the providers that have a <PREFIX>PROVIDER_<NAME>_API_KEY set.
// os.Environ is not consulted through the injected env, so the known names are probed.
func providerNamesFromEnv(dist Dist, env func(string) string) []string {
	var out []string
	for _, name := range []string{"anthropic", "openai", "openrouter", "ollama", "vllm", "groq", "together", "mistral", "deepseek", "gemini"} {
		v := dist.EnvPrefix + "PROVIDER_" + strings.ToUpper(name) + "_API_KEY"
		if env(v) != "" {
			out = append(out, name)
		}
	}
	return out
}

func flagLayer(flags []Override) Layer {
	l := Layer{Name: "flag", Path: "flag", Node: yaml.MapNode("flag", 0)}
	for _, f := range flags {
		src := "flag:" + f.Flag
		if f.Flag == "" {
			src = "flag:--set " + f.Path
		}
		setPath(l.Node, strings.Split(f.Path, "."), coerceValue(f.Value, src))
		l.Present = true
	}
	return l
}

func setPath(m *yaml.Node, path []string, v *yaml.Node) {
	for len(path) > 1 {
		next := m.Get(path[0])
		if next == nil || next.Kind != yaml.Map {
			next = yaml.MapNode(v.File, 0)
			m.Set(path[0], next)
		}
		m, path = next, path[1:]
	}
	m.Set(path[0], v)
}

// ParseFlags reads the config-relevant flags out of an argv and returns the rest untouched.
// `--no-<feature>` maps to <feature>.enabled=false; `--no-human-gate` is refused.
func ParseFlags(args []string) (overrides []Override, rest []string, err error) {
	known := map[string]string{
		"--model": "models.default", "--mode": "mode", "--approve": "law.humanGate.approve",
		"--format": "output.format", "--max-steps": "law.budget.steps", "--budget": "law.budget.minutes",
		"--small-model": "smallModel", "--log-level": "logLevel", "--profile": "tools.profile",
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := a, "", false
		if eq := strings.IndexByte(a, '='); eq > 0 && strings.HasPrefix(a, "--") {
			name, val, hasVal = a[:eq], a[eq+1:], true
		}
		switch {
		case name == "--dry-run":
			overrides = append(overrides, Override{Path: "law.humanGate.dryRun", Value: "true", Flag: name})
		case name == "--strict":
			overrides = append(overrides, Override{Path: "law.humanGate.strict", Value: "true", Flag: name})
		case strings.HasPrefix(name, "--no-"):
			feature := kebabToPath(name[5:])
			if feature == "humanGate" || feature == "law.humanGate" || feature == "human-gate" {
				return nil, nil, fmt.Errorf("%s does not exist: the human gate is switched off only by a config file (law.humanGate.enabled)", name)
			}
			overrides = append(overrides, Override{Path: feature + ".enabled", Value: "false", Flag: name})
		case name == "--set":
			if !hasVal {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("--set needs key=value")
				}
				i++
				val = args[i]
			}
			k, v, ok := strings.Cut(val, "=")
			if !ok || k == "" {
				return nil, nil, fmt.Errorf("--set needs key=value, got %q", val)
			}
			if k == "law.humanGate.enabled" {
				return nil, nil, fmt.Errorf("--set %s: the human gate is switched off only by a config file", k)
			}
			overrides = append(overrides, Override{Path: k, Value: v})
		default:
			path, ok := known[name]
			if !ok {
				rest = append(rest, a)
				continue
			}
			if !hasVal {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("%s needs a value", name)
				}
				i++
				val = args[i]
			}
			if name == "--approve" {
				val = "[" + val + "]"
			}
			overrides = append(overrides, Override{Path: path, Value: val, Flag: name})
		}
	}
	// a list value for --approve
	for i := range overrides {
		if overrides[i].Path == "law.humanGate.approve" {
			overrides[i].Value = strings.TrimSpace(overrides[i].Value)
		}
	}
	return overrides, rest, nil
}

// kebabToPath turns `law.human-gate` into `law.humanGate`, one segment at a time.
func kebabToPath(s string) string {
	segs := strings.Split(s, ".")
	for i, seg := range segs {
		parts := strings.Split(seg, "-")
		for j := 1; j < len(parts); j++ {
			if parts[j] != "" {
				parts[j] = strings.ToUpper(parts[j][:1]) + parts[j][1:]
			}
		}
		segs[i] = strings.Join(parts, "")
	}
	return strings.Join(segs, ".")
}

// normalize folds the accepted spellings into the canonical keys, keeping origins:
// `model` → models.default; provider `baseUrl`/`kind`; permission `{tool, pattern}` →
// `match`; tools.question → tools.ask; agent-one office and rank words → canonical.
func normalize(n *yaml.Node) *yaml.Node {
	if n == nil || n.Kind != yaml.Map {
		return n
	}
	if m := n.Get("model"); m != nil {
		models := n.Get("models")
		if models == nil || models.Kind != yaml.Map {
			models = yaml.MapNode(m.File, m.Line)
			n.Set("models", models)
		}
		if models.Get("default") == nil {
			models.Set("default", m)
		}
		deleteKey(n, "model")
	}
	if ps := n.Get("providers"); ps != nil && ps.Kind == yaml.Map {
		for _, p := range ps.Vals {
			renameKey(p, "baseUrl", "baseURL")
			renameKey(p, "kind", "type")
		}
	}
	if perms := n.Get("permissions"); perms != nil {
		if rules := perms.Get("rules"); rules != nil && rules.Kind == yaml.List {
			for _, r := range rules.Items {
				if r.Kind != yaml.Map || r.Get("match") != nil {
					continue
				}
				tool, pat := r.Get("tool"), r.Get("pattern")
				if tool != nil && pat != nil && tool.Kind == yaml.String && pat.Kind == yaml.String {
					r.Set("match", yaml.StringNode(tool.Str+":"+pat.Str, tool.File, tool.Line))
					deleteKey(r, "tool")
					deleteKey(r, "pattern")
				}
			}
		}
	}
	if tools := n.Get("tools"); tools != nil {
		renameKey(tools, "question", "ask")
	}
	if models := n.Get("models"); models != nil && models.Kind == yaml.Map {
		if off := models.Get("offices"); off != nil {
			for from, to := range officeAliases {
				renameKey(off, from, to)
			}
		}
		if ranks := models.Get("ranks"); ranks != nil {
			for from, to := range rankAliases {
				renameKey(ranks, from, to)
			}
		}
	}
	return n
}

// foldTimeouts makes `timeout` the one spelling: anywhere in the tree, `timeoutMs` and
// `maxTimeoutMs` (a number of milliseconds) become `timeout` / `maxTimeout` durations. Both
// spellings in one map is an error, never a silent pick.
func foldTimeouts(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.List:
		for _, it := range n.Items {
			if err := foldTimeouts(it); err != nil {
				return err
			}
		}
	case yaml.Map:
		for _, pair := range [][2]string{{"timeoutMs", "timeout"}, {"maxTimeoutMs", "maxTimeout"}} {
			ms := n.Get(pair[0])
			if ms == nil {
				continue
			}
			if n.Get(pair[1]) != nil {
				return fmt.Errorf("%s: both %s and %s are set; use %s", ms.Where(), pair[0], pair[1], pair[1])
			}
			if ms.Kind != yaml.Int {
				return fmt.Errorf("%s: %s is a whole number of milliseconds, got %s", ms.Where(), pair[0], ms.Kind)
			}
			n.Set(pair[0], yaml.StringNode(strconv.FormatInt(ms.Int, 10)+"ms", ms.File, ms.Line))
			renameKey(n, pair[0], pair[1])
		}
		for _, v := range n.Vals {
			if err := foldTimeouts(v); err != nil {
				return err
			}
		}
	}
	return nil
}

// officeAliases and rankAliases fold agent-one's words (lexicon triad.*, rank.*) to the
// canonical keys; canonical keys pass through.
var officeAliases = map[string]string{"analyst": "great-sage", "judge": "raphael", "drafter": "ciel", "great_sage": "great-sage", "greatsage": "great-sage"}
var rankAliases = map[string]string{
	"coord": "elf", "coordinator": "elf", "domain": "orc", "domain-owner": "orc", "zone": "slime", "zone-worker": "slime",
	"service": "kijin", "service-owner": "kijin", "auditor": "dark-elf", "dark_elf": "dark-elf",
	"principal-coordinator": "high-elf", "high_elf": "high-elf", "principal-domain-owner": "high-orc", "high_orc": "high-orc",
}

func renameKey(m *yaml.Node, from, to string) {
	if m == nil || m.Kind != yaml.Map {
		return
	}
	for i, k := range m.Keys {
		if k == from {
			if m.Get(to) == nil {
				m.Keys[i] = to
			} else {
				deleteKey(m, from)
			}
			return
		}
	}
}

func deleteKey(m *yaml.Node, key string) {
	for i, k := range m.Keys {
		if k == key {
			m.Keys = append(m.Keys[:i], m.Keys[i+1:]...)
			m.Vals = append(m.Vals[:i], m.Vals[i+1:]...)
			return
		}
	}
}

// merge lays over onto base: maps merge deep, lists concatenate (scalar lists de-duplicate),
// scalars last-wins, null leaves the base value. The result is a fresh tree.
func merge(base, over *yaml.Node) *yaml.Node {
	if over == nil || over.Kind == yaml.Null {
		return clone(base)
	}
	if base == nil || base.Kind == yaml.Null {
		return clone(over)
	}
	switch {
	case base.Kind == yaml.Map && over.Kind == yaml.Map:
		out := clone(base)
		for i, k := range over.Keys {
			out.Set(k, merge(out.Get(k), over.Vals[i]))
		}
		return out
	case base.Kind == yaml.List && over.Kind == yaml.List:
		out := clone(base)
		for _, it := range over.Items {
			if it.Scalar() && containsScalar(out.Items, it) {
				continue
			}
			out.Items = append(out.Items, clone(it))
		}
		return out
	}
	return clone(over)
}

func containsScalar(items []*yaml.Node, s *yaml.Node) bool {
	for _, it := range items {
		if it.Scalar() && yaml.Equal(it, s) {
			return true
		}
	}
	return false
}

func clone(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Keys = append([]string(nil), n.Keys...)
	c.Vals = make([]*yaml.Node, len(n.Vals))
	for i, v := range n.Vals {
		c.Vals[i] = clone(v)
	}
	c.Items = make([]*yaml.Node, len(n.Items))
	for i, v := range n.Items {
		c.Items[i] = clone(v)
	}
	return &c
}

func collectOrigins(n *yaml.Node, path string, out map[string]Origin, layerOf map[string]string) {
	if n == nil {
		return
	}
	if path != "" {
		out[path] = originOf(n, layerOf)
	}
	switch n.Kind {
	case yaml.Map:
		for i, k := range n.Keys {
			collectOrigins(n.Vals[i], join(path, k), out, layerOf)
		}
	case yaml.List:
		for i, it := range n.Items {
			collectOrigins(it, fmt.Sprintf("%s[%d]", path, i), out, layerOf)
		}
	}
}

func originOf(n *yaml.Node, layerOf map[string]string) Origin {
	o := Origin{File: n.File, Line: n.Line}
	switch {
	case n.File == "default":
		o.Layer, o.Line = "default", 0
	case strings.HasPrefix(n.File, "env:"):
		o.Layer = "env"
		if strings.HasSuffix(n.File, "CONFIG_CONTENT") {
			o.Layer = "env-file"
		} else {
			o.Line = 0
		}
	case strings.HasPrefix(n.File, "flag:"):
		o.Layer, o.Line = "flag", 0
	default:
		o.Layer = layerOf[n.File]
		if o.Layer == "" {
			o.Layer = "env-file"
		}
	}
	return o
}

// Origin returns where a dotted key's live value came from; ok is false for a key no layer set.
func (c *Config) Origin(path string) (Origin, bool) {
	o, ok := c.Origins[path]
	return o, ok
}

// Where renders a key's origin for messages, or "(unset)".
func (c *Config) Where(path string) string {
	if o, ok := c.Origins[path]; ok {
		return o.String()
	}
	return "(unset)"
}

// implicitEnabled makes `enabled` default to true for named map entries (providers, MCP
// servers, custom tools, MCP tools) that did not say otherwise.
func (c *Config) implicitEnabled(seen map[string]bool) {
	for name, p := range c.Providers {
		if !seen["providers."+name+".enabled"] {
			p.Enabled = true
		}
	}
	for name, s := range c.MCP.Servers {
		if !seen["mcp.servers."+name+".enabled"] {
			s.Enabled = true
		}
		for tn, t := range s.Tools {
			if !seen["mcp.servers."+name+".tools."+tn+".enabled"] {
				t.Enabled = true
			}
		}
	}
	for name, t := range c.Tools.Custom {
		if !seen["tools.custom."+name+".enabled"] {
			t.Enabled = true
		}
	}
}

// findRoot walks up from dir to the nearest directory holding any distribution's world dir.
func findRoot(dir string) (root, dist string) {
	for d := dir; ; d = filepath.Dir(d) {
		for _, n := range Names() {
			if isDir(filepath.Join(d, dists[n].WorldDir)) {
				return d, n
			}
		}
		if filepath.Dir(d) == d {
			return "", ""
		}
	}
}

func walkUp(dir, worldDir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if isDir(filepath.Join(d, worldDir)) {
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Paths lists each layer's path and whether it was present, in precedence order.
func (c *Config) Paths() []string {
	var out []string
	for _, l := range c.Layers {
		mark := "absent"
		if l.Present {
			mark = "present"
		}
		out = append(out, fmt.Sprintf("%-8s %-7s %s", l.Name, mark, l.Path))
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
