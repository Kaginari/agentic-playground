package onto

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const usage = `usage: isekai onto [--root DIR] <command>
  check                          load, reason, validate the shapes; findings as @F
  show                           the reasoned graph as Turtle (asserted + derived)
  project <creature> [--budget N]  plain lines a creature may see, nearest first (default budget 1000 tokens)
  assert <body> <kind> <text…>   record a Court's @U line as a Fact (kind: law|colony|territory)
`

// CLI runs `isekai onto …` and returns the exit code: 0 ok / PASS, 1 FAIL or
// refused, 2 usage or load error. Answers speak the wire.
// CLILayout is the layout the CLI walks and loads; a distribution's binary sets it.
var CLILayout = DefaultLayout()

func CLI(args []string, stdout, stderr io.Writer) int {
	root := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		switch {
		case args[0] == "--root" && len(args) > 1:
			root, args = args[1], args[2:]
		case strings.HasPrefix(args[0], "--root="):
			root, args = strings.TrimPrefix(args[0], "--root="), args[1:]
		default:
			fmt.Fprintf(stderr, "@? unknown flag %s\n%s", args[0], usage)
			return 2
		}
	}
	if len(args) == 0 {
		io.WriteString(stderr, usage)
		return 2
	}
	if root == "" {
		cwd, _ := os.Getwd()
		r, err := FindRootIn(cwd, CLILayout)
		if err != nil {
			fmt.Fprintf(stderr, "@? %s\n", err)
			return 2
		}
		root = r
	}
	w, err := LoadLayout(root, CLILayout)
	if err != nil {
		fmt.Fprintf(stderr, "@S FAIL load\n@F %s\n@E 0\n", err)
		return 2
	}
	var ans bytes.Buffer
	finish := func(out io.Writer) {
		fmt.Fprintf(&ans, "@E %d\n", ans.Len())
		io.Copy(out, &ans)
	}
	switch args[0] {
	case "check":
		fs := w.Validate()
		g := w.Graph
		status := "PASS"
		if len(fs) > 0 {
			status = "FAIL"
		}
		fmt.Fprintf(&ans, "@S %s %d findings · %d creatures · %d minds · %d facts · %d triples\n", status, len(fs),
			len(g.Instances(cCreature)), len(g.Instances(cMind)), len(g.Instances(cFact)), g.Len())
		for _, f := range fs {
			fmt.Fprintf(&ans, "@F %s\n", f)
		}
		for _, n := range w.Notes {
			fmt.Fprintf(&ans, "@? %s\n", n)
		}
		finish(stdout)
		if status == "FAIL" {
			return 1
		}
		return 0
	case "show":
		asserted := len(w.Graph.Asserted())
		fmt.Fprintf(stdout, "# %d triples: %d asserted, %d derived (never written back)\n", w.Graph.Len(), asserted, w.Graph.Len()-asserted)
		if err := Write(stdout, w.Graph.All(), w.Prefixes); err != nil {
			fmt.Fprintf(stderr, "@? %s\n", err)
			return 2
		}
		return 0
	case "project":
		rest := args[1:]
		budget := 1000
		var name string
		for i := 0; i < len(rest); i++ {
			switch {
			case rest[i] == "--budget" && i+1 < len(rest):
				n, err := strconv.Atoi(rest[i+1])
				if err != nil || n < 0 {
					fmt.Fprintf(stderr, "@? --budget wants a number of tokens, got %q\n", rest[i+1])
					return 2
				}
				budget, i = n, i+1
			case strings.HasPrefix(rest[i], "--budget="):
				n, err := strconv.Atoi(strings.TrimPrefix(rest[i], "--budget="))
				if err != nil || n < 0 {
					fmt.Fprintf(stderr, "@? --budget wants a number of tokens\n")
					return 2
				}
				budget = n
			case name == "":
				name = rest[i]
			default:
				fmt.Fprintf(stderr, "@? unexpected argument %q\n", rest[i])
				return 2
			}
		}
		if name == "" {
			fmt.Fprintf(stderr, "@? project needs a creature\n%s", usage)
			return 2
		}
		lines, err := w.Project(name, budget)
		if err != nil {
			fmt.Fprintf(stderr, "@? %s\n", err)
			return 1
		}
		for _, l := range lines {
			fmt.Fprintln(stdout, l)
		}
		return 0
	case "assert":
		if len(args) < 4 {
			fmt.Fprintf(stderr, "@? assert needs <body> <kind> <text>\n%s", usage)
			return 2
		}
		fact, err := w.AssertUnsaid(args[1], args[2], strings.Join(args[3:], " "))
		if err != nil {
			fmt.Fprintf(&ans, "@S FAIL assert\n@? %s\n", err)
			finish(stderr)
			return 1
		}
		fmt.Fprintf(&ans, "@S PASS asserted\n@F %s: %s knows %s (%s)\n", rel(root, OntologyDir(root))+"/graph/unsaid.ttl", strings.ToLower(args[1]), fact.Local(), strings.ToLower(args[2]))
		finish(stdout)
		return 0
	}
	fmt.Fprintf(stderr, "@? unknown command %q\n%s", args[0], usage)
	return 2
}
