// Package loop is the turn engine: a model turn is a plan; each tool call it asks for is one
// step worked to six beats — perceive → recall → plan → act → verify → record — journaled in
// the shape loop.js writes. Budgets are readings; past one the run checkpoints and stops
// honestly. Hooks let the workspace, memory, toolbox, onto and compaction packages wire in without
// this package importing them.
package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/instrument"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
)

// Statuses and exit codes, loop.js's.
const (
	Done       = "DONE"
	Dry        = "DRY"
	Fail       = "FAIL"
	Escalate   = "ESCALATE"
	Denied     = "DENIED"
	Checkpoint = "CHECKPOINT"
)

// Exit maps a status to loop.js's exit code.
func Exit(status string) int {
	switch status {
	case Fail:
		return 2
	case Escalate:
		return 3
	case Denied:
		return 4
	case Checkpoint:
		return 5
	}
	return 0
}

const (
	DefaultSteps   = 50
	DefaultMinutes = 30
	DefaultRetries = 2
	tailBytes      = 400
)

// Budget is every ceiling the engine reads before it acts. Zero takes the default; a negative
// value switches that budget off.
type Budget struct {
	Steps     int     // tool steps per turn
	Minutes   float64 // wall clock per session
	Retries   int     // consecutive failed acts before the run escalates
	MaxTokens int     // provider output ceiling per call (0: provider default)
	Context   instrument.Budget
}

func (b Budget) steps() int {
	if b.Steps == 0 {
		return DefaultSteps
	}
	return b.Steps
}
func (b Budget) minutes() float64 {
	if b.Minutes == 0 {
		return DefaultMinutes
	}
	return b.Minutes
}
func (b Budget) retries() int {
	if b.Retries == 0 {
		return DefaultRetries
	}
	if b.Retries < 0 {
		return 1 << 30
	}
	return b.Retries
}

// Recall is what the recall beat brings into a step: anchors and manifest lines, never agents.
type Recall struct {
	Anchors []string // path#section
	Tools   []string // @T lines
	Holes   []string
	Rebuilt bool
}

// StepRecord is one tool step as the journal and the report see it.
type StepRecord struct {
	ID        string              `json:"id"`
	N         int                 `json:"n"`
	Tool      string              `json:"tool"`
	Input     json.RawMessage     `json:"input"`
	Class     tool.Classification `json:"-"`
	Effective string              `json:"class"`
	Gate      gate.Answer         `json:"-"`
	Status    string              `json:"status"` // done | failed | denied | refused | would-ask | would-run
	Attempts  int                 `json:"attempts"`
	Ms        int64               `json:"ms"`
	Result    tool.Result         `json:"-"`
	Wrote     []string            `json:"wrote,omitempty"`
	Recall    *Recall             `json:"-"`
}

// Hooks are the seams other packages set. Every one is optional.
type Hooks struct {
	// System builds the system prompt for a provider call; nil uses the engine's default.
	System func(s *Session) string
	// Perceive overrides the context reading (memory status's method); nil reads the last
	// provider usage.
	Perceive func(s *Session) *instrument.Context
	// Recall runs before each provider call with the current ask; anchors, not payloads.
	Recall func(ctx context.Context, s *Session, ask string) Recall
	// Record runs after each done step (memory flows up); holes it returns are reported.
	Record func(ctx context.Context, s *Session, st *StepRecord) []string
	// EndGate runs when the model finishes a turn that wrote files: the Domain owner's gate. It returns
	// the verdict word for the report and holes; an error fails the turn.
	EndGate func(ctx context.Context, s *Session, r *Result) (verdict string, holes []string, err error)
	// GateRetries is how many times a failed end gate goes back to the model within the same
	// turn, its reasons as the message, before the turn fails (law.gate.retries).
	GateRetries int
	// Drain is the compaction seam: called when the context reading crosses the stress line,
	// before the engine checkpoints. Returning true means the context was drained and the run
	// may continue; false (or nil hook) means checkpoint.
	Drain func(ctx context.Context, s *Session, c instrument.Context) (bool, error)
	// Missing answers a call to a tool the registry does not hold (binary.md §When a tool is
	// missing): the error the model reads and whether the turn stops here (the doom-loop
	// guard). nil: "unknown tool" and never a stop.
	Missing func(ctx context.Context, s *Session, call provider.ToolCall) (content string, stop bool)
	// Decide is the permission rule before the gate (config.Decide): allow silences the gate
	// for this act (logged as pre-approved by rule), deny refuses it, ask forces the gate even
	// for a read or a write. An empty Action leaves the class default.
	Decide func(s *Session, st *StepRecord, cls tool.Classification) Decision
	// PreTool runs before an act; a non-empty string blocks it with that reason (a hook's
	// stderr). PostTool runs after a done act.
	PreTool  func(ctx context.Context, s *Session, st *StepRecord) string
	PostTool func(ctx context.Context, s *Session, st *StepRecord)
	// State reports the agent's state for the live subagent: thinking · tool · waiting on gate ·
	// done.
	State func(s *Session, state string)
	// Observe sees every tool step twice: "start" once its class is settled (before the gate)
	// and "end" once its record is final (done, failed, denied, refused, would-ask, would-run,
	// or a missing tool). The terminal UI draws its blocks from it.
	Observe func(s *Session, st *StepRecord, phase string)
	// Inbox hands lines typed by the human mid-turn; they ride the next tool-result message.
	Inbox func(s *Session) []string
	// Budget reads the session and subagent budgets before a model call; a non-empty reason
	// checkpoints the run honestly.
	Budget func(s *Session) string
}

// Decision is a permission rule's word on an act.
type Decision struct {
	Action string // allow | ask | deny | ""
	Why    string // the rule and its origin
}

// Lexicon holds every user-facing word a distribution may rename.
type Lexicon struct {
	Agent        string // default agent name: "ephemeral-subagent"
	Human        string // who the gate asks: "the human"
	WorkspaceDir string // ".agent-one"
	Policy       string // one line the default system prompt opens with
}

func (l Lexicon) agent() string {
	if l.Agent == "" {
		return "ephemeral-subagent"
	}
	return l.Agent
}
func (l Lexicon) human() string {
	if l.Human == "" {
		return "the human"
	}
	return l.Human
}
func (l Lexicon) workspaceDir() string {
	if l.WorkspaceDir == "" {
		return tool.DefaultWorkspaceDir
	}
	return l.WorkspaceDir
}

// Engine is one configured loop. Every field is a switch.
type Engine struct {
	Provider provider.Provider
	Tools    *tool.Registry
	Gate     *gate.Gate
	Root     string
	As       string
	Budget   Budget
	Hooks    Hooks
	Policy   tool.Policy
	Lexicon  Lexicon
	Journal  string    // directory; "" means <Root>/<WorkspaceDir>/instruments/loop; "-" disables
	Trace    io.Writer // step-by-step trace for the human; nil is quiet
	Unsaid   bool      // the commission asked +unsaid: a report with no @U is a hole
	Cap      int       // @CAP on the final report; 0 means no ceiling
	IsRecord func(rel string) bool
	Wire     bool              // the answer is expected on the wire (a Subagent): providers with guided decoding constrain it
	OnDelta  func(text string) // streamed text, as it arrives
}

// Session is a running conversation on an Engine: the messages, the readings, the journal.
type Session struct {
	Engine    *Engine
	RunID     string
	Journal   *Journal
	Messages  []provider.Message
	Started   time.Time
	Turns     int
	Steps     int
	Spend     provider.Usage
	Last      provider.Usage
	Context   instrument.Context // the session's own goroutine reads it directly; others use Reading
	ctxMu     sync.RWMutex       // guards Context against readers on other goroutines (status line, board)
	Wrote     []string           // the writes the gate has yet to see; cleared once a turn's gate ran
	Ask       string             // the first ask of the session (the commission)
	failures  int
	base      snapshot // the workspace tree as last stamped (snapshot.go)
	nowatch   bool     // the tree could not be stamped: shell writes go unseen, the hole named
	gated     bool     // the last turn's writes passed the end gate
	gateTries int      // failed gates sent back to the model this turn
}

// Result is one turn's outcome.
type Result struct {
	RunID   string             `json:"runId"`
	Status  string             `json:"@S"`
	Text    string             `json:"text"`
	Report  wire.Report        `json:"-"`
	IsWire  bool               `json:"wire"`
	Verdict string             `json:"verdict,omitempty"`
	Wrote   []string           `json:"wrote,omitempty"` // every write the end gate saw, whatever tool made it
	Steps   []*StepRecord      `json:"steps"`
	Holes   []string           `json:"@?"`
	Usage   provider.Usage     `json:"usage"`
	Context instrument.Context `json:"context"`
	Journal string             `json:"journal,omitempty"`
	Turns   int                `json:"turns"`
}

func (r *Result) hole(h string) {
	for _, x := range r.Holes {
		if x == h {
			return
		}
	}
	r.Holes = append(r.Holes, h)
}

// Exit is the process exit code for this result.
func (r *Result) Exit() int { return Exit(r.Status) }

func (e *Engine) as() string {
	if e.As != "" {
		return e.As
	}
	return e.Lexicon.agent()
}

func (e *Engine) env() tool.Env {
	return tool.Env{Root: e.Root, WorkspaceDir: e.Lexicon.workspaceDir(), Policy: e.Policy, IsRecord: e.IsRecord}
}

// JournalDir is where this engine journals; "" when journaling is off.
func (e *Engine) JournalDir() string {
	if e.Journal == "-" {
		return ""
	}
	if e.Journal != "" {
		return e.Journal
	}
	return filepath.Join(e.Root, e.Lexicon.workspaceDir(), "instruments", "loop")
}

func newRunID(as string) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, as)
	return fmt.Sprintf("%s-%s-%s", safe, time.Now().UTC().Format("20060102150405"), hex.EncodeToString(b))
}

// NewSession opens a session: a run id, a journal (unless off or dry), an empty conversation.
func (e *Engine) NewSession() *Session {
	s := &Session{Engine: e, RunID: newRunID(e.as()), Started: time.Now()}
	s.setContext(e.Budget.Context.Unread("no provider call yet"))
	if dir := e.JournalDir(); dir != "" && !(e.Gate != nil && e.Gate.DryRun) {
		s.Journal = &Journal{Path: filepath.Join(dir, s.RunID+".jsonl")}
	}
	return s
}

// Run is the one-shot form: a fresh session, one turn.
func (e *Engine) Run(ctx context.Context, ask string) (*Result, error) {
	return e.NewSession().Turn(ctx, ask)
}

func (s *Session) trace(format string, a ...interface{}) {
	if s.Engine.Trace != nil {
		fmt.Fprintf(s.Engine.Trace, format+"\n", a...)
	}
}

func tail(str string) string {
	if len(str) > tailBytes {
		return "…" + str[len(str)-tailBytes:]
	}
	return str
}

func (s *Session) perceive() instrument.Context {
	var hooked *instrument.Context
	if s.Engine.Hooks.Perceive != nil {
		hooked = s.Engine.Hooks.Perceive(s)
	}
	var c instrument.Context
	switch {
	case hooked != nil:
		c = *hooked
	case s.Turns == 0:
		c = s.Engine.Budget.Context.Unread("no provider call yet")
	default:
		c = s.Engine.Budget.Context.Reading(s.Last)
	}
	s.setContext(c)
	return c
}

func (s *Session) setContext(c instrument.Context) {
	s.ctxMu.Lock()
	s.Context = c
	s.ctxMu.Unlock()
}

// Reading is the session's context occupancy, safe from any goroutine.
func (s *Session) Reading() instrument.Context {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()
	return s.Context
}

func (s *Session) overWallClock() (bool, string) {
	m := s.Engine.Budget.minutes()
	if m < 0 {
		return false, ""
	}
	if el := time.Since(s.Started); el >= time.Duration(m*float64(time.Minute)) {
		return true, fmt.Sprintf("wall-clock budget reached (%g min)", m)
	}
	return false, ""
}

// DefaultSystem is the system prompt when no hook builds one: who the agent is, where it
// stands, the four classes and the gate, the wire it answers in.
func (e *Engine) DefaultSystem(s *Session) string {
	var b strings.Builder
	if e.Lexicon.Policy != "" {
		b.WriteString(e.Lexicon.Policy + "\n\n")
	}
	fmt.Fprintf(&b, "You are %s, an agent working inside the workspace rooted at %s. Ownership is inward: every path is relative to that root.\n", e.as(), e.Root)
	b.WriteString("Every tool call is one step with a class — read, write (inside the workspace), outward (beyond it), destructive (irreversible). Outward and destructive steps are put to " + e.Lexicon.human() + " at a gate before they run; a denial ends the turn. Declare `class` on bash when you know a command reaches further than it looks; a declaration can only tighten.\n")
	b.WriteString("Budgets are readings: steps, wall clock and the context window are checked before every step; past one, the run checkpoints and stops.\n")
	if e.Wire {
		b.WriteString("Answer on the wire: `@S <status>` opens, one `@F <file:line> <fact>` per finding, `@? <hole>` for anything you could not settle (never guess), `@U <policy|team|domain> <text>` for each thing you know that nothing on disk says, `@E <bytes>` closes. No greetings, nothing long — a long thing lives on disk and crosses as a path.\n")
	} else {
		b.WriteString("You speak to " + e.Lexicon.human() + " directly: answer in plain language, in full sentences — what you did, what you found, and anything you could not settle. The wire (`@S … @E`) is for the reports of bodies you dispatch, never for the human.\n")
	}
	if e.Tools != nil {
		fmt.Fprintf(&b, "Tools: %s.\n", strings.Join(e.Tools.Names(), ", "))
	}
	return b.String()
}

func (s *Session) system() string {
	if s.Engine.Hooks.System != nil {
		return s.Engine.Hooks.System(s)
	}
	return s.Engine.DefaultSystem(s)
}

func (s *Session) defs() []provider.ToolDef {
	if s.Engine.Tools == nil {
		return nil
	}
	var out []provider.ToolDef
	for _, d := range s.Engine.Tools.Defs() {
		out = append(out, provider.ToolDef{Name: d.Name, Description: d.Description, Schema: d.Schema, Declare: d.Declare})
	}
	return out
}

// Turn runs one ask to its end: the model plans, each tool call is a step, the model answers.
func (s *Session) Turn(ctx context.Context, ask string) (*Result, error) {
	if strings.TrimSpace(ask) == "" {
		return nil, fmt.Errorf("loop: empty ask")
	}
	s.open(ask)
	if s.gated {
		// the last turn's writes met the gate; this turn gates only its own
		s.Wrote, s.gated = nil, false
	}
	if n := len(s.Messages); n > 0 && s.Messages[n-1].Role == provider.User {
		// the last turn stopped on its tool results (denied, escalated, checkpointed): the ask
		// rides that message — two user messages in a row is a shape no provider takes
		s.Messages[n-1].Text = strings.TrimSpace(s.Messages[n-1].Text + "\n" + ask)
	} else {
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: ask})
	}
	return s.drive(ctx)
}

func (s *Session) open(ask string) {
	e := s.Engine
	if s.Ask != "" {
		return
	}
	s.Ask = ask
	g := s.gate()
	s.Journal.Log(Event{"t": "run", "id": s.RunID, "as": e.as(), "ask": ask, "provider": e.Provider.Name(), "root": e.Root,
		"budget":  map[string]interface{}{"steps": e.Budget.steps(), "minutes": e.Budget.minutes(), "retries": e.Budget.retries(), "context": e.Budget.Context.Unread("").Limit, "stress": e.Budget.Context.Unread("").Stress},
		"approve": approveList(g), "strict": g.Strict, "dryRun": g.DryRun, "tools": e.toolNames()})
}

func (s *Session) gate() *gate.Gate {
	if s.Engine.Gate == nil {
		s.Engine.Gate = gate.New()
	}
	return s.Engine.Gate
}

// drive runs the conversation from its current state (ending on a user message) to the end
// of the turn.
func (s *Session) drive(ctx context.Context) (*Result, error) {
	e := s.Engine
	if e.Provider == nil {
		return nil, fmt.Errorf("loop: no provider")
	}
	g := s.gate()
	r := &Result{RunID: s.RunID, Journal: journalRel(s), Holes: []string{}, Steps: []*StepRecord{}}
	s.watch(r) // the turn's baseline: every write from here on is seen, whatever tool makes it
	stepsThisTurn := 0
	s.gateTries = 0
	end := func(status string, why string) (*Result, error) {
		r.Status = status
		if why != "" {
			r.hole(why)
		}
		r.Usage, r.Context, r.Turns = s.Spend, s.Context, s.Turns
		if status != Done && status != Dry {
			s.saveTranscript()
		}
		s.Journal.Log(Event{"t": "end", "status": status, "turns": s.Turns, "steps": s.Steps, "ms": time.Since(s.Started).Milliseconds(), "holes": r.Holes})
		s.state("done")
		return r, nil
	}
	resumeHint := func() string { return "resume: agent-one resume " + s.RunID }

	pauses := 0
	for {
		// PERCEIVE (the turn): budgets before the provider call
		if over, why := s.overWallClock(); over {
			return end(Checkpoint, why+" before the next model call — "+resumeHint())
		}
		if e.Hooks.Budget != nil {
			if why := e.Hooks.Budget(s); why != "" {
				s.Journal.Log(Event{"t": "budget", "why": why, "before": "turn"})
				return end(Checkpoint, why+" — "+resumeHint())
			}
		}
		c := s.perceive()
		if c.Stressed() {
			s.Journal.Log(Event{"t": "budget", "context": c, "before": "turn"})
			drained := false
			if e.Hooks.Drain != nil {
				ok, err := e.Hooks.Drain(ctx, s, c)
				if err != nil {
					r.hole("drain failed: " + err.Error())
				}
				drained = ok
				s.Journal.Log(Event{"t": "drain", "ok": ok, "before": c.Tokens, "after": s.perceive().Tokens})
			}
			if !drained {
				return end(Checkpoint, fmt.Sprintf("context in the stress zone (%d ≥ %d tok) — write what is not yet durable, then resume in a fresh session: %s", c.Tokens, c.Stress, resumeHint()))
			}
		}
		// RECALL (the turn): anchors for the current ask, never payloads
		var rc Recall
		if e.Hooks.Recall != nil {
			rc = e.Hooks.Recall(ctx, s, currentAsk(s))
			s.Journal.Log(Event{"t": "recall", "id": fmt.Sprintf("t%d", s.Turns+1), "q": currentAsk(s), "rebuilt": rc.Rebuilt, "hits": rc.Anchors, "tools": rc.Tools, "holes": rc.Holes})
			for _, h := range rc.Holes {
				r.hole("recall: " + h)
			}
		}
		req := provider.Request{System: s.system(), Messages: s.Messages, Tools: s.defs(), MaxTokens: e.Budget.MaxTokens, Wire: e.Wire, OnDelta: e.OnDelta}
		if len(rc.Anchors)+len(rc.Tools) > 0 {
			req.SystemTail = recallBlock(rc)
		}
		// PLAN: the model's turn
		s.state("thinking")
		resp, err := e.Provider.Complete(ctx, req)
		s.Turns++
		if err != nil {
			s.Journal.Log(Event{"t": "turn", "n": s.Turns, "error": err.Error()})
			return end(Fail, "provider: "+err.Error())
		}
		s.Last = resp.Usage
		s.Spend = s.Spend.Add(resp.Usage)
		s.perceive() // the reading the loop acts on (a Perceive hook may scale the stress line) is the one reported
		s.Journal.Log(Event{"t": "turn", "n": s.Turns, "model": resp.Model, "stop": string(resp.Stop), "usage": resp.Usage, "context": s.Context, "calls": len(resp.Message.ToolCalls)})
		s.Messages = append(s.Messages, resp.Message)
		if resp.Message.Text != "" && len(resp.Message.ToolCalls) > 0 {
			s.trace("  %s", strings.TrimSpace(resp.Message.Text))
		}
		if len(resp.Message.ToolCalls) == 0 {
			if resp.Stop == provider.StopPause && pauses < 3 {
				// the server paused a long turn: resend to continue, a bounded number of times
				pauses++
				s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: "continue"})
				continue
			}
			res, err := s.finish(ctx, r, resp, end)
			if err == errGateRetry {
				continue // the gate's reasons are the model's next message
			}
			return res, err
		}
		// each tool call is a step
		var results []provider.ToolResult
		stop, stopWhy := "", ""
		for _, call := range resp.Message.ToolCalls {
			if stop != "" {
				results = append(results, provider.ToolResult{ID: call.ID, Content: "not run: the turn stopped (" + stop + ")", IsError: true})
				continue
			}
			st, res, status, why := s.step(ctx, g, call, r, stepsThisTurn)
			results = append(results, res)
			r.Steps = append(r.Steps, st)
			if st.Status == "done" || st.Status == "failed" || st.Status == "refused" {
				stepsThisTurn++
			}
			if status != "" {
				stop, stopWhy = status, why
			}
		}
		next := provider.Message{Role: provider.User, ToolResults: results}
		if e.Hooks.Inbox != nil {
			if lines := e.Hooks.Inbox(s); len(lines) > 0 {
				next.Text = strings.Join(lines, "\n")
				s.Journal.Log(Event{"t": "inbox", "n": len(lines)})
			}
		}
		s.Messages = append(s.Messages, next)
		if stop != "" {
			return end(stop, stopWhy)
		}
	}
}

func (s *Session) state(st string) {
	if s.Engine.Hooks.State != nil {
		s.Engine.Hooks.State(s, st)
	}
}

func (s *Session) observe(st *StepRecord, phase string) {
	if s.Engine.Hooks.Observe != nil {
		s.Engine.Hooks.Observe(s, st, phase)
	}
}

// step works one tool call through the six beats. It returns the record, the result for the
// model, and a stop status when the turn must end here.
func (s *Session) step(ctx context.Context, g *gate.Gate, call provider.ToolCall, r *Result, stepsThisTurn int) (*StepRecord, provider.ToolResult, string, string) {
	e := s.Engine
	s.Steps++
	st := &StepRecord{ID: fmt.Sprintf("s%d", s.Steps), N: s.Steps, Tool: call.Name, Input: call.Input, Status: "pending"}
	deny := func(msg string) provider.ToolResult {
		st.Result = tool.Result{Output: msg, Err: true} // the record keeps what the model read
		s.observe(st, "end")
		return provider.ToolResult{ID: call.ID, Content: msg, IsError: true}
	}
	// PERCEIVE: budgets before the step
	if max := e.Budget.steps(); max > 0 && stepsThisTurn >= max {
		st.Status = "pending"
		return st, deny("not run: step budget reached"), Checkpoint, fmt.Sprintf("step budget reached (%d) before %s — resume: agent-one resume %s", max, st.ID, s.RunID)
	}
	if over, why := s.overWallClock(); over {
		return st, deny("not run: " + why), Checkpoint, why + " before " + st.ID
	}
	s.Journal.Log(Event{"t": "perceive", "id": st.ID, "steps": stepsThisTurn, "ms": time.Since(s.Started).Milliseconds(), "context": s.Context})
	t, ok := e.Tools.Get(call.Name)
	if !ok {
		st.Status = "failed"
		msg, stop := fmt.Sprintf("unknown tool %q", call.Name), false
		if call.Name == "_malformed" {
			var f struct{ Fault, Raw string }
			_ = json.Unmarshal(call.Input, &f)
			msg = "malformed tool call: " + f.Fault + " — write one <tool_call>{\"name\", \"input\"}</tool_call> per call"
		} else if e.Hooks.Missing != nil {
			msg, stop = e.Hooks.Missing(ctx, s, call)
		}
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "failed", "attempts": 0, "why": "missing tool", "msg": msg})
		s.trace("✗ %s %s missing: %s", st.ID, call.Name, lastLine(msg))
		if stop {
			return st, deny(msg), Escalate, msg
		}
		return st, deny(msg), "", ""
	}
	// PLAN: the class, then the gate
	env := e.env()
	cls, holes := t.Settle(env, call.Input)
	st.Class, st.Effective = cls, cls.Class.String()
	for _, h := range holes {
		r.hole(st.ID + ": " + h)
	}
	s.observe(st, "start")
	if env.Policy != nil {
		if err := env.Policy(tool.Access{Tool: t.Name, Class: cls.Class, Paths: cls.Paths, Input: call.Input}); err != nil {
			st.Status = "refused"
			s.Journal.Log(Event{"t": "gate", "id": st.ID, "tool": t.Name, "effective": st.Effective, "why": cls.Why, "needed": true, "decision": "refused", "by": "policy"})
			s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "refused", "attempts": 0})
			s.trace("✗ %s %s [%s] refused: %v", st.ID, t.Name, st.Effective, err)
			return st, deny("refused: " + err.Error()), "", ""
		}
	}
	dry := g.DryRun
	greq := gate.Request{ID: st.ID, Tool: t.Name, Class: cls.Class, Why: cls.Why, Summary: summary(call), Agent: e.as(), Input: call.Input}
	var ans gate.Answer
	rule := Decision{}
	if e.Hooks.Decide != nil {
		rule = e.Hooks.Decide(s, st, cls)
	}
	switch rule.Action {
	case "deny":
		st.Status = "refused"
		s.Journal.Log(Event{"t": "gate", "id": st.ID, "tool": t.Name, "effective": st.Effective, "why": rule.Why, "needed": true, "decision": "refused", "by": "rule"})
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "refused", "attempts": 0})
		s.trace("✗ %s %s [%s] denied by rule (%s)", st.ID, t.Name, st.Effective, rule.Why)
		return st, deny("denied by permission rule: " + rule.Why), "", ""
	case "allow":
		ans = gate.Answer{Needed: g.Needs(cls.Class), Decision: gate.Approved, By: "rule", Why: rule.Why}
		if !ans.Needed {
			ans.Decision = gate.NotNeeded
		}
		if dry && cls.Class > tool.Read {
			ans.Decision, ans.By = gate.WouldAsk, "dry-run"
		}
	case "ask":
		greq.Force = true
		fallthrough
	default:
		if g.Needs(cls.Class) || greq.Force {
			s.state("waiting on gate")
		}
		ans = g.Ask(greq)
	}
	st.Gate = ans
	s.Journal.Log(Event{"t": "gate", "id": st.ID, "tool": t.Name, "heuristic": st.Effective, "effective": st.Effective, "why": cls.Why, "needed": ans.Needed, "decision": string(ans.Decision), "by": ans.By, "rule": rule.Why})
	switch ans.Decision {
	case gate.Denied:
		st.Status = "denied"
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "denied", "attempts": 0})
		s.trace("✗ %s %s [%s] denied at the gate (%s)", st.ID, t.Name, st.Effective, ans.Why)
		return st, deny("denied at the gate: " + ans.Why), Denied, fmt.Sprintf("%s denied at the gate (%s: %s) — %s decides", st.ID, st.Effective, ans.Why, e.Lexicon.human())
	case gate.WouldAsk:
		st.Status = "would-ask"
		s.trace("? %s %s [%s] would ask (%s)", st.ID, t.Name, st.Effective, cls.Why)
		return st, deny(fmt.Sprintf("not run (dry run): would ask %s — %s: %s", e.Lexicon.human(), st.Effective, cls.Why)), "", ""
	}
	if dry && cls.Class > tool.Read {
		st.Status = "would-run"
		s.trace("· %s %s [%s] would run", st.ID, t.Name, st.Effective)
		return st, deny(fmt.Sprintf("not run (dry run): would run — %s: %s", st.Effective, cls.Why)), "", ""
	}
	// ACT
	if e.Hooks.PreTool != nil {
		if why := e.Hooks.PreTool(ctx, s, st); why != "" {
			st.Status = "refused"
			s.Journal.Log(Event{"t": "hook", "id": st.ID, "kind": "preTool", "blocked": true, "why": why})
			s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "refused", "attempts": 0})
			s.trace("✗ %s %s [%s] blocked by a preTool hook: %s", st.ID, t.Name, st.Effective, lastLine(why))
			return st, deny("blocked by a preTool hook: " + why), "", ""
		}
	}
	s.trace("→ %s %s [%s] %s", st.ID, t.Name, st.Effective, summary(call))
	st.Attempts = 1
	s.Journal.Log(Event{"t": "act", "id": st.ID, "attempt": 1, "start": true, "tool": t.Name, "summary": summary(call)})
	s.state("tool")
	t0 := time.Now()
	res := t.Run(ctx, env, call.Input)
	st.Ms = time.Since(t0).Milliseconds()
	st.Result = res
	if e.Hooks.PostTool != nil {
		e.Hooks.PostTool(ctx, s, st)
	}
	s.Journal.Log(Event{"t": "act", "id": st.ID, "attempt": 1, "ok": !res.Err, "ms": st.Ms, "out": tail(res.Output), "wrote": res.Wrote})
	// VERIFY: the tool's own reading — an error result is a failed act
	s.Journal.Log(Event{"t": "verify", "id": st.ID, "attempt": 1, "code": map[bool]int{false: 0, true: 1}[res.Err]})
	for _, w := range res.Wrote {
		st.Wrote = append(st.Wrote, env.Rel(w))
		s.Wrote = appendUnique(s.Wrote, env.Rel(w))
	}
	if cls.Class >= tool.Write {
		// what the act changed on disk, whether or not the tool named it (the shell never does)
		for _, w := range s.seen() {
			st.Wrote = appendUnique(st.Wrote, w)
			s.Wrote = appendUnique(s.Wrote, w)
		}
	}
	// RECORD
	if res.Err {
		st.Status = "failed"
		s.failures++
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "failed", "attempts": 1, "ms": st.Ms, "last": tail(res.Output)})
		s.trace("  failed (%dms): %s", st.Ms, lastLine(res.Output))
		s.observe(st, "end")
		if s.failures > e.Budget.retries() {
			return st, provider.ToolResult{ID: call.ID, Content: res.Output, IsError: true}, Escalate,
				fmt.Sprintf("%d consecutive failed acts, last %s (%s) — one hop up: the dispatcher decides; journal %s", s.failures, st.ID, lastLine(res.Output), journalRel(s))
		}
		return st, provider.ToolResult{ID: call.ID, Content: res.Output, IsError: true}, "", ""
	}
	s.failures = 0
	st.Status = "done"
	if e.Hooks.Record != nil {
		for _, h := range e.Hooks.Record(ctx, s, st) {
			r.hole(st.ID + " record: " + h)
		}
	}
	s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "done", "attempts": 1, "ms": st.Ms, "wrote": st.Wrote})
	s.trace("  ok (%dms)%s", st.Ms, map[bool]string{true: " wrote " + strings.Join(st.Wrote, " "), false: ""}[len(st.Wrote) > 0])
	s.observe(st, "end")
	out := res.Output
	if out == "" {
		out = "(no output)"
	}
	return st, provider.ToolResult{ID: call.ID, Content: out}, "", ""
}

// errGateRetry tells drive that finish sent a failed gate back to the model.
var errGateRetry = errors.New("gate retry")

func (s *Session) finish(ctx context.Context, r *Result, resp provider.Response, end func(string, string) (*Result, error)) (*Result, error) {
	e := s.Engine
	r.Text = resp.Message.Text
	if resp.Stop == provider.StopMaxTokens {
		r.hole("the model's answer was cut by the output ceiling (max_tokens)")
	}
	if resp.Stop == provider.StopRefusal {
		r.hole("the provider refused the request")
	}
	rep, isWire := wire.ParseReport(r.Text)
	r.Report, r.IsWire = rep, isWire
	if isWire && e.Unsaid && len(rep.Unsaid) == 0 {
		r.hole("report carries no @U line — nothing unsaid, or the duty failed (the dispatcher may ask)")
	}
	if !isWire && e.Unsaid {
		r.hole("answer is not on the wire (no @S)")
	}
	// a write a read-classified command slipped in (`xargs touch`) is still a write
	if late := s.seen(); len(late) > 0 {
		s.Journal.Log(Event{"t": "watch", "id": "turn", "wrote": late})
		for _, w := range late {
			s.Wrote = appendUnique(s.Wrote, w)
		}
	}
	if e.Hooks.EndGate != nil && len(s.Wrote) > 0 {
		before := len(r.Holes)
		verdict, holes, err := e.Hooks.EndGate(ctx, s, r)
		r.Verdict = verdict
		r.Wrote = append([]string(nil), s.Wrote...) // what the gate saw (the hook may have narrowed it)
		s.gated = true
		for _, h := range holes {
			r.hole(h)
		}
		s.Journal.Log(Event{"t": "gate", "id": "turn", "kind": "end", "verdict": verdict, "wrote": s.Wrote, "holes": holes, "error": errString(err)})
		if err != nil && s.gateTries < e.Hooks.GateRetries {
			// the gate's reasons go back to the model once more: it fixes what they name, and
			// the whole turn's writes are gated again
			s.gateTries++
			r.Holes = append(r.Holes[:before], fmt.Sprintf("gate failed and was sent back (%d of %d): %s", s.gateTries, e.Hooks.GateRetries, verdict))
			s.Journal.Log(Event{"t": "gate", "id": "turn", "kind": "retry", "try": s.gateTries, "verdict": verdict})
			s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: fmt.Sprintf(
				"The gate failed this turn's writes (%d of %d sent back):\n%s\nFix what it names in the same turn — the writes stand as they are — then answer again.", s.gateTries, e.Hooks.GateRetries, verdict)})
			return nil, errGateRetry
		}
		if err != nil {
			return end(Fail, "end-of-turn gate: "+err.Error())
		}
	} else {
		r.Wrote = append([]string(nil), s.Wrote...)
	}
	if e.Gate != nil && e.Gate.DryRun {
		return end(Dry, "")
	}
	return end(Done, "")
}

// Emit renders the turn's answer for the human or the dispatcher: the model's report under
// @CAP when it spoke on the wire, else its text; the engine's own holes are appended.
func (r *Result) Emit(cap int) string {
	if !r.IsWire {
		parts := []string{}
		if t := strings.TrimSpace(r.Text); t != "" {
			parts = append(parts, t)
		}
		parts = append(parts, prefix("@? ", r.Holes)...)
		return strings.Join(parts, "\n")
	}
	rep := r.Report
	if r.Status != Done && r.Status != Dry {
		rep.Status = r.Status + " — " + rep.Status
	}
	for _, h := range r.Holes {
		rep.Holes = append(rep.Holes, h)
	}
	return rep.Emit(cap, "")
}

func (s *Session) saveTranscript() {
	if s.Journal == nil {
		return
	}
	p := strings.TrimSuffix(s.Journal.Path, ".jsonl") + ".transcript.json"
	b, err := json.Marshal(map[string]interface{}{"runId": s.RunID, "ask": s.Ask, "messages": s.Messages, "wrote": s.Wrote, "steps": s.Steps, "spend": s.Spend})
	if err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// Resume reopens a checkpointed, denied or escalated run from its transcript. Budgets restart
// with this process; a tool call cut mid-flight is answered "interrupted", never replayed —
// the model re-issues it and the gate asks again.
func (e *Engine) Resume(ctx context.Context, runID string, ask string) (*Result, error) {
	dir := e.JournalDir()
	if dir == "" {
		return nil, fmt.Errorf("loop: journaling is off; nothing to resume from")
	}
	p := filepath.Join(dir, runID+".transcript.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("loop: no transcript for %s (%v) — only CHECKPOINT, DENIED, ESCALATE and FAIL runs keep one", runID, err)
	}
	var t struct {
		Ask      string             `json:"ask"`
		Messages []provider.Message `json:"messages"`
		Wrote    []string           `json:"wrote"`
		Steps    int                `json:"steps"`
		Spend    provider.Usage     `json:"spend"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	s := e.NewSession()
	s.RunID = runID
	if s.Journal != nil {
		s.Journal.Path = filepath.Join(dir, runID+".jsonl")
	}
	s.Messages, s.Wrote, s.Steps, s.Spend, s.Ask = t.Messages, t.Wrote, t.Steps, t.Spend, t.Ask
	s.Journal.Log(Event{"t": "resume", "id": runID, "messages": len(s.Messages), "ask": ask})
	if ask == "" {
		ask = "resumed — continue from where the run stopped"
	}
	n := len(s.Messages)
	switch {
	case n > 0 && s.Messages[n-1].Role == provider.Assistant && len(s.Messages[n-1].ToolCalls) > 0:
		var rs []provider.ToolResult
		for _, c := range s.Messages[n-1].ToolCalls {
			rs = append(rs, provider.ToolResult{ID: c.ID, Content: "interrupted before this ran — re-issue it if still needed", IsError: true})
		}
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, ToolResults: rs, Text: ask})
	case n > 0 && s.Messages[n-1].Role == provider.User:
		s.Messages[n-1].Text = strings.TrimSpace(s.Messages[n-1].Text + "\n" + ask)
	default:
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: ask})
	}
	return s.drive(ctx)
}

func journalRel(s *Session) string {
	if s.Journal == nil {
		return ""
	}
	if rel, err := filepath.Rel(s.Engine.Root, s.Journal.Path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return s.Journal.Path
}

func currentAsk(s *Session) string {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == provider.User && s.Messages[i].Text != "" {
			return s.Messages[i].Text
		}
	}
	return s.Ask
}

func recallBlock(rc Recall) string {
	var b strings.Builder
	if len(rc.Anchors) > 0 {
		b.WriteString("@RECALL " + strings.Join(rc.Anchors, " ") + "\n")
	}
	if len(rc.Tools) > 0 {
		b.WriteString("@TOOLS\n" + strings.Join(rc.Tools, "\n") + "\n")
	}
	return b.String()
}

func summary(c provider.ToolCall) string {
	var m map[string]interface{}
	if json.Unmarshal(c.Input, &m) != nil {
		return string(c.Input)
	}
	for _, k := range []string{"command", "path", "pattern"} {
		if v, ok := m[k].(string); ok {
			if k == "path" {
				if p, ok := m["pattern"].(string); ok {
					v = p + " in " + v
				}
			}
			return oneLine(v)
		}
	}
	return oneLine(string(c.Input))
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	l := lines[len(lines)-1]
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" && !strings.HasPrefix(t, "[") {
			l = t
			break
		}
	}
	if len(l) > 120 {
		l = l[:120] + "…"
	}
	return l
}

func appendUnique(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func prefix(p string, xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = p + x
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func approveList(g *gate.Gate) []string {
	var out []string
	for c, ok := range g.Approve {
		if ok {
			out = append(out, c.String())
		}
	}
	return out
}

func (e *Engine) toolNames() []string {
	if e.Tools == nil {
		return nil
	}
	return e.Tools.Names()
}
