package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/wire"
	"github.com/Kaginari/agentic-playground/isekai/world"
)

// REPL is the live session (binary.md §Live session): a turn runs in the background; a line
// typed mid-turn is queued and reaches the body at its next tool step; Courts may run in the
// background and wake the session with their reports; slash commands read the instruments.
func (a *App) REPL(ctx context.Context) int {
	e, err := a.Engine()
	if err != nil {
		fmt.Fprintf(a.Opt.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	s := e.NewSession()
	out, errw := a.Opt.Out, a.Opt.Err
	a.Hooks.SessionStart(ctx)
	fmt.Fprintf(errw, "%s · %s · %s · session %s · /help\n", a.Cfg.Dist.Name, a.mountModel.Ref.Model, a.Root, a.SessionID)
	if p := a.OffProse(); p != "" {
		fmt.Fprintln(errw, p)
	}
	for _, h := range a.Holes() {
		fmt.Fprintln(errw, "@? "+h)
	}
	streamed := 0
	var outMu sync.Mutex
	if a.Cfg.Output.Stream && a.Cfg.Output.Format != "json" {
		a.OnDelta = func(t string) {
			outMu.Lock()
			fmt.Fprint(out, t)
			streamed += len(t)
			outMu.Unlock()
		}
		e.OnDelta = a.OnDelta
	}
	a.Inbox = func(ls *loop.Session) []string { return a.Court.Inbox(bodyName(ls)) }
	if a.Cfg.UI.AnnounceCourts {
		a.announce(errw)
	}
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(a.Opt.In)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	wake := make(chan struct{}, 1)
	a.Court.onWake = func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	var sig chan os.Signal
	if f, ok := a.Opt.In.(*os.File); ok && f == os.Stdin {
		sig = make(chan os.Signal, 2)
		signal.Notify(sig, os.Interrupt)
		defer signal.Stop(sig)
	}
	var turnDone chan *loop.Result
	var cancelTurn context.CancelFunc
	interrupts := 0
	prompt := func() {
		if a.Cfg.UI.StatusLine {
			fmt.Fprintln(errw, a.Court.StatusLine(s))
		}
		fmt.Fprint(errw, "> ")
	}
	start := func(text string) {
		tctx, cancel := context.WithCancel(ctx)
		cancelTurn = cancel
		turnDone = make(chan *loop.Result, 1)
		streamed = 0
		go func() {
			r, err := s.Turn(tctx, text)
			if err != nil {
				r = &loop.Result{Status: loop.Fail, Holes: []string{err.Error()}}
			}
			turnDone <- r
		}()
	}
	finish := func(r *loop.Result) int {
		cancelTurn()
		turnDone, cancelTurn = nil, nil
		outMu.Lock()
		if streamed > 0 {
			fmt.Fprintln(out)
		}
		outMu.Unlock()
		a.printResult(r, streamed > 0)
		a.Sessions.Sync(a.SessionID, s, a.mountModel.Ref.Model)
		if r.Verdict != "" {
			a.record(s, "verdict", map[string]interface{}{"verdict": r.Verdict, "wrote": s.Wrote})
		}
		if r.Status == loop.Checkpoint {
			fmt.Fprintf(errw, "the session hit a budget — a good point to stop; resume with: %s resume %s\n", a.Cfg.Dist.Name, a.SessionID)
		}
		return r.Exit()
	}
	prompt()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				if turnDone != nil {
					finish(<-turnDone)
				}
				fmt.Fprintln(errw)
				return 0
			}
			line = strings.TrimSpace(line)
			if line == "" {
				if turnDone == nil {
					prompt()
				}
				continue
			}
			if strings.HasPrefix(line, "/") {
				if quit := a.slash(ctx, line, s, turnDone != nil); quit {
					if turnDone != nil {
						cancelTurn()
						finish(<-turnDone)
					}
					return 0
				}
				// a command that became a turn starts now: left on the wake queue, a line already
				// read (a piped `/quit`) could win the race and end the session before it ran
				if turnDone == nil {
					if queued := a.Court.TakeWake(); len(queued) > 0 {
						start(strings.Join(queued, "\n\n"))
						continue
					}
					prompt()
				}
				continue
			}
			if turnDone != nil {
				_ = a.Court.Send(world.Rimuru, line)
				fmt.Fprintln(errw, "queued for the next tool step")
				continue
			}
			if why := a.Hooks.UserPrompt(ctx, line); why != "" {
				fmt.Fprintln(errw, "refused by a userPrompt hook: "+why)
				prompt()
				continue
			}
			start(line)
		case r := <-turnDone:
			finish(r)
			// what arrived after the turn's last tool step is not lost: it opens the next turn
			pending := append(a.Court.Inbox(world.Rimuru), a.Court.TakeWake()...)
			if len(pending) > 0 {
				start(strings.Join(pending, "\n\n"))
				continue
			}
			prompt()
		case <-wake:
			if turnDone != nil {
				for _, rep := range a.Court.TakeWake() {
					_ = a.Court.Send(world.Rimuru, rep)
				}
				continue
			}
			if reports := a.Court.TakeWake(); len(reports) > 0 {
				start(strings.Join(reports, "\n\n"))
			}
		case <-sig:
			interrupts++
			if turnDone != nil && interrupts == 1 {
				fmt.Fprintln(errw, "\ninterrupting the turn (Ctrl-C again to exit)")
				cancelTurn()
				continue
			}
			fmt.Fprintln(errw)
			if turnDone != nil {
				cancelTurn()
				finish(<-turnDone)
			}
			return 0
		}
	}
}

// announce prints court starts and reports between prompts.
func (a *App) announce(w io.Writer) {
	prev := a.OnState
	started := map[string]bool{}
	var mu sync.Mutex
	a.OnState = func(body, state string) {
		if prev != nil {
			prev(body, state)
		}
		if body == world.Rimuru {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case !started[body] && state != "done":
			started[body] = true
			fmt.Fprintf(w, "→ %s started\n", body)
		case started[body] && state == "done":
			delete(started, body)
			fmt.Fprintf(w, "← %s reported\n", body)
		}
	}
}

// printResult prints a turn's answer for the human: prose, or the wire when the model spoke
// it; the holes always.
func (a *App) printResult(r *loop.Result, alreadyStreamed bool) {
	out := a.Opt.Out
	switch a.Cfg.Output.Format {
	case "json":
		fmt.Fprintln(out, a.resultJSON(r))
		return
	case "wire":
		fmt.Fprintln(out, r.Emit(0))
		return
	}
	if !alreadyStreamed {
		if t := strings.TrimSpace(r.Text); t != "" {
			fmt.Fprintln(out, t)
		}
	}
	for _, h := range r.Holes {
		fmt.Fprintln(out, "@? "+h)
	}
	if r.Verdict != "" && r.Verdict != "pass" {
		fmt.Fprintln(out, "gate: "+r.Verdict)
	}
}

// slash runs a REPL command; true means quit.
func (a *App) slash(ctx context.Context, line string, s *loop.Session, busy bool) bool {
	out, errw := a.Opt.Out, a.Opt.Err
	cmd, rest, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	rest = strings.TrimSpace(rest)
	switch cmd {
	case "quit", "exit", "q":
		return true
	case "help", "?":
		fmt.Fprintln(out, "/agents · /send <body> <text> · /usage · /status · /config [explain] · /compact · /handoff [read] · /sessions · /resume <id> · /quit")
		var names []string
		for _, c := range a.Found.Commands {
			names = append(names, "/"+c.Name)
		}
		for _, p := range a.MCP.Prompts {
			names = append(names, "/"+p.Server+":"+p.Name)
		}
		if len(names) > 0 {
			fmt.Fprintln(out, "commands: "+strings.Join(names, " "))
		}
	case "agents", "court":
		for _, l := range a.Court.Lines() {
			fmt.Fprintln(out, l)
		}
	case "send":
		body, text, _ := strings.Cut(rest, " ")
		if body == "" || strings.TrimSpace(text) == "" {
			fmt.Fprintln(errw, "usage: /send <body> <text>")
			return false
		}
		if err := a.Court.Send(body, strings.TrimSpace(text)); err != nil {
			fmt.Fprintln(errw, err.Error())
		} else {
			fmt.Fprintf(out, "queued for %s's next tool step\n", body)
		}
	case "usage":
		recs, _ := ReadUsage(a.Journal.Dir)
		for _, l := range RollupOf(recs, timeZero(), a.SessionID).Lines() {
			fmt.Fprintln(out, l)
		}
	case "status":
		for _, l := range a.StatusLines() {
			fmt.Fprintln(out, l)
		}
		fmt.Fprintln(out, a.Court.StatusLine(s))
	case "config":
		if rest == "explain" {
			fmt.Fprint(out, a.Cfg.Explain())
		} else {
			for _, l := range a.Cfg.StatusLines() {
				fmt.Fprintln(out, l)
			}
			for _, l := range a.Cfg.Paths() {
				fmt.Fprintln(out, "layer "+l)
			}
		}
	case "compact":
		if busy {
			fmt.Fprintln(errw, "a turn is running — /compact waits for it")
			return false
		}
		if a.Drainer == nil {
			fmt.Fprintln(errw, "compaction is off (compaction.enabled)")
			return false
		}
		ok, err := a.Drainer.Drain(ctx, s, s.Context)
		switch {
		case err != nil:
			fmt.Fprintln(errw, "drain: "+err.Error())
		case ok && a.Drainer.Last != nil:
			fmt.Fprintf(out, "drained: %d → %d tokens, %d pointers, %d notes, %d facts\n", a.Drainer.Last.Before, a.Drainer.Last.After, len(a.Drainer.Last.Pointers), a.Drainer.Last.Notes, a.Drainer.Last.Facts)
			a.Sessions.Sync(a.SessionID, s, a.mountModel.Ref.Model)
		}
	case "handoff":
		if busy {
			fmt.Fprintln(errw, "a turn is running — /handoff waits for it")
			return false
		}
		if sub, arg, _ := strings.Cut(rest, " "); sub == "read" {
			p := strings.TrimSpace(arg)
			if p == "" {
				p, _ = a.latestHandoff()
			} else if !filepath.IsAbs(p) {
				p = filepath.Join(a.Root, p)
			}
			b, err := os.ReadFile(p)
			if p == "" || err != nil {
				fmt.Fprintln(errw, "no handoff to read (/handoff writes one)")
				return false
			}
			fmt.Fprintf(errw, "handoff %s → a turn\n", relOrAbs(a.Root, p))
			a.queueTurn(handoffRead(relOrAbs(a.Root, p), string(b)))
			return false
		}
		fmt.Fprintln(errw, "handoff → a turn")
		a.queueTurn(a.handoffAsk(ctx, rest, s.Ask))
	case "sessions":
		if a.Sessions == nil {
			fmt.Fprintln(errw, "sessions are off (sessions.enabled)")
			return false
		}
		list, _ := a.Sessions.List()
		for _, i := range list {
			fmt.Fprintln(out, i.Line())
		}
	case "resume":
		if busy {
			fmt.Fprintln(errw, "a turn is running")
			return false
		}
		if err := a.loadSession(s, rest); err != nil {
			fmt.Fprintln(errw, "resume: "+err.Error())
		} else {
			fmt.Fprintf(out, "resumed %s: %d messages\n", rest, len(s.Messages))
		}
	default:
		if strings.Contains(cmd, ":") && a.MCP != nil {
			server, name, _ := strings.Cut(cmd, ":")
			args := map[string]string{}
			for _, kv := range strings.Fields(rest) {
				if k, v, ok := strings.Cut(kv, "="); ok {
					args[k] = v
				}
			}
			text, err := a.MCP.Prompt(ctx, server, name, args)
			if err != nil {
				fmt.Fprintln(errw, "prompt: "+err.Error())
				return false
			}
			fmt.Fprintln(errw, "prompt "+cmd+" → a turn")
			a.queueTurn(text)
			return false
		}
		if c, ok := a.Found.Command(cmd); ok {
			fmt.Fprintf(errw, "command /%s → a turn\n", c.Name)
			a.queueTurn(c.Expand(rest))
			return false
		}
		fmt.Fprintf(errw, "no command /%s — /help lists them\n", cmd)
	}
	return false
}

// queueTurn hands a command's expanded text to the REPL as if typed: it rides the wake queue.
func (a *App) queueTurn(text string) {
	a.Court.Wake(text)
}

// loadSession rebuilds a loop session from a session file.
func (a *App) loadSession(s *loop.Session, id string) error {
	if a.Sessions == nil {
		return fmt.Errorf("sessions are off (sessions.enabled)")
	}
	if id == "" {
		list, _ := a.Sessions.List()
		for _, i := range list {
			if i.ID != a.SessionID {
				id = i.ID
				break
			}
		}
		if id == "" {
			return fmt.Errorf("no earlier session to resume")
		}
	}
	events, err := a.Sessions.Read(id)
	if err != nil {
		return err
	}
	msgs, ask := Messages(events)
	if len(msgs) == 0 {
		return fmt.Errorf("session %s holds no messages", id)
	}
	if msgs[len(msgs)-1].Role != "user" {
		// end on a user message: an open tool call is answered as interrupted, never replayed
		last := msgs[len(msgs)-1]
		var results []provider_ToolResult
		for _, c := range last.ToolCalls {
			results = append(results, provider_ToolResult{ID: c.ID, Content: "interrupted before this ran — re-issue it if still needed", IsError: true})
		}
		if len(results) > 0 {
			msgs = append(msgs, provider_Message{Role: "user", ToolResults: results})
		}
	}
	s.Messages, s.Ask = msgs, ask
	a.SessionID = id
	a.Sessions.Sync(id, s, a.mountModel.Ref.Model)
	_ = a.Sessions.Append(id, SessionEvent{Kind: "resume", Fields: map[string]interface{}{"messages": len(msgs), "at": time.Now().UTC().Format(time.RFC3339)}})
	return nil
}

// resultJSON renders a turn as one JSON line (run --json).
func (a *App) resultJSON(r *loop.Result) string {
	type step struct {
		ID     string `json:"id"`
		Tool   string `json:"tool"`
		Class  string `json:"class"`
		Status string `json:"status"`
		Ms     int64  `json:"ms"`
	}
	var steps []step
	for _, st := range r.Steps {
		steps = append(steps, step{st.ID, st.Tool, st.Effective, st.Status, st.Ms})
	}
	tot := a.Journal.Total()
	var usd *float64
	if tot.Calls > 0 && tot.Unpriced == 0 {
		u := tot.USD
		usd = &u
	}
	var rep *wire.Report
	if r.IsWire {
		cp := r.Report
		rep = &cp
	}
	v := map[string]interface{}{
		"@S": r.Status, "status": r.Status, "text": r.Text, "wire": r.IsWire, "report": rep, "steps": steps, "@?": r.Holes,
		"usage": map[string]int{"input": tot.Usage.Input, "output": tot.Usage.Output, "cacheRead": tot.Usage.CacheRead, "cacheWrite": tot.Usage.CacheWrite},
		"usd":   usd, "context": r.Context, "verdict": r.Verdict, "session": a.SessionID, "run": r.RunID, "journal": r.Journal, "exit": r.Exit(),
		"usageJournal": a.Journal.Path(),
	}
	b, _ := jsonMarshal(v)
	return string(b)
}
