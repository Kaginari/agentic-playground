package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/workspace"
)

// A review is two independent reviewers on two roles' models (by default judge and drafter, so
// two models when config gives them two), in parallel, each with the same neutral brief and a
// read-only shelf — anything above a read is refused, never asked. Their reports go to the session,
// which merges them: dedupe, keep what matters, a numbered shortlist marked by who found it, and
// nothing fixed before the operator approves (after davidondrej/skills' total-review, MIT).

var reviewOffices = []string{"judge", "drafter"}

// reviewRange is what to review: the argument, else the uncommitted changes, else the last commit.
func (a *App) reviewRange(ctx context.Context, arg string) (rng, what string) {
	if arg = strings.TrimSpace(arg); arg != "" {
		return arg, "the changes in " + arg
	}
	if len(gitLines(ctx, a.Root, "status", "--short", "--untracked-files=no")) > 0 {
		return "HEAD", "the uncommitted changes (git diff HEAD)"
	}
	return "HEAD~1..HEAD", "the last commit (HEAD~1..HEAD)"
}

func reviewBrief(rng, what string, stat []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review %s in this repository as a thorough senior developer.\n", what)
	fmt.Fprintf(&b, "Read the diff (`git diff %s`), the changed code in full, the code around it and its tests.\n", rng)
	if len(stat) > 0 {
		b.WriteString("\nFiles:\n")
		for _, l := range stat[:min(len(stat), 40)] {
			b.WriteString("  " + l + "\n")
		}
	}
	b.WriteString("\nReview only — do not change any file. Report the serious or critical issues: each with file:line, what is " +
		"wrong, why it matters, and the fix. Separate what you verified from what you suspect, and name what you could not " +
		"check. End with one line: ready to merge, or not, and why. Plain English, concise.")
	return b.String()
}

func reviewMerge(what string, roles []string, reports []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Two independent reviewers reviewed %s. Their full reports follow.\n\n", what)
	for i, r := range reports {
		fmt.Fprintf(&b, "=== reviewer [%s]\n%s\n\n", roles[i], strings.TrimSpace(r))
	}
	fmt.Fprintf(&b, "Merge and triage them. Combine and deduplicate: an issue both found counts once. Judge each finding — "+
		"a real bug or risk, or a style preference, a theoretical edge case, a non-issue? Agreement alone does not make an "+
		"issue real; read the code when unsure. Keep only what matters.\n\nAnswer: a numbered list of the real issues, one "+
		"line each (what it is + where), marked [both], [%s] or [%s] — the ones both found first. Then one line: how many "+
		"findings you dropped as overthinking. Then ask the operator to approve fixing these, or to adjust the list. Do not "+
		"fix anything before the operator approves.", roles[0], roles[1])
	return b.String()
}

// reviewer is a read-only engine on an role's model.
func (a *App) reviewer(role string) *loop.Engine {
	b := a.Build()
	b.Role = role
	b.Depth = 1
	b.Subagent.Enabled = false
	e := a.Workspace.Engine(workspace.Orchestrator, b)
	e.Cap = 0
	decide := e.Hooks.Decide
	e.Hooks.Decide = func(s *loop.Session, st *loop.StepRecord, cls tool.Classification) loop.Decision {
		if cls.Class > tool.Read {
			return loop.Decision{Action: "deny", Why: "a reviewer reads only"}
		}
		if decide != nil {
			return decide(s, st, cls)
		}
		return loop.Decision{}
	}
	a.mu.Lock()
	a.engines = append(a.engines, e)
	a.mu.Unlock()
	return e
}

// runReview runs the two reviewers in parallel and returns the merge ask.
func (a *App) runReview(ctx context.Context, arg string, note func(string)) (string, error) {
	if a.mount == nil {
		return "", fmt.Errorf("no model: %s", strings.Join(a.Providers.Holes(), "; "))
	}
	rng, what := a.reviewRange(ctx, arg)
	statArgs := []string{"diff", "--stat", rng}
	stat := gitLines(ctx, a.Root, statArgs...)
	if len(stat) == 0 {
		return "", fmt.Errorf("nothing to review in %s", what)
	}
	brief := reviewBrief(rng, what, stat)
	reports := make([]string, len(reviewOffices))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []string
	for i, role := range reviewOffices {
		m, _ := a.Cfg.ResolveModel("", "", role, "")
		note(fmt.Sprintf("review: reviewer [%s] on %s", role, m.Ref.Model))
		wg.Add(1)
		go func(i int, role string) {
			defer wg.Done()
			e := a.reviewer(role)
			r, err := e.NewSession().Turn(ctx, brief)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				errs = append(errs, role+": "+err.Error())
			case r.Status != loop.Done:
				errs = append(errs, fmt.Sprintf("%s: the review ended %s", role, r.Status))
			default:
				reports[i] = r.Text
				note(fmt.Sprintf("review: [%s] reported", role))
			}
		}(i, role)
	}
	wg.Wait()
	if len(errs) > 0 {
		return "", fmt.Errorf("a reviewer did not finish — %s (a partial review is not a review)", strings.Join(errs, "; "))
	}
	return reviewMerge(what, reviewOffices, reports), nil
}
