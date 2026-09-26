package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/config/yaml"
)

// Op is one proposed config write: set a dotted path to a value, or delete it (Value "").
// Value is text in the config's own syntax: a scalar (`true`, `30s`, `vllm/x`) or a
// one-line flow container (`[ls, -la, "{{path}}"]`, `{enabled: false}`).
type Op struct {
	Path   string
	Value  string
	Delete bool
}

// Patch drafts a config write as a unified diff against file, without writing. YAML files
// are edited in place (comments and order kept); a JSON file is re-emitted. The patched
// file is trial-loaded through the same layers, so a patch that the law refuses is refused
// here, before the human is asked.
func (c *Config) Patch(file string, ops []Op) (diff string, err error) {
	before, after, err := c.patched(file, ops)
	if err != nil {
		return "", err
	}
	return yaml.Diff(rel(c.Root, file), before, after), nil
}

// ApplyPatch writes the patched file after the same trial load; call it on the human's yes.
func (c *Config) ApplyPatch(file string, ops []Op) error {
	_, after, err := c.patched(file, ops)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, after, 0o644)
}

func (c *Config) patched(file string, ops []Op) (before, after []byte, err error) {
	if len(ops) == 0 {
		return nil, nil, fmt.Errorf("patch: no operations")
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(c.Root, file)
	}
	before, err = c.opts.readFile(file)
	if err != nil && !strings.Contains(err.Error(), "no such file") {
		return nil, nil, err
	}
	after = before
	switch strings.ToLower(filepath.Ext(file)) {
	case ".yaml", ".yml":
		for _, op := range ops {
			var v *yaml.Node
			if !op.Delete {
				if v, err = yaml.Parse([]byte(op.Value), "patch:"+op.Path); err != nil {
					return nil, nil, fmt.Errorf("patch %s: %w", op.Path, err)
				}
			}
			after, err = yaml.Edit(after, file, strings.Split(op.Path, "."), v)
			if err != nil {
				return nil, nil, err
			}
		}
	case ".json":
		n := yaml.MapNode(file, 1)
		if len(before) > 0 {
			if n, err = yaml.ParseJSON(before, file); err != nil {
				return nil, nil, err
			}
			if n.Kind != yaml.Map {
				return nil, nil, fmt.Errorf("%s: a config document must be a map", file)
			}
		}
		for _, op := range ops {
			var v *yaml.Node
			if !op.Delete {
				if v, err = yaml.Parse([]byte(op.Value), "patch:"+op.Path); err != nil {
					return nil, nil, fmt.Errorf("patch %s: %w", op.Path, err)
				}
			}
			applyOp(n, strings.Split(op.Path, "."), v)
		}
		after = []byte(n.Pretty() + "\n")
	default:
		return nil, nil, fmt.Errorf("%s: config files are .yaml, .yml or .json", file)
	}
	// Trial load with the patched file in place of the disk's.
	o := c.opts
	o.Overlay = map[string][]byte{}
	for k, v := range c.opts.Overlay {
		o.Overlay[k] = v
	}
	o.Overlay[file] = after
	if _, err := LoadWith(o); err != nil {
		return nil, nil, fmt.Errorf("the patched config does not load: %w", err)
	}
	return before, after, nil
}

func applyOp(n *yaml.Node, path []string, v *yaml.Node) {
	for len(path) > 1 {
		next := n.Get(path[0])
		if next == nil || next.Kind != yaml.Map {
			if v == nil {
				return
			}
			next = yaml.MapNode(n.File, 0)
			n.Set(path[0], next)
		}
		n, path = next, path[1:]
	}
	if v == nil {
		deleteKey(n, path[0])
		return
	}
	n.Set(path[0], v)
}

// Change is one key whose live value moved between two loads.
type Change struct {
	Key    string
	Before string
	After  string
	Origin Origin // after
}

func (ch Change) String() string {
	switch {
	case ch.Before == "":
		return fmt.Sprintf("+ %s: %s — %s", ch.Key, ch.After, ch.Origin)
	case ch.After == "":
		return fmt.Sprintf("- %s (was %s)", ch.Key, ch.Before)
	}
	return fmt.Sprintf("~ %s: %s → %s — %s", ch.Key, ch.Before, ch.After, ch.Origin)
}

// Reload loads the config again with the same options and reports what changed, with
// origins. A load that fails returns the old config untouched and the error.
func (c *Config) Reload() (*Config, []Change, error) {
	next, err := LoadWith(c.opts)
	if err != nil {
		return c, nil, err
	}
	return next, Delta(c, next), nil
}

// Delta compares the leaves of two configs.
func Delta(a, b *Config) []Change {
	la, lb := leaves(a.tree), leaves(b.tree)
	keys := map[string]bool{}
	for k := range la {
		keys[k] = true
	}
	for k := range lb {
		keys[k] = true
	}
	var out []Change
	for k := range keys {
		if la[k] == lb[k] {
			continue
		}
		out = append(out, Change{Key: k, Before: la[k], After: lb[k], Origin: b.Origins[k]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func leaves(n *yaml.Node) map[string]string {
	out := map[string]string{}
	var walk func(n *yaml.Node, path string)
	walk = func(n *yaml.Node, path string) {
		switch {
		case n == nil:
		case n.Kind == yaml.Map:
			for i, k := range n.Keys {
				walk(n.Vals[i], join(path, k))
			}
		case n.Kind == yaml.List:
			for i, it := range n.Items {
				walk(it, fmt.Sprintf("%s[%d]", path, i))
			}
		default:
			out[path] = n.JSON()
		}
	}
	walk(n, "")
	return out
}

// ProjectFile is the project layer's path (the file a proposal lands in), in the format
// the layer already uses or config.yaml when none exists yet.
func (c *Config) ProjectFile() string {
	for _, l := range c.Layers {
		if l.Name == "project" {
			return l.Path
		}
	}
	return filepath.Join(c.Root, c.Dist.WorldDir, "config.yaml")
}

// ProposeEnable drafts the patch that switches a tool back on (the second naming of a need).
func (c *Config) ProposeEnable(tool string) (string, error) {
	path := "tools." + tool + ".enabled"
	if _, ok := c.Tools.Custom[tool]; ok {
		path = "tools.custom." + tool + ".enabled"
	} else if _, ok := Builtins[tool]; !ok {
		return "", fmt.Errorf("no tool %q to enable — draft a tools.custom entry instead", tool)
	}
	return c.Patch(c.ProjectFile(), []Op{{Path: path, Value: "true"}})
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

// Equal reports whether two configs decode to the same values (origins aside).
func Equal(a, b *Config) bool { return reflect.DeepEqual(leaves(a.tree), leaves(b.tree)) }
