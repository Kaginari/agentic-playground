package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/wire"
)

// Check is one injected rule check (config supplies them): a command whose exit code is the
// reading. It runs at the end of every turn that wrote files, after the four checks.
type Check struct {
	Name    string
	Command string
	Timeout time.Duration
	Scope   string // all | rank:<rank> | member:<name>; "" = all
}

// GateOptions is policy.gate: the Domain owner's four checks at turn end, each its own switch, plus the
// injected checks and whether the verdict is recorded.
type GateOptions struct {
	Enabled        bool
	RightAuthor    bool
	InvariantsHold bool
	DutiesDone     bool
	DocTruthful    bool
	Checks         []Check
	Log            bool          // append the verdict to log.md
	Retries        int           // a failed gate goes back to the model this many times per turn
	Timeout        time.Duration // per verify command; 0 = 120s
}

// Verdict is one gate run.
type Verdict struct {
	Word    string   // pass | fail | n/a (no domain owners)
	Reasons []string // why it failed
	Holes   []string
	Checked []string // what ran
}

func (v Verdict) String() string {
	if len(v.Reasons) == 0 {
		return v.Word
	}
	return v.Word + " — " + strings.Join(v.Reasons, "; ")
}

// Gate runs the checks over a turn's writes for an agent and returns the verdict. It is the
// EndGate hook's agent, exposed so a dispatcher can run it on its own account.
func (w *Workspace) Gate(ctx context.Context, opt GateOptions, as string, wrote []string, rep *wire.Report, isWire bool, ask string) Verdict {
	v := Verdict{Word: "pass"}
	if !opt.Enabled {
		return Verdict{Word: "off", Holes: []string{"policy.gate is off — the turn's writes were not checked"}}
	}
	// the verify lines as they stood before the turn: an agent edits its own doc in the same turn,
	// so the post-turn lines alone would let it rewrite the check it is judged by
	before := w.verifyLines()
	_ = w.Reload() // docs may have changed this turn; the roster is read fresh
	fail := func(why string) { v.Reasons = append(v.Reasons, why) }
	domainOwners := w.GateHolders()
	if !domainOwners {
		v.Word = "Gate: n/a (no gate holder)"
		if policyTable(w) {
			v.Word = or(w.Lex.GateNA, v.Word)
		}
	}
	self := w.Member(as)
	touched := map[string]*Member{}
	if self != nil {
		touched[self.Name] = self
	}
	for _, rel := range wrote {
		owner := w.Owner(rel)
		if owner != nil {
			touched[owner.Name] = owner
		}
		// 1. right zone worker authored
		if opt.RightAuthor && domainOwners {
			switch {
			case owner != nil && (self == nil || owner.Name != self.Name) && !(self != nil && self.InOwnership(rel)):
				fail(fmt.Sprintf("%s: %s belongs to %s, written by %s", w.Lex.Checks[0], rel, owner.Name, or(as, "orchestrator")))
			case owner == nil && self != nil && !self.InOwnership(rel) && len(self.Ownership) > 0:
				fail(fmt.Sprintf("%s: %s is outside %s's ownership", w.Lex.Checks[0], rel, self.Name))
			}
		}
		// 4. doc truthful — Docs-as-code
		if opt.DocTruthful && owner != nil && owner.Doc != "" && !contains(wrote, owner.Doc) && normRel(rel) != normRel(owner.Doc) {
			fail(fmt.Sprintf("%s (Docs-as-code): %s changed under %s's ownership but %s did not change in the same turn", w.Lex.Checks[3], rel, owner.Name, owner.Doc))
		}
	}
	v.Checked = append(v.Checked, w.Lex.Checks[0], w.Lex.Checks[3])
	// 2. invariants hold — verify commands are readings
	if opt.InvariantsHold {
		names := make([]string, 0, len(touched))
		for n := range touched {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			now := touched[n].Verify
			cmds := append([]string(nil), before[n]...)
			for _, c := range before[n] {
				if !contains(now, c) {
					v.Holes = append(v.Holes, fmt.Sprintf("%s: %s's verify `%s` was removed or changed this turn — the pre-turn line still ran; the gate holder confirms the change", w.Lex.Checks[1], n, c))
				}
			}
			for _, c := range now {
				if !contains(cmds, c) {
					cmds = append(cmds, c)
				}
			}
			for _, cmd := range cmds {
				if code, tail := w.run(ctx, cmd, opt.Timeout); code != 0 {
					fail(fmt.Sprintf("%s: %s verify `%s` exit %d%s", w.Lex.Checks[1], n, cmd, code, tail))
				}
			}
		}
		v.Checked = append(v.Checked, w.Lex.Checks[1])
	}
	// 3. duties done — the commission is answered on the wire
	if opt.DutiesDone {
		c := wire.ParseCommission(ask)
		commissioned := c.Ask != "" || c.Unsaid || c.Scope != ""
		switch {
		case commissioned && !isWire:
			fail(w.Lex.Checks[2] + ": the commission asked on the wire and the answer carries no @S")
		case isWire && rep != nil && strings.TrimSpace(rep.Status) == "":
			fail(w.Lex.Checks[2] + ": @S is empty")
		case commissioned && c.Unsaid && rep != nil && len(rep.Unsaid) == 0:
			v.Holes = append(v.Holes, w.Lex.Checks[2]+": +unsaid was asked and no @U came back")
		}
		v.Checked = append(v.Checked, w.Lex.Checks[2])
	}
	// injected checks, those whose scope reaches this agent
	rank := w.RankOf(as).Name
	if rank == Orchestrator {
		rank = ""
	}
	for _, ck := range opt.Checks {
		if strings.TrimSpace(ck.Command) == "" || !(Rule{Scope: ck.Scope}).Applies(as, rank) {
			continue
		}
		if code, tail := w.run(ctx, ck.Command, ck.Timeout); code != 0 {
			fail(fmt.Sprintf("check %s: `%s` exit %d%s", or(ck.Name, "unnamed"), ck.Command, code, tail))
		}
		v.Checked = append(v.Checked, "check "+or(ck.Name, ck.Command))
	}
	if len(v.Reasons) > 0 {
		v.Word = "fail"
	}
	return v
}

func (w *Workspace) run(ctx context.Context, command string, timeout time.Duration) (int, string) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "bash", "-c", command)
	cmd.Dir = w.Root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	if cctx.Err() == context.DeadlineExceeded {
		code = 124
	}
	tail := strings.TrimSpace(out.String())
	if len(tail) > 160 {
		tail = "…" + tail[len(tail)-160:]
	}
	if tail != "" {
		tail = " — " + oneLine(tail)
	}
	return code, tail
}

// EndGate is the loop hook: the gate over the turn, the verdict appended to log.md, a fail
// returned as an error so the turn fails (Policy 3: nothing lands without the gate).
func (w *Workspace) EndGate(opt GateOptions) func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
	return func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
		as := s.Engine.As
		if as == "" {
			as = s.Engine.Lexicon.Agent
		}
		// a Subagent's writes were gated on its own account: the dispatcher gates only its own
		s.Wrote = w.ownWrites(s)
		if len(s.Wrote) == 0 {
			return "", nil, nil
		}
		v := w.Gate(ctx, opt, as, s.Wrote, &r.Report, r.IsWire, s.Ask)
		holes := append([]string(nil), v.Holes...)
		if opt.Log && opt.Enabled {
			result := "done"
			if v.Word == "fail" {
				result = "failed — gate"
			}
			title := "gate " + v.Word
			if !isVerdictWord(v.Word) {
				title = v.Word // already `Gate: n/a (no domain owners)`
			}
			e := Entry{Author: or(as, "orchestrator"), Title: title + " — " + oneLine(firstLine(s.Ask)), Task: firstLine(s.Ask), Files: s.Wrote, Gate: v.String(), Result: result}
			if r.IsWire {
				for _, u := range r.Report.Unsaid {
					e.Learned = append(e.Learned, "@U "+u.Kind+" "+u.Text)
				}
			}
			if err := w.AppendLog(e); err != nil {
				holes = append(holes, "log.md: "+err.Error())
			}
		}
		if v.Word == "fail" {
			return v.String(), holes, fmt.Errorf("gate fail — %s", strings.Join(v.Reasons, "; "))
		}
		return v.String(), holes, nil
	}
}

// AppendLog appends one entry to the workspace's log (Policy 4).
func (w *Workspace) AppendLog(e Entry) error {
	l, err := OpenLog(w.LogPath())
	if err != nil {
		return err
	}
	return l.Append(e)
}

// LogPath is <root>/<workspaceDir>/log.md.
func (w *Workspace) LogPath() string { return w.Dir() + "/" + w.Lex.Log }

// domain owners is true for a verdict word from a workspace with a gate holder (pass | fail), false for the
// no-gate-holder line, which already reads `Gate: n/a (…)`.
func isVerdictWord(word string) bool { return word == "pass" || word == "fail" }

func policyTable(w *Workspace) bool {
	d := DefaultRanks(w.Lex)
	if len(d) != len(w.Ranks) {
		return false
	}
	for i := range d {
		if d[i].Name != w.Ranks[i].Name {
			return false
		}
	}
	return true
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

// verifyLines is each member's verify commands as the roster holds them now.
func (w *Workspace) verifyLines() map[string][]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[string][]string{}
	for _, c := range w.Members {
		out[c.Name] = append([]string(nil), c.Verify...)
	}
	return out
}
