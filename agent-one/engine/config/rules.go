package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Fact is an injected policy as the ontology takes it: a plain struct, no onto import.
type Fact struct {
	ID     string
	Kind   string // always "policy"
	Text   string
	Scope  string
	Origin Origin
}

// Check is a rule the end-of-turn gate runs; a non-zero exit fails the turn.
type Check struct {
	ID      string
	Command string
	Scope   string
	Timeout time.Duration
	Origin  Origin
}

// CheckResult is one check's reading.
type CheckResult struct {
	Check  Check
	Exit   int
	Output string
	Err    error
}

func (r CheckResult) Passed() bool { return r.Err == nil && r.Exit == 0 }

// resolveRules gives every rule an id, checks its shape and scope, and reads `file` relative
// to the config file that declared it (the workspace root for env content).
func (c *Config) resolveRules() error {
	seen := map[string]bool{}
	for i, r := range c.Rules {
		path := fmt.Sprintf("rules[%d]", i)
		r.Origin, _ = c.Origins[path]
		if r.ID == "" {
			r.ID = fmt.Sprintf("rule%d", i+1)
		}
		if seen[r.ID] {
			return fmt.Errorf("%s: %s.id: %q is used twice", c.Where(path), path, r.ID)
		}
		seen[r.ID] = true
		if r.Text != "" && r.File != "" {
			return fmt.Errorf("%s: %s: text and file are one or the other", c.Where(path), path)
		}
		if r.Text == "" && r.File == "" && r.Check == "" {
			return fmt.Errorf("%s: %s: a rule needs text, file or check", c.Where(path), path)
		}
		if r.Scope == "" {
			r.Scope = "all"
		}
		if !validScope(r.Scope) {
			return fmt.Errorf("%s: %s.scope: %q is not all, rank:<rank> or member:<name>", c.Where(path+".scope"), path, r.Scope)
		}
		if r.File != "" {
			base := c.Root
			if o, ok := c.Origins[path+".file"]; ok && o.Line > 0 && !strings.HasPrefix(o.File, "env:") {
				base = filepath.Dir(o.File)
			}
			p := r.File
			if !filepath.IsAbs(p) {
				p = filepath.Join(base, p)
			}
			b, err := c.opts.readFile(p)
			if err != nil {
				return fmt.Errorf("%s: %s.file: %s: %v", c.Where(path+".file"), path, p, err)
			}
			r.Resolved = p
			r.Text = string(b)
		}
	}
	return nil
}

func (o Options) readFile(p string) ([]byte, error) {
	if b, ok := o.Overlay[p]; ok {
		return b, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no such file")
		}
		return nil, err
	}
	return b, nil
}

func validScope(s string) bool {
	if s == "all" {
		return true
	}
	k, v, ok := strings.Cut(s, ":")
	return ok && v != "" && (k == "rank" || k == "member")
}

// scopeApplies matches a rule's scope to a agent: rank words are read in either lexicon.
func scopeApplies(scope, rank, member string) bool {
	switch {
	case scope == "all":
		return true
	case strings.HasPrefix(scope, "rank:"):
		want := canonicalRank(scope[5:])
		return rank != "" && (canonicalRank(rank) == want || strings.EqualFold(rank, scope[5:]))
	case strings.HasPrefix(scope, "member:"):
		return member != "" && member == scope[7:]
	}
	return false
}

// RulesFor lists the rules that apply to a agent, in layer order.
func (c *Config) RulesFor(rank, member string) []*Rule {
	var out []*Rule
	for _, r := range c.Rules {
		if scopeApplies(r.Scope, rank, member) {
			out = append(out, r)
		}
	}
	return out
}

// PromptRules renders the prose rules for the system prompt, placed after the principles.
func (c *Config) PromptRules(rank, member string) string {
	var b strings.Builder
	for _, r := range c.RulesFor(rank, member) {
		if r.Text == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("## Rules\n")
		}
		fmt.Fprintf(&b, "- [%s] %s\n", r.ID, strings.TrimSpace(r.Text))
	}
	return b.String()
}

// PolicyFacts returns the prose rules as ontology Policy facts for a agent.
func (c *Config) PolicyFacts(rank, member string) []Fact {
	var out []Fact
	for _, r := range c.RulesFor(rank, member) {
		if r.Text == "" {
			continue
		}
		out = append(out, Fact{ID: r.ID, Kind: "policy", Text: strings.TrimSpace(r.Text), Scope: r.Scope, Origin: r.Origin})
	}
	return out
}

// Checks returns the commands the end-of-turn gate runs for a agent.
func (c *Config) Checks(rank, member string) []Check {
	var out []Check
	for _, r := range c.RulesFor(rank, member) {
		if r.Check == "" {
			continue
		}
		t := r.Timeout.D()
		if t == 0 {
			t = 60 * time.Second
		}
		out = append(out, Check{ID: r.ID, Command: r.Check, Scope: r.Scope, Timeout: t, Origin: r.Origin})
	}
	return out
}

// RunCheck runs one check with `sh -c` in dir; env nil inherits the process environment.
func RunCheck(ctx context.Context, dir string, env []string, ch Check) CheckResult {
	res := CheckResult{Check: ch}
	ctx, cancel := context.WithTimeout(ctx, ch.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", ch.Command)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	res.Output = out.String()
	if ee, ok := err.(*exec.ExitError); ok {
		res.Exit = ee.ExitCode()
	} else if err != nil {
		res.Err = err
		res.Exit = -1
	}
	if ctx.Err() == context.DeadlineExceeded {
		res.Err = fmt.Errorf("timed out after %s", ch.Timeout)
	}
	return res
}

// RunChecks runs every check for a agent and returns the readings; any failure fails the turn.
func RunChecks(ctx context.Context, dir string, checks []Check) (results []CheckResult, ok bool) {
	ok = true
	for _, ch := range checks {
		r := RunCheck(ctx, dir, nil, ch)
		if !r.Passed() {
			ok = false
		}
		results = append(results, r)
	}
	return results, ok
}
