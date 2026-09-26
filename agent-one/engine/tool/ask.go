package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Question is what the model puts to the human.
type Question struct {
	Text    string
	Options []string
	Free    bool // a free-text answer is accepted beside the options
}

// Asker answers a question — on the TTY in a session, from a script in tests. It returns the
// human's words; an error means nobody could answer (no TTY), and the turn should end with @?.
type Asker func(ctx context.Context, q Question) (string, error)

// AskOptions is tools.ask.
type AskOptions struct {
	Enabled bool
	Asker   Asker
}

// AskTool is `ask`: a question with options, answered by the injected Asker (the same prompt
// surface the gate uses). Class read: it reaches the human, not the workspace.
func AskTool(opt AskOptions) *Tool {
	return &Tool{
		Name:        "ask",
		Description: "Ask the human a question, optionally with numbered options; the answer comes back as text. Use it instead of guessing (Absolute Rule III).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"question":{"type":"string"},"options":{"type":"array","items":{"type":"string"}},"free":{"type":"boolean","description":"accept a free-text answer beside the options"}},"required":["question"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			return Classification{Class: Read, Why: "asks the human"}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Question string
				Options  []string
				Free     bool
			}
			if err := decode(in, &a); err != nil || strings.TrimSpace(a.Question) == "" {
				return fail("ask: question is required")
			}
			if !opt.Enabled {
				return fail("ask is off (tools.ask.enabled: false)")
			}
			if opt.Asker == nil {
				return fail("ask: nobody can answer here (no TTY) — end the turn with @? naming the question instead")
			}
			ans, err := opt.Asker(ctx, Question{Text: a.Question, Options: a.Options, Free: a.Free})
			if err != nil {
				return fail("ask: %v — end the turn with @? naming the question", err)
			}
			return Result{Output: strings.TrimSpace(ans)}
		},
	}
}

// TTYAsker prints the question and options on out and reads one line from in; a number picks
// an option, anything else is the answer verbatim (refused when the options are exclusive).
func TTYAsker(in io.Reader, out io.Writer) Asker {
	r := bufio.NewReader(in)
	return func(ctx context.Context, q Question) (string, error) {
		fmt.Fprintf(out, "\n? %s\n", q.Text)
		for i, o := range q.Options {
			fmt.Fprintf(out, "   %d) %s\n", i+1, o)
		}
		fmt.Fprint(out, "> ")
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("no answer (%v)", err)
		}
		line = strings.TrimSpace(line)
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(q.Options) {
			return q.Options[n-1], nil
		}
		if len(q.Options) > 0 && !q.Free {
			for _, o := range q.Options {
				if strings.EqualFold(o, line) {
					return o, nil
				}
			}
			return "", fmt.Errorf("%q is not one of the %d options", line, len(q.Options))
		}
		return line, nil
	}
}
