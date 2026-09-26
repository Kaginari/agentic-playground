package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/sandbox"
)

// GitOptions is tools.git.
type GitOptions struct {
	Enabled bool
	Binary  string           // default "git"
	Timeout time.Duration    // default 2 min
	Sandbox *sandbox.Sandbox // nil: no sandbox; env still scrubbed
}

var gitReadSubs = map[string]bool{"status": true, "diff": true, "log": true, "show": true, "blame": true, "rev-parse": true, "ls-files": true, "describe": true, "cat-file": true, "shortlog": true, "grep": true, "count-objects": true, "rev-list": true, "branch": true, "remote": true, "tag": true, "stash": true, "config": true, "help": true, "version": true, "reflog": true, "name-rev": true, "ls-tree": true, "diff-tree": true, "check-ignore": true, "merge-base": true, "worktree": true}
var gitWriteSubs = map[string]bool{"add": true, "commit": true, "checkout": true, "switch": true, "merge": true, "init": true, "apply": true, "cherry-pick": true, "notes": true, "restore": true, "revert": true, "mv": true, "am": true}
var gitOutwardSubs = map[string]bool{"push": true, "fetch": true, "pull": true, "clone": true, "ls-remote": true, "submodule": true}
var gitDestructiveSubs = map[string]bool{"reset": true, "clean": true, "filter-branch": true, "filter-repo": true, "rebase": true, "gc": true, "prune": true, "rm": true}

// ClassifyGit settles the class of a git invocation from its subcommand and flags; the shell
// classifier's git rules are applied on top, so the two never disagree downward.
func (e Env) ClassifyGit(args []string) Classification {
	sub, flags := "", []string{}
	for i, a := range args {
		if !strings.HasPrefix(a, "-") && sub == "" {
			sub = a
			flags = args[i+1:]
			break
		}
	}
	c := Classification{Class: Read, Why: "git " + sub + " (read)"}
	has := func(fs ...string) bool {
		for _, f := range flags {
			for _, want := range fs {
				if f == want || strings.HasPrefix(f, want+"=") {
					return true
				}
			}
		}
		return false
	}
	switch {
	case sub == "":
		c = Classification{Class: Read, Why: "git (no subcommand)"}
	case gitDestructiveSubs[sub]:
		c = Classification{Class: Destructive, Why: "git " + sub + " rewrites history or the tree"}
	case sub == "push" && (has("--force", "-f", "--force-with-lease") || hasPlusRef(flags)):
		c = Classification{Class: Destructive, Why: "git push --force"}
	case sub == "branch" && has("-d", "-D", "-M", "-m", "--delete", "--move"):
		c = Classification{Class: Destructive, Why: "git branch delete/rename"}
	case sub == "tag" && has("-d", "--delete"):
		c = Classification{Class: Destructive, Why: "git tag delete"}
	case sub == "stash" && len(flags) > 0 && (flags[0] == "drop" || flags[0] == "clear" || flags[0] == "pop"):
		c = Classification{Class: Destructive, Why: "git stash " + flags[0]}
	case (sub == "checkout" || sub == "restore") && (has("--", ".") || (len(flags) > 0 && flags[len(flags)-1] == ".")):
		c = Classification{Class: Destructive, Why: "git " + sub + " discards working changes"}
	case sub == "remote" && len(flags) > 0 && flags[0] != "-v" && flags[0] != "show" && flags[0] != "get-url":
		c = Classification{Class: Outward, Why: "git remote " + flags[0]}
	case gitOutwardSubs[sub]:
		c = Classification{Class: Outward, Why: "git " + sub + " ↔ remote"}
	case sub == "worktree" && len(flags) > 0 && flags[0] != "list":
		c = Classification{Class: Write, Why: "git worktree " + flags[0]}
	case sub == "stash" && (len(flags) == 0 || (flags[0] != "list" && flags[0] != "show")):
		c = Classification{Class: Write, Why: "git stash"}
	case sub == "tag" && len(flags) > 0 && !has("-l", "--list") && !strings.HasPrefix(flags[0], "-"):
		c = Classification{Class: Write, Why: "git tag create"}
	case sub == "branch" && len(flags) > 0 && !has("-a", "-r", "-v", "-vv", "--list", "-l", "--show-current"):
		c = Classification{Class: Write, Why: "git branch create"}
	case sub == "config" && !has("--get", "--list", "-l", "--get-all", "--get-regexp"):
		c = Classification{Class: Write, Why: "git config set"}
	case gitWriteSubs[sub]:
		c = Classification{Class: Write, Why: "git " + sub + " (local write)"}
	case gitReadSubs[sub]:
	default:
		c = Classification{Class: Write, Why: "git " + sub + " (unknown subcommand: write)"}
	}
	// The shell classifier's git rules tighten to outward/destructive only: the table above is
	// the authority on read versus write (the shell reading calls every unknown word a write).
	if sc := e.ClassifyCommand("git " + strings.Join(args, " ")); sc.Class >= Outward && sc.Class > c.Class {
		c = sc
	}
	return c
}

func hasPlusRef(flags []string) bool {
	for _, f := range flags {
		if strings.HasPrefix(f, "+") {
			return true
		}
	}
	return false
}

// GitTool runs git with an argv (never through a shell), classified per subcommand.
func GitTool(opt GitOptions) *Tool {
	argsOf := func(in json.RawMessage) ([]string, error) {
		var a struct {
			Args    []string
			Command string
		}
		if err := decode(in, &a); err != nil {
			return nil, err
		}
		if len(a.Args) > 0 {
			return a.Args, nil
		}
		return SplitArgs(strings.TrimPrefix(strings.TrimSpace(a.Command), "git "))
	}
	return &Tool{
		Name:        "git",
		Description: "Run git in the world (argv, no shell). status/diff/log/show read; commit/add write; push/fetch/pull outward; reset --hard, clean, rebase, push --force destructive. args: [\"log\",\"-3\"] or command: \"log -3\".",
		Schema:      json.RawMessage(`{"type":"object","properties":{"args":{"type":"array","items":{"type":"string"}},"command":{"type":"string"},"cwd":{"type":"string"}}}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			args, err := argsOf(in)
			if err != nil {
				return Classification{Class: Write, Why: "git (unparsed)"}
			}
			return env.ClassifyGit(args)
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			if !opt.Enabled {
				return fail("git is off (tools.git.enabled: false)")
			}
			args, err := argsOf(in)
			if err != nil || len(args) == 0 {
				return fail("git: args (or command) is required")
			}
			var a struct{ Cwd string }
			_ = decode(in, &a)
			dir := env.Dir()
			if a.Cwd != "" {
				dir = env.Resolve(a.Cwd)
			}
			bin := opt.Binary
			if bin == "" {
				bin = "git"
			}
			t := opt.Timeout
			if t <= 0 {
				t = 2 * time.Minute
			}
			cctx, cancel := context.WithTimeout(ctx, t)
			defer cancel()
			network := env.ClassifyGit(args).Class >= Outward
			cmd := opt.Sandbox.Command(cctx, sandbox.Call{Cwd: dir, Network: network}, os.Environ(), append([]string{bin}, args...)...)
			cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "PAGER=cat")
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			runErr := cmd.Run()
			code := 0
			if runErr != nil {
				if ee, ok := runErr.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else {
					return fail("git: %v", runErr)
				}
			}
			var b strings.Builder
			b.WriteString(out.String())
			if errb.Len() > 0 {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteString("\n")
				}
				b.WriteString(errb.String())
			}
			if cctx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(&b, "\n[killed after %s]", t)
				code = 124
			}
			if code != 0 {
				fmt.Fprintf(&b, "\n[exit %d]", code)
			}
			return Result{Output: clip(b.String()), Err: code != 0}
		},
	}
}

// SplitArgs splits a command line into words with shell-like quoting (single, double,
// backslash); no expansion happens.
func SplitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false
	quote := rune(0)
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
			in = true
		case r == '\\' && quote != '\'':
			esc = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			in = true
		case r == ' ' || r == '\t' || r == '\n':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}
