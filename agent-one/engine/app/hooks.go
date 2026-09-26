package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/wire"
)

// ShellHooks runs config's shell hooks (config.md §Hook contract): one JSON object on stdin
// and the same fields as HOOK_* env vars; exit 0 continues, exit 2 blocks the act with stderr
// as the reason the model reads, any other exit is logged and ignored. Stdout that starts
// with @S is read as the wire and folded into the loop journal, never into the prompt.
type ShellHooks struct {
	Cfg     *config.Config
	Root    string
	Session string
	// Journal receives what a hook's wire said ("" event for the session ones).
	Journal func(event string, fields map[string]interface{})
}

// hookInput is the JSON a hook reads.
type hookInput struct {
	Event   string          `json:"event"`
	Session string          `json:"session"`
	Tool    string          `json:"tool,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Class   string          `json:"class,omitempty"`
	Paths   []string        `json:"paths,omitempty"`
	Output  string          `json:"output,omitempty"`
	Prompt  string          `json:"prompt,omitempty"`
}

// Result is one hook run.
type HookResult struct {
	Command string
	Exit    int
	Blocked bool
	Reason  string
	Wire    *wire.Report
	Err     error
}

func (h *ShellHooks) enabled() bool { return h != nil && h.Cfg != nil && h.Cfg.Hooks.Enabled }

func matches(pattern, name string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	for _, alt := range strings.Split(pattern, "|") {
		alt = strings.TrimSpace(alt)
		if alt == name {
			return true
		}
		if re, err := regexp.Compile("^(?:" + alt + ")$"); err == nil && re.MatchString(name) {
			return true
		}
	}
	return false
}

// run executes every hook in the list whose match fits, in order; the first block wins.
func (h *ShellHooks) run(ctx context.Context, hooks []*config.Hook, in hookInput) []HookResult {
	if !h.enabled() {
		return nil
	}
	timeout := orDur(h.Cfg.Hooks.Timeout.D(), 10*time.Second)
	var out []HookResult
	for _, hk := range hooks {
		if hk == nil || strings.TrimSpace(hk.Command) == "" || !matches(hk.Match, in.Tool) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(cctx, "sh", "-c", hk.Command)
		cmd.Dir = h.Root
		payload, _ := json.Marshal(in)
		cmd.Stdin = bytes.NewReader(payload)
		env := []string{"HOOK_EVENT=" + in.Event, "HOOK_SESSION=" + in.Session, "HOOK_TOOL=" + in.Tool, "HOOK_CLASS=" + in.Class, "HOOK_PATHS=" + strings.Join(in.Paths, "\n"), "HOOK_INPUT=" + string(in.Input), "AGENT_ONE_ROOT=" + h.Root}
		if len(in.Paths) > 0 {
			env = append(env, "FILE="+in.Paths[0])
		}
		cmd.Env = append(cmd.Environ(), env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		cancel()
		r := HookResult{Command: hk.Command}
		if ee, ok := err.(*exec.ExitError); ok {
			r.Exit = ee.ExitCode()
		} else if err != nil {
			r.Exit, r.Err = -1, err
		}
		if cctx.Err() == context.DeadlineExceeded {
			r.Exit, r.Err = 124, fmt.Errorf("hook timed out after %s", timeout)
		}
		if r.Exit == 2 {
			r.Blocked = true
			r.Reason = strings.TrimSpace(stderr.String())
			if r.Reason == "" {
				r.Reason = "blocked by hook `" + hk.Command + "`"
			}
		}
		if s := strings.TrimSpace(stdout.String()); strings.HasPrefix(s, "@S") {
			if rep, ok := wire.ParseReport(s); ok {
				r.Wire = &rep
			}
		}
		if h.Journal != nil {
			fields := map[string]interface{}{"command": hk.Command, "exit": r.Exit, "blocked": r.Blocked}
			if r.Wire != nil {
				fields["findings"], fields["holes"] = r.Wire.Findings, r.Wire.Holes
			}
			if r.Err != nil {
				fields["error"] = r.Err.Error()
			}
			h.Journal(in.Event, fields)
		}
		out = append(out, r)
		if r.Blocked {
			break
		}
	}
	return out
}

// PreTool is the loop seam: the reason when a hook blocked the act, else "".
func (h *ShellHooks) PreTool(ctx context.Context, s *loop.Session, st *loop.StepRecord) string {
	if !h.enabled() || len(h.Cfg.Hooks.PreTool) == 0 {
		return ""
	}
	for _, r := range h.run(ctx, h.Cfg.Hooks.PreTool, hookInput{Event: "preTool", Session: h.Session, Tool: st.Tool, Input: st.Input, Class: st.Effective, Paths: relPaths(s, st)}) {
		if r.Blocked {
			return r.Reason
		}
	}
	return ""
}

// PostTool runs after a done act.
func (h *ShellHooks) PostTool(ctx context.Context, s *loop.Session, st *loop.StepRecord) {
	if !h.enabled() || len(h.Cfg.Hooks.PostTool) == 0 {
		return
	}
	out := st.Result.Output
	if len(out) > 4096 {
		out = out[:4096]
	}
	h.run(ctx, h.Cfg.Hooks.PostTool, hookInput{Event: "postTool", Session: h.Session, Tool: st.Tool, Input: st.Input, Class: st.Effective, Paths: relPaths(s, st), Output: out})
}

// SessionStart, Stop, PreCompact and UserPrompt run the session-level lists.
func (h *ShellHooks) SessionStart(ctx context.Context) {
	if h.enabled() {
		h.run(ctx, h.Cfg.Hooks.SessionStart, hookInput{Event: "sessionStart", Session: h.Session})
	}
}

func (h *ShellHooks) Stop(ctx context.Context) {
	if h.enabled() {
		h.run(ctx, h.Cfg.Hooks.Stop, hookInput{Event: "stop", Session: h.Session})
	}
}

// PreCompact returns the block reason when a hook refused the drain.
func (h *ShellHooks) PreCompact(ctx context.Context) string {
	if !h.enabled() {
		return ""
	}
	for _, r := range h.run(ctx, h.Cfg.Hooks.PreCompact, hookInput{Event: "preCompact", Session: h.Session}) {
		if r.Blocked {
			return r.Reason
		}
	}
	return ""
}

// UserPrompt runs the userPrompt hooks (parity with Claude Code's UserPromptSubmit); a block
// refuses the turn with the reason.
func (h *ShellHooks) UserPrompt(ctx context.Context, text string) string {
	if !h.enabled() {
		return ""
	}
	for _, r := range h.run(ctx, h.Cfg.Hooks.UserPrompt, hookInput{Event: "userPrompt", Session: h.Session, Prompt: text}) {
		if r.Blocked {
			return r.Reason
		}
	}
	return ""
}

func relPaths(s *loop.Session, st *loop.StepRecord) []string {
	var out []string
	for _, p := range st.Class.Paths {
		rel := p
		if r, err := relTo(s.Engine.Root, p); err == nil {
			rel = r
		}
		out = append(out, rel)
	}
	return out
}
