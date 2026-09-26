// Package compact is the drain (binary.md §Compaction): when the context reading crosses the
// threshold, each piece of context moves to its home — file reads become pointers, spent tool
// outputs shrink to their journal line, the unsaid and the working notes come out of one model call,
// landed changes get their log entry — and the context is rebuilt from those homes by pointer.
// Every pass has its own switch; `summary` is the generic fallback. A drain that cannot be
// verified aborts and keeps the old context.
package compact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/instrument"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
)

// Switch is one pass's enable flag.
type Switch struct{ Enabled bool }

// Trigger is the reading the drain fires on: the token line against the policy budget, or a
// fraction of the model's real window; the lower wins. Never a provider overflow error.
type Trigger struct {
	Tokens   int     // 0 = the budget's stress line
	Fraction float64 // 0 = unused
	Window   int     // the model's real window; 0 = unknown, fraction unused
}

// Threshold is the token count the drain fires at, given the budget's stress line.
func (t Trigger) Threshold(stress int) int {
	th := t.Tokens
	if th <= 0 {
		th = stress
	}
	if t.Fraction > 0 && t.Window > 0 {
		if f := int(float64(t.Window) * t.Fraction); f < th {
			th = f
		}
	}
	return th
}

// Passes are the drain's stages, each its own switch (compaction.passes.*).
type Passes struct {
	Pointerize   Switch
	TrimSpent    Switch
	Unsaid       Switch
	WorkingNotes Switch
	Episode      Switch
	Verify       Switch
}

// Options is compaction.*.
type Options struct {
	Enabled   bool
	Strategy  string // drain | summary
	Trigger   Trigger
	Passes    Passes
	KeepTurns int // exchanges kept verbatim at the tail (0 = 4)
	Cap       int // @CAP on the unsaid call (0 = 4096)
	Journal   bool
}

// Defaults is the drain with every pass on.
func Defaults() Options {
	on := Switch{Enabled: true}
	return Options{Enabled: true, Strategy: "drain", Passes: Passes{on, on, on, on, on, on}, KeepTurns: 4, Journal: true}
}

// Homes is where each drained piece goes; the workspace package provides them. Any nil func is a
// home the drain does not have (noted, never guessed).
type Homes struct {
	Root         string
	WorkspaceDir string // ".agent-one"
	Principles   func() string
	Remember     func(as, kind, text string) error // a shared note (kind "" for an @F)
	Assert       func(as, kind, text string) error // an ontology Fact on the agent
	Episode      func(as string, wrote []string, workingNotes []string) error
	Provider     provider.Provider // for the model passes; nil = the session's
}

// Pointer is one file read, grep or glob that left the context: path, range and digest.
type Pointer struct {
	ID     string `json:"id"`
	Tool   string `json:"tool"`
	Path   string `json:"path,omitempty"`
	From   int    `json:"from,omitempty"`
	To     int    `json:"to,omitempty"`
	Query  string `json:"query,omitempty"`
	Digest string `json:"digest"`
}

// Line renders the pointer for the table; `now` is the current digest when it differs.
func (p Pointer) Line(flag string) string {
	s := "@P " + p.ID + " "
	switch p.Tool {
	case "read":
		s += p.Path
		if p.From > 0 {
			s += fmt.Sprintf(":%d-%d", p.From, p.To)
		}
	default:
		s += p.Tool + " " + p.Query
	}
	s += " · sha256:" + p.Digest
	if flag != "" {
		s += " (" + flag + ")"
	}
	return s
}

// Report is one drain's journal.
type Report struct {
	Strategy     string    `json:"strategy"`
	Before       int       `json:"before"`
	After        int       `json:"after"`
	Drained      int       `json:"drained"` // messages that left the context
	Kept         int       `json:"kept"`
	Pointers     []Pointer `json:"pointers"`
	Notes        int       `json:"notes"`
	Facts        int       `json:"facts"`
	WorkingNotes []string  `json:"workingNotes"`
	Open         []string  `json:"open"`
	Wrote        []string  `json:"wrote"`
	Passes       []string  `json:"passes"`
	Flags        []string  `json:"flags"`
	Aborted      string    `json:"aborted,omitempty"`
}

// Drainer is the configured compaction.
type Drainer struct {
	Opt   Options
	Homes Homes
	Last  *Report // the last drain's report (nil before one)
	Now   func() time.Time
}

// New builds a drainer.
func New(opt Options, h Homes) *Drainer { return &Drainer{Opt: opt, Homes: h, Now: time.Now} }

// Hooks are the loop seams: Perceive lowers the stress line to the trigger, Drain drains.
func (d *Drainer) Hooks() loop.Hooks { return loop.Hooks{Perceive: d.Perceive, Drain: d.Drain} }

// Perceive re-reads the session's last usage against the trigger so the loop's stress check
// fires at the configured line, not only at the budget's.
func (d *Drainer) Perceive(s *loop.Session) *instrument.Context {
	if !d.Opt.Enabled || s.Turns == 0 {
		return nil
	}
	c := s.Engine.Budget.Context.Reading(s.Last)
	if th := d.Opt.Trigger.Threshold(c.Stress); th < c.Stress {
		c.Stress = th
		switch {
		case c.Tokens >= th:
			c.Zone = instrument.STRESS
		case float64(c.Tokens) >= float64(th)*0.75:
			c.Zone = instrument.NEAR
		default:
			c.Zone = instrument.OK
		}
	}
	return &c
}

func (d *Drainer) workspaceDir() string {
	if d.Homes.WorkspaceDir == "" {
		return ".agent-one"
	}
	return d.Homes.WorkspaceDir
}

// WorkingNotesPath is <root>/<workspaceDir>/instruments/working-notes/<run>.md.
func (d *Drainer) WorkingNotesPath(run string) string {
	return filepath.Join(d.Homes.Root, d.workspaceDir(), "instruments", "working-notes", run+".md")
}

// DrainedPath is the short tier's file for a session's drained turns.
func (d *Drainer) DrainedPath(run string) string {
	return filepath.Join(d.Homes.Root, d.workspaceDir(), "memory", "short", run+".drain.jsonl")
}

func (d *Drainer) keep() int {
	if d.Opt.KeepTurns <= 0 {
		return 4
	}
	return d.Opt.KeepTurns
}

func (d *Drainer) cap() int {
	if d.Opt.Cap <= 0 {
		return 4096
	}
	return d.Opt.Cap
}

// Tokens estimates a conversation's size the toolbox's way: about four bytes a token.
func Tokens(msgs []provider.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Text)
		for _, c := range m.ToolCalls {
			n += len(c.Name) + len(c.Input)
		}
		for _, r := range m.ToolResults {
			n += len(r.Content)
		}
	}
	return (n + 3) / 4
}

// Drain is the loop's Drain hook. It returns true when the context was drained and verified.
func (d *Drainer) Drain(ctx context.Context, s *loop.Session, c instrument.Context) (bool, error) {
	if !d.Opt.Enabled {
		return false, nil
	}
	as := s.Engine.As
	if as == "" {
		as = s.Engine.Lexicon.Agent
	}
	old := s.Messages
	rep := &Report{Strategy: d.Opt.Strategy, Before: Tokens(old)}
	if rep.Strategy == "" {
		rep.Strategy = "drain"
	}
	d.Last = rep
	fail := func(why string) (bool, error) {
		rep.Aborted = why
		s.Messages = old
		d.journal(s, "drain-abort", map[string]interface{}{"why": why, "report": rep})
		return false, fmt.Errorf("drain aborted: %s", why)
	}
	// what leaves and what stays
	cut := d.cutIndex(old)
	if cut <= 1 {
		return fail("nothing to drain: fewer than one finished exchange")
	}
	drained := cloneMessages(old[1:cut])
	tail := old[cut:]
	rep.Drained, rep.Kept = len(drained), len(tail)
	calls := callsOf(drained)
	rep.Open = openHoles(drained)
	rep.Wrote = wroteIn(drained, calls, d.Homes.Root)

	var rebuilt []provider.Message
	var workingNotes []string
	switch rep.Strategy {
	case "summary":
		text, err := d.summarize(ctx, s, drained)
		if err != nil {
			return fail("summary call: " + err.Error())
		}
		rep.Passes = append(rep.Passes, "summary")
		head := s.Ask + "\n\n@SUMMARY\n" + strings.TrimSpace(text)
		if len(rep.Open) > 0 {
			head += "\n@OPEN\n" + strings.Join(rep.Open, "\n")
		}
		rebuilt = append(rebuilt, provider.Message{Role: provider.User, Text: head})
	default:
		if d.Opt.Passes.Pointerize.Enabled {
			rep.Pointers = d.pointerize(drained, calls)
			rep.Passes = append(rep.Passes, "pointerize")
		}
		if d.Opt.Passes.TrimSpent.Enabled {
			d.trimSpent(drained, calls, s)
			rep.Passes = append(rep.Passes, "trimSpent")
		}
		if d.Opt.Passes.Unsaid.Enabled || d.Opt.Passes.WorkingNotes.Enabled {
			r, err := d.modelPass(ctx, s, drained)
			if err != nil {
				return fail("unsaid/workingNotes call: " + err.Error())
			}
			if d.Opt.Passes.Unsaid.Enabled {
				n, f, flags := d.land(as, r)
				rep.Notes, rep.Facts = n, f
				rep.Flags = append(rep.Flags, flags...)
				rep.Passes = append(rep.Passes, "unsaid")
			}
			if d.Opt.Passes.WorkingNotes.Enabled {
				for _, o := range r.Other {
					if strings.HasPrefix(o, "@D ") {
						workingNotes = append(workingNotes, strings.TrimSpace(o[3:]))
					}
				}
				rep.Passes = append(rep.Passes, "workingNotes")
			}
		}
		if len(workingNotes) == 0 {
			workingNotes = d.mechanicalWorkingNotes(s, rep)
		}
		if len(workingNotes) > memory.WorkingNotesLimit {
			rep.Flags = append(rep.Flags, fmt.Sprintf("workingNotes holds %d notes, over the limit of %d — a stress reading", len(workingNotes), memory.WorkingNotesLimit))
		}
		rep.WorkingNotes = workingNotes
		if d.Opt.Passes.WorkingNotes.Enabled {
			if err := d.writeWorkingNotes(s.RunID, workingNotes); err != nil {
				rep.Flags = append(rep.Flags, "workingNotes file: "+err.Error())
			}
		}
		if d.Opt.Passes.Episode.Enabled && len(rep.Wrote) > 0 && d.Homes.Episode != nil {
			if err := d.Homes.Episode(as, rep.Wrote, workingNotes); err != nil {
				rep.Flags = append(rep.Flags, "episode: "+err.Error())
			}
			rep.Passes = append(rep.Passes, "episode")
		}
		rebuilt = append(rebuilt, provider.Message{Role: provider.User, Text: d.rebuiltHead(s, rep)})
	}
	rebuilt = append(rebuilt, tail...)
	rep.After = Tokens(rebuilt)
	// the drained turns stay recallable
	if err := d.index(s.RunID, old[1:cut]); err != nil {
		rep.Flags = append(rep.Flags, "short tier: "+err.Error())
	}
	if d.Opt.Passes.Verify.Enabled || rep.Strategy == "summary" {
		if err := d.verify(s, rebuilt, rep); err != nil {
			return fail(err.Error())
		}
		rep.Passes = append(rep.Passes, "verify")
	}
	s.Messages = rebuilt
	s.Last = provider.Usage{Input: rep.After}
	d.journal(s, "drain-report", map[string]interface{}{"report": rep})
	return true, nil
}

// cutIndex is the index of the first message that stays: the assistant message opening the
// last KeepTurns exchanges. At least one exchange is drained; the ask (message 0) stays always.
func (d *Drainer) cutIndex(msgs []provider.Message) int {
	var idx []int
	for i, m := range msgs {
		if m.Role == provider.Assistant {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return 0
	}
	keep := d.keep()
	if keep >= len(idx) {
		keep = len(idx) - 1
	}
	if keep <= 0 {
		return len(msgs)
	}
	return idx[len(idx)-keep]
}

func cloneMessages(ms []provider.Message) []provider.Message {
	out := make([]provider.Message, len(ms))
	for i, m := range ms {
		out[i] = m
		out[i].ToolCalls = append([]provider.ToolCall(nil), m.ToolCalls...)
		out[i].ToolResults = append([]provider.ToolResult(nil), m.ToolResults...)
	}
	return out
}

func callsOf(ms []provider.Message) map[string]provider.ToolCall {
	out := map[string]provider.ToolCall{}
	for _, m := range ms {
		for _, c := range m.ToolCalls {
			out[c.ID] = c
		}
	}
	return out
}

func openHoles(ms []provider.Message) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range ms {
		if m.Role != provider.Assistant {
			continue
		}
		for _, l := range strings.Split(m.Text, "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "@? ") && !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out
}

func inputOf(c provider.ToolCall) map[string]interface{} {
	var m map[string]interface{}
	_ = json.Unmarshal(c.Input, &m)
	if m == nil {
		m = map[string]interface{}{}
	}
	return m
}

func str(m map[string]interface{}, k string) string {
	s, _ := m[k].(string)
	return s
}

func wroteIn(ms []provider.Message, calls map[string]provider.ToolCall, root string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range ms {
		for _, r := range m.ToolResults {
			c, ok := calls[r.ID]
			if !ok || r.IsError || (c.Name != "write" && c.Name != "edit") {
				continue
			}
			p := str(inputOf(c), "path")
			if p == "" {
				continue
			}
			if filepath.IsAbs(p) {
				if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
					p = rel
				}
			}
			if !seen[p] {
				seen[p] = true
				out = append(out, filepath.ToSlash(p))
			}
		}
	}
	return out
}

func digestOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:4])
}

func (d *Drainer) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(d.Homes.Root, p)
}

// pointerize replaces every successful read / grep / glob result with a pointer; duplicate
// reads of one path collapse to the latest.
func (d *Drainer) pointerize(ms []provider.Message, calls map[string]provider.ToolCall) []Pointer {
	var ps []Pointer
	latest := map[string]int{} // path → index in ps of the latest read
	n := 0
	for i := range ms {
		for j := range ms[i].ToolResults {
			r := &ms[i].ToolResults[j]
			c, ok := calls[r.ID]
			if !ok || r.IsError {
				continue
			}
			in := inputOf(c)
			switch c.Name {
			case "read":
				path := str(in, "path")
				n++
				p := Pointer{ID: fmt.Sprintf("p%d", n), Tool: "read", Path: path}
				if b, err := os.ReadFile(d.abs(path)); err == nil {
					p.Digest = digestOf(b)
				} else {
					p.Digest = "missing"
				}
				p.From, p.To = lineRange(in, r.Content)
				if k, dup := latest[path]; dup {
					ps[k].ID, p.ID = p.ID, ps[k].ID // the earlier read's slot is superseded
					ps[k] = p
					r.Content = "→ " + p.ID + " (superseded read of " + path + ")"
					continue
				}
				latest[path] = len(ps)
				ps = append(ps, p)
				r.Content = "→ " + p.ID
			case "grep", "glob":
				n++
				q := str(in, "pattern")
				if p := str(in, "path"); p != "" {
					q += " in " + p
				}
				p := Pointer{ID: fmt.Sprintf("p%d", n), Tool: c.Name, Query: q, Digest: digestOf([]byte(r.Content))}
				ps = append(ps, p)
				r.Content = "→ " + p.ID + " (" + oneLine(firstLine(r.Content)) + " …)"
			}
		}
	}
	return ps
}

func lineRange(in map[string]interface{}, out string) (int, int) {
	from := 1
	if o, ok := in["offset"].(float64); ok && o > 0 {
		from = int(o)
	}
	lines := 0
	for _, l := range strings.Split(out, "\n") {
		if t := strings.TrimLeft(l, " "); len(t) > 0 && t[0] >= '0' && t[0] <= '9' && strings.Contains(l, "\t") {
			lines++
		}
	}
	if lines == 0 {
		return 0, 0
	}
	return from, from + lines - 1
}

// trimSpent shrinks every other finished tool result to its journal line.
func (d *Drainer) trimSpent(ms []provider.Message, calls map[string]provider.ToolCall, s *loop.Session) {
	journal := ""
	if s.Journal != nil {
		if rel, err := filepath.Rel(d.Homes.Root, s.Journal.Path); err == nil {
			journal = rel
		} else {
			journal = s.Journal.Path
		}
	}
	for i := range ms {
		for j := range ms[i].ToolResults {
			r := &ms[i].ToolResults[j]
			c, ok := calls[r.ID]
			if !ok || strings.HasPrefix(r.Content, "→ p") {
				continue
			}
			state := "ok"
			if r.IsError {
				state = "error"
			}
			line := fmt.Sprintf("%s %s · %s · tail %q", c.Name, oneLine(summary(c)), state, tail(r.Content, 120))
			if journal != "" {
				line += " · full output in " + journal
			}
			if len(line) < len(r.Content) {
				r.Content = line
			}
		}
	}
}

func summary(c provider.ToolCall) string {
	in := inputOf(c)
	for _, k := range []string{"command", "path", "pattern", "agent"} {
		if v := str(in, k); v != "" {
			if len(v) > 80 {
				v = v[:80] + "…"
			}
			return v
		}
	}
	return ""
}

// render turns the drained messages into the text the model pass reads.
func render(ms []provider.Message) string {
	var b strings.Builder
	for i, m := range ms {
		fmt.Fprintf(&b, "[%d %s]\n", i+1, m.Role)
		if m.Text != "" {
			b.WriteString(strings.TrimSpace(m.Text) + "\n")
		}
		for _, c := range m.ToolCalls {
			fmt.Fprintf(&b, "call %s %s %s\n", c.ID, c.Name, oneLine(string(c.Input)))
		}
		for _, r := range m.ToolResults {
			fmt.Fprintf(&b, "result %s: %s\n", r.ID, strings.TrimSpace(r.Content))
		}
	}
	return b.String()
}

func (d *Drainer) provider(s *loop.Session) provider.Provider {
	if d.Homes.Provider != nil {
		return d.Homes.Provider
	}
	return s.Engine.Provider
}

func (d *Drainer) principles() string {
	if d.Homes.Principles != nil {
		return d.Homes.Principles()
	}
	return ""
}

// modelPass is the one model call of the drain: the unsaid over the drained turns, on the
// wire, and the working notes as @D lines.
func (d *Drainer) modelPass(ctx context.Context, s *loop.Session, drained []provider.Message) (wire.Report, error) {
	as := s.Engine.As
	if as == "" {
		as = s.Engine.Lexicon.Agent
	}
	sys := strings.TrimSpace(d.principles() + "\n\nYou are draining " + as + "'s context: these turns are about to leave it. Answer on the wire only.")
	c := wire.Commission{Root: d.Homes.Root, Scope: fmt.Sprintf("the %d drained messages of run %s, below", len(drained), s.RunID), Ask: "findings", Unsaid: d.Opt.Passes.Unsaid.Enabled, Cap: d.cap()}
	var ask strings.Builder
	ask.WriteString(c.String())
	ask.WriteString("Rules: `@S` opens; one `@F path:line fact` per finding worth keeping; `@U policy|team|domain <text>` for each thing that was in this head and is on no file — one per line, never a summary")
	if d.Opt.Passes.WorkingNotes.Enabled {
		fmt.Fprintf(&ask, "; then the working notes as at most %d lines `@D %s <note>`: the goal, where the plan stands, decisions taken and why, open holes as `@?`, the next step — working memory, not history", memory.WorkingNotesLimit, d.Now().Format("2006-01-02"))
	}
	ask.WriteString("; `@E <bytes>` closes.\n\nThe ask being worked: " + oneLine(s.Ask) + "\n\n" + render(drained))
	req := provider.Request{System: sys, Messages: []provider.Message{{Role: provider.User, Text: ask.String()}}, MaxTokens: s.Engine.Budget.MaxTokens}
	resp, err := d.provider(s).Complete(ctx, req)
	if err != nil {
		return wire.Report{}, err
	}
	s.Spend = s.Spend.Add(resp.Usage)
	rep, ok := wire.ParseReport(resp.Message.Text)
	if !ok {
		return wire.Report{}, fmt.Errorf("the drain's answer is not on the wire (no @S)")
	}
	return rep, nil
}

// land sends the unsaid home: a Fact and a shared note per @U, a note per @F.
func (d *Drainer) land(as string, r wire.Report) (notes, facts int, flags []string) {
	for _, u := range r.Unsaid {
		if u.Kind == "" {
			flags = append(flags, "@U without a kind not landed: "+oneLine(u.Text))
			continue
		}
		if d.Homes.Assert != nil {
			if err := d.Homes.Assert(as, u.Kind, u.Text); err != nil {
				flags = append(flags, "assert: "+err.Error())
			} else {
				facts++
			}
		}
		if d.Homes.Remember != nil {
			if err := d.Homes.Remember(as, u.Kind, u.Text); err != nil {
				flags = append(flags, "remember: "+err.Error())
			} else {
				notes++
			}
		}
	}
	if d.Homes.Remember != nil {
		for _, f := range r.Findings {
			if err := d.Homes.Remember(as, "", "@F "+f); err != nil {
				flags = append(flags, "remember: "+err.Error())
			} else {
				notes++
			}
		}
	}
	if d.Homes.Assert == nil && d.Homes.Remember == nil && len(r.Unsaid) > 0 {
		flags = append(flags, fmt.Sprintf("%d @U lines had no home (no Assert/Remember wired)", len(r.Unsaid)))
	}
	return
}

func (d *Drainer) mechanicalWorkingNotes(s *loop.Session, rep *Report) []string {
	day := d.Now().Format("2006-01-02")
	workingNotes := []string{day + " goal: " + oneLine(firstLine(s.Ask)), fmt.Sprintf("%s position: turn %d, %d messages drained, %d kept", day, s.Turns, rep.Drained, rep.Kept)}
	for _, o := range rep.Open {
		if len(workingNotes) >= memory.WorkingNotesLimit-1 {
			break
		}
		workingNotes = append(workingNotes, day+" open: "+strings.TrimPrefix(o, "@? "))
	}
	return append(workingNotes, day+" next: continue from the kept turns")
}

func (d *Drainer) writeWorkingNotes(run string, workingNotes []string) error {
	p := d.WorkingNotesPath(run)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# workingNotes — %s\n\n## Working notes\n", run)
	for _, t := range workingNotes {
		b.WriteString("- " + t + "\n")
	}
	return os.WriteFile(p, []byte(b.String()), 0o644)
}

func (d *Drainer) rebuiltHead(s *loop.Session, rep *Report) string {
	var b strings.Builder
	b.WriteString(s.Ask)
	b.WriteString("\n\n@NOTES " + filepath.ToSlash(rel(d.Homes.Root, d.WorkingNotesPath(s.RunID))) + "\n")
	for _, t := range rep.WorkingNotes {
		b.WriteString("@D " + t + "\n")
	}
	if len(rep.Open) > 0 {
		b.WriteString("@OPEN\n" + strings.Join(rep.Open, "\n") + "\n")
	}
	if len(rep.Pointers) > 0 {
		b.WriteString("@POINTERS re-read on demand; a pointer whose digest changed is flagged, not trusted\n")
		for _, p := range rep.Pointers {
			b.WriteString(p.Line(d.flag(p)) + "\n")
		}
	}
	fmt.Fprintf(&b, "@DRAINED %d messages of this run left the context after turn %d; they are in %s (the `drained` tool recalls them by question)\n", rep.Drained, s.Turns, filepath.ToSlash(rel(d.Homes.Root, d.DrainedPath(s.RunID))))
	return strings.TrimSpace(b.String())
}

// flag is the pointer's current standing: "" when the file still matches, else why not.
func (d *Drainer) flag(p Pointer) string {
	if p.Tool != "read" {
		return ""
	}
	b, err := os.ReadFile(d.abs(p.Path))
	if err != nil {
		return "missing"
	}
	if now := digestOf(b); now != p.Digest {
		return "changed since: now sha256:" + now
	}
	return ""
}

// Check re-resolves every pointer of a report: the flags, one per pointer that no longer
// matches (Principle 9: a changed file is flagged, never trusted).
func (d *Drainer) Check(ps []Pointer) []string {
	var out []string
	for _, p := range ps {
		if f := d.flag(p); f != "" {
			out = append(out, p.ID+" "+p.Path+": "+f)
		}
	}
	return out
}

func (d *Drainer) verify(s *loop.Session, rebuilt []provider.Message, rep *Report) error {
	if len(rebuilt) == 0 || !strings.Contains(rebuilt[0].Text, s.Ask) {
		return fmt.Errorf("verify: the ask is not in the rebuilt context verbatim")
	}
	for _, o := range rep.Open {
		if !strings.Contains(rebuilt[0].Text, o) {
			return fmt.Errorf("verify: open hole lost: %s", o)
		}
	}
	for _, p := range rep.Pointers {
		if p.Tool == "read" {
			if _, err := os.Stat(d.abs(p.Path)); err != nil {
				return fmt.Errorf("verify: pointer %s does not resolve (%s missing)", p.ID, p.Path)
			}
		}
	}
	if rep.After >= rep.Before {
		return fmt.Errorf("verify: tokens after (%d) not below before (%d)", rep.After, rep.Before)
	}
	if len(rebuilt) > 1 && rebuilt[1].Role != provider.Assistant {
		return fmt.Errorf("verify: the kept tail does not start with the model's turn")
	}
	return nil
}

func (d *Drainer) summarize(ctx context.Context, s *loop.Session, drained []provider.Message) (string, error) {
	sys := strings.TrimSpace(d.principles() + "\n\nYou are compacting a conversation: write the summary that lets the work continue — the ask, what was done, decisions and why, every open `@?`, paths touched. Terse; paths, not payloads.")
	req := provider.Request{System: sys, Messages: []provider.Message{{Role: provider.User, Text: "The ask: " + s.Ask + "\n\n" + render(drained)}}, MaxTokens: s.Engine.Budget.MaxTokens}
	resp, err := d.provider(s).Complete(ctx, req)
	if err != nil {
		return "", err
	}
	s.Spend = s.Spend.Add(resp.Usage)
	if strings.TrimSpace(resp.Message.Text) == "" {
		return "", fmt.Errorf("empty summary")
	}
	return resp.Message.Text, nil
}

// Drained is one message that left a context, kept in the short tier.
type Drained struct {
	Run  string `json:"run"`
	N    int    `json:"n"`
	At   string `json:"at"`
	Role string `json:"role"`
	Text string `json:"text"`
}

func (d *Drainer) index(run string, ms []provider.Message) error {
	p := d.DrainedPath(run)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	base := 0
	for _, raw := range memory.ReadJSONL(p) {
		var x Drained
		if json.Unmarshal(raw, &x) == nil && x.N > base {
			base = x.N
		}
	}
	at := d.Now().UTC().Format(time.RFC3339)
	for i, m := range ms {
		line, err := json.Marshal(Drained{Run: run, N: base + i + 1, At: at, Role: string(m.Role), Text: render([]provider.Message{m})})
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// RecallDrained ranks a session's drained messages by meaning (BM25 over the same tokens the
// memory tiers use) and returns the best k.
func RecallDrained(path, q string, k int) []Drained {
	var all []Drained
	for _, raw := range memory.ReadJSONL(path) {
		var x Drained
		if json.Unmarshal(raw, &x) == nil {
			all = append(all, x)
		}
	}
	if len(all) == 0 || k <= 0 {
		return nil
	}
	qtf := memory.TermCounts(q)
	if len(qtf.Order) == 0 {
		return nil
	}
	tfs := make([]*memory.TF, len(all))
	docs := make([]memory.Doc, len(all))
	for i, x := range all {
		tfs[i] = memory.TermCounts(x.Text)
		docs[i] = termDoc{tfs[i]}
	}
	st := memory.StatsOf(docs)
	type hit struct {
		s float64
		d Drained
	}
	var hits []hit
	for i, x := range all {
		if s := memory.BM25(qtf, tfs[i].Map, tfs[i].DL, st.AvgDL, st.IDFQ); s > 0 {
			hits = append(hits, hit{s, x})
		}
	}
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].s > hits[j-1].s; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	if len(hits) > k {
		hits = hits[:k]
	}
	out := make([]Drained, len(hits))
	for i, h := range hits {
		out[i] = h.d
	}
	return out
}

func (d *Drainer) journal(s *loop.Session, t string, fields map[string]interface{}) {
	if !d.Opt.Journal || s.Journal == nil {
		return
	}
	e := loop.Event{"t": t, "id": s.RunID}
	for k, v := range fields {
		e[k] = v
	}
	s.Journal.Log(e)
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

type termDoc struct{ tf *memory.TF }

func (t termDoc) Terms() (map[string]int, int) { return t.tf.Map, t.tf.DL }

// RecallTool is `drained`: bring back a drained message of this session by question, from the
// short tier — nothing that left the context is lost.
func (d *Drainer) RecallTool() *tool.Tool {
	return &tool.Tool{
		Name:        "drained",
		Description: "Recall messages that were drained out of this session's context, by question (the short tier). Returns the best matches verbatim.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"question":{"type":"string"},"run":{"type":"string"},"k":{"type":"integer"}},"required":["question","run"]}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var a struct {
				Question, Run string
				K             int
			}
			if err := json.Unmarshal(in, &a); err != nil || a.Question == "" || a.Run == "" {
				return tool.Result{Output: "drained: question and run are required", Err: true}
			}
			if a.K <= 0 {
				a.K = 3
			}
			hits := RecallDrained(d.DrainedPath(a.Run), a.Question, a.K)
			if len(hits) == 0 {
				return tool.Result{Output: "no drained message of run " + a.Run + " matches " + strconv.Quote(a.Question)}
			}
			var b strings.Builder
			for _, h := range hits {
				fmt.Fprintf(&b, "--- drained #%d (%s, %s)\n%s\n", h.N, h.Role, h.At, strings.TrimSpace(h.Text))
			}
			return tool.Result{Output: b.String()}
		},
	}
}
