package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/memory"
	"github.com/Kaginari/agentic-playground/isekai/onto"
	"github.com/Kaginari/agentic-playground/isekai/toolbox"
	"github.com/Kaginari/agentic-playground/isekai/world"
)

// IO is what the CLI talks through; tests swap it.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	Env func(string) string
}

// cliFlags are the binary's own flags, taken before config's ParseFlags reads the rest.
type cliFlags struct {
	root, dist, session, format string
	json, quiet, noBoard, bench  bool
	since                        string
}

func splitFlags(args []string) (cliFlags, []string) {
	var f cliFlags
	var rest []string
	take := func(i *int, name string) string {
		a := args[*i]
		if eq := strings.IndexByte(a, '='); eq > 0 {
			return a[eq+1:]
		}
		if *i+1 < len(args) {
			*i++
			return args[*i]
		}
		return ""
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := a
		if eq := strings.IndexByte(a, '='); eq > 0 {
			name = a[:eq]
		}
		switch name {
		case "--root", "-root":
			f.root = take(&i, name)
		case "--dist":
			f.dist = take(&i, name)
		case "--session", "-session":
			f.session = take(&i, name)
		case "--since":
			f.since = take(&i, name)
		case "--json", "-json":
			f.json = true
		case "--quiet", "-quiet", "-q":
			f.quiet = true
		case "--no-board":
			f.noBoard = true
		case "--bench":
			f.bench = true
		default:
			rest = append(rest, a)
		}
	}
	return f, rest
}

var commands = []struct{ name, summary string }{
	{"repl", "a live session (the default)"},
	{"run", "run one ask to its end: run [--json] [--format text|json|wire] \"<ask>\""},
	{"resume", "resume a session: resume <id> [ask]"},
	{"sessions", "list the sessions of this world"},
	{"status", "the honesty rule and the instrument board"},
	{"config", "config show [--yaml] | explain | check | path | patch"},
	{"memory", "the memory instrument (status, index, recall, remember, forget)"},
	{"toolbox", "the toolbox instrument (index, brief, pick, load, status)"},
	{"onto", "the ontology (check, project, query)"},
	{"usage", "the usage journal: usage [--session <id>] [--since <date>]"},
	{"bench", "a fixed task set on every configured model (mock always; real providers with keys)"},
	{"selftest", "every package's selftest, one @S PASS n checks"},
	{"init", "found a world here: init [--bench]"},
	{"board", "serve the board without a session"},
	{"version", "print the version"},
	{"help", "this list"},
}

// Main is the entry of both binaries: dist names the distribution the binary was built as.
func Main(dist string, args []string, io IO, v Version) int {
	if io.Env == nil {
		io.Env = os.Getenv
	}
	if io.In == nil {
		io.In = os.Stdin
	}
	if io.Out == nil {
		io.Out = os.Stdout
	}
	if io.Err == nil {
		io.Err = os.Stderr
	}
	name, args := command(args)
	switch name {
	case "version":
		fmt.Fprintln(io.Out, versionLine(dist, v))
		return 0
	case "help", "-h", "--help":
		fmt.Fprintf(io.Out, "%s [command] [flags]\n", dist)
		for _, c := range commands {
			fmt.Fprintf(io.Out, "  %-9s %s\n", c.name, c.summary)
		}
		fmt.Fprintln(io.Out, "flags: --root <dir> --model <provider/id> --approve <classes> --dry-run --strict --set key=value --no-<feature> --format text|json|wire --json --quiet --session <id>")
		return 0
	}
	f, rest := splitFlags(args)
	if f.dist != "" {
		dist = f.dist
	}
	overrides, leftover, err := config.ParseFlags(rest)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if f.json {
		overrides = append(overrides, config.Override{Path: "output.format", Value: "json", Flag: "--json"})
	}
	if f.format != "" {
		overrides = append(overrides, config.Override{Path: "output.format", Value: f.format, Flag: "--format"})
	}
	opt := Options{Dist: dist, Root: f.root, Env: io.Env, Flags: overrides, In: io.In, Out: io.Out, Err: io.Err, Version: v, Session: f.session, Quiet: f.quiet, NoBoard: f.noBoard}
	if opt.Root != "" {
		opt.Cwd = opt.Root
	}
	switch name {
	case "init":
		return cmdInit(dist, f, io)
	case "config":
		cargs := []string{"--dist", dist}
		if f.root != "" {
			cargs = append(cargs, "--root", f.root)
		}
		return config.CLI(append(cargs, rest...), io.Out, io.Err)
	case "memory", "toolbox", "onto":
		return cmdInstrument(name, dist, f, leftover, io)
	case "selftest":
		return Selftest(dist, io)
	}
	a, err := New(opt)
	if err != nil {
		if name == "status" || name == "sessions" || name == "usage" {
			opt.NoWorld = true
			if a2, err2 := New(opt); err2 == nil {
				a = a2
				a.hole(err.Error())
			} else {
				fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
				return 2
			}
		} else {
			fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
			return 2
		}
	}
	defer a.Close()
	ctx := context.Background()
	switch name {
	case "repl":
		a.startBoard(ctx)
		return a.REPL(ctx)
	case "run":
		return a.cmdRun(ctx, strings.TrimSpace(strings.Join(leftover, " ")), io)
	case "resume":
		if len(leftover) == 0 {
			fmt.Fprintf(io.Err, "@S FAIL\n@? resume needs a session id (see `%s sessions`)\n", dist)
			return 2
		}
		return a.cmdResume(ctx, leftover[0], strings.TrimSpace(strings.Join(leftover[1:], " ")), io)
	case "sessions":
		return a.cmdSessions(io)
	case "status":
		return a.cmdStatus(io)
	case "usage":
		return a.cmdUsage(f, io)
	case "bench":
		return a.Bench(ctx, io)
	case "board":
		return a.cmdBoard(ctx, io)
	}
	fmt.Fprintf(io.Err, "@S FAIL\n@? unknown command %q — `%s help` lists them\n", name, dist)
	return 2
}

// valueFlags are the config flags that take a value in the next token.
var valueFlags = map[string]bool{"--model": true, "--mode": true, "--approve": true, "--format": true, "--max-steps": true, "--budget": true, "--small-model": true, "--log-level": true, "--profile": true, "--set": true,
	"--root": true, "-root": true, "--dist": true, "--session": true, "-session": true, "--since": true}

// command finds the subcommand in an argv where flags may come first (`isekai --root x run
// "…"`), and returns the argv without it. No command means the REPL.
func command(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if valueFlags[a] {
				i++
			}
			continue
		}
		return a, append(append([]string(nil), args[:i]...), args[i+1:]...)
	}
	return "repl", args
}

func versionLine(dist string, v Version) string {
	return fmt.Sprintf("%s %s (%s, %s, %s/%s)", dist, orStr(v.Version, "dev"), orStr(v.Commit, "none"), orStr(v.Date, "unknown"), runtime.GOOS, runtime.GOARCH)
}

func cmdInit(dist string, f cliFlags, io IO) int {
	root := f.root
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)
	made, err := Init(dist, root)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? init: %v\n", err)
		return 2
	}
	if len(made) == 0 {
		fmt.Fprintf(io.Out, "@S OK %s already founded at %s — nothing touched\n", dist, root)
		return 0
	}
	fmt.Fprintf(io.Out, "@S OK %s founded at %s\n", dist, root)
	for _, m := range made {
		fmt.Fprintln(io.Out, "@F "+m)
	}
	if f.bench {
		fmt.Fprintln(io.Out, "@F bench: config comes from "+strings.ToUpper(strings.ReplaceAll(dist, "-", "_"))+"_CONFIG_CONTENT; the gate needs a TTY or a file-layer pre-approval")
	}
	return 0
}

// cmdInstrument delegates to the memory, toolbox and ontology CLIs under the distribution's
// world dir and root.
func cmdInstrument(name, dist string, f cliFlags, args []string, io IO) int {
	d, err := parseDist(dist)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	memory.DefaultWorldDir = d.WorldDir
	root := f.root
	if root == "" {
		cwd, _ := os.Getwd()
		if r, ok := findWorld(cwd, d.WorldDir); ok {
			root = r
		}
	}
	switch name {
	case "memory":
		if root != "" {
			args = append([]string{root}, args...)
		}
		return memory.CLI(args, io.Out, io.Err)
	case "toolbox":
		if root != "" {
			args = append([]string{root}, args...)
		}
		return toolbox.CLI(args, io.Out, io.Err)
	default:
		lex := lexiconFor(d.Name)
		onto.CLILayout = lex.Layout()
		if root != "" {
			args = append([]string{"--root", root}, args...)
		}
		return onto.CLI(args, io.Out, io.Err)
	}
}

func findWorld(dir, worldDir string) (string, bool) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if st, err := os.Stat(filepath.Join(d, worldDir)); err == nil && st.IsDir() {
			return d, true
		}
		up := filepath.Dir(d)
		if up == d {
			return "", false
		}
		d = up
	}
}

// cmdRun is the non-interactive form: stdin (when not a TTY) is appended to the ask; the
// answer is printed in the output format; the exit code is the loop's.
func (a *App) cmdRun(ctx context.Context, ask string, io IO) int {
	if f, ok := io.In.(*os.File); !ok || f != os.Stdin || !isTerminal(f) {
		if extra, _ := readAllLimited(io.In, 4<<20); strings.TrimSpace(extra) != "" {
			ask = strings.TrimSpace(ask + "\n" + extra)
		}
	}
	if ask == "" {
		fmt.Fprintf(io.Err, "@S FAIL\n@? run needs an ask: %s run \"<ask>\"\n", a.Cfg.Dist.Name)
		return 2
	}
	e, err := a.Engine()
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	a.Hooks.SessionStart(ctx)
	if why := a.Hooks.UserPrompt(ctx, ask); why != "" {
		fmt.Fprintf(io.Err, "@S FAIL\n@? refused by a userPrompt hook: %s\n", why)
		return 2
	}
	if a.Cfg.Output.Stream && a.Cfg.Output.Format == "text" {
		e.OnDelta = func(t string) { fmt.Fprint(io.Out, t) }
	}
	s := e.NewSession()
	r, err := s.Turn(ctx, ask)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if e.OnDelta != nil && strings.TrimSpace(r.Text) != "" {
		fmt.Fprintln(io.Out)
	}
	a.printResult(r, e.OnDelta != nil)
	a.Sessions.Sync(a.SessionID, s, a.mountModel.Ref.Model)
	if r.Verdict != "" {
		a.record(s, "verdict", map[string]interface{}{"verdict": r.Verdict, "wrote": s.Wrote})
	}
	if !a.Opt.Quiet {
		fmt.Fprintln(io.Err, a.Court.StatusLine(s))
	}
	return r.Exit()
}

func (a *App) cmdResume(ctx context.Context, id, ask string, io IO) int {
	e, err := a.Engine()
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	s := e.NewSession()
	if err := a.loadSession(s, id); err != nil {
		// a loop transcript (a checkpointed run) is the other thing a session can resume from
		if r, err2 := e.Resume(ctx, id, ask); err2 == nil {
			a.printResult(r, false)
			return r.Exit()
		}
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if ask == "" {
		ask = "resumed — continue from where the session stopped"
	}
	if io.In != nil {
		if f, ok := io.In.(*os.File); ok && f == os.Stdin && isTerminal(f) {
			fmt.Fprintf(io.Err, "resumed %s (%d messages)\n", id, len(s.Messages))
			return a.REPL(ctx)
		}
	}
	r, err := s.Turn(ctx, ask)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	a.printResult(r, false)
	a.Sessions.Sync(a.SessionID, s, a.mountModel.Ref.Model)
	return r.Exit()
}

func (a *App) cmdSessions(io IO) int {
	if a.Sessions == nil {
		fmt.Fprintln(io.Err, "@S FAIL\n@? sessions are off (sessions.enabled)")
		return 2
	}
	list, err := a.Sessions.List()
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if len(list) == 0 {
		fmt.Fprintf(io.Out, "no sessions under %s\n", a.Sessions.Dir)
		return 0
	}
	for _, i := range list {
		fmt.Fprintln(io.Out, i.Line())
	}
	return 0
}

func (a *App) cmdStatus(io IO) int {
	if a.World == nil {
		for _, l := range a.Cfg.StatusLines() {
			fmt.Fprintln(io.Out, l)
		}
		for _, h := range a.Holes() {
			fmt.Fprintln(io.Out, "@? "+h)
		}
		return 0
	}
	for _, l := range a.StatusLines() {
		fmt.Fprintln(io.Out, l)
	}
	fmt.Fprintf(io.Out, "world: %s · %s · %d creatures · %d minds · %d commands · %d foreign bodies\n", a.Root, a.Lex.Vocab, len(a.World.Creatures), len(a.Found.Skills), len(a.Found.Commands), len(a.Found.Agents))
	if dir := a.journalDir(); dir != "-" {
		if dir == "" {
			dir = filepath.Join(a.Root, a.Cfg.Dist.WorldDir, "instruments", "loop")
		}
		runs, _ := loop.Runs(dir)
		fmt.Fprintf(io.Out, "runs: %d journals under %s\n", len(runs), relOrAbs(a.Root, dir))
		for i, r := range runs {
			if i == 10 {
				fmt.Fprintf(io.Out, "  … %d more\n", len(runs)-10)
				break
			}
			fmt.Fprintf(io.Out, "  %s — %s — as %s — %d steps\n", r.ID, r.Status, r.As, len(r.Steps))
		}
	}
	if a.Sessions != nil {
		list, _ := a.Sessions.List()
		fmt.Fprintf(io.Out, "sessions: %d under %s\n", len(list), a.Sessions.Dir)
	}
	if bl := a.boardLine(); bl != "" {
		fmt.Fprintln(io.Out, bl)
	}
	return 0
}

func (a *App) cmdUsage(f cliFlags, io IO) int {
	dir := filepath.Join(a.Root, a.Cfg.Dist.WorldDir, "instruments", "usage")
	recs, err := ReadUsage(dir)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	var since time.Time
	if f.since != "" {
		for _, layout := range []string{"2006-01-02", time.RFC3339} {
			if t, err := time.Parse(layout, f.since); err == nil {
				since = t
				break
			}
		}
		if since.IsZero() {
			fmt.Fprintf(io.Err, "@S FAIL\n@? --since wants YYYY-MM-DD or RFC3339, got %q\n", f.since)
			return 2
		}
	}
	if len(recs) == 0 {
		fmt.Fprintf(io.Out, "no usage journaled under %s\n", relOrAbs(a.Root, dir))
		return 0
	}
	for _, l := range RollupOf(recs, since, f.session).Lines() {
		fmt.Fprintln(io.Out, l)
	}
	return 0
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func readAllLimited(r io.Reader, n int64) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, n))
	return string(b), err
}

// ranksSummary is one line per rank for the human.
func ranksSummary(rs world.Ranks) string {
	var names []string
	for _, r := range rs {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
