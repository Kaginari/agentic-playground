package memory

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Args is the JS tools' argv shape: `--x v` takes the next element whatever it is, listed
// booleans take none, `-k v` is the one short flag, everything else is positional.
type Args struct {
	Flags map[string]string
	Set   map[string]bool // present with a value (a trailing `--x` is present but undefined)
	Bool  map[string]bool
	Pos   []string
}

func ParseArgs(argv []string, bools []string) *Args {
	a := &Args{Flags: map[string]string{}, Set: map[string]bool{}, Bool: map[string]bool{}}
	isBool := map[string]bool{}
	for _, b := range bools {
		isBool[b] = true
	}
	for i := 0; i < len(argv); i++ {
		s := argv[i]
		switch {
		case strings.HasPrefix(s, "--") && isBool[s[2:]]:
			a.Bool[s[2:]] = true
		case s == "-k" || strings.HasPrefix(s, "--"):
			k := "k"
			if s != "-k" {
				k = s[2:]
			}
			if i+1 < len(argv) {
				i++
				a.Flags[k], a.Set[k] = argv[i], true
			} else {
				a.Set[k] = false
			}
		default:
			a.Pos = append(a.Pos, s)
		}
	}
	return a
}

// Str is flags.x: the value, and whether it is defined.
func (a *Args) Str(k string) (string, bool) { return a.Flags[k], a.Set[k] }

// Or is `flags.x || dflt`.
func (a *Args) Or(k, dflt string) string {
	if v, ok := a.Str(k); ok && v != "" {
		return v
	}
	return dflt
}

// PosInt parses `-k`-style flags the JS way: undefined → dflt; else parseInt, and !(n > 0) fails.
func (a *Args) PosInt(k string, dflt int, what string) (int, error) {
	v, ok := a.Str(k)
	if !ok {
		return dflt, nil
	}
	n, isNum := JSParseInt(v)
	if !isNum || !(n > 0) {
		return 0, failf("%s needs a positive integer, got %s", what, v)
	}
	if n > 1<<31 {
		n = 1 << 31
	}
	return int(n), nil
}

// Root takes a leading positional root (one with a .isekai/), else cwd; then the command.
func (a *Args) Root(dflt string) (root, cmd string) {
	root = os.Getenv("PWD")
	if wd, err := os.Getwd(); err == nil {
		root = wd
	}
	if len(a.Pos) > 0 && Exists(filepath.Join(a.Pos[0], ".isekai")) {
		root, _ = filepath.Abs(a.Pos[0])
		a.Pos = a.Pos[1:]
	}
	cmd = dflt
	if len(a.Pos) > 0 {
		cmd, a.Pos = a.Pos[0], a.Pos[1:]
	}
	return root, cmd
}

// WriteFail prints a Fail on the wire (stderr) and returns the exit code 2.
func WriteFail(stderr io.Writer, err error) int {
	holes := []string{err.Error()}
	if f, ok := err.(*Fail); ok {
		holes = f.Holes
	}
	lines := []string{"@S FAIL"}
	for _, h := range holes {
		lines = append(lines, "@? "+h)
	}
	fmt.Fprintln(stderr, strings.Join(lines, "\n"))
	return 2
}

// CLI is `isekai memory <args>`: the same commands, flags and output lines as memory.js.
func CLI(args []string, stdout, stderr io.Writer) int { return cli(args, stdout, stderr, "") }

func cli(args []string, stdout, stderr io.Writer, home string) (code int) {
	a := ParseArgs(args, []string{"json", "machine", "no-cache", "short"})
	root, cmd := a.Root("status")
	w, err := Open(root)
	if err != nil {
		return WriteFail(stderr, err)
	}
	if home != "" {
		w.Home = home
	}
	defer func() {
		if r := recover(); r != nil {
			code = WriteFail(stderr, failf("%s crashed: %v", cmd, r))
		}
	}()
	as := NormAs(a.Or("as", ""))
	js := a.Bool["json"]
	holes := func(hs []string) {
		for _, h := range hs {
			fmt.Fprintf(stdout, "@? %s\n", h)
		}
	}
	switch cmd {
	case "status":
		r := w.Status(as)
		if js {
			JSONLine(stdout, r)
			return 0
		}
		printStatus(stdout, r)
	case "index":
		r, err := w.Index()
		if err != nil {
			return WriteFail(stderr, err)
		}
		if js {
			JSONLine(stdout, r)
			return 0
		}
		fmt.Fprintf(stdout, "@S INDEXED %d memories — episodic %d · procedural %d · semantic %d → %s\n", r.N, r.Episodic, r.Procedural, r.Semantic, r.Src)
		holes(r.Holes)
	case "recall":
		q := JSTrim(strings.Join(a.Pos, " "))
		if q == "" {
			return WriteFail(stderr, failf("recall needs a question"))
		}
		K, err := a.PosInt("k", 5, "-k")
		if err != nil {
			return WriteFail(stderr, err)
		}
		r, err := w.Recall(q, RecallOpts{As: as, Tier: a.Or("tier", ""), Kind: a.Or("kind", ""), K: K, NoCache: a.Bool["no-cache"]})
		if err != nil {
			return WriteFail(stderr, err)
		}
		if js {
			JSONLine(stdout, r)
			return 0
		}
		printRecall(stdout, r)
	case "remember":
		text := JSTrim(strings.Join(a.Pos, " "))
		if text == "" {
			return WriteFail(stderr, failf("remember needs a note"))
		}
		r, err := w.Remember(text, RememberOpts{As: as, Machine: a.Bool["machine"], Tag: a.Or("tag", ""), Kind: a.Or("kind", "")})
		if err != nil {
			return WriteFail(stderr, err)
		}
		if js {
			JSONLine(stdout, r)
			return 0
		}
		fmt.Fprintf(stdout, "@S REMEMBERED %s → %s\n", r.Scope, r.Src)
		holes(r.Holes)
	case "forget":
		if !a.Bool["short"] {
			return WriteFail(stderr, failf("forget only clears SHORT memory (--short); long and shared are records (Law 4)"))
		}
		r, err := w.Forget(as)
		if err != nil {
			return WriteFail(stderr, err)
		}
		if js {
			JSONLine(stdout, r)
			return 0
		}
		fmt.Fprintf(stdout, "@S FORGOT %d cached recall%s for %s\n", r.N, plural(r.N), r.As)
	case "selftest":
		n, err := SelftestIn(w.Root, stdout)
		if err != nil {
			if _, ok := err.(*Fail); ok {
				return WriteFail(stderr, err)
			}
			return 1
		}
		_ = n
	default:
		return WriteFail(stderr, failf("unknown command %s — status | index | recall | remember | forget | selftest", cmd))
	}
	return 0
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func printStatus(out io.Writer, r *StatusReport) {
	c, wm := r.Short.ContextWindow, r.Short.WorkingMemory
	fmt.Fprintf(out, "memory — %s · as %s\n", r.World, r.As)
	if c.Available {
		fmt.Fprintf(out, "SHORT   context %s / %s tok (%s)\n", Locale(float64(c.Tokens)), Locale(float64(c.Limit)), c.Zone)
	} else {
		fmt.Fprintf(out, "SHORT   context @? %s\n", c.Why)
	}
	line := fmt.Sprintf("        working %d desk%s", wm.Desks, plural(wm.Desks))
	if len(wm.Stressed) > 0 {
		line += " · STRESSED: " + strings.Join(wm.Stressed, ", ")
	}
	if len(wm.AtLimit) > 0 {
		line += " · at limit: " + strings.Join(wm.AtLimit, ", ")
	}
	fmt.Fprintln(out, line)
	sc := r.Short.SemanticCache
	fmt.Fprintf(out, "        cache   %d recall%s cached, %d hit%s  (%s)\n", sc.Entries, plural(sc.Entries), sc.Hits, plural(sc.Hits), sc.Src)
	if r.Long.Indexed {
		line = fmt.Sprintf("LONG    episodic %d · procedural %d · semantic %d  (built %s", *r.Long.Episodic, *r.Long.Procedural, *r.Long.Semantic, *r.Long.BuiltAt)
		if r.Long.Head != nil && *r.Long.Head != "" {
			line += " @ " + UTF16Slice(*r.Long.Head, 0, 7)
		}
		line += ")"
		if r.Long.Stale {
			line += " — STALE"
		}
		fmt.Fprintln(out, line)
	} else {
		fmt.Fprintf(out, "LONG    @? %s\n", r.Why)
	}
	u := r.Shared.Unsaid
	fmt.Fprintf(out, "SHARED  world %d · machine %d · unsaid law %d · colony %d · territory %d\n", r.Shared.World, r.Shared.Machine, u.Law, u.Colony, u.Territory)
	for _, h := range r.Holes {
		fmt.Fprintf(out, "@? %s\n", h)
	}
}

func printRecall(out io.Writer, r *RecallResult) {
	if r.S == "HIT" {
		fmt.Fprintf(out, "@S HIT cache≈%s as=%s k=%d\n", ToFixedStr(r.hitSim, 2), r.As, len(r.Results))
	} else {
		fmt.Fprintf(out, "@S MISS as=%s k=%d\n", r.As, len(r.Results))
	}
	for _, x := range r.Results {
		fmt.Fprint(out, FormatResult(x)+"\n")
	}
	for _, h := range r.Holes {
		fmt.Fprintf(out, "@? %s\n", h)
	}
	fmt.Fprintf(out, "@E %d\n", r.Bytes())
}

// FormatResult is one `@F` line of a recall.
func FormatResult(x Result) string {
	sec := ""
	if x.Sec != 0 {
		sec = "#" + JSNum(float64(x.Sec))
	}
	extra := ""
	if x.Rel != 0 {
		extra += " +rel " + JSNum(x.Rel)
	}
	if x.Rec != 0 {
		extra += " +rec " + JSNum(x.Rec)
	}
	return fmt.Sprintf("@F %s%s — %s (sim %s%s) — %s — %s", x.Src, sec, JSNum(x.Score), JSNum(x.Sim), extra, x.Kind, x.Title)
}
