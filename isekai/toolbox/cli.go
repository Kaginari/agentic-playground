package toolbox

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/memory"
)

// CLI is `isekai toolbox <args>`: the same commands, flags and output lines as toolbox.js.
func CLI(args []string, stdout, stderr io.Writer) int { return cli(args, stdout, stderr, "") }

func cli(args []string, stdout, stderr io.Writer, pathEnv string) (code int) {
	a := memory.ParseArgs(args, []string{"json", "map"})
	root, cmd := a.Root("status")
	w, err := Open(root)
	if err != nil {
		return memory.WriteFail(stderr, err)
	}
	w.Path = pathEnv
	defer func() {
		if r := recover(); r != nil {
			code = memory.WriteFail(stderr, failf("%s crashed: %v", cmd, r))
		}
	}()
	as := memory.NormAs(a.Or("as", ""))
	js := a.Bool["json"]
	holes := func(hs []string) {
		for _, h := range hs {
			fmt.Fprintf(stdout, "@? %s\n", h)
		}
	}
	kinds := func() ([]string, error) {
		v := a.Or("kind", "")
		if v == "" {
			return nil, nil
		}
		var ks []string
		for _, k := range strings.Split(v, ",") {
			if k = memory.JSTrim(k); k != "" {
				if !contains(Kinds, k) {
					return nil, failf("--kind must be %s, got %s", strings.Join(Kinds, "|"), k)
				}
				ks = append(ks, k)
			}
		}
		return ks, nil
	}
	pickOpts := func() (PickOpts, error) {
		o := PickOpts{As: as}
		var err error
		if o.K, err = a.PosInt("k", 5, "-k"); err != nil {
			return o, err
		}
		if o.Budget, err = a.PosInt("budget", BudgetDefault, "--budget"); err != nil {
			return o, err
		}
		o.Min = MinDefault
		if v, ok := a.Str("min"); ok {
			f, isNum := memory.JSParseFloat(v)
			if !isNum || !(f >= 0) {
				return o, failf("--min needs a score ≥ 0, got %s", v)
			}
			o.Min = f
		}
		o.Kinds, err = kinds()
		return o, err
	}
	switch cmd {
	case "index":
		r, err := w.Index()
		if err != nil {
			return memory.WriteFail(stderr, err)
		}
		if js {
			memory.JSONLine(stdout, r.JSON(w))
			return 0
		}
		fmt.Fprintf(stdout, "@S INDEXED %d entries — %s → %s\n", r.Reg.N, kindsLine(r.ByKind), w.rel(w.RegistryPath()))
		holes(r.Holes)
	case "status":
		budget := float64(BudgetDefault)
		if v, ok := a.Str("budget"); ok {
			budget, _ = memory.JSParseInt(v)
		}
		r := w.Status(budget)
		if js {
			memory.JSONLine(stdout, r.JSON(w))
			return 0
		}
		printStatus(stdout, w, r)
	case "pick", "brief":
		q := memory.JSTrim(strings.Join(a.Pos, " "))
		if q == "" {
			return memory.WriteFail(stderr, failf("%s needs the turn's ask", cmd))
		}
		o, err := pickOpts()
		if err != nil {
			return memory.WriteFail(stderr, err)
		}
		if cmd == "brief" {
			b, err := w.Brief(q, o)
			if err != nil {
				return memory.WriteFail(stderr, err)
			}
			if js {
				memory.JSONLine(stdout, b.JSON())
				return 0
			}
			fmt.Fprintln(stdout, b.Head)
			for _, l := range b.Lines {
				fmt.Fprintln(stdout, l)
			}
			holes(b.Holes)
			return 0
		}
		o.Cmd = "pick"
		P, err := w.DoPick(q, o)
		if err != nil {
			return memory.WriteFail(stderr, err)
		}
		if js {
			memory.JSONLine(stdout, P.PickJSON())
			return 0
		}
		for _, l := range P.PickLines() {
			fmt.Fprintln(stdout, l)
		}
		holes(P.Holes)
		fmt.Fprintf(stdout, "@E %d\n", P.PickBytes())
	case "load":
		name := ""
		if len(a.Pos) > 0 {
			name = a.Pos[0]
		}
		o := LoadOpts{As: as, Kind: a.Or("kind", ""), Map: a.Bool["map"]}
		if v, ok := a.Str("sec"); ok {
			n, isNum := memory.JSParseInt(v)
			if !isNum || !(n >= 0) {
				return memory.WriteFail(stderr, failf("--sec needs a section number (0 = preamble), got %s", v))
			}
			sec := int(math.Min(n, 1<<31))
			o.Sec = &sec
		}
		r, err := w.Load(name, o)
		if err != nil {
			return memory.WriteFail(stderr, err)
		}
		e := r.Entry
		if r.IsMap {
			if js {
				secs := []memory.OJ{}
				for _, s := range r.Map.Sections {
					secs = append(secs, memory.OJ{memory.P("n", s.N), memory.P("title", s.T), memory.P("depth", s.Depth), memory.P("bytes", s.B), memory.P("tokens", TOK(s.Text))})
				}
				memory.JSONLine(stdout, memory.OJ{memory.P("@S", "MAP"), memory.P("name", e.Name), memory.P("kind", e.Kind), memory.P("path", e.Path), memory.P("preambleBytes", len(r.Map.Preamble)), memory.P("sections", secs), memory.P("@?", r.Holes)})
				return 0
			}
			fmt.Fprintf(stdout, "@S MAP %s sections=%d preamble=%dtok — %s\n", e.Name, len(r.Map.Sections), TOK(r.Map.Preamble), e.PathStr())
			rows := [][]any{}
			for _, s := range r.Map.Sections {
				fmt.Fprintf(stdout, "@F #%d — %s %s — %dtok\n", s.N, strings.Repeat("#", s.Depth), s.T, TOK(s.Text))
				rows = append(rows, []any{s.N, s.T, s.B})
			}
			holes(r.Holes)
			b, _ := memory.MarshalJS(rows)
			fmt.Fprintf(stdout, "@E %d\n", len(b))
			return 0
		}
		if js {
			memory.JSONLine(stdout, memory.OJ{memory.P("@S", "LOAD"), memory.P("name", e.Name), memory.P("kind", e.Kind), memory.P("path", e.Path), memory.P("sec", r.Sec), memory.P("tokens", r.Tokens), memory.P("text", r.Text), memory.P("@?", r.Holes)})
			return 0
		}
		fmt.Fprintf(stdout, "@S LOAD %s %s sec=%v tokens=%d — %s\n", e.Kind, e.Name, r.Sec, r.Tokens, pathOr(e.PathStr()))
		fmt.Fprintln(stdout, r.Text)
		holes(r.Holes)
		fmt.Fprintf(stdout, "@E %d\n", len(r.Text))
	case "explain":
		name := ""
		if len(a.Pos) > 0 {
			name = a.Pos[0]
		}
		e, hs, err := w.Explain(name, a.Or("kind", ""))
		if err != nil {
			return memory.WriteFail(stderr, err)
		}
		if js {
			memory.JSONLine(stdout, ExplainJSON(e, hs))
			return 0
		}
		printExplain(stdout, e, hs)
	case "selftest":
		if _, err := SelftestIn(w.Root, stdout); err != nil {
			if _, ok := err.(*Fail); ok {
				return memory.WriteFail(stderr, err)
			}
			return 1
		}
	default:
		return memory.WriteFail(stderr, failf("unknown command %s — index | status | pick | brief | load | explain | selftest", cmd))
	}
	return 0
}

func printStatus(out io.Writer, w *World, r *StatusReport) {
	reg := r.Loaded.Reg
	fmt.Fprintf(out, "toolbox — %s\n", r.World)
	line := fmt.Sprintf("REGISTRY %d entries — %s", reg.N, kindsLine(r.ByKind))
	if r.Loaded.Live {
		line += "  (live harvest)"
	} else {
		line += "  (built " + reg.BuiltAt
		if reg.Head != nil && *reg.Head != "" {
			line += " @ " + memory.UTF16Slice(*reg.Head, 0, 7)
		}
		line += ")"
		if r.Stale {
			line += " — STALE"
		}
	}
	fmt.Fprintln(out, line)
	pct := memory.JSRound(r.Resident / r.Budget * 100)
	fmt.Fprintf(out, "COST     resident if all injected ≈ %s tok vs budget %s (%s%%) · full if all loaded ≈ %s tok\n", memory.Locale(r.Resident), memory.Locale(r.Budget), memory.JSNum(pct), memory.Locale(r.Full))
	if r.Ext > 0 {
		line = fmt.Sprintf("EXTERNAL %d installed", r.Ext-len(r.Missing))
		if len(r.Missing) > 0 {
			line += " · missing: " + strings.Join(r.Missing, ", ")
		}
	} else {
		line = fmt.Sprintf("EXTERNAL none — %s is empty or absent", w.rel(w.ExtraPath()))
	}
	fmt.Fprintln(out, line)
	i := r.Ins
	fmt.Fprintf(out, "LOADS    offered %d (%d distinct, %d pick%s) · loaded %d (%d distinct) · resident cost %s tok · loaded cost %s tok  (%s)\n",
		i.Offered, i.OfferedDistinct, i.Offers, plural(i.Offers), i.Loads, i.LoadedDistinct, memory.Locale(i.ResidentTokens), memory.Locale(i.LoadedTokens), i.Src)
	for _, h := range r.Holes {
		fmt.Fprintf(out, "@? %s\n", h)
	}
}

func printExplain(out io.Writer, e *Entry, holes []string) {
	fmt.Fprintf(out, "@S EXPLAIN %s %s\n", e.Kind, e.Name)
	line := "@F path — " + pathOr(e.PathStr())
	if len(e.Srcs) > 1 {
		line += " (also " + strings.Join(e.Srcs[1:], ", ") + ")"
	}
	fmt.Fprintln(out, line)
	l2 := ""
	if e.Kind == "tool" {
		l2 = fmt.Sprintf(": header; file %dtok", e.FileTokens)
	} else if e.Sections != 0 {
		l2 = fmt.Sprintf(": %d sections, --sec N", e.Sections)
	}
	fmt.Fprintf(out, "@F cost — resident %stok (level 1: name + description) · load %stok (level 2%s)\n", memory.JSNum(e.Cost.Resident), memory.JSNum(e.Cost.Full), l2)
	line = "@F serves — " + e.Serves
	if len(e.Wearers) > 0 {
		line += " · worn by " + strings.Join(e.Wearers, ", ")
	}
	if e.Mode != nil && *e.Mode != "" {
		line += " · mode " + *e.Mode
	}
	if e.Model != nil && *e.Model != "" {
		line += " · model " + *e.Model
	}
	if e.Kind == "external" {
		inst := "undefined"
		if e.Installed != nil {
			inst = fmt.Sprint(*e.Installed)
		}
		line += " · installed " + inst
	}
	fmt.Fprintln(out, line)
	var ts []string
	for _, t := range e.Triggers {
		ts = append(ts, fmt.Sprintf("\"%s\" (%s)", t.T, t.Src))
	}
	fmt.Fprintf(out, "@F triggers — %s\n", strings.Join(ts, " · "))
	fmt.Fprintf(out, "@F description — %s\n", memory.CollapseWS(e.Description))
	for _, h := range holes {
		fmt.Fprintf(out, "@? %s\n", h)
	}
	fmt.Fprintf(out, "@E %d\n", len(e.Description))
}
