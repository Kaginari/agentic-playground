// Package gate is the human gate: outward (Principle 7) and destructive (Policy 6) acts always pass
// it — a real answer on a TTY, an explicit pre-approval per class, or a dry run that only says
// what it would ask. Nothing is auto-approved; a denial stops the turn.
package gate

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/Kaginari/agent-one/tool"
)

// Decision is the gate's answer.
type Decision string

const (
	NotNeeded Decision = "not-needed"
	Approved  Decision = "approved"
	Denied    Decision = "denied"
	WouldAsk  Decision = "would-ask"
)

// Request is one act put to the gate.
type Request struct {
	ID      string     // the step id (journal and report name it)
	Tool    string     // the tool about to run
	Class   tool.Class // the settled class
	Why     string     // the classifier's reason
	Summary string     // the act, one line (the command, the path)
	Force   bool       // a permission rule said ask: the gate asks whatever the class
}

// Answer is the decision with its provenance.
type Answer struct {
	Needed   bool
	Decision Decision
	By       string // "tty" | "pre-approved" | "dry-run" | "no-tty" | ""
	Why      string
}

// Gate holds the session's gate policy. Every field is configuration: the pre-approved
// classes (the human's explicit word), Strict (writes ask too), DryRun (report, never run).
type Gate struct {
	Approve map[tool.Class]bool
	Strict  bool
	DryRun  bool
	In      io.Reader // defaults to os.Stdin
	Out     io.Writer // defaults to os.Stderr
	IsTTY   func() bool
	Prompt  func(Request) string // the human-facing question; defaults to DefaultPrompt
	Flag    string               // the flag named in a denial ("--approve" by default)
}

// New builds a gate with no pre-approvals.
func New() *Gate { return &Gate{Approve: map[tool.Class]bool{}} }

// Needs reports whether a class must pass the gate under this policy.
func (g *Gate) Needs(c tool.Class) bool {
	return c >= tool.Outward || (g != nil && g.Strict && c == tool.Write)
}

// ParseApprove reads "outward,destructive" into a class set; unknown names are errors.
func ParseApprove(s string) (map[tool.Class]bool, error) {
	out := map[tool.Class]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c, ok := tool.ParseClass(part)
		if !ok {
			return nil, fmt.Errorf("gate: %q is not a class (read|write|outward|destructive)", part)
		}
		out[c] = true
	}
	return out, nil
}

// DefaultPrompt is the human-facing question.
func DefaultPrompt(r Request) string {
	return fmt.Sprintf("gate %s — %s (%s)\n   %s: %s\n   approve? [y/N] ", r.ID, r.Class, r.Why, r.Tool, r.Summary)
}

var yes = regexp.MustCompile(`^(?i)y(es)?$`)

// Ask puts an act to the gate. It never returns Approved without a human's word: a pre-approval
// given on the command line, or an answer on a TTY.
func (g *Gate) Ask(r Request) Answer {
	if !g.Needs(r.Class) && !r.Force {
		return Answer{Decision: NotNeeded}
	}
	flag := g.Flag
	if flag == "" {
		flag = "--approve"
	}
	if g.Approve[r.Class] && !r.Force {
		return Answer{Needed: true, Decision: Approved, By: "pre-approved", Why: r.Why}
	}
	if g.DryRun {
		return Answer{Needed: true, Decision: WouldAsk, By: "dry-run", Why: r.Why}
	}
	if g.isTTY() {
		prompt := g.Prompt
		if prompt == nil {
			prompt = DefaultPrompt
		}
		out := g.Out
		if out == nil {
			out = os.Stderr
		}
		in := g.In
		if in == nil {
			in = os.Stdin
		}
		fmt.Fprint(out, prompt(r))
		line, _ := bufio.NewReader(in).ReadString('\n')
		if yes.MatchString(strings.TrimSpace(line)) {
			return Answer{Needed: true, Decision: Approved, By: "tty", Why: r.Why}
		}
		return Answer{Needed: true, Decision: Denied, By: "tty", Why: r.Why}
	}
	return Answer{Needed: true, Decision: Denied, By: "no-tty", Why: fmt.Sprintf("%s; no TTY to ask — pass %s %s to pre-approve", r.Why, flag, r.Class)}
}

func (g *Gate) isTTY() bool {
	if g.IsTTY != nil {
		return g.IsTTY()
	}
	return StdinIsTTY()
}

// StdinIsTTY reports whether both stdin and stderr are terminals.
func StdinIsTTY() bool {
	for _, f := range []*os.File{os.Stdin, os.Stderr} {
		fi, err := f.Stat()
		if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}
