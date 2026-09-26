package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// fileLayers are the layers whose word is the human's written word.
var fileLayers = map[string]bool{"global": true, "project": true, "local": true}

// finish runs every post-decode step and the policy's refusals.
func (c *Config) finish() error {
	steps := []func() error{
		c.checkScalars, c.checkProviders, c.checkTools, c.checkMCP, c.checkPermissions,
		c.resolveRules, c.buildRanks, c.checkModels, c.checkGate, c.checkCompaction,
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return err
		}
	}
	sort.Strings(c.Holes)
	return nil
}

func (c *Config) checkScalars() error {
	enum := func(path, val string, allowed ...string) error {
		if val == "" || contains(allowed, val) {
			return nil
		}
		return fmt.Errorf("%s: %s: %q is not %s", c.Where(path), path, val, strings.Join(allowed, " | "))
	}
	if err := enum("mode", c.Mode, "build", "plan"); err != nil {
		return err
	}
	if err := enum("output.format", c.Output.Format, "text", "json", "wire"); err != nil {
		return err
	}
	if err := enum("compaction.strategy", c.Compaction.Strategy, "drain", "summary"); err != nil {
		return err
	}
	if err := enum("logLevel", strings.ToUpper(c.LogLevel), "DEBUG", "INFO", "WARN", "ERROR"); err != nil {
		return err
	}
	if err := enum("output.color", c.Output.Color, "auto", "always", "never"); err != nil {
		return err
	}
	return nil
}

// checkGate applies the refusals around the human gate: a full gate off is honored only from
// a config file; standing approvals never come from env; classes must exist.
func (c *Config) checkGate() error {
	if !c.Policy.HumanGate.Enabled {
		o, _ := c.Origins["policy.humanGate.enabled"]
		if !fileLayers[o.Layer] {
			return fmt.Errorf("%s: policy.humanGate.enabled: false is honored only from a config file (global, project or local) — never env or a flag", o)
		}
	}
	for i, a := range c.Policy.HumanGate.Approve {
		path := fmt.Sprintf("policy.humanGate.approve[%d]", i)
		if _, ok := classOrder[a]; !ok {
			return fmt.Errorf("%s: %s: %q is not read, write, outward or destructive", c.Where(path), path, a)
		}
		o, _ := c.Origins[path]
		if o.Layer == "env" || o.Layer == "env-file" {
			return fmt.Errorf("%s: %s: a standing pre-approval is the human's written word — a config file or --approve, never env", o, path)
		}
	}
	return nil
}

func (c *Config) checkCompaction() error {
	if c.Compaction.Enabled && c.Compaction.Trigger.Tokens >= c.Policy.Budget.ContextTokens {
		return fmt.Errorf("%s: compaction.trigger.tokens %d ≥ policy.budget.contextTokens %d — a drain that never fires is a disabled drain; say enabled: false instead",
			c.Where("compaction.trigger.tokens"), c.Compaction.Trigger.Tokens, c.Policy.Budget.ContextTokens)
	}
	if c.Policy.Budget.StressTokens > c.Policy.Budget.ContextTokens {
		return fmt.Errorf("%s: policy.budget.stressTokens %d is past contextTokens %d", c.Where("policy.budget.stressTokens"), c.Policy.Budget.StressTokens, c.Policy.Budget.ContextTokens)
	}
	return nil
}

// Finding is one honesty-rule line: a switched-off policy feature and where it was switched.
type Finding struct {
	Key    string
	Origin Origin
}

func (f Finding) String() string { return fmt.Sprintf("@? off %s — %s", f.Key, f.Origin) }

// honestyRoots are the sections whose `enabled: false` is a finding, not silence.
var honestyRoots = []string{"policy", "memory", "instruments", "toolbox", "ontology", "compaction", "hooks", "permissions", "mcp", "discovery"}

// Off lists every policy feature whose live value is enabled: false, plus the standing
// approvals and status knobs the spec treats the same way, each with its origin. A feature
// the binary ships off (origin default) is not listed: the finding is a switch someone threw.
func (c *Config) Off() []Finding {
	var out []Finding
	seen := map[string]bool{}
	add := func(key string) {
		if seen[key] {
			return
		}
		seen[key] = true
		o, ok := c.Origins[key]
		if !ok || o.Layer == "default" {
			return // shipped off (a LATER feature): nobody threw a switch
		}
		out = append(out, Finding{Key: key, Origin: o})
	}
	walkEnabled(reflect.ValueOf(c).Elem(), "", func(path string, on bool) {
		if on {
			return
		}
		for _, root := range honestyRoots {
			if path == root+".enabled" || strings.HasPrefix(path, root+".") {
				add(path)
				return
			}
		}
	})
	for i, a := range c.Policy.HumanGate.Approve {
		if a == "destructive" {
			add(fmt.Sprintf("policy.humanGate.approve[%d]", i))
		}
	}
	if !c.Instruments.Status.ShowOff {
		add("instruments.status.showOff")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// walkEnabled visits every `enabled` bool reachable from a struct, with its dotted path.
func walkEnabled(v reflect.Value, path string, visit func(string, bool)) {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			walkEnabled(v.Elem(), path, visit)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
			if tag == "" || tag == "-" {
				continue
			}
			f := v.Field(i)
			p := join(path, tag)
			if tag == "enabled" && f.Kind() == reflect.Bool {
				visit(p, f.Bool())
				continue
			}
			walkEnabled(f, p, visit)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkEnabled(v.MapIndex(k), join(path, k.String()), visit)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			walkEnabled(v.Index(i), fmt.Sprintf("%s[%d]", path, i), visit)
		}
	}
}

// OffLines renders the honesty-rule findings for status, one line each.
func (c *Config) OffLines() []string {
	var out []string
	for _, f := range c.Off() {
		out = append(out, f.String())
	}
	return out
}
