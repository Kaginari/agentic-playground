package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/instrument"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/provider/mock"
	"github.com/Kaginari/agentic-playground/isekai/tool"
	"github.com/Kaginari/agentic-playground/isekai/wire"
)

// cmdSelftest runs the in-binary checks in a throwaway world (Law 5: under <root>/.isekai/tmp/
// when a world is found, else the system temp dir) and answers on the wire.
func cmdSelftest(args []string, io IO) int {
	var fails []string
	checks := 0
	ok := func(cond bool, what string) {
		checks++
		if !cond {
			fails = append(fails, what)
		}
	}
	base := ""
	if root, found := FindRoot("."); found {
		base = filepath.Join(root, WorldDir, "tmp")
		_ = os.MkdirAll(base, 0o755)
	}
	T, err := os.MkdirTemp(base, "selftest-")
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? cannot make a throwaway world: %v\n", err)
		return 2
	}
	defer os.RemoveAll(T)
	W := filepath.Join(T, "world")
	must := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(s), 0o644)
	}
	must(filepath.Join(W, WorldDir, "isekai.md"), "# Law\n")
	must(filepath.Join(W, WorldDir, "log.md"), "# Chronicle\n")
	must(filepath.Join(W, "README.md"), "# world\nthe notes file is notes/out.txt\n")
	env := tool.Env{Root: W}

	// classifier: the heuristic errs toward asking; a declaration only tightens
	C := func(cmd string, want tool.Class) {
		got := env.ClassifyCommand(cmd)
		ok(got.Class == want, fmt.Sprintf("classify %q → %s, got %s (%s)", cmd, want, got.Class, got.Why))
	}
	C("cat README.md", tool.Read)
	C("git status && git log -1", tool.Read)
	C("echo hi > out.txt", tool.Write)
	C("node build.js", tool.Write)
	C("mkdir -p a/b", tool.Write)
	C("git push origin main", tool.Outward)
	C("curl -s https://x.y", tool.Outward)
	C(`bash -c "wget x"`, tool.Outward)
	C("ssh host ls", tool.Outward)
	C("cat /etc/hosts", tool.Read)
	C("cat /home/nobody/secret", tool.Outward)
	C("cp a ../../../../../../../../outside", tool.Outward)
	C("rm -rf build", tool.Destructive)
	C("git reset --hard", tool.Destructive)
	C("git branch -D x", tool.Destructive)
	C("git push --force", tool.Destructive)
	C("find . -name x -delete", tool.Destructive)
	C("echo x > .isekai/log.md", tool.Destructive)
	C("echo x >> .isekai/log.md", tool.Write)
	C("sed -i s/a/b/ .isekai/isekai.md", tool.Destructive)
	C("ls | xargs rm", tool.Destructive)
	C("tee -a .isekai/log.md < x", tool.Write)
	C("tee .isekai/log.md < x", tool.Destructive)
	C("go get github.com/x/y", tool.Outward)
	C("go test ./...", tool.Read)
	bash, _ := tool.Builtins().Get("bash")
	settle := func(input string) tool.Class { c, _ := bash.Settle(env, []byte(input)); return c.Class }
	ok(settle(`{"command":"git push","class":"read"}`) == tool.Outward, "declared read cannot lower outward")
	ok(settle(`{"command":"echo x > f","class":"outward"}`) == tool.Outward, "declared outward raises a write")
	ok(settle(`{"command":"cat f","class":"destructive"}`) == tool.Destructive, "declared destructive raises a read")
	_, holes := bash.Settle(env, []byte(`{"command":"git push","class":"read"}`))
	ok(len(holes) == 1 && strings.Contains(holes[0], "only tightens"), "a lowering declaration is a hole")
	wt, _ := tool.Builtins().Get("write")
	c, _ := wt.Settle(env, []byte(`{"path":".isekai/log.md","content":"x"}`))
	ok(c.Class == tool.Destructive, "write onto a record is destructive")
	c, _ = wt.Settle(env, []byte(`{"path":"/tmp/elsewhere","content":"x"}`))
	ok(c.Class == tool.Outward, "write outside the world is outward")

	// wire: parse, emit, @CAP
	cm := wire.ParseCommission("@ROOT /w\n@SCOPE pkg a\n@ASK findings +unsaid\n@CAP 300\nprose line")
	ok(cm.Root == "/w" && cm.Ask == "findings" && cm.Unsaid && cm.Cap == 300 && len(cm.Body) == 1, "commission parses")
	rep, isWire := wire.ParseReport("@S PASS\n@F a.go:1 one\n@F b.go:2 two\n@? hole\n@U colony the fact\n@E 60")
	ok(isWire && rep.Status == "PASS" && len(rep.Findings) == 2 && len(rep.Holes) == 1 && rep.Unsaid[0].Kind == "colony" && rep.Bytes == 60, "report parses")
	out := rep.Emit(0, "")
	ok(strings.HasPrefix(out, "@S PASS\n@? hole\n@U colony the fact\n") && strings.HasSuffix(out, "@E "+fmt.Sprint(len(out))), "emit closes with an exact @E: "+out)
	big := wire.Report{Status: "PASS", Unsaid: []wire.Unsaid{{Kind: "law", Text: "kept"}}}
	for i := 0; i < 50; i++ {
		big.Findings = append(big.Findings, fmt.Sprintf("file%d.go:%d a finding that takes some bytes", i, i))
	}
	capped := big.Emit(400, "")
	ok(len(capped) <= 400 && strings.Contains(capped, "@U law kept") && strings.Contains(capped, "@? @CAP 400:") && strings.Contains(capped, "line") && strings.Contains(capped, "cut"), fmt.Sprintf("@CAP cuts findings, keeps @U, names the cut (%d bytes)", len(capped)))
	raw, _ := wire.ParseReport("S|PASS\nF|a.go:3|claim")
	ok(raw.Status == "PASS" && len(raw.Findings) == 1, "raw pipe form parses")

	// gate: nothing auto-approved
	g := gate.New()
	g.IsTTY = func() bool { return false }
	a := g.Ask(gate.Request{ID: "x", Class: tool.Outward, Why: "git ↔ remote"})
	ok(a.Decision == gate.Denied && a.By == "no-tty" && strings.Contains(a.Why, "--approve outward"), "no TTY denies and names the flag")
	ok(g.Ask(gate.Request{Class: tool.Write}).Decision == gate.NotNeeded, "a write needs no gate")
	g.Strict = true
	ok(g.Ask(gate.Request{Class: tool.Write}).Decision == gate.Denied, "strict gates writes")
	g.Strict = false
	g.Approve, _ = gate.ParseApprove("outward")
	ok(g.Ask(gate.Request{Class: tool.Outward}).Decision == gate.Approved && g.Ask(gate.Request{Class: tool.Destructive}).Decision == gate.Denied, "pre-approval is per class")
	g.DryRun = true
	ok(g.Ask(gate.Request{Class: tool.Destructive}).Decision == gate.WouldAsk, "dry run says would-ask")
	tty := gate.New()
	tty.IsTTY = func() bool { return true }
	tty.In, tty.Out = strings.NewReader("y\n"), &bytes.Buffer{}
	ok(tty.Ask(gate.Request{Class: tool.Outward}).Decision == gate.Approved, "a TTY y approves")
	tty.In = strings.NewReader("\n")
	ok(tty.Ask(gate.Request{Class: tool.Outward}).Decision == gate.Denied, "a TTY empty answer denies")
	_, err = gate.ParseApprove("network")
	ok(err != nil, "an unknown class in --approve is an error")

	// instrument: the reading, the zones
	b := instrument.Budget{}
	ok(b.Reading(provider.Usage{Input: 1000, CacheRead: 185000}).Zone == instrument.STRESS, "185k is the stress zone")
	ok(b.Reading(provider.Usage{Input: 140000}).Zone == instrument.NEAR && b.Reading(provider.Usage{Input: 1000}).Zone == instrument.OK, "NEAR at 75% of stress, OK below")
	ok(strings.Contains(b.Reading(provider.Usage{Input: 12345}).String(), "12,345 / 200,000"), "status line groups digits")
	ok(b.Unread("x").Zone == instrument.UNREAD, "no reading is UNREAD, not zero")

	// the loop, on the mock: read → write → answer on the wire
	m := mock.New(
		mock.Call("c1", "read", map[string]interface{}{"path": "README.md"}),
		mock.Call("c2", "write", map[string]interface{}{"path": "notes/out.txt", "content": "hello\n"}),
		mock.Text("@S DONE\n@F notes/out.txt:1 written\n@U colony the notes file is notes/out.txt\n@E 0"),
	)
	e := &loop.Engine{Provider: m, Tools: tool.Builtins(), Gate: gate.New(), Root: W, As: "slime-notes", Lexicon: loop.Lexicon{WorldDir: WorldDir}, Unsaid: true}
	e.Gate.IsTTY = func() bool { return false }
	r, err := e.Run(context.Background(), "write the notes file")
	ok(err == nil && r.Status == loop.Done && len(r.Steps) == 2 && r.Steps[0].Status == "done" && r.Steps[1].Status == "done", fmt.Sprintf("run A: %v %+v", err, r))
	ok(r.Steps[0].Effective == "read" && r.Steps[1].Effective == "write" && r.Steps[1].Wrote[0] == "notes/out.txt", "run A: classes and wrote")
	ok(r.IsWire && len(r.Report.Unsaid) == 1 && len(r.Holes) == 0, fmt.Sprintf("run A: wire report with @U, no holes: %v", r.Holes))
	data, _ := os.ReadFile(filepath.Join(W, "notes", "out.txt"))
	ok(string(data) == "hello\n", "run A: the file was written")
	ok(len(m.Requests) == 3 && strings.Contains(m.Requests[0].System, "slime-notes") && len(m.Requests[0].Tools) == 6, "run A: system prompt names the body, six tools offered")
	ok(len(m.Requests[2].Messages) == 5 && m.Requests[2].Messages[4].ToolResults[0].ID == "c2", "run A: conversation shape (user, assistant, user results, assistant, user results)")
	J := filepath.Join(W, WorldDir, "instruments", "loop", r.RunID+".jsonl")
	ev, _ := loop.ReadJournal(J)
	ts := []string{}
	for _, x := range ev {
		ts = append(ts, x["t"].(string))
	}
	ok(len(ev) > 0 && ts[0] == "run" && ts[len(ts)-1] == "end", "journal: run first, end last — "+strings.Join(ts, ","))
	ok(strings.Contains(strings.Join(ts, ","), "perceive,gate,act,act,verify,record"), "journal: the beats per step — "+strings.Join(ts, ","))
	st := loop.State(J, ev)
	ok(st.Status == "DONE" && len(st.Steps) == 2 && st.Steps[1].Status == "done" && st.Turns == 3, fmt.Sprintf("journal state reads back: %+v", st))
	ok(!strings.Contains(string(mustRead(J)), "hello\\n\"}"), "journal points, it does not carry the written content")

	// denial stops the turn (exit 4), the model never sees the act run
	m = mock.New(mock.Call("c1", "bash", map[string]interface{}{"command": "git push origin main"}), mock.Text("never"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "publish")
	ok(r.Status == loop.Denied && r.Exit() == 4 && r.Steps[0].Status == "denied" && len(m.Requests) == 1, fmt.Sprintf("run B: denied stops the turn: %s %v", r.Status, r.Holes))
	ok(len(r.Holes) == 1 && strings.Contains(r.Holes[0], "outward") && strings.Contains(r.Holes[0], "git ↔ remote"), "run B: the hole names the class and reason")
	_, err = os.Stat(strings.TrimSuffix(filepath.Join(W, WorldDir, "instruments", "loop", r.RunID+".jsonl"), ".jsonl") + ".transcript.json")
	ok(err == nil, "run B: a transcript was kept for resume")
	// pre-approval lets it run (the command itself fails outside a repo: a failed act, not a denial)
	e.Gate.Approve, _ = gate.ParseApprove("outward")
	m = mock.New(mock.Call("c1", "bash", map[string]interface{}{"command": "echo pushed > pushed.txt", "class": "outward"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "publish")
	ok(r.Status == loop.Done && r.Steps[0].Gate.By == "pre-approved" && r.Steps[0].Effective == "outward", fmt.Sprintf("run C: declared outward, pre-approved, ran: %s %v", r.Status, r.Holes))
	e.Gate.Approve = nil
	// dry run: would-ask, nothing written, nothing journaled
	e.Gate.DryRun = true
	nJ := countJournals(W)
	m = mock.New(mock.Call("c1", "write", map[string]interface{}{"path": "dry.txt", "content": "x"}), mock.Call("c2", "bash", map[string]interface{}{"command": "curl x"}), mock.Text("@S DRY\n@U law x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "dry")
	_, errDry := os.Stat(filepath.Join(W, "dry.txt"))
	ok(r.Status == loop.Dry && r.Steps[0].Status == "would-run" && r.Steps[1].Status == "would-ask" && errDry != nil && countJournals(W) == nJ, fmt.Sprintf("run D: dry run reports, runs nothing, journals nothing: %+v", r.Steps))
	e.Gate.DryRun = false
	// step budget checkpoints (exit 5)
	e.Budget.Steps = 1
	m = mock.New(mock.Calls(provider.ToolCall{ID: "a", Name: "read", Input: []byte(`{"path":"README.md"}`)}, provider.ToolCall{ID: "b", Name: "read", Input: []byte(`{"path":"README.md"}`)}), mock.Text("x"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "two reads")
	ok(r.Status == loop.Checkpoint && r.Exit() == 5 && strings.Contains(r.Holes[0], "step budget reached (1)"), fmt.Sprintf("run E: checkpoint at the step budget: %s %v", r.Status, r.Holes))
	e.Budget.Steps = 0
	// consecutive failures escalate (exit 3)
	m = mock.New(mock.Call("1", "bash", map[string]interface{}{"command": "false"}), mock.Call("2", "bash", map[string]interface{}{"command": "false"}), mock.Call("3", "bash", map[string]interface{}{"command": "echo boom >&2; false"}), mock.Text("x"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "doom")
	ok(r.Status == loop.Escalate && r.Exit() == 3 && len(r.Steps) == 3 && strings.Contains(r.Holes[0], "3 consecutive failed acts") && strings.Contains(r.Holes[0], "boom"), fmt.Sprintf("run F: escalates after retries: %s %v", r.Status, r.Holes))
	// context in the stress zone checkpoints before the next call; a drain hook may continue
	m = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: []byte(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 1000, CacheRead: 185000}}, mock.Text("x"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "stress")
	ok(r.Status == loop.Checkpoint && strings.Contains(r.Holes[0], "stress zone (186000 ≥ 180000") && r.Context.Zone == instrument.STRESS, fmt.Sprintf("run G: stress zone checkpoints: %s %v", r.Status, r.Holes))
	drained := false
	e.Hooks.Drain = func(ctx context.Context, s *loop.Session, c instrument.Context) (bool, error) {
		drained = true
		s.Last = provider.Usage{Input: 100}
		return true, nil
	}
	m = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: []byte(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 1000, CacheRead: 185000}}, mock.Text("@S DONE\n@U law x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "stress")
	ok(drained && r.Status == loop.Done, "run H: the drain hook continues the run")
	e.Hooks.Drain = nil
	// policy (territory) refuses without stopping the turn
	e.Policy = func(a tool.Access) error {
		for _, p := range a.Paths {
			if a.Class >= tool.Write && !strings.HasPrefix(p, filepath.Join(W, "notes")) {
				return fmt.Errorf("outside the territory notes/: %s", filepath.Base(p))
			}
		}
		return nil
	}
	m = mock.New(mock.Call("1", "write", map[string]interface{}{"path": "elsewhere.txt", "content": "x"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "territory")
	_, errT := os.Stat(filepath.Join(W, "elsewhere.txt"))
	ok(r.Status == loop.Done && r.Steps[0].Status == "refused" && errT != nil && strings.Contains(m.Requests[1].Messages[2].ToolResults[0].Content, "refused: outside the territory"), fmt.Sprintf("run I: policy refuses, the model reads the refusal: %+v", r.Steps[0]))
	e.Policy = nil
	// end-of-turn gate hook sees what was written
	var sawWrote []string
	e.Hooks.EndGate = func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
		sawWrote = s.Wrote
		return "pass", nil, nil
	}
	m = mock.New(mock.Call("1", "edit", map[string]interface{}{"path": "notes/out.txt", "old": "hello", "new": "bye"}), mock.Text("@S DONE\n@U law x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "edit")
	ok(r.Status == loop.Done && r.Verdict == "pass" && len(sawWrote) == 1 && sawWrote[0] == "notes/out.txt", fmt.Sprintf("run J: the end gate sees the written paths: %v %v", r.Verdict, sawWrote))
	e.Hooks.EndGate = nil
	// no @U when +unsaid was asked is a hole
	m = mock.New(mock.Text("@S DONE\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "silent")
	ok(r.Status == loop.Done && len(r.Holes) == 1 && strings.Contains(r.Holes[0], "no @U"), "run K: a report with no @U is flagged")
	// resume a denied run: the open tool call is answered, the model continues
	m = mock.New(mock.Call("c1", "bash", map[string]interface{}{"command": "git push"}))
	e.Provider = m
	r, _ = e.Run(context.Background(), "publish")
	m = mock.New(mock.Text("@S DONE\n@U law x\n@E 0"))
	e.Provider = m
	r2, err := e.Resume(context.Background(), r.RunID, "skip the push")
	ok(err == nil && r2.Status == loop.Done && r2.RunID == r.RunID && len(m.Requests[0].Messages) == 3 && strings.Contains(m.Requests[0].Messages[2].Text, "skip the push"), fmt.Sprintf("run L: resume continues the same run: %v %+v", err, r2))
	ev, _ = loop.ReadJournal(filepath.Join(W, WorldDir, "instruments", "loop", r.RunID+".jsonl"))
	n := 0
	for _, x := range ev {
		if x["t"] == "resume" {
			n++
		}
	}
	ok(n == 1, "run L: a resume line was journaled")

	// the CLI, end to end on the mock
	var so, se bytes.Buffer
	cio := IO{In: strings.NewReader(""), Out: &so, Err: &se, Env: func(string) string { return "" }}
	code := Main([]string{"run", "-root", W, "-provider", "mock", "-quiet", "hello"}, cio)
	ok(code == 0 && strings.HasPrefix(so.String(), "@S MOCK\n@? the mock provider has no script"), "cli run on the mock answers on the wire: "+so.String())
	so.Reset()
	se.Reset()
	code = Main([]string{"status", "-root", W}, cio)
	ok(code == 0 && strings.HasPrefix(so.String(), "@S OK root=") && strings.Contains(so.String(), "provider=none configured") && strings.Contains(so.String(), "ANTHROPIC_API_KEY unset"), "cli status lists readings without secrets: "+so.String())
	so.Reset()
	code = Main([]string{"status", "-root", W, r.RunID}, cio)
	ok(code == 0 && strings.HasPrefix(so.String(), "@S DONE run="+r.RunID), "cli status <run-id>: "+so.String())
	so.Reset()
	ok(Main([]string{"nonsense"}, cio) == 2 && Main([]string{"run", "-root", W, "-provider", "mock"}, cio) == 2 && Main([]string{"run", "-root", W, "-provider", "mock", "-approve", "network", "x"}, cio) == 2, "cli: unknown command, missing ask, bad --approve are FAILs")
	so.Reset()
	se.Reset()
	code = Main([]string{"repl", "-root", W, "-provider", "mock", "-quiet"}, IO{In: strings.NewReader("hi\n/status\n/quit\n"), Out: &so, Err: &se, Env: func(string) string { return "" }})
	ok(code == 0 && strings.Contains(so.String(), "@S MOCK") && strings.Contains(so.String(), "turns 1"), "cli repl: a line is a turn, /status a reading, /quit leaves: "+so.String())
	_, errRoot := os.Stat(filepath.Join(T, "..", "nonexistent"))
	ok(errRoot != nil, "sanity")

	if len(fails) > 0 {
		fmt.Fprintln(io.Out, "@S FAIL")
		for _, f := range fails {
			fmt.Fprintln(io.Out, "@F selftest — "+f)
		}
		return 1
	}
	fmt.Fprintf(io.Out, "@S PASS %d checks\n", checks)
	return 0
}

func mustRead(p string) []byte {
	b, _ := os.ReadFile(p)
	return b
}

func countJournals(W string) int {
	ents, _ := os.ReadDir(filepath.Join(W, WorldDir, "instruments", "loop"))
	n := 0
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			n++
		}
	}
	return n
}
