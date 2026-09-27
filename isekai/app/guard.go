package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/guard"
)

// theGuard is the effective denylist: built-in, the machine-wide file, config's files.
func (a *App) theGuard() *guard.Guard {
	if !a.Cfg.Guard.Enabled {
		return nil
	}
	home := a.Opt.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	g := guard.ForHome(home, expandPaths(a.Cfg.Guard.Files, a.Root, home)...)
	if len(a.Cfg.Guard.Patterns) > 0 {
		g.AddPatterns(a.Cfg.Guard.Patterns, "config "+a.Cfg.Where("guard.patterns"))
	}
	return g
}

// guardHook refuses a bash command the guard matches, before the policy and the gate.
func (a *App) guardHook() func(tool string, input json.RawMessage) string {
	g := a.theGuard()
	if g == nil {
		return nil
	}
	return func(name string, input json.RawMessage) string {
		if name != "bash" {
			return ""
		}
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) != nil || in.Command == "" {
			return ""
		}
		if r := g.Match(in.Command); r != nil {
			return guard.Refusal(r)
		}
		return ""
	}
}

// guardLine is the status line for the guard.
func (a *App) guardLine() string {
	g := a.theGuard()
	if g == nil {
		return "@? guard: off (guard.enabled " + a.Cfg.Where("guard.enabled") + ") — catastrophic commands reach the gate only"
	}
	srcs := map[string]bool{}
	for _, r := range g.Rules {
		src := r.Source
		if i := len(src) - 1; i > 0 {
			for i > 0 && src[i] != ':' {
				i--
			}
			src = src[:i]
		}
		srcs[src] = true
	}
	line := fmt.Sprintf("guard: %d patterns (%d source%s)", len(g.Rules), len(srcs), plural(len(srcs)))
	for _, n := range g.Notes {
		line += "\n@? guard: " + n
	}
	return line
}

const guardUsage = `usage: %[1]s guard <command>
  check <command…>          is it blocked? (exit 2 when it is)
  test                      the corpus against the effective list (built-in + the machine-wide file)
  show                      every pattern and where it comes from
  export                    the built-in list, for ~/.agents/hooks/dangerous-patterns.txt
  hook [--cursor]           an agent's pre-tool hook: hook JSON on stdin, exit 2 blocks
  install [--claude] [--opencode] [--yes]
                            wire the hook into Claude Code and OpenCode (shows the change; --yes writes)
`

// cmdGuard is `<dist> guard …`: the guard on its own, no world needed.
func cmdGuard(dist string, args []string, io IO) int {
	home := io.Env("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	g := guard.ForHome(home)
	if len(args) == 0 {
		fmt.Fprintf(io.Err, guardUsage, dist)
		return 2
	}
	switch args[0] {
	case "check":
		cmd := strings.Join(args[1:], " ")
		if r := g.Match(cmd); r != nil {
			fmt.Fprintf(io.Out, "blocked — %s\n  pattern: %s\n", r.Source, r.Pattern)
			return 2
		}
		fmt.Fprintln(io.Out, "allowed — no pattern matches (the classifier and the gate still see it)")
		return 0
	case "test":
		passed, wrong := g.Test()
		for _, w := range wrong {
			fmt.Fprintln(io.Out, "FAIL "+w)
		}
		fmt.Fprintf(io.Out, "passed: %d, failed: %d\n", passed, len(wrong))
		for _, n := range g.Notes {
			fmt.Fprintln(io.Out, "note: "+n)
		}
		if len(wrong) > 0 {
			return 1
		}
		return 0
	case "show":
		for _, r := range g.Rules {
			fmt.Fprintf(io.Out, "%-48s %s\n", r.Source, r.Pattern)
		}
		return 0
	case "export":
		fmt.Fprint(io.Out, guard.DefaultPatterns())
		return 0
	case "hook":
		return g.Hook(io.In, io.Out, io.Err, len(args) > 1 && args[1] == "--cursor")
	case "install":
		return guardInstall(dist, home, args[1:], io)
	}
	fmt.Fprintf(io.Err, guardUsage, dist)
	return 2
}

// guardInstall wires `<binary> guard hook` into Claude Code's settings and an OpenCode plugin,
// and writes the machine-wide patterns file when there is none. Without --yes it only says what
// it would change: these are the human's own agents, outside the world.
func guardInstall(dist, home string, args []string, io IO) int {
	claude, opencode, yes := false, false, false
	for _, a := range args {
		switch a {
		case "--claude":
			claude = true
		case "--opencode":
			opencode = true
		case "--yes":
			yes = true
		}
	}
	if !claude && !opencode {
		claude, opencode = true, true
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? guard install: %v\n", err)
		return 2
	}
	command := exe + " guard hook"
	type change struct{ path, what, body string }
	var todo []change
	shared := filepath.Join(home, guard.SharedPath)
	if !guard.Exists(shared) {
		todo = append(todo, change{shared, "the machine-wide denylist (the built-in list, yours to extend)", guard.DefaultPatterns()})
	}
	if claude {
		p := filepath.Join(home, ".claude", "settings.json")
		cur, _ := os.ReadFile(p)
		out, changed, err := guard.ClaudeSettings(cur, command)
		switch {
		case err != nil:
			fmt.Fprintf(io.Err, "@? %s: %v — left as it is\n", p, err)
		case changed:
			todo = append(todo, change{p, "Claude Code: a PreToolUse hook on Bash → " + command, string(out)})
		default:
			fmt.Fprintf(io.Out, "ok  %s already runs the guard\n", tilde(p, home))
		}
	}
	if opencode {
		p := filepath.Join(home, ".config", "opencode", "plugins", "command-guard.ts")
		body := guard.OpenCodePlugin(exe)
		if cur, _ := os.ReadFile(p); string(cur) != body {
			todo = append(todo, change{p, "OpenCode: a plugin that asks the guard before every bash call", body})
		} else {
			fmt.Fprintf(io.Out, "ok  %s already runs the guard\n", tilde(p, home))
		}
	}
	for _, c := range todo {
		verb := "would write"
		if yes {
			verb = "wrote"
			if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err == nil {
				err = os.WriteFile(c.path, []byte(c.body), 0o644)
			}
			if err != nil {
				fmt.Fprintf(io.Err, "@? %s: %v\n", c.path, err)
				return 2
			}
		}
		fmt.Fprintf(io.Out, "%s %s — %s\n", verb, tilde(c.path, home), c.what)
	}
	if !yes && len(todo) > 0 {
		fmt.Fprintf(io.Out, "\nnothing written: `%s guard install --yes` writes it (restart those agents after)\n", dist)
	}
	return 0
}
