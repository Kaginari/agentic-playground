// Command isekai runs an agent session on a world: `isekai` (REPL), `isekai run "<ask>"`,
// `isekai resume <run-id>`, `isekai status`, `isekai selftest`. Later packages register their
// own subcommands through Register.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/gate"
	"github.com/Kaginari/agentic-playground/isekai/instrument"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
	"github.com/Kaginari/agentic-playground/isekai/provider/anthropic"
	"github.com/Kaginari/agentic-playground/isekai/provider/mock"
	"github.com/Kaginari/agentic-playground/isekai/provider/openai"
	"github.com/Kaginari/agentic-playground/isekai/tool"
	"github.com/Kaginari/agentic-playground/isekai/wire"
)

// Command is one subcommand. Run returns the process exit code.
type Command struct {
	Name    string
	Summary string
	Run     func(args []string, io IO) int
}

// IO is what a command talks through; tests swap it.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	Env func(string) string
}

var registry = map[string]Command{}

// Register adds a subcommand; later packages call it from an init.
func Register(c Command) { registry[c.Name] = c }

// WorldDir is the world directory name this distribution looks for.
var WorldDir = ".isekai"

func init() {
	Register(Command{"run", "run one ask to its end and print the report", cmdRun})
	Register(Command{"repl", "a line REPL on one session (the default)", cmdRepl})
	Register(Command{"resume", "resume a checkpointed, denied or escalated run", cmdResume})
	Register(Command{"status", "the world's readings: root, provider, journals", cmdStatus})
	Register(Command{"selftest", "in-binary checks, answered on the wire", cmdSelftest})
	Register(Command{"help", "list the commands", cmdHelp})
}

func main() {
	os.Exit(Main(os.Args[1:], IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Env: os.Getenv}))
}

// Main dispatches; exported for the selftest.
func Main(args []string, io IO) int {
	name := "repl"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	c, ok := registry[name]
	if !ok {
		fmt.Fprintf(io.Err, "@S FAIL\n@? unknown command %q — %s\n", name, names())
		return 2
	}
	return c.Run(args, io)
}

func names() string {
	var ns []string
	for n := range registry {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return strings.Join(ns, " | ")
}

func cmdHelp(args []string, io IO) int {
	var ns []string
	for n := range registry {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	fmt.Fprintln(io.Out, "isekai [command] [flags]")
	for _, n := range ns {
		fmt.Fprintf(io.Out, "  %-9s %s\n", n, registry[n].Summary)
	}
	return 0
}

// FindRoot walks up from dir to the nearest directory holding WorldDir.
func FindRoot(dir string) (string, bool) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if st, err := os.Stat(filepath.Join(d, WorldDir)); err == nil && st.IsDir() {
			return d, true
		}
		up := filepath.Dir(d)
		if up == d {
			return "", false
		}
		d = up
	}
}

// engineFlags are the flags every session command shares.
type engineFlags struct {
	fs        *flag.FlagSet
	root      string
	prov      string
	model     string
	as        string
	approve   string
	strict    bool
	dryRun    bool
	maxSteps  int
	maxMin    float64
	retries   int
	cap       int
	unsaid    bool
	jsonOut   bool
	quiet     bool
	noJournal bool
}

func newFlags(name string, out io.Writer) *engineFlags {
	f := &engineFlags{fs: flag.NewFlagSet(name, flag.ContinueOnError)}
	f.fs.SetOutput(out)
	f.fs.StringVar(&f.root, "root", "", "world root (default: nearest ancestor with "+WorldDir+")")
	f.fs.StringVar(&f.prov, "provider", "", "anthropic | openai | mock (default: ISEKAI_PROVIDER, else anthropic if ANTHROPIC_API_KEY is set, else openai)")
	f.fs.StringVar(&f.model, "model", "", "model id (default: ISEKAI_MODEL, else the provider's)")
	f.fs.StringVar(&f.as, "as", "", "the body this session runs as")
	f.fs.StringVar(&f.approve, "approve", "", "pre-approved classes, e.g. outward,destructive")
	f.fs.BoolVar(&f.strict, "strict", false, "writes pass the gate too")
	f.fs.BoolVar(&f.dryRun, "dry-run", false, "run nothing but reads; report what would ask")
	f.fs.IntVar(&f.maxSteps, "max-steps", 0, "tool steps per turn (default 50; -1 unlimited)")
	f.fs.Float64Var(&f.maxMin, "max-minutes", 0, "wall clock per session (default 30; -1 unlimited)")
	f.fs.IntVar(&f.retries, "retries", 0, "consecutive failed acts before ESCALATE (default 2)")
	f.fs.IntVar(&f.cap, "cap", wire.DefaultCap, "@CAP on the report in bytes (0: none)")
	f.fs.BoolVar(&f.unsaid, "unsaid", false, "ask +unsaid: a report with no @U is a hole")
	f.fs.BoolVar(&f.jsonOut, "json", false, "print the result as one JSON line")
	f.fs.BoolVar(&f.quiet, "quiet", false, "no step trace on stderr")
	f.fs.BoolVar(&f.noJournal, "no-journal", false, "do not journal this run")
	return f
}

func (f *engineFlags) engine(io IO) (*loop.Engine, error) {
	root := f.root
	if root == "" {
		r, ok := FindRoot(".")
		if !ok {
			cwd, _ := os.Getwd()
			return nil, fmt.Errorf("no %s/ at or above %s — pass -root", WorldDir, cwd)
		}
		root = r
	}
	root, _ = filepath.Abs(root)
	p, err := newProvider(f.prov, f.model, io.Env)
	if err != nil {
		return nil, err
	}
	g := gate.New()
	if g.Approve, err = gate.ParseApprove(f.approve); err != nil {
		return nil, err
	}
	g.Strict, g.DryRun, g.In, g.Out = f.strict, f.dryRun, io.In, io.Err
	e := &loop.Engine{
		Provider: p, Tools: tool.Builtins(), Gate: g, Root: root, As: f.as,
		Budget:  loop.Budget{Steps: f.maxSteps, Minutes: f.maxMin, Retries: f.retries},
		Lexicon: loop.Lexicon{WorldDir: WorldDir, Law: "The law of this world is " + WorldDir + "/isekai.md; its crest is read first, always."},
		Unsaid:  f.unsaid, Cap: f.cap,
	}
	if f.noJournal {
		e.Journal = "-"
	}
	if !f.quiet {
		e.Trace = io.Err
	}
	return e, nil
}

func newProvider(name, model string, env func(string) string) (provider.Provider, error) {
	if env == nil {
		env = os.Getenv
	}
	provider.Getenv = env
	if name == "" {
		name = env("ISEKAI_PROVIDER")
	}
	if name == "" {
		if env("ANTHROPIC_API_KEY") != "" {
			name = "anthropic"
		} else if env("OPENAI_API_KEY") != "" || env("OPENAI_BASE_URL") != "" {
			name = "openai"
		} else {
			return nil, fmt.Errorf("no provider: set ANTHROPIC_API_KEY, or OPENAI_API_KEY / OPENAI_BASE_URL, or -provider")
		}
	}
	switch name {
	case "anthropic":
		return anthropic.New(model)
	case "openai", "openrouter", "ollama":
		return openai.New(model)
	case "mock":
		m := mock.New()
		m.Fallback = &provider.Response{Message: provider.Message{Role: provider.Assistant, Text: "@S MOCK\n@? the mock provider has no script\n@E 0"}, Stop: provider.StopEnd}
		return m, nil
	}
	return nil, fmt.Errorf("unknown provider %q (anthropic | openai | mock)", name)
}

func printResult(r *loop.Result, f *engineFlags, io IO) {
	if f.jsonOut {
		b, _ := jsonLine(r)
		fmt.Fprintln(io.Out, b)
		return
	}
	fmt.Fprintln(io.Out, r.Emit(f.cap))
}

func cmdRun(args []string, io IO) int {
	f := newFlags("run", io.Err)
	if err := f.fs.Parse(args); err != nil {
		return 2
	}
	ask := strings.TrimSpace(strings.Join(f.fs.Args(), " "))
	if ask == "" {
		fmt.Fprintln(io.Err, "@S FAIL\n@? run needs an ask: isekai run \"<ask>\"")
		return 2
	}
	e, err := f.engine(io)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if c := wire.ParseCommission(ask); c.Unsaid {
		e.Unsaid = true
		if c.Cap > 0 {
			f.cap = c.Cap
		}
	}
	r, err := e.Run(context.Background(), ask)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	printResult(r, f, io)
	if !f.quiet {
		fmt.Fprintln(io.Err, status(e, r).Line())
	}
	return r.Exit()
}

func cmdResume(args []string, io IO) int {
	f := newFlags("resume", io.Err)
	if err := f.fs.Parse(args); err != nil {
		return 2
	}
	rest := f.fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(io.Err, "@S FAIL\n@? resume needs a run-id (see isekai status)")
		return 2
	}
	e, err := f.engine(io)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	r, err := e.Resume(context.Background(), rest[0], strings.Join(rest[1:], " "))
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	printResult(r, f, io)
	return r.Exit()
}

func status(e *loop.Engine, r *loop.Result) instrument.Status {
	s := instrument.Status{Provider: e.Provider.Name(), Body: e.As}
	if r != nil {
		s.Turns, s.Steps, s.Spend, s.Context, s.Journal = r.Turns, len(r.Steps), r.Usage, r.Context, r.Journal
	}
	return s
}

func cmdRepl(args []string, io IO) int {
	f := newFlags("repl", io.Err)
	if err := f.fs.Parse(args); err != nil {
		return 2
	}
	e, err := f.engine(io)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	s := e.NewSession()
	fmt.Fprintf(io.Err, "isekai · %s · root %s · run %s · /status /quit\n", e.Provider.Name(), e.Root, s.RunID)
	sc := bufio.NewScanner(io.In)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	var last *loop.Result
	for {
		fmt.Fprint(io.Err, "> ")
		if !sc.Scan() {
			fmt.Fprintln(io.Err)
			return 0
		}
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			continue
		case line == "/quit" || line == "/exit":
			return 0
		case line == "/status":
			st := instrument.Status{Provider: e.Provider.Name(), Body: e.As, Turns: s.Turns, Steps: s.Steps, Spend: s.Spend, Context: s.Context, Journal: s.RunID}
			fmt.Fprintln(io.Out, st.Line())
			continue
		}
		last, err = s.Turn(context.Background(), line)
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
			continue
		}
		printResult(last, f, io)
		if !f.quiet {
			fmt.Fprintln(io.Err, status(e, last).Line())
		}
		if last.Status == loop.Checkpoint {
			fmt.Fprintln(io.Err, "the session hit a budget — this is a good point to stop; resume with: isekai resume "+s.RunID)
			return last.Exit()
		}
	}
}

func cmdStatus(args []string, io IO) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Err)
	root := fs.String("root", "", "world root")
	jsonOut := fs.Bool("json", false, "JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r := *root
	if r == "" {
		var ok bool
		if r, ok = FindRoot("."); !ok {
			fmt.Fprintf(io.Err, "@S FAIL\n@? no %s/ at or above the working directory\n", WorldDir)
			return 2
		}
	}
	env := io.Env
	if env == nil {
		env = os.Getenv
	}
	dir := filepath.Join(r, WorldDir, "instruments", "loop")
	runs, err := loop.Runs(dir)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	provName := "none configured"
	if p, err := newProvider("", "", env); err == nil {
		provName = p.Name()
	}
	if len(fs.Args()) > 0 {
		id := fs.Args()[0]
		ev, err := loop.ReadJournal(filepath.Join(dir, id+".jsonl"))
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? no journal %s\n", id)
			return 2
		}
		st := loop.State(filepath.Join(dir, id+".jsonl"), ev)
		if *jsonOut {
			b, _ := jsonLine(st)
			fmt.Fprintln(io.Out, b)
			return 0
		}
		fmt.Fprintf(io.Out, "@S %s run=%s as=%s turns=%d events=%d journal=%s\n", st.Status, st.ID, st.As, st.Turns, st.Events, rel(r, st.Path))
		for _, s := range st.Steps {
			fmt.Fprintf(io.Out, "@F %s — %s — %s — %s\n", s.ID, s.Status, s.Tool, s.Class)
		}
		return 0
	}
	if *jsonOut {
		b, _ := jsonLine(map[string]interface{}{"@S": "OK", "root": r, "provider": provName, "journals": rel(r, dir), "runs": runs})
		fmt.Fprintln(io.Out, b)
		return 0
	}
	fmt.Fprintf(io.Out, "@S OK root=%s provider=%s runs=%d journals=%s\n", r, provName, len(runs), rel(r, dir))
	fmt.Fprintf(io.Out, "@F keys — ANTHROPIC_API_KEY %s · OPENAI_API_KEY %s · OPENAI_BASE_URL %s · ISEKAI_MODEL %s\n", set(env("ANTHROPIC_API_KEY")), set(env("OPENAI_API_KEY")), or(env("OPENAI_BASE_URL"), "unset"), or(env("ISEKAI_MODEL"), "unset"))
	for i, run := range runs {
		if i == 20 {
			fmt.Fprintf(io.Out, "@F … %d more\n", len(runs)-20)
			break
		}
		done := 0
		for _, s := range run.Steps {
			if s.Status == "done" {
				done++
			}
		}
		ctx := ""
		if t, ok := run.Context["tokens"].(float64); ok {
			ctx = fmt.Sprintf(" — context %d", int(t))
		}
		fmt.Fprintf(io.Out, "@F %s — %s — %d/%d steps done — as %s — %s%s\n", run.ID, run.Status, done, len(run.Steps), run.As, run.At, ctx)
	}
	return 0
}

func set(v string) string {
	if v == "" {
		return "unset"
	}
	return "set"
}

func or(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}
