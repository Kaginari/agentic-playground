package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/wire"
)

// A scripted step of the mock provider (providers.<name>.script, a JSON file holding an
// array of these). A step answers when its `when` is a substring of the latest user message
// (its text and tool results); a step with no `when` answers in order. Steps are consumed
// once unless `repeat` is set.
type scriptStep struct {
	When   string          `json:"when"`
	Text   string          `json:"text"`
	Calls  []scriptCall    `json:"calls"`
	Stop   string          `json:"stop"`
	Usage  *provider.Usage `json:"usage"`
	Repeat bool            `json:"repeat"`
	Model  string          `json:"model"`
	used   bool
}

type scriptCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// scriptProvider replays a script file; nothing leaves the process. With no script it
// answers one honest line on the wire.
type scriptProvider struct {
	name  string
	path  string
	mu    sync.Mutex
	steps []*scriptStep
	n     int
	usage provider.Usage
}

func newScriptProvider(name, script, root string) (*scriptProvider, error) {
	p := &scriptProvider{name: name, usage: provider.Usage{Input: 1000, Output: 50}}
	if script == "" {
		return p, nil
	}
	path := script
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	p.path = path
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mock script: %w", err)
	}
	if err := json.Unmarshal(data, &p.steps); err != nil {
		return nil, fmt.Errorf("mock script %s: %w", path, err)
	}
	return p, nil
}

func (p *scriptProvider) Name() string { return p.name }

func lastUser(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != provider.User {
			continue
		}
		var b strings.Builder
		b.WriteString(msgs[i].Text)
		for _, r := range msgs[i].ToolResults {
			b.WriteString("\n" + r.Content)
		}
		return b.String()
	}
	return ""
}

func (p *scriptProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if err := provider.Validate(req); err != nil {
		return provider.Response{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	last := lastUser(req.Messages)
	var step *scriptStep
	for _, s := range p.steps {
		if s.used || s.When == "" || !strings.Contains(last, s.When) {
			continue
		}
		step = s
		break
	}
	if step == nil {
		for _, s := range p.steps {
			if !s.used && s.When == "" {
				step = s
				break
			}
		}
	}
	p.usage.Input += 500
	resp := provider.Response{Message: provider.Message{Role: provider.Assistant}, Usage: p.usage, Model: p.name, Stop: provider.StopEnd}
	if step == nil {
		rep := wire.Report{Status: "DONE"}
		if p.path == "" {
			rep.Status = "MOCK"
			rep.Holes = []string{"the mock provider has no script (providers.<name>.script)"}
		} else {
			rep.Holes = []string{fmt.Sprintf("mock script %s exhausted after %d calls", filepath.Base(p.path), p.n)}
		}
		resp.Message.Text = rep.Emit(0, "")
		return resp, nil
	}
	if !step.Repeat {
		step.used = true
	}
	resp.Message.Text = step.Text
	for i, c := range step.Calls {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("mock_%d_%d", p.n, i+1)
		}
		in := c.Input
		if len(in) == 0 {
			in = json.RawMessage(`{}`)
		}
		resp.Message.ToolCalls = append(resp.Message.ToolCalls, provider.ToolCall{ID: id, Name: c.Name, Input: in})
	}
	if step.Usage != nil {
		resp.Usage = *step.Usage
	}
	if step.Model != "" {
		resp.Model = step.Model
	}
	switch {
	case len(resp.Message.ToolCalls) > 0:
		resp.Stop = provider.StopToolUse
	case step.Stop != "":
		resp.Stop = provider.StopReason(step.Stop)
	}
	if req.OnDelta != nil && resp.Message.Text != "" {
		req.OnDelta(resp.Message.Text)
	}
	return resp, nil
}
