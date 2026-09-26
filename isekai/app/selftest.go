package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/instrument"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/memory"
	"github.com/Kaginari/agentic-playground/isekai/onto"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/provider/mock"
	"github.com/Kaginari/agentic-playground/isekai/tool"
	"github.com/Kaginari/agentic-playground/isekai/toolbox"
	"github.com/Kaginari/agentic-playground/isekai/wire"
)

// Selftest runs every package's selftest and the engine's own checks in a throwaway world,
// and answers on the wire: one `@S PASS n checks`, or `@S FAIL` with one @F per failure.
func Selftest(dist string, io IO) int {
	var fails []string
	total := 0
	var quiet bytes.Buffer
	run := func(name string, f func() (int, error)) {
		n, err := f()
		total += n
		if err != nil {
			fails = append(fails, name+": "+firstLine(err.Error()))
		}
	}
	run("config", func() (int, error) { return config.SelftestIn(&quiet) })
	run("memory", memory.Selftest)
	run("toolbox", toolbox.Selftest)
	run("onto", onto.Selftest)
	run("core", func() (int, error) { return coreSelftest(dist) })
	run("app", func() (int, error) { return appSelftest(dist) })
	if len(fails) > 0 {
		fmt.Fprintln(io.Out, "@S FAIL")
		for _, f := range fails {
			fmt.Fprintln(io.Out, "@F selftest — "+f)
		}
		fmt.Fprintf(io.Out, "@E %d\n", 0)
		return 1
	}
	fmt.Fprintf(io.Out, "@S PASS %d checks\n@E 0\n", total)
	return 0
}

// coreSelftest is the engine's own checks: the classifier, the wire, the gate, the readings,
// the loop on the mock. It lives here so both binaries carry it.
func coreSelftest(dist string) (int, error) {
	var fails []string
	checks := 0
	ok := func(cond bool, what string) {
		checks++
		if !cond {
			fails = append(fails, what)
		}
	}
	T, err := os.MkdirTemp("", "isekai-selftest-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(T)
	d, err := parseDist(dist)
	if err != nil {
		return 0, err
	}
	W := filepath.Join(T, "world")
	must := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(s), 0o644)
	}
	must(filepath.Join(W, d.WorldDir, d.LawFile), "# Law\n")
	must(filepath.Join(W, d.WorldDir, "log.md"), "# Chronicle\n")
	must(filepath.Join(W, "README.md"), "# world\n")
	env := tool.Env{Root: W, WorldDir: d.WorldDir}
	C := func(cmd string, want tool.Class) {
		got := env.ClassifyCommand(cmd)
		ok(got.Class == want, fmt.Sprintf("classify %q → %s, got %s (%s)", cmd, want, got.Class, got.Why))
	}
	C("cat README.md", tool.Read)
	C("git status && git log -1", tool.Read)
	C("echo hi > out.txt", tool.Write)
	C("git push origin main", tool.Outward)
	C("curl -s https://x.y", tool.Outward)
	C("rm -rf build", tool.Destructive)
	C("git reset --hard", tool.Destructive)
	C("echo x > "+d.WorldDir+"/log.md", tool.Destructive)
	C("echo x >> "+d.WorldDir+"/log.md", tool.Write)
	bash, _ := tool.Builtins().Get("bash")
	settle := func(input string) tool.Class { c, _ := bash.Settle(env, []byte(input)); return c.Class }
	ok(settle(`{"command":"git push","class":"read"}`) == tool.Outward, "declared read cannot lower outward")
	ok(settle(`{"command":"cat f","class":"destructive"}`) == tool.Destructive, "declared destructive raises a read")
	// wire
	cm := wire.ParseCommission("@ROOT /w\n@SCOPE pkg a\n@ASK findings +unsaid\n@CAP 300\nprose line")
	ok(cm.Root == "/w" && cm.Ask == "findings" && cm.Unsaid && cm.Cap == 300, "commission parses")
	rep, isWire := wire.ParseReport("@S PASS\n@F a.go:1 one\n@? hole\n@U colony the fact\n@E 60")
	ok(isWire && rep.Status == "PASS" && len(rep.Findings) == 1 && rep.Unsaid[0].Kind == "colony", "report parses")
	big := wire.Report{Status: "PASS", Unsaid: []wire.Unsaid{{Kind: "law", Text: "kept"}}}
	for i := 0; i < 50; i++ {
		big.Findings = append(big.Findings, fmt.Sprintf("file%d.go:%d a finding that takes some bytes", i, i))
	}
	capped := big.Emit(400, "")
	ok(len(capped) <= 400 && strings.Contains(capped, "@U law kept"), "@CAP cuts findings, keeps @U")
	// gate
	g := gate.New()
	g.IsTTY = func() bool { return false }
	a := g.Ask(gate.Request{ID: "x", Class: tool.Outward, Why: "git ↔ remote"})
	ok(a.Decision == gate.Denied && a.By == "no-tty", "no TTY denies")
	ok(g.Ask(gate.Request{Class: tool.Write}).Decision == gate.NotNeeded, "a write needs no gate")
	ok(g.Ask(gate.Request{Class: tool.Read, Force: true}).Decision == gate.Denied, "a rule's ask forces the gate")
	g.Approve, _ = gate.ParseApprove("outward")
	ok(g.Ask(gate.Request{Class: tool.Outward}).Decision == gate.Approved && g.Ask(gate.Request{Class: tool.Destructive}).Decision == gate.Denied, "pre-approval is per class")
	// readings
	b := instrument.Budget{}
	ok(b.Reading(provider.Usage{Input: 1000, CacheRead: 185000}).Zone == instrument.STRESS, "185k is the stress zone")
	ok(b.Unread("x").Zone == instrument.UNREAD, "no reading is UNREAD, not zero")
	// the loop on the mock
	m := mock.New(
		mock.Call("c1", "read", map[string]interface{}{"path": "README.md"}),
		mock.Call("c2", "write", map[string]interface{}{"path": "notes/out.txt", "content": "hello\n"}),
		mock.Text("@S DONE\n@F notes/out.txt:1 written\n@U colony the notes file is notes/out.txt\n@E 0"),
	)
	e := &loop.Engine{Provider: m, Tools: tool.Builtins(), Gate: gate.New(), Root: W, As: "slime-notes", Lexicon: loop.Lexicon{WorldDir: d.WorldDir}, Unsaid: true}
	e.Gate.IsTTY = func() bool { return false }
	r, err := e.Run(context.Background(), "write the notes file")
	ok(err == nil && r.Status == loop.Done && len(r.Steps) == 2 && r.Steps[1].Wrote[0] == "notes/out.txt", fmt.Sprintf("run A: %v %+v", err, r))
	ok(r.IsWire && len(r.Report.Unsaid) == 1 && len(r.Holes) == 0, "run A: wire report with @U")
	ok(len(m.Requests) == 3 && strings.Contains(m.Requests[0].System, "slime-notes"), "run A: the system prompt names the body")
	J := filepath.Join(W, d.WorldDir, "instruments", "loop", r.RunID+".jsonl")
	ev, _ := loop.ReadJournal(J)
	ok(len(ev) > 0 && ev[0]["t"] == "run" && ev[len(ev)-1]["t"] == "end", "journal: run first, end last")
	m = mock.New(mock.Call("c1", "bash", map[string]interface{}{"command": "git push origin main"}), mock.Text("never"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "publish")
	ok(r.Status == loop.Denied && r.Exit() == 4 && len(m.Requests) == 1, "run B: denied stops the turn")
	e.Budget.Steps = 1
	m = mock.New(mock.Calls(provider.ToolCall{ID: "a", Name: "read", Input: []byte(`{"path":"README.md"}`)}, provider.ToolCall{ID: "b", Name: "read", Input: []byte(`{"path":"README.md"}`)}), mock.Text("x"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "two reads")
	ok(r.Status == loop.Checkpoint && r.Exit() == 5, "run E: checkpoint at the step budget")
	e.Budget.Steps = 0
	m = mock.New(mock.Call("c1", "nosuch", map[string]interface{}{}), mock.Call("c2", "nosuch", map[string]interface{}{}), mock.Text("x"))
	e.Provider = m
	e.Hooks.Missing = func(ctx context.Context, s *loop.Session, call provider.ToolCall) (string, bool) {
		return "missing " + call.Name, len(s.Messages) > 3
	}
	r, _ = e.Run(context.Background(), "missing")
	ok(r.Status == loop.Escalate && len(r.Steps) == 2, fmt.Sprintf("run M: the missing hook stops the turn: %s", r.Status))
	if len(fails) > 0 {
		return checks, fmt.Errorf("%s", strings.Join(fails, " · "))
	}
	return checks, nil
}

// appSelftest opens the engine on a founded world with a scripted mock and runs one turn.
func appSelftest(dist string) (int, error) {
	var fails []string
	checks := 0
	ok := func(cond bool, what string) {
		checks++
		if !cond {
			fails = append(fails, what)
		}
	}
	T, err := os.MkdirTemp("", "isekai-appselftest-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(T)
	d, _ := parseDist(dist)
	root := filepath.Join(T, "world")
	home := filepath.Join(T, "home")
	_ = os.MkdirAll(home, 0o755)
	made, err := Init(d.Name, root)
	ok(err == nil && len(made) > 0, "init founds a world")
	again, err := Init(d.Name, root)
	ok(err == nil && len(again) == 0, "init is idempotent")
	law, _ := os.ReadFile(filepath.Join(root, d.WorldDir, d.LawFile))
	ok(strings.Contains(string(law), "## The Crest") || strings.Contains(string(law), "## "+lexiconFor(d.Name).Crest), "the embedded law has a crest")
	script := filepath.Join(root, d.WorldDir, "tmp", "s.json")
	_ = os.WriteFile(script, []byte(`[{"calls":[{"name":"bash","input":{"command":"echo hi > hi.txt && cat hi.txt"}}]},{"text":"@S DONE\n@U colony hi\n@E 22"}]`), 0o644)
	_ = os.WriteFile(filepath.Join(root, d.WorldDir, "config.yaml"), []byte("models: {default: mock/m}\nproviders: {mock: {enabled: true, script: "+script+"}}\ntools: {bash: {sandbox: none}}\nui: {board: {autostart: false}}\n"), 0o644)
	var out, errb bytes.Buffer
	env := func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	a, err := New(Options{Dist: d.Name, Root: root, Cwd: root, Home: home, Env: env, In: strings.NewReader(""), Out: &out, Err: &errb, Quiet: true, IsTTY: func() bool { return false }, NoBoard: true})
	if err != nil {
		return checks, err
	}
	defer a.Close()
	e, err := a.Engine()
	ok(err == nil, "engine builds")
	if err == nil {
		r, err := e.Run(context.Background(), "say hi")
		ok(err == nil && r.Status == loop.Done && len(r.Steps) == 1 && strings.Contains(r.Steps[0].Result.Output, "hi"), fmt.Sprintf("a turn on the mock: %v %+v", err, r))
		recs, _ := ReadUsage(a.Journal.Dir)
		ok(len(recs) == 2 && recs[0].Body == "rimuru" && recs[0].USD == nil, "usage journaled, unpriced")
		ok(strings.Contains(strings.Join(a.StatusLines(), "\n"), "sandbox: none"), "status names the sandbox mode")
	}
	code := Main(d.Name, []string{"version"}, IO{Out: &out, Err: &errb, Env: env}, Version{Version: "t"})
	ok(code == 0 && strings.HasPrefix(out.String(), d.Name+" t ("), "version prints the line")
	if len(fails) > 0 {
		return checks, fmt.Errorf("%s", strings.Join(fails, " · "))
	}
	return checks, nil
}
