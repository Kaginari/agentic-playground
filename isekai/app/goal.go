package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/loop"
)

// A goal is a contract the session keeps working at, turn after turn, until a validation command
// proves it met (after davidondrej/skills' goal-loop, MIT): an objective, what to read first, what
// must not change, the command, a stop condition. The binary runs the command itself after every
// turn — the model can neither skip nor edit it — and the gate's "tests intact" stops a turn that
// makes the tests easier. It stops when the command passes, when the model says the rest needs the
// human (`@? human: …`), at the turn ceiling, at a budget, or when a turn fails.

type goalContract struct {
	objective, read, constraints, validate string
	maxTurns                               int
}

const goalDefaultTurns = 12

func (g goalContract) ask() string {
	var b strings.Builder
	b.WriteString("You are working to a goal. It continues across turns until its validation passes.\n\n")
	fmt.Fprintf(&b, "Objective: %s\n", g.objective)
	if g.read != "" {
		fmt.Fprintf(&b, "Read first: %s\n", g.read)
	}
	if g.constraints != "" {
		fmt.Fprintf(&b, "Constraints: %s\n", g.constraints)
	}
	fmt.Fprintf(&b, "Validate: `%s` — the binary runs it after each of your turns; exit 0 ends the goal.\n", g.validate)
	b.WriteString("Stop when: the validation passes, OR when further changes need the human's input — then end your answer with one line `@? human: <what you need>`.\n\n")
	b.WriteString("Rules: work in small checkpoints and say what each did. Do not delete, skip, weaken or narrow tests to make the goal pass " +
		"(the gate checks). Do not refactor unrelated code or add dependencies. Do not edit the validation to make it pass.")
	return b.String()
}

// goalNext is the ask for the turn after a failed validation.
func goalNext(g goalContract, turn, code int, tail string) string {
	return fmt.Sprintf("Validation after turn %d: `%s` exit %d.\n\n%s\n\nContinue toward the objective under the same contract: %s",
		turn, g.validate, code, tail, g.objective)
}

// validateGoal runs the validation in the world root; its exit code and the output's last lines.
func (a *App) validateGoal(ctx context.Context, cmd string) (int, string) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, "bash", "-c", cmd)
	c.Dir = a.Root
	out, err := c.CombinedOutput()
	code := 0
	if err != nil {
		code = 1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > 40 {
		lines = append([]string{fmt.Sprintf("… %d lines above", len(lines)-40)}, lines[len(lines)-40:]...)
	}
	return code, strings.Join(lines, "\n")
}

// cmdGoal runs a goal to its end in one session.
func (a *App) cmdGoal(ctx context.Context, g goalContract, io IO) int {
	if strings.TrimSpace(g.objective) == "" || strings.TrimSpace(g.validate) == "" {
		fmt.Fprintf(io.Err, "@S FAIL\n@? goal needs an objective and a validation: %s goal --validate \"<cmd>\" [--read <files>] [--constraints <text>] [--max-turns N] \"<objective>\"\n", a.Cfg.Dist.Name)
		return 2
	}
	if g.maxTurns <= 0 {
		g.maxTurns = goalDefaultTurns
	}
	e, err := a.Engine()
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	a.Hooks.SessionStart(ctx)
	s := e.NewSession()
	ask := g.ask()
	for turn := 1; turn <= g.maxTurns; turn++ {
		fmt.Fprintf(io.Err, "goal · turn %d of %d\n", turn, g.maxTurns)
		r, err := s.Turn(ctx, ask)
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? goal: %v\n", err)
			return 2
		}
		a.printResult(r, false)
		a.Sessions.Sync(a.SessionID, s, a.mountModel.Ref.Model)
		if r.Status != loop.Done {
			fmt.Fprintf(io.Err, "goal stopped at turn %d: the turn ended %s\n", turn, r.Status)
			return r.Exit()
		}
		if i := strings.Index(r.Text, "@? human:"); i >= 0 {
			fmt.Fprintf(io.Err, "goal paused at turn %d for the human: %s\n", turn, strings.TrimSpace(strings.SplitN(r.Text[i+len("@? human:"):], "\n", 2)[0]))
			return 3
		}
		code, tail := a.validateGoal(ctx, g.validate)
		if code == 0 {
			fmt.Fprintf(io.Out, "goal met after %d turn%s: `%s` passes\n", turn, plural(turn), g.validate)
			return 0
		}
		fmt.Fprintf(io.Err, "goal · validation exit %d\n", code)
		ask = goalNext(g, turn, code, tail)
	}
	fmt.Fprintf(io.Err, "goal not met after %d turns: `%s` still fails — raise --max-turns or tighten the objective\n", g.maxTurns, g.validate)
	return 1
}
