package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/config/yaml"
	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/tool"
	"github.com/Kaginari/agentic-playground/isekai/tui"
	"github.com/Kaginari/agentic-playground/isekai/wire"
	"github.com/Kaginari/agentic-playground/isekai/world"
)

// TUI runs the live session as the terminal UI (canon/tui.md): the same engine, sessions,
// Courts and gate as the REPL, drawn as blocks. It is chosen on a terminal; the REPL stays for
// pipes and --plain.
func (a *App) TUI(ctx context.Context) int {
	h, err := a.tuiHost(ctx)
	if err != nil {
		fmt.Fprintf(a.Opt.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	ui := tui.Start(ctx, h, tui.DetectTheme(a.Opt.Env, a.Cfg.Dist.EnvPrefix), h.words, nil, nil)
	// the mascot drops in unless <PREFIX>NO_INTRO says otherwise
	if v := a.Opt.Env(a.Cfg.Dist.EnvPrefix + "NO_INTRO"); v == "" || v == "0" || v == "false" {
		ui.Model.SetIntro(true)
	}
	h.attach(ui)
	return h.run(ui.Run)
}

// sender is where the host's events go: the program, or a test harness.
type sender interface{ Send(msg tea.Msg) }

// wantsTUI says whether the session should be the terminal UI: a terminal on both ends, not
// dumb, and neither --plain nor <PREFIX>PLAIN said otherwise.
func (a *App) wantsTUI(plain bool) bool {
	if plain || a.Opt.Plain {
		return false
	}
	if v := a.Opt.Env(a.Cfg.Dist.EnvPrefix + "PLAIN"); v != "" && v != "0" && v != "false" {
		return false
	}
	if a.Opt.Env("TERM") == "dumb" {
		return false
	}
	in, ok := a.Opt.In.(*os.File)
	if !ok || in != os.Stdin || !isTerminal(in) {
		return false
	}
	out, ok := a.Opt.Out.(*os.File)
	return ok && isTerminal(out)
}

// tuiHost is the app behind the program: it runs turns, routes the loop's events to blocks and
// answers the program's asks. in/out nil means the process terminal.
type tuiHost struct {
	a   *App
	ctx context.Context
	e   *loop.Engine
	s   *loop.Session
	ui  sender
	out *bytes.Buffer // what slash commands and the engine's writers print
	mu  sync.Mutex

	cancel     context.CancelFunc
	running    bool
	streamed   bool
	denyReason string
	quiet      bool              // notices held back (a reload the UI already announces)
	before     map[string]string // step id → file text before a write-class act
	courtAsk   map[string]string
	courtSeen  map[string]bool
	courtStart map[string]time.Time
	files      []string
	filesAt    time.Time
	words      tui.Words
	exit       int
}

func (a *App) tuiWords() tui.Words {
	w := tui.DefaultWords()
	w.Dist = a.Cfg.Dist.Name
	w.ConfigRel = filepath.ToSlash(filepath.Join(a.Cfg.Dist.WorldDir, "config.local.yaml"))
	w.SessionName = world.Rimuru
	return w
}

func (a *App) tuiHost(ctx context.Context) (*tuiHost, error) {
	// the engine writes no trace on the terminal the program owns; what it prints is drained
	// into notices
	a.Opt.Quiet = true
	buf := &bytes.Buffer{}
	h := &tuiHost{a: a, ctx: ctx, out: buf, before: map[string]string{}, courtAsk: map[string]string{}, courtSeen: map[string]bool{}, courtStart: map[string]time.Time{}, words: a.tuiWords()}
	out, errw, quiet := a.Opt.Out, a.Opt.Err, a.Opt.Quiet
	a.Opt.Out, a.Opt.Err = buf, buf
	a.Notify = func(s string) {
		h.mu.Lock()
		quiet := h.quiet
		h.mu.Unlock()
		if !quiet {
			h.send(tui.EvNotice{Text: s})
		}
	}
	// the gate's choice block, the ask tool's question
	a.Gate.Answer = h.approve
	a.Shelf.Asker = h.ask
	a.Missing.Ask = h.ask
	// the loop's events
	a.OnDelta = func(t string) {
		h.mu.Lock()
		h.streamed = true
		h.mu.Unlock()
		h.send(tui.EvDelta{Text: t})
	}
	a.OnStep = h.observe
	prevState := a.OnState
	a.OnState = func(body, state string) {
		if prevState != nil {
			prevState(body, state)
		}
		h.state(body, state)
	}
	a.Court.onFinish = h.courtFinished
	e, err := a.Engine()
	if err != nil {
		// no program will own the terminal: why it did not start is said there
		a.Opt.Out, a.Opt.Err, a.Opt.Quiet = out, errw, quiet
		return nil, err
	}
	h.e = e
	h.s = e.NewSession()
	a.Inbox = func(ls *loop.Session) []string { return a.Court.Inbox(bodyName(ls)) }
	a.Court.onWake = h.woke
	return h, nil
}

func (h *tuiHost) attach(ui sender) {
	h.mu.Lock()
	h.ui = ui
	h.mu.Unlock()
}

func (h *tuiHost) run(loop func() error) int {
	a := h.a
	a.Hooks.SessionStart(h.ctx)
	err := loop()
	h.mu.Lock()
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// the turn goroutine ends on its own; give it a moment to sync the session
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		running := h.running
		h.mu.Unlock()
		if !running || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil && err != context.Canceled {
		fmt.Fprintln(os.Stderr, "tui: "+err.Error())
		return 2
	}
	return h.exit
}

func (h *tuiHost) send(ev tea.Msg) {
	h.mu.Lock()
	ui := h.ui
	h.mu.Unlock()
	if ui != nil {
		ui.Send(ev)
	}
}

// --- tui.Host ---

func (h *tuiHost) Welcome() tui.Welcome {
	a := h.a
	w := tui.Welcome{Dist: a.Cfg.Dist.Name, Version: orStr(a.Opt.Version.Version, "dev"), World: tilde(a.Root, a.Opt.Home), WorldWord: h.words.World, Model: a.mountModel.Ref.Model, Session: a.SessionID}
	if p, age := a.latestHandoff(); p != "" && age < 14*24*time.Hour {
		w.Handoff = relOrAbs(a.Root, p) + " (" + age.Round(time.Minute).String() + " ago) — /handoff read"
	}
	var offices []string
	for _, o := range sortedKeys(a.Cfg.Models.Offices) {
		offices = append(offices, o+" → "+a.Cfg.Models.Offices[o].Model)
	}
	w.Offices = offices
	a.mu.Lock()
	if a.boardAddr != "" {
		w.Board = "http://" + a.boardAddr + "/"
	}
	a.mu.Unlock()
	w.Off = len(a.Cfg.Off())
	w.Holes = a.Holes()
	return w
}

func (h *tuiHost) Footer() tui.FooterView {
	a := h.a
	tot := a.Journal.Total()
	cost := "unpriced"
	switch {
	case tot.Calls == 0:
		cost = "no calls yet"
	case tot.Unpriced == 0:
		cost = fmt.Sprintf("$%.4f", tot.USD)
	}
	ctx, pct, known := "ctx —", 0, false
	if r := readingOf(h.s); r.Available {
		pct, known = r.Percent(), true
		ctx = fmt.Sprintf("ctx %d%%", r.Percent())
		if r.Stressed() {
			ctx += " (stress)"
		}
	}
	return tui.FooterView{Model: a.mountModel.Ref.Model, Ctx: ctx, Cost: cost, Live: a.Court.Live(), World: tilde(a.Root, a.Opt.Home), Courts: h.words.Courts, Tokens: tot.Tokens(), CtxPct: pct, CtxKnown: known}
}

func (h *tuiHost) Submit(text string) string {
	if why := h.a.Hooks.UserPrompt(h.ctx, text); why != "" {
		return "refused by a userPrompt hook: " + why
	}
	h.start(text, false)
	return ""
}

func (h *tuiHost) Queue(text string) string {
	_ = h.a.Court.Send(world.Rimuru, text)
	return "reaches the body at its next tool step"
}

func (h *tuiHost) Interrupt() {
	h.mu.Lock()
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (h *tuiHost) Slash(line string) ([]string, bool) {
	h.mu.Lock()
	busy := h.running
	h.mu.Unlock()
	quit := h.a.slash(h.ctx, line, h.s, busy)
	return h.drain(), quit
}

// drain takes what the engine printed on the app's writers since the last drain.
func (h *tuiHost) drain() []string {
	h.mu.Lock()
	s := h.out.String()
	h.out.Reset()
	h.mu.Unlock()
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

var builtinCommands = []tui.MenuItem{
	{Name: "help", Description: "commands and shortcuts"},
	{Name: "board", Description: "the board, full screen: agents, the ontology graph, offices, usage"},
	{Name: "agents", Description: "the live court: every body, its state, context and spend"},
	{Name: "send", Description: "/send <body> <text> — a line for a running body"},
	{Name: "usage", Description: "the usage journal of this session"},
	{Name: "status", Description: "the honesty rule and the instrument board"},
	{Name: "config", Description: "config [explain] — the switchboard and where each value came from"},
	{Name: "compact", Description: "drain the context now"},
	{Name: "review", Description: "/review [range] — two reviewers on two models, one merged shortlist; nothing fixed before you approve"},
	{Name: "handoff", Description: "/handoff [focus] writes one for a fresh session · /handoff read picks the last up"},
	{Name: "sessions", Description: "the sessions of this world"},
	{Name: "resume", Description: "/resume <id> — reopen a session"},
	{Name: "clear", Description: "clear the screen"},
	{Name: "quit", Description: "end the session"},
}

func (h *tuiHost) Commands() []tui.MenuItem {
	a := h.a
	items := append([]tui.MenuItem(nil), builtinCommands...)
	for _, c := range a.Found.Commands {
		d := c.Description
		if d == "" {
			d = firstLine(strings.TrimSpace(c.Template))
		}
		if c.Source != "" {
			d += " (" + c.Source + ")"
		}
		items = append(items, tui.MenuItem{Name: c.Name, Description: strings.TrimSpace(d)})
	}
	if a.MCP != nil {
		for _, p := range a.MCP.Prompts {
			items = append(items, tui.MenuItem{Name: p.Server + ":" + p.Name, Description: orStr(p.Description, "an MCP prompt")})
		}
	}
	return items
}

// Complete lists world paths starting with prefix (the listing is cached for a few seconds).
func (h *tuiHost) Complete(prefix string) []string {
	h.mu.Lock()
	if h.files == nil || time.Since(h.filesAt) > 5*time.Second {
		h.files = h.listFiles()
		h.filesAt = time.Now()
	}
	files := h.files
	h.mu.Unlock()
	var out []string
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			out = append(out, f)
			if len(out) >= 50 {
				break
			}
		}
	}
	if len(out) == 0 {
		lp := strings.ToLower(prefix)
		for _, f := range files {
			if strings.Contains(strings.ToLower(f), lp) {
				out = append(out, f)
				if len(out) >= 50 {
					break
				}
			}
		}
	}
	return out
}

func (h *tuiHost) listFiles() []string {
	root := h.a.Root
	var out []string
	n := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if p != root && (name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".cache")) {
				return filepath.SkipDir
			}
			return nil
		}
		n++
		if n > 20000 {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}

// --- turns ---

func (h *tuiHost) start(text string, auto bool) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		_ = h.a.Court.Send(world.Rimuru, text)
		return
	}
	tctx, cancel := context.WithCancel(h.ctx)
	h.cancel, h.running, h.streamed = cancel, true, false
	h.mu.Unlock()
	if auto {
		h.send(tui.EvTurnStart{Text: text, Auto: true})
	}
	go func() {
		r, err := h.s.Turn(tctx, text)
		if err != nil {
			r = &loop.Result{Status: loop.Fail, Holes: []string{err.Error()}}
		}
		h.finish(r, tctx.Err() != nil && h.ctx.Err() == nil)
	}()
}

func (h *tuiHost) finish(r *loop.Result, interrupted bool) {
	a := h.a
	h.mu.Lock()
	cancel := h.cancel
	streamed := h.streamed
	reason := h.denyReason
	h.denyReason = ""
	h.cancel, h.running = nil, false
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.Sessions.Sync(a.SessionID, h.s, a.mountModel.Ref.Model)
	if r.Verdict != "" {
		a.record(h.s, "verdict", map[string]interface{}{"verdict": r.Verdict, "wrote": h.s.Wrote})
	}
	ev := tui.EvTurnDone{Status: r.Status, Text: r.Text, Streamed: streamed, Verdict: r.Verdict, Interrupted: interrupted}
	if r.Verdict != "" {
		ev.LogRel = filepath.ToSlash(filepath.Join(a.Cfg.Dist.WorldDir, "log.md"))
	}
	for _, hole := range r.Holes {
		if interrupted && strings.Contains(hole, "context canceled") {
			continue
		}
		ev.Holes = append(ev.Holes, hole)
	}
	if r.Status == loop.Checkpoint {
		ev.Hint = fmt.Sprintf("the session hit a budget — a good point to stop; resume with: %s resume %s", a.Cfg.Dist.Name, a.SessionID)
	}
	if lines := h.drain(); len(lines) > 0 {
		h.send(tui.EvLines{Lines: lines})
	}
	h.send(ev)
	if h.ctx.Err() != nil {
		return
	}
	// a denial with a reason opens the next turn with the human's words (the law: a denial
	// ends the turn; the reason reaches the model as the human speaking)
	if r.Status == loop.Denied && reason != "" {
		h.send(tui.EvTurnStart{Text: reason})
		h.start(reason, false)
		return
	}
	pending := append(a.Court.Inbox(world.Rimuru), a.Court.TakeWake()...)
	if len(pending) > 0 {
		h.start(strings.Join(pending, "\n\n"), true)
	}
}

func (h *tuiHost) woke() {
	h.mu.Lock()
	running := h.running
	h.mu.Unlock()
	if running {
		for _, rep := range h.a.Court.TakeWake() {
			_ = h.a.Court.Send(world.Rimuru, rep)
		}
		return
	}
	if reports := h.a.Court.TakeWake(); len(reports) > 0 {
		h.start(strings.Join(reports, "\n\n"), true)
	}
}

// --- the loop's events ---

var diffTools = map[string]bool{"write": true, "edit": true, "multiedit": true, "patch": true, "str_replace_based_edit_tool": true}

func (h *tuiHost) observe(s *loop.Session, st *loop.StepRecord, phase string) {
	body := bodyName(s)
	if body != world.Rimuru {
		if phase == "start" {
			h.send(tui.EvState{Body: body, State: "tool", Tool: st.Tool})
		}
		return
	}
	switch phase {
	case "start":
		if st.Tool == "dispatch" {
			var in struct{ Body, Ask string }
			_ = json.Unmarshal(st.Input, &in)
			h.mu.Lock()
			h.courtAsk[strings.ToLower(in.Body)] = in.Ask
			h.mu.Unlock()
		}
		if diffTools[st.Tool] {
			if p := stepPath(st.Input); p != "" {
				h.mu.Lock()
				h.before[st.ID] = readCapped(filepath.Join(h.a.Root, p))
				h.mu.Unlock()
			}
		}
		if st.Tool == "dispatch" || st.Tool == "ask" {
			// a Court has its own block; a question is its own block
			return
		}
		h.send(tui.EvToolStart{Tool: h.toolView(st)})
	case "end":
		if st.Tool == "dispatch" && (st.Status == "done" || st.Status == "failed") {
			if rep, ok := wire.ParseReport(st.Result.Output); ok {
				var in struct{ Body string }
				_ = json.Unmarshal(st.Input, &in)
				h.courtReport(strings.ToLower(in.Body), rep, st.Result.Err, st.Result.Output)
				return
			}
		}
		if st.Tool == "ask" && st.Status == "done" {
			return
		}
		v := h.toolView(st)
		if diffTools[st.Tool] && st.Status == "done" {
			h.mu.Lock()
			before, had := h.before[st.ID]
			delete(h.before, st.ID)
			h.mu.Unlock()
			if p := stepPath(st.Input); p != "" && had {
				d := tui.DiffText(before, readCapped(filepath.Join(h.a.Root, p)), 2)
				d.Path = p
				v.Diff = &d
			}
		}
		h.send(tui.EvToolEnd{Tool: v})
	}
}

func (h *tuiHost) toolView(st *loop.StepRecord) tui.ToolView {
	v := tui.ToolView{ID: st.ID, Name: st.Tool, Summary: stepSummary(st.Input), Class: st.Effective, Status: st.Status, Ms: st.Ms, Output: st.Result.Output, Wrote: st.Wrote, Link: fileLink(h.a.Root, st.Input)}
	if st.Tool == "str_replace_based_edit_tool" {
		v.Name = "edit"
	}
	switch st.Status {
	case "denied":
		v.Why = st.Gate.Why
	case "refused", "would-ask":
		v.Why = st.Result.Output
	case "pending":
		v.Status, v.Why = "refused", st.Result.Output
	case "failed":
		if st.Tool == "" || st.Effective == "" {
			v.Class = ""
		}
	}
	return v
}

func (h *tuiHost) state(body, state string) {
	if body == world.Rimuru {
		h.send(tui.EvState{Body: body, State: state})
		return
	}
	h.mu.Lock()
	seen := h.courtSeen[body]
	if !seen {
		h.courtSeen[body] = true
		h.courtStart[body] = time.Now()
	}
	ask := h.courtAsk[body]
	h.mu.Unlock()
	if !seen {
		v := tui.CourtView{Name: body, State: state, Ask: ask, Word: h.words.Court}
		for _, b := range h.a.Court.Bodies() {
			if b.Name == body {
				v.Rank, v.Office, v.Model = b.Rank, b.Office, b.Model
			}
		}
		h.send(tui.EvCourt{Court: v})
		return
	}
	if state != "done" {
		h.send(tui.EvState{Body: body, State: state})
	}
}

// courtReport lands a Court's report as its block (foreground dispatch).
func (h *tuiHost) courtReport(body string, rep wire.Report, failed bool, raw string) {
	h.mu.Lock()
	started := h.courtStart[body]
	delete(h.courtSeen, body)
	delete(h.courtStart, body)
	h.mu.Unlock()
	v := tui.CourtView{Name: body, State: "done", Failed: failed, Word: h.words.Court, Report: reportView(rep)}
	if !started.IsZero() {
		v.Elapsed = time.Since(started)
	}
	for _, b := range h.a.Court.Bodies() {
		if b.Name == body {
			v.Rank, v.Office, v.Model = b.Rank, b.Office, b.Model
		}
	}
	h.send(tui.EvCourt{Court: v})
}

// courtFinished is a background Court's report landing.
func (h *tuiHost) courtFinished(name, report string) {
	if report == "" {
		return
	}
	rep, ok := wire.ParseReport(report)
	if !ok {
		rep = wire.Report{Status: "DONE", Other: strings.Split(strings.TrimSpace(report), "\n")}
	}
	h.courtReport(name, rep, strings.HasPrefix(rep.Status, "FAIL"), report)
}

func reportView(rep wire.Report) *tui.ReportView {
	v := &tui.ReportView{Status: rep.Status, Findings: rep.Findings, Verdicts: rep.Verdicts, Holes: rep.Holes, Other: rep.Other}
	for _, u := range rep.Unsaid {
		v.Unsaid = append(v.Unsaid, u.Kind+": "+u.Text)
	}
	return v
}

// --- the gate and the ask tool ---

func (h *tuiHost) approve(r gate.Request) (gate.Answer, bool) {
	a := h.a
	match := ruleMatch(r, tool.Env{Root: a.Root, WorldDir: a.Cfg.Dist.WorldDir})
	why := r.Why
	switch r.Class {
	case tool.Outward:
		why = "reaches beyond the " + h.words.World + " (" + r.Why + ") — nothing leaves without your word"
	case tool.Destructive:
		why = "irreversible (" + r.Why + ") — no destruction without your consent"
	}
	view := tui.ChoiceView{Title: "Approval", Class: r.Class.String(), Tool: r.Tool, Summary: r.Summary, Why: why,
		Options: []string{"Yes", "Yes, and don't ask again for " + match, "No, and tell the model why"},
		Notes:   []string{"", "writes a rule to " + h.words.ConfigRel, ""},
		Reasons: []string{"", "", "why not?"}}
	if r.Body != "" && r.Body != world.Rimuru {
		view.Body = r.Body
	}
	reply := make(chan tui.ChoiceAnswer, 1)
	h.send(tui.EvChoice{View: view, Reply: reply})
	var ans tui.ChoiceAnswer
	select {
	case ans = <-reply:
	case <-h.ctx.Done():
		return gate.Answer{Decision: gate.Denied, By: "tui", Why: "the session ended before an answer"}, true
	}
	switch {
	case ans.Aborted:
		return gate.Answer{Decision: gate.Denied, By: "tui", Why: "the session ended before an answer"}, true
	case ans.Index == 0:
		return gate.Answer{Decision: gate.Approved, By: "tty", Why: r.Why}, true
	case ans.Index == 1:
		where, err := h.addRule(match)
		if err != nil {
			h.send(tui.EvError{Text: "the rule was not written: " + err.Error()})
			return gate.Answer{Decision: gate.Approved, By: "tty", Why: r.Why}, true
		}
		h.send(tui.EvNotice{Text: "rule " + match + " → allow written to " + where})
		return gate.Answer{Decision: gate.Approved, By: "tty", Why: r.Why + "; rule " + match + " written"}, true
	}
	reason := strings.TrimSpace(ans.Text)
	h.mu.Lock()
	h.denyReason = reason
	h.mu.Unlock()
	if reason == "" {
		return gate.Answer{Decision: gate.Denied, By: "tty", Why: h.words.Human + " said no"}, true
	}
	return gate.Answer{Decision: gate.Denied, By: "tty", Why: h.words.Human + " said: " + reason}, true
}

// ruleMatch is the permission rule proposed for "don't ask again": the tool and a glob on
// the act's subject — the command's first two words for bash and git (`bash:git push*`), the
// host for a URL, the subject itself otherwise.
func ruleMatch(r gate.Request, env tool.Env) string {
	subject := subjectOf(r.Tool, r.Input, env)
	if subject == "" {
		subject = strings.TrimSpace(r.Summary)
	}
	switch r.Tool {
	case "bash", "git":
		words := strings.Fields(subject)
		if len(words) == 0 {
			return r.Tool + ":*"
		}
		n := 1
		if len(words) > 1 && !strings.HasPrefix(words[1], "-") {
			n = 2
		}
		return r.Tool + ":" + strings.Join(words[:n], " ") + "*"
	case "webfetch":
		if i := strings.Index(subject, "://"); i > 0 {
			rest := subject[i+3:]
			host, _, _ := strings.Cut(rest, "/")
			return r.Tool + ":" + subject[:i+3] + host + "/*"
		}
	}
	return r.Tool + ":" + subject
}

// addRule appends an allow rule to the local config layer and reloads the config live, so the
// next step already reads it. The local file is the human's machine-local word: gitignored,
// never the project's.
func (h *tuiHost) addRule(match string) (string, error) {
	a := h.a
	file := filepath.Join(a.Root, a.Cfg.Dist.WorldDir, "config.local.yaml")
	for _, l := range a.Cfg.Layers {
		if l.Name == "local" && l.Present && l.Path != "" {
			file = l.Path
		}
	}
	var items []string
	if b, err := os.ReadFile(file); err == nil {
		if n, err := yaml.Parse(b, file); err == nil && n != nil && n.Kind == yaml.Map {
			if p := n.Get("permissions"); p != nil {
				if rs := p.Get("rules"); rs != nil && rs.Kind == yaml.List {
					for _, it := range rs.Items {
						if it.Kind != yaml.Map {
							continue
						}
						m, act := it.Get("match"), it.Get("action")
						if m == nil && it.Get("tool") != nil {
							m = &yaml.Node{Kind: yaml.String, Str: it.Get("tool").Text() + ":" + it.Get("pattern").Text()}
						}
						if m == nil || act == nil {
							continue
						}
						if m.Text() == match {
							items = nil
							break
						}
						items = append(items, fmt.Sprintf("{match: %s, action: %s}", yamlQuote(m.Text()), act.Text()))
					}
				}
			}
		}
	}
	items = append(items, fmt.Sprintf("{match: %s, action: allow}", yamlQuote(match)))
	op := config.Op{Path: "permissions.rules", Value: "[" + strings.Join(items, ", ") + "]"}
	if err := a.Cfg.ApplyPatch(file, []config.Op{op}); err != nil {
		return "", err
	}
	next, changes, err := a.Cfg.Reload()
	if err != nil {
		return "", fmt.Errorf("written, but the reload failed: %w", err)
	}
	h.mu.Lock()
	h.quiet = true
	h.mu.Unlock()
	a.reload(next, changes)
	h.mu.Lock()
	h.quiet = false
	h.mu.Unlock()
	a.Missing.SetConfig(next)
	return relOrAbs(a.Root, file), nil
}

func yamlQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ask answers the ask tool (and a Genesis proposal) with a question block.
func (h *tuiHost) ask(ctx context.Context, q tool.Question) (string, error) {
	view := tui.ChoiceView{Title: "Question", Why: q.Text, Options: q.Options, Free: q.Free || len(q.Options) == 0}
	reply := make(chan tui.ChoiceAnswer, 1)
	h.send(tui.EvChoice{View: view, Reply: reply})
	select {
	case ans := <-reply:
		switch {
		case ans.Aborted:
			return "", fmt.Errorf("no answer: the session ended")
		case ans.Index >= 0 && ans.Index < len(q.Options):
			return q.Options[ans.Index], nil
		}
		return ans.Text, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-h.ctx.Done():
		return "", fmt.Errorf("no answer: the session ended")
	}
}

// --- helpers ---

func stepSummary(in json.RawMessage) string {
	var m map[string]interface{}
	if json.Unmarshal(in, &m) != nil {
		return oneLineText(string(in))
	}
	for _, k := range []string{"command", "path", "pattern", "url", "query", "body", "question", "name"} {
		if v, ok := m[k].(string); ok && v != "" {
			if k == "path" {
				if p, ok := m["pattern"].(string); ok {
					v = p + " in " + v
				}
			}
			if k == "body" {
				if ask, ok := m["ask"].(string); ok {
					v += " — " + ask
				}
			}
			return oneLineText(v)
		}
	}
	if args, ok := m["args"].([]interface{}); ok {
		var parts []string
		for _, a := range args {
			parts = append(parts, fmt.Sprint(a))
		}
		return oneLineText(strings.Join(parts, " "))
	}
	return oneLineText(string(in))
}

func stepPath(in json.RawMessage) string {
	var m struct{ Path string }
	_ = json.Unmarshal(in, &m)
	return strings.TrimSpace(m.Path)
}

func oneLineText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

const diffCap = 512 << 10

func readCapped(p string) string {
	b, err := os.ReadFile(p)
	if err != nil || len(b) > diffCap || bytes.IndexByte(b, 0) >= 0 {
		return ""
	}
	return string(b)
}

func tilde(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// fileLink is a file:// URL for a step whose input names a path inside the world; "" otherwise.
func fileLink(root string, raw json.RawMessage) string {
	var input struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(raw, &input)
	p := input.Path
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)
	if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
