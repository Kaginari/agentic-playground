// Package anthropic speaks the Messages API over net/http. The key comes from ANTHROPIC_API_KEY
// and never crosses the wire elsewhere; the model from the flag or ISEKAI_MODEL.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/provider"
)

const (
	DefaultBaseURL = "https://api.anthropic.com"
	DefaultModel   = "claude-opus-5-5"
	Version        = "2023-06-01"
	defaultMax     = 16000
)

// Client is one Messages API endpoint.
type Client struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// New reads ANTHROPIC_API_KEY (required), ANTHROPIC_BASE_URL and ISEKAI_MODEL.
func New(model string) (*Client, error) {
	key := provider.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, errors.New("anthropic: ANTHROPIC_API_KEY is not set (location only: the key never crosses the wire)")
	}
	if model == "" {
		model = provider.DefaultModel(DefaultModel)
	}
	base := provider.Getenv("ANTHROPIC_BASE_URL")
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{APIKey: key, Model: model, BaseURL: base, HTTP: &http.Client{Timeout: 10 * time.Minute}}, nil
}

func (c *Client) Name() string { return "anthropic/" + c.Model }

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
	Tools     []toolDef `json:"tools,omitempty"`
}

type response struct {
	Model       string  `json:"model"`
	Content     []block `json:"content"`
	StopReason  string  `json:"stop_reason"`
	StopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	} `json:"stop_details"`
	Usage struct {
		Input      int `json:"input_tokens"`
		Output     int `json:"output_tokens"`
		CacheRead  int `json:"cache_read_input_tokens"`
		CacheWrite int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Encode turns a Request into the Messages API body. Exported so tests can read the shape.
func Encode(model string, req provider.Request) request {
	r := request{Model: model, MaxTokens: req.MaxTokens, System: req.System}
	if r.MaxTokens <= 0 {
		r.MaxTokens = defaultMax
	}
	for _, m := range req.Messages {
		var blocks []block
		for _, tr := range m.ToolResults {
			blocks = append(blocks, block{Type: "tool_result", ToolUseID: tr.ID, Content: tr.Content, IsError: tr.IsError})
		}
		if m.Text != "" {
			blocks = append(blocks, block{Type: "text", Text: m.Text})
		}
		for _, tc := range m.ToolCalls {
			in := tc.Input
			if len(in) == 0 {
				in = json.RawMessage(`{}`)
			}
			blocks = append(blocks, block{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: in})
		}
		r.Messages = append(r.Messages, message{Role: string(m.Role), Content: blocks})
	}
	for _, t := range req.Tools {
		s := t.Schema
		if len(s) == 0 {
			s = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		r.Tools = append(r.Tools, toolDef{Name: t.Name, Description: t.Description, InputSchema: s})
	}
	return r
}

// Complete performs one Messages API call.
func (c *Client) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if err := provider.Validate(req); err != nil {
		return provider.Response{}, err
	}
	body, err := json.Marshal(Encode(c.Model, req))
	if err != nil {
		return provider.Response{}, err
	}
	hr, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.BaseURL, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return provider.Response{}, err
	}
	hr.Header.Set("content-type", "application/json")
	hr.Header.Set("x-api-key", c.APIKey)
	hr.Header.Set("anthropic-version", Version)
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(hr)
	if err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return provider.Response{}, err
	}
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: HTTP %d, unreadable body: %w", res.StatusCode, err)
	}
	if res.StatusCode != 200 {
		msg := strings.TrimSpace(string(raw))
		if out.Error != nil {
			msg = out.Error.Type + ": " + out.Error.Message
		}
		return provider.Response{}, fmt.Errorf("anthropic: HTTP %d: %s", res.StatusCode, msg)
	}
	resp := provider.Response{Model: out.Model, Message: provider.Message{Role: provider.Assistant}}
	resp.Usage = provider.Usage{Input: out.Usage.Input, Output: out.Usage.Output, CacheRead: out.Usage.CacheRead, CacheWrite: out.Usage.CacheWrite}
	var text []string
	for _, b := range out.Content {
		switch b.Type {
		case "text":
			text = append(text, b.Text)
		case "tool_use":
			resp.Message.ToolCalls = append(resp.Message.ToolCalls, provider.ToolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	resp.Message.Text = strings.Join(text, "\n")
	switch out.StopReason {
	case "end_turn", "stop_sequence":
		resp.Stop = provider.StopEnd
	case "tool_use":
		resp.Stop = provider.StopToolUse
	case "max_tokens":
		resp.Stop = provider.StopMaxTokens
	case "refusal":
		resp.Stop = provider.StopRefusal
		if out.StopDetails != nil && resp.Message.Text == "" {
			resp.Message.Text = "[refused: " + out.StopDetails.Category + " — " + out.StopDetails.Explanation + "]"
		}
	default:
		resp.Stop = provider.StopOther
	}
	if len(resp.Message.ToolCalls) > 0 {
		resp.Stop = provider.StopToolUse
	}
	return resp, nil
}
