package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A handoff is what a fresh session needs to continue without re-asking, re-discovering or
// repeating a mistake (after davidondrej/skills' handoff, MIT). The binary gathers what it knows —
// the first ask, the uncommitted changes, the last log entries, the previous handoff — and the
// model writes the rest into <workspace>/handoffs/<time>.md: state (done · partial · not started),
// decisions and why, traps, pointers, open work. The next session is told one is waiting; reading
// it hands the model the file with one rule: verify every claim against the code, then wait.

const handoffTemplate = `# HANDOFF: <short title of the work>
Generated: <timestamp> · Session focus: <one line>

## 1. Goal
<the overall objective in 1–3 sentences>

## 2. Why this matters
<motivation, constraints, hard requirements not already in the law or config>

## 3. Current state
<status, not actions — DONE: … · PARTIAL: … · NOT STARTED: …>

## 4. Key decisions (and why)
<choices and reasons that the code cannot tell>

## 5. Traps and dead ends
<failed approaches and pitfalls, with why>

## 6. Relevant files and pointers
<paths with line ranges; log.md entries, commits, canon docs — pointed at, never copied>

## 7. Open work
<what remains and what depends on what — status, not commands>`

func (a *App) handoffDir() string { return filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "handoffs") }

// latestHandoff is the newest handoff file and its age; "" when there is none.
func (a *App) latestHandoff() (string, time.Duration) {
	ents, err := os.ReadDir(a.handoffDir())
	if err != nil {
		return "", 0
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", 0
	}
	sort.Strings(names)
	p := filepath.Join(a.handoffDir(), names[len(names)-1])
	info, err := os.Stat(p)
	if err != nil {
		return "", 0
	}
	return p, time.Since(info.ModTime())
}

// handoffAsk is the turn that writes a handoff: the gathered facts, the template, the rules.
func (a *App) handoffAsk(ctx context.Context, focus, firstAsk string) string {
	now := time.Now()
	rel := filepath.ToSlash(filepath.Join(a.Cfg.Dist.WorkspaceDir, "handoffs", now.Format("2006-01-02-150405")+".md"))
	var b strings.Builder
	fmt.Fprintf(&b, "Write a handoff for a fresh session, to %s with the write tool.\n\n", rel)
	if focus != "" {
		fmt.Fprintf(&b, "The next session's focus: %s\n\n", focus)
	}
	b.WriteString("What the binary knows (verify, don't copy):\n")
	if firstAsk != "" {
		fmt.Fprintf(&b, "- this session began with: %q\n", oneLineOf(firstAsk))
	}
	if st := gitLines(ctx, a.Root, "status", "--short"); len(st) > 0 {
		b.WriteString("- uncommitted changes:\n")
		for _, l := range st[:min(len(st), 30)] {
			b.WriteString("    " + l + "\n")
		}
	}
	if lg := gitLines(ctx, a.Root, "log", "--oneline", "-8"); len(lg) > 0 {
		b.WriteString("- recent commits:\n")
		for _, l := range lg {
			b.WriteString("    " + l + "\n")
		}
	}
	for _, t := range lastLogTitles(filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, a.Lex.Log), 6) {
		b.WriteString("- log.md: " + t + "\n")
	}
	if prev, _ := a.latestHandoff(); prev != "" {
		fmt.Fprintf(&b, "- the previous handoff is %s: read it and carry forward what still holds; do not start from scratch\n", relOrAbs(a.Root, prev))
	}
	b.WriteString("\nFill every section of this template (write `None` for an empty one):\n\n```\n" + handoffTemplate + "\n```\n\n")
	b.WriteString("Rules: describe state, not orders (\"logout is not started\", not \"implement logout\"). Point at artifacts " +
		"by path, never copy them. Keep the why that the code cannot tell. Name secrets by location only, never their values. " +
		"Leave out what the law, the config or the code already says. Then answer with the path you wrote, one line.")
	return b.String()
}

// handoffRead is the turn that picks a handoff up.
func handoffRead(path, text string) string {
	return "A previous session left this handoff (" + path + "):\n\n" + text +
		"\n\nBefore responding, read every file listed under \"Relevant files and pointers\". Do not summarise, paraphrase " +
		"or claim you already have the context — actually read each file. Treat every claim in this handoff as context to " +
		"verify against the code, not a fact to trust. Then wait for the operator's instructions before taking any action."
}

func gitLines(ctx context.Context, root string, args ...string) []string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// lastLogTitles is the last n `### [...]` headings of log.md.
func lastLogTitles(path string, n int) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var titles []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "### [") {
			titles = append(titles, strings.TrimPrefix(l, "### "))
		}
	}
	if len(titles) > n {
		titles = titles[len(titles)-n:]
	}
	return titles
}
