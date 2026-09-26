// Package provider is the one standing outward act of the binary: a chat turn with a model,
// carrying tool definitions out and tool calls, text and token usage back. Every provider
// speaks this shape; the loop never sees a vendor's wire format.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Role of a Message. Tool results ride on a user message (ToolResults), as every vendor
// expects them to follow the assistant's tool calls.
type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
)

// ToolDef is a tool as the model sees it: a name, a description and a JSON Schema for its input.
// Declare carries a vendor-defined declaration keyed by provider type (e.g. "anthropic" →
// {"type":"bash_20250124","name":"bash"}); a provider that finds its key sends that object
// instead of the schema, so the model drives the tool it was trained on.
type ToolDef struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Schema      json.RawMessage            `json:"schema"`
	Declare     map[string]json.RawMessage `json:"declare,omitempty"`
}

// DeclareFor returns the vendor declaration for a provider type, if the tool has one.
func (d ToolDef) DeclareFor(provider string) (json.RawMessage, bool) {
	raw, ok := d.Declare[provider]
	return raw, ok && len(raw) > 0
}

// ToolCall is the model asking for a tool to run; ID pairs it with its ToolResult.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// Extra is provider data that must travel back unchanged with this call on the next turn
	// (Gemini's extra_content.google.thought_signature); the harness never reads it.
	Extra json.RawMessage `json:"extra,omitempty"`
}

// ToolResult answers one ToolCall. IsError tells the model the act failed.
type ToolResult struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// Message is one turn of the conversation. An assistant message may carry ToolCalls; a user
// message may carry ToolResults (then Text, if any, follows them). Opaque holds vendor blocks
// the provider that produced the message must see again on replay (Anthropic's thinking
// blocks with their signatures); every other provider ignores them.
type Message struct {
	Role        Role              `json:"role"`
	Text        string            `json:"text,omitempty"`
	ToolCalls   []ToolCall        `json:"tool_calls,omitempty"`
	ToolResults []ToolResult      `json:"tool_results,omitempty"`
	Opaque      []json.RawMessage `json:"opaque,omitempty"`
}

// Usage is what the provider reported for one call. Context() is the occupancy reading the
// instruments use: everything that was in the window on this call (context-check.sh's method).
type Usage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
}

// Context is the window occupancy this call saw: input plus what was read from and written to
// the cache. It is the reading, not a billing figure.
func (u Usage) Context() int { return u.Input + u.CacheRead + u.CacheWrite }

// Add sums two usages (for a run's total spend; Context() of a sum is meaningless).
func (u Usage) Add(o Usage) Usage {
	return Usage{u.Input + o.Input, u.Output + o.Output, u.CacheRead + o.CacheRead, u.CacheWrite + o.CacheWrite}
}

// Request is one chat turn: the system prompt, the conversation so far, the tools on offer.
// System is the stable prefix (principles, identity, rules) a provider may cache; SystemTail is
// what changes per call (the recall manifest) and is never cached. OnDelta, when set, receives
// text as it streams in; Wire says the answer is expected on the envelope (a provider with
// guided decoding constrains the output to its schema and renders the wire itself).
type Request struct {
	System     string
	SystemTail string
	Messages   []Message
	Tools      []ToolDef
	MaxTokens  int // 0 means the provider's default
	OnDelta    func(text string)
	Wire       bool
}

// SystemText is the whole system prompt as one string (providers without block prompts).
func (r Request) SystemText() string {
	if r.SystemTail == "" {
		return r.System
	}
	if r.System == "" {
		return r.SystemTail
	}
	return r.System + "\n" + r.SystemTail
}

// StopReason is why the model stopped, normalised across vendors.
type StopReason string

const (
	StopEnd       StopReason = "end"        // the model finished its answer
	StopToolUse   StopReason = "tool_use"   // the model wants tools run
	StopMaxTokens StopReason = "max_tokens" // cut by the output ceiling
	StopRefusal   StopReason = "refusal"    // the provider's safety layer declined
	StopPause     StopReason = "pause_turn" // the server paused a long turn; resend to continue
	StopOther     StopReason = "other"
)

// Response is the model's message plus the reading that came with it.
type Response struct {
	Message Message
	Usage   Usage
	Stop    StopReason
	Model   string
}

// Provider is one model behind one interface.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}

// Validate checks the conversation shape every vendor requires: roles alternate, tool results
// answer the tool calls that precede them, nothing is empty.
func Validate(req Request) error {
	if len(req.Messages) == 0 {
		return errors.New("provider: no messages")
	}
	var pending map[string]bool
	for i, m := range req.Messages {
		switch m.Role {
		case User:
			if i > 0 && req.Messages[i-1].Role == User {
				return fmt.Errorf("provider: message %d: two user messages in a row", i)
			}
			if len(pending) > 0 {
				if len(m.ToolResults) != len(pending) {
					return fmt.Errorf("provider: message %d: %d tool results for %d tool calls", i, len(m.ToolResults), len(pending))
				}
				for _, r := range m.ToolResults {
					if !pending[r.ID] {
						return fmt.Errorf("provider: message %d: tool result %q answers no call", i, r.ID)
					}
				}
				pending = nil
			} else if len(m.ToolResults) > 0 {
				return fmt.Errorf("provider: message %d: tool results with no preceding tool calls", i)
			}
			if strings.TrimSpace(m.Text) == "" && len(m.ToolResults) == 0 {
				return fmt.Errorf("provider: message %d: empty user message", i)
			}
		case Assistant:
			if i == 0 || req.Messages[i-1].Role != User {
				return fmt.Errorf("provider: message %d: assistant message must follow a user message", i)
			}
			if len(m.ToolCalls) > 0 {
				pending = map[string]bool{}
				for _, c := range m.ToolCalls {
					if c.ID == "" || c.Name == "" {
						return fmt.Errorf("provider: message %d: tool call without id or name", i)
					}
					pending[c.ID] = true
				}
			}
		default:
			return fmt.Errorf("provider: message %d: unknown role %q", i, m.Role)
		}
	}
	if req.Messages[len(req.Messages)-1].Role != User {
		return errors.New("provider: conversation must end with a user message")
	}
	return nil
}

// Getenv is the environment lookup providers use; tests swap it.
var Getenv = func(key string) string { return lookupEnv(key) }
