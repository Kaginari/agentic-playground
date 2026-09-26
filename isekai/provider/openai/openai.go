// Package openai speaks chat-completions over net/http. The base URL is configurable
// (OPENAI_BASE_URL) so OpenRouter, Ollama and any compatible endpoint answer the same call.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/provider"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-4o"
)

// Client is one chat-completions endpoint. An empty APIKey is allowed (Ollama).
type Client struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// New reads OPENAI_API_KEY (optional), OPENAI_BASE_URL and ISEKAI_MODEL.
func New(model string) (*Client, error) {
	if model == "" {
		model = provider.DefaultModel(DefaultModel)
	}
	base := provider.Getenv("OPENAI_BASE_URL")
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{APIKey: provider.Getenv("OPENAI_API_KEY"), Model: model, BaseURL: base, HTTP: &http.Client{Timeout: 10 * time.Minute}}, nil
}

func (c *Client) Name() string { return "openai/" + c.Model }

type function struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
}

type toolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolDef struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type request struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	Tools     []toolDef `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type response struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
		Details    struct {
			Cached int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Encode turns a Request into the chat-completions body: the system prompt becomes the first
// message, each tool result its own `tool` message.
func Encode(model string, req provider.Request) request {
	r := request{Model: model, MaxTokens: req.MaxTokens}
	if req.System != "" {
		r.Messages = append(r.Messages, message{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		for _, tr := range m.ToolResults {
			content := tr.Content
			if tr.IsError {
				content = "ERROR: " + content
			}
			r.Messages = append(r.Messages, message{Role: "tool", ToolCallID: tr.ID, Content: content})
		}
		if m.Role == provider.Assistant {
			am := message{Role: "assistant", Content: m.Text}
			for _, tc := range m.ToolCalls {
				args := string(tc.Input)
				if args == "" {
					args = "{}"
				}
				am.ToolCalls = append(am.ToolCalls, toolCall{ID: tc.ID, Type: "function", Function: function{Name: tc.Name, Arguments: args}})
			}
			r.Messages = append(r.Messages, am)
		} else if m.Text != "" {
			r.Messages = append(r.Messages, message{Role: "user", Content: m.Text})
		}
	}
	for _, t := range req.Tools {
		s := t.Schema
		if len(s) == 0 {
			s = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		r.Tools = append(r.Tools, toolDef{Type: "function", Function: function{Name: t.Name, Description: t.Description, Parameters: s}})
	}
	return r
}

// Complete performs one chat-completions call.
func (c *Client) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if err := provider.Validate(req); err != nil {
		return provider.Response{}, err
	}
	body, err := json.Marshal(Encode(c.Model, req))
	if err != nil {
		return provider.Response{}, err
	}
	hr, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return provider.Response{}, err
	}
	hr.Header.Set("content-type", "application/json")
	if c.APIKey != "" {
		hr.Header.Set("authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(hr)
	if err != nil {
		return provider.Response{}, fmt.Errorf("openai: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return provider.Response{}, err
	}
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return provider.Response{}, fmt.Errorf("openai: HTTP %d, unreadable body: %w", res.StatusCode, err)
	}
	if res.StatusCode != 200 || out.Error != nil {
		msg := strings.TrimSpace(string(raw))
		if out.Error != nil {
			msg = out.Error.Type + ": " + out.Error.Message
		}
		return provider.Response{}, fmt.Errorf("openai: HTTP %d: %s", res.StatusCode, msg)
	}
	if len(out.Choices) == 0 {
		return provider.Response{}, fmt.Errorf("openai: no choices in the response")
	}
	ch := out.Choices[0]
	resp := provider.Response{Model: out.Model, Message: provider.Message{Role: provider.Assistant, Text: ch.Message.Content}}
	resp.Usage = provider.Usage{Input: out.Usage.Prompt - out.Usage.Details.Cached, Output: out.Usage.Completion, CacheRead: out.Usage.Details.Cached}
	for _, tc := range ch.Message.ToolCalls {
		in := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(in) {
			in = json.RawMessage(`{}`)
		}
		resp.Message.ToolCalls = append(resp.Message.ToolCalls, provider.ToolCall{ID: tc.ID, Name: tc.Function.Name, Input: in})
	}
	switch ch.FinishReason {
	case "stop":
		resp.Stop = provider.StopEnd
	case "tool_calls", "function_call":
		resp.Stop = provider.StopToolUse
	case "length":
		resp.Stop = provider.StopMaxTokens
	default:
		resp.Stop = provider.StopOther
	}
	if len(resp.Message.ToolCalls) > 0 {
		resp.Stop = provider.StopToolUse
	}
	return resp, nil
}
