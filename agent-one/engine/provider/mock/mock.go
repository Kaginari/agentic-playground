// Package mock is a scripted provider for tests and the selftest: it answers with the responses
// it was given, in order, and records every request it saw. Nothing leaves the process.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Kaginari/agent-one/provider"
)

// Provider replays Script; once the script is spent it answers Fallback, or an error.
type Provider struct {
	mu       sync.Mutex
	Script   []provider.Response
	Fallback *provider.Response
	Requests []provider.Request
	Validate bool // run provider.Validate on every request (default on via New)
	usage    provider.Usage
}

// New builds a mock that validates every request and reports a small, growing usage so the
// context reading moves like a real provider's.
func New(script ...provider.Response) *Provider {
	return &Provider{Script: script, Validate: true, usage: provider.Usage{Input: 1000, Output: 50}}
}

func (p *Provider) Name() string { return "mock" }

// Complete pops the next scripted response. Its Usage, if zero, is filled with a rising reading.
func (p *Provider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if p.Validate {
		if err := provider.Validate(req); err != nil {
			return provider.Response{}, err
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Requests = append(p.Requests, req)
	var r provider.Response
	switch {
	case len(p.Script) > 0:
		r, p.Script = p.Script[0], p.Script[1:]
	case p.Fallback != nil:
		r = *p.Fallback
	default:
		return provider.Response{}, fmt.Errorf("mock: script exhausted after %d requests", len(p.Requests))
	}
	if r.Usage == (provider.Usage{}) {
		p.usage.Input += 500
		r.Usage = p.usage
	}
	if r.Message.Role == "" {
		r.Message.Role = provider.Assistant
	}
	if r.Stop == "" {
		if len(r.Message.ToolCalls) > 0 {
			r.Stop = provider.StopToolUse
		} else {
			r.Stop = provider.StopEnd
		}
	}
	if r.Model == "" {
		r.Model = "mock"
	}
	return r, nil
}

// Text is a scripted final answer.
func Text(s string) provider.Response {
	return provider.Response{Message: provider.Message{Role: provider.Assistant, Text: s}, Stop: provider.StopEnd}
}

// Call is a scripted tool call; input is any JSON-marshalable value.
func Call(id, name string, input interface{}) provider.Response {
	raw, _ := json.Marshal(input)
	return provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: id, Name: name, Input: raw}}}, Stop: provider.StopToolUse}
}

// Calls is a scripted response with several tool calls.
func Calls(calls ...provider.ToolCall) provider.Response {
	return provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: calls}, Stop: provider.StopToolUse}
}
