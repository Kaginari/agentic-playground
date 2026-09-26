package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/shell"
)

// BashOptions is tools.bash: the agent's persistent shell session. Session nil means one is
// started lazily from Shell (Root filled from the env on first call).
type BashOptions struct {
	Enabled   bool
	Session   *shell.Session
	Shell     shell.Options // used when Session is nil
	Timeout   time.Duration // default per command (0: env.Timeout, else 10 min)
	OutputCap int           // bytes handed to the model (0: outputCap)
}

// AnthropicBash is the declaration of Claude's own trained bash tool.
var AnthropicBash = json.RawMessage(`{"type":"bash_20250124","name":"bash"}`)

// AnthropicEditor is the declaration of Claude's own trained editor.
var AnthropicEditor = json.RawMessage(`{"type":"text_editor_20250728","name":"str_replace_based_edit_tool"}`)

// NewBashTool is `bash` on the persistent shell: cwd and exports survive between calls, a
// command is classified before it runs (the classifier path is unchanged), `restart` throws the
// session away, `background` starts a named logged job. Under an anthropic provider it is
// declared as bash_20250124; elsewhere the custom schema below. The returned close kills the
// session and its jobs (a agent's session dies with its task); the caller owns a Session it
// passed in and close leaves that one alone.
func NewBashTool(opt BashOptions) (*Tool, func()) {
	var (
		mu   sync.Mutex
		sess = opt.Session
	)
	closer := func() {
		mu.Lock()
		defer mu.Unlock()
		if sess != nil && sess != opt.Session {
			sess.Close()
			sess = nil
		}
	}
	session := func(env Env) (*shell.Session, error) {
		mu.Lock()
		defer mu.Unlock()
		if sess != nil {
			return sess, nil
		}
		so := opt.Shell
		if so.Root == "" {
			so.Root = env.Root
		}
		if so.Cwd == "" {
			so.Cwd = env.Dir()
		}
		s, err := shell.New(so)
		if err != nil {
			return nil, err
		}
		sess = s
		return sess, nil
	}
	base := BashTool()
	t := &Tool{
		Name:        "bash",
		Description: "Run a command in the agent's persistent shell (cwd and exports survive between calls). Its class is read from the command (git push, curl are outward; rm, git reset are destructive); `class` may declare a tighter one. {\"restart\":true} resets the shell; \"background\": true|\"name\" starts a logged job.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"restart":{"type":"boolean"},"background":{"description":"true or a job name","type":["boolean","string"]},"cwd":{"type":"string"},"timeout":{"type":"integer","description":"seconds"},"class":{"type":"string","enum":["read","write","outward","destructive"]}}}`),
		Class:       Read,
		Declare:     map[string]json.RawMessage{"anthropic": AnthropicBash},
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Restart bool }
			_ = decode(in, &a)
			if a.Restart {
				return Classification{Class: Read, Why: "restarts the shell"}
			}
			return base.Classify(env, in)
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Command, Cwd string
				Restart      bool
				Background   json.RawMessage
				Timeout      int
				Class        string
			}
			if err := decode(in, &a); err != nil {
				return fail("bash: %v", err)
			}
			if !opt.Enabled {
				return fail("bash is off (tools.bash.enabled: false)")
			}
			s, err := session(env)
			if err != nil {
				return fail("bash: %v", err)
			}
			if a.Restart {
				if err := s.Restart(); err != nil {
					return fail("bash: restart: %v", err)
				}
				return Result{Output: "shell restarted (cwd " + env.Rel(s.Cwd()) + ", exports reset)"}
			}
			cmd := strings.TrimSpace(a.Command)
			if cmd == "" {
				return fail("bash: command is required")
			}
			// The gate already passed this call; if it reads as outward, it may reach the net.
			settled, _ := base.Settle(env, in)
			network := settled.Class >= Outward
			if a.Cwd != "" {
				cmd = "cd " + shellQuote(env.Resolve(a.Cwd)) + " && { " + cmd + "\n}"
			}
			if name, bg := backgroundName(a.Background); bg {
				j, err := s.Background(name, cmd, network)
				if err != nil {
					return fail("bash: background: %v", err)
				}
				return Result{Output: fmt.Sprintf("job %s started (pid %d), output → %s", j.Name, j.PID, env.Rel(j.Log))}
			}
			t := time.Duration(a.Timeout) * time.Second
			if t <= 0 {
				t = opt.Timeout
			}
			if t <= 0 && env.Timeout > 0 {
				t = time.Duration(env.Timeout) * time.Second
			}
			before := s.Cwd()
			r, err := s.Run(ctx, cmd, shell.RunOptions{Timeout: t, Network: network})
			if err != nil {
				return fail("bash: %v", err)
			}
			var b strings.Builder
			b.WriteString(r.Output)
			note := func(f string, args ...interface{}) {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteString("\n")
				}
				fmt.Fprintf(&b, f+"\n", args...)
			}
			if r.Clipped > 0 {
				note("[%d bytes clipped]", r.Clipped)
			}
			if r.TimedOut {
				note("[timed out after %s — processes killed]", t.Round(time.Second))
			}
			if r.Exit != 0 && !r.TimedOut {
				note("[exit %d, %s]", r.Exit, r.Duration.Round(time.Millisecond))
			}
			if r.Cwd != "" && r.Cwd != before {
				note("[cwd → %s]", env.Rel(r.Cwd))
			}
			cap := opt.OutputCap
			if cap <= 0 {
				cap = outputCap
			}
			out := b.String()
			if len(out) > cap {
				out = out[:cap] + fmt.Sprintf("\n… [%d bytes clipped]", len(out)-cap)
			}
			return Result{Output: out, Err: r.Exit != 0}
		},
	}
	return t, closer
}

func backgroundName(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return "", b
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s, true
	}
	return "", false
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
