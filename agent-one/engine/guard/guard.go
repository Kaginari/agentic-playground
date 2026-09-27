// Package guard is the global dangerous-command guard: a denylist of catastrophic, irreversible
// shell commands, refused before the classifier and the operator gate see them — no approval can run
// a match. The built-in list is always on; a machine-wide file (~/.agents/hooks/dangerous-patterns.txt,
// the path other agents' hooks read too) and a config file add to it, never take away.
//
// It guards against accidents, not a determined agent: `python -c "shutil.rmtree(...)"` slips past
// any regex. The sandbox and the gate remain the containment; this is the last word on the worst.
package guard

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed patterns.txt
var defaultPatterns string

// SharedPath is the machine-wide denylist other agents' hooks read; relative to home.
const SharedPath = ".agents/hooks/dangerous-patterns.txt"

// Rule is one compiled pattern and where it came from.
type Rule struct {
	Pattern string
	Source  string
	re      *regexp.Regexp
}

// Guard is the effective denylist.
type Guard struct {
	Rules []Rule
	Notes []string // files that could not be read or patterns that did not compile
}

// Default is the built-in list alone.
func Default() *Guard {
	g := &Guard{}
	g.add(defaultPatterns, "built-in")
	return g
}

// Load is the built-in list plus each file that exists (a missing file is no finding; an unreadable
// one or a bad pattern is a note).
func Load(files ...string) *Guard {
	g := Default()
	for _, f := range files {
		if f == "" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			if !os.IsNotExist(err) {
				g.Notes = append(g.Notes, fmt.Sprintf("%s: %v", f, err))
			}
			continue
		}
		g.add(string(b), f)
	}
	return g
}

// ForHome is Load with the machine-wide file under home.
func ForHome(home string, extra ...string) *Guard {
	g := Load(append([]string{filepath.Join(home, SharedPath)}, extra...)...)
	if h := filepath.Clean(home); home != "" && h != "/" {
		// the home by its own path: `rm -rf ~` is caught by the list, `rm -rf /home/you` by this
		g.add(`(^|[;&|(`+"`"+`[:space:]])rm[[:space:]]+(-[a-zA-Z-]+[[:space:]]+)*"?`+regexp.QuoteMeta(h)+`/?\*?"?([[:space:]]|$|;|&)`, "home")
	}
	return g
}

func (g *Guard) add(text, source string) {
	sc := bufio.NewScanner(strings.NewReader(text))
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		re, err := regexp.Compile(line)
		if err != nil {
			g.Notes = append(g.Notes, fmt.Sprintf("%s:%d: %v", source, n, err))
			continue
		}
		g.Rules = append(g.Rules, Rule{Pattern: line, Source: fmt.Sprintf("%s:%d", source, n), re: re})
	}
}

// Match returns the first rule a command matches, or nil.
func (g *Guard) Match(cmd string) *Rule {
	if g == nil {
		return nil
	}
	for i := range g.Rules {
		if g.Rules[i].re.MatchString(cmd) {
			return &g.Rules[i]
		}
	}
	return nil
}

// Refusal is the message a blocked command answers with: what matched, and not to work around it.
func Refusal(r *Rule) string {
	return "blocked by the global dangerous-command guard (" + r.Source + "): this command is catastrophic or irreversible. " +
		"Do not retry it or work around the guard; tell the operator what you meant to do instead."
}

// AddPatterns adds pattern lines from a named source (config): a bad one is a note, never a crash.
func (g *Guard) AddPatterns(lines []string, source string) {
	g.add(strings.Join(lines, "\n"), source)
}

// DefaultPatterns is the built-in list as text, for the machine-wide file.
func DefaultPatterns() string { return defaultPatterns }
