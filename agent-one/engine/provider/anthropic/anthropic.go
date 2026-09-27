// Package anthropic speaks the Messages API over net/http. The key comes from ANTHROPIC_API_KEY
// and never crosses the wire elsewhere; the model from the flag or AGENT_ONE_MODEL.
//
// What the client sends, each switchable on the Client: Claude's own trained tools where a
// ToolDef declares one (bash_20250124, text_editor_20250728 — schema-less); adaptive thinking;
// output_config.effort; refusal fallbacks (body `fallbacks: "default"` + the beta header);
// cache_control on the last stable system block so the principles and the tools are read from the
// cache; streaming (SSE) when a delta callback is set. Thinking blocks come back opaque and are
// replayed on the next call, as the API requires with tool use.
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/provider"
)

const (
	DefaultBaseURL = "https://api.anthropic.com"
	DefaultModel   = "claude-opus-5"
	Version        = "2023-06-01"
	FallbackBeta   = "server-side-fallback-2026-07-01"
	defaultMax     = 16000
)

// Client is one Messages API endpoint.
type Client struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
	Headers map[string]string // extra headers (never a credential: those come from APIKey)

	Thinking  bool   // adaptive thinking (default on via New)
	Effort    string // output_config.effort: low | medium | high | xhigh | max ("" = the model's default)
	Fallbacks bool   // refusal fallbacks (default on via New)
	Cache     bool   // cache_control on the stable system prefix (default on via New)
	Stream    bool   // stream when the request carries OnDelta (default on via New)
	MaxTokens int    // default max_tokens (0 = 16000)
}

// New reads ANTHROPIC_API_KEY (required), ANTHROPIC_BASE_URL and AGENT_ONE_MODEL, with every
// switch on.
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
	return &Client{APIKey: key, Model: model, BaseURL: base, HTTP: &http.Client{Timeout: 10 * time.Minute}, Thinking: true, Fallbacks: true, Cache: true, Stream: true}, nil
}

func (c *Client) Name() string { return "anthropic/" + c.Model }

type cacheControl struct {
	Type string `json:"type"`
}

type block struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      string          `json:"content,omitempty"`
	IsError      bool            `json:"is_error,omitempty"`
	CacheControl *cacheControl   `json:"cache_control,omitempty"`
}

type message struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

type toolDef struct {
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	InputSchema         json.RawMessage `json:"input_schema"`
	EagerInputStreaming bool            `json:"eager_input_streaming,omitempty"`
}

type request struct {
	Model        string            `json:"model"`
	MaxTokens    int               `json:"max_tokens"`
	System       []block           `json:"system,omitempty"`
	Messages     []message         `json:"messages"`
	Tools        []json.RawMessage `json:"tools,omitempty"`
	Thinking     json.RawMessage   `json:"thinking,omitempty"`
	OutputConfig json.RawMessage   `json:"output_config,omitempty"`
	Fallbacks    string            `json:"fallbacks,omitempty"`
	Stream       bool              `json:"stream,omitempty"`
}

type response struct {
	Model       string            `json:"model"`
	Content     []json.RawMessage `json:"content"`
	StopReason  string            `json:"stop_reason"`
	StopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	} `json:"stop_details"`
	Usage usage `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type usage struct {
	Input      int `json:"input_tokens"`
	Output     int `json:"output_tokens"`
	CacheRead  int `json:"cache_read_input_tokens"`
	CacheWrite int `json:"cache_creation_input_tokens"`
}

func (u usage) reading() provider.Usage {
	return provider.Usage{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
}

func raw(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// Encode turns a Request into the Messages API body. Exported so tests can read the shape.
func Encode(model string, req provider.Request) request {
	return (&Client{Model: model, Thinking: true, Fallbacks: true, Cache: true, Stream: true}).encode(req, false)
}

func (c *Client) encode(req provider.Request, stream bool) request {
	r := request{Model: c.Model, MaxTokens: req.MaxTokens, Stream: stream}
	if r.MaxTokens <= 0 {
		r.MaxTokens = c.MaxTokens
	}
	if r.MaxTokens <= 0 {
		r.MaxTokens = defaultMax
	}
	if req.System != "" {
		b := block{Type: "text", Text: req.System}
		if c.Cache {
			b.CacheControl = &cacheControl{Type: "ephemeral"}
		}
		r.System = append(r.System, b)
	}
	if req.SystemTail != "" {
		r.System = append(r.System, block{Type: "text", Text: req.SystemTail})
	}
	for _, m := range req.Messages {
		var blocks []json.RawMessage
		for _, tr := range m.ToolResults {
			blocks = append(blocks, raw(block{Type: "tool_result", ToolUseID: tr.ID, Content: tr.Content, IsError: tr.IsError}))
		}
		if m.Role == provider.Assistant {
			// thinking blocks come first, as they were produced
			blocks = append(blocks, m.Opaque...)
		}
		if m.Text != "" {
			blocks = append(blocks, raw(block{Type: "text", Text: m.Text}))
		}
		for _, tc := range m.ToolCalls {
			in := tc.Input
			if len(in) == 0 {
				in = json.RawMessage(`{}`)
			}
			blocks = append(blocks, raw(block{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: in}))
		}
		r.Messages = append(r.Messages, message{Role: string(m.Role), Content: blocks})
	}
	for _, t := range req.Tools {
		if d, ok := t.DeclareFor("anthropic"); ok {
			r.Tools = append(r.Tools, d)
			continue
		}
		s := t.Schema
		if len(s) == 0 {
			s = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		r.Tools = append(r.Tools, raw(toolDef{Name: t.Name, Description: t.Description, InputSchema: s, EagerInputStreaming: stream}))
	}
	if c.Thinking {
		r.Thinking = json.RawMessage(`{"type":"adaptive"}`)
	}
	if c.Effort != "" {
		r.OutputConfig = raw(map[string]string{"effort": c.Effort})
	}
	if c.Fallbacks {
		r.Fallbacks = "default"
	}
	return r
}

// Complete performs one Messages API call, streaming when the request wants deltas.
func (c *Client) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if err := provider.Validate(req); err != nil {
		return provider.Response{}, err
	}
	stream := c.Stream && req.OnDelta != nil
	body, err := json.Marshal(c.encode(req, stream))
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
	if c.Fallbacks {
		hr.Header.Set("anthropic-beta", FallbackBeta)
	}
	for k, v := range c.Headers {
		hr.Header.Set(k, v)
	}
	if stream {
		hr.Header.Set("accept", "text/event-stream")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(hr)
	if err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: %w", err)
	}
	defer res.Body.Close()
	if stream && res.StatusCode == 200 && strings.HasPrefix(res.Header.Get("content-type"), "text/event-stream") {
		return c.readStream(res.Body, req.OnDelta, req.OnThinking)
	}
	rawBody, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return provider.Response{}, err
	}
	var out response
	if err := json.Unmarshal(rawBody, &out); err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: HTTP %d, unreadable body: %w", res.StatusCode, err)
	}
	if res.StatusCode != 200 {
		msg := strings.TrimSpace(string(rawBody))
		if out.Error != nil {
			msg = out.Error.Type + ": " + out.Error.Message
		}
		return provider.Response{}, fmt.Errorf("anthropic: HTTP %d: %s", res.StatusCode, msg)
	}
	resp := decode(out)
	if req.OnThinking != nil {
		for _, rb := range resp.Message.Opaque {
			var t struct{ Type, Thinking string }
			if json.Unmarshal(rb, &t) == nil && t.Type == "thinking" && t.Thinking != "" {
				req.OnThinking(t.Thinking)
			}
		}
	}
	if req.OnDelta != nil && resp.Message.Text != "" {
		req.OnDelta(resp.Message.Text)
	}
	return resp, nil
}

// decode folds a non-streamed response into the provider's shape.
func decode(out response) provider.Response {
	resp := provider.Response{Model: out.Model, Message: provider.Message{Role: provider.Assistant}, Usage: out.Usage.reading()}
	var text []string
	for _, rb := range out.Content {
		var b block
		if json.Unmarshal(rb, &b) != nil {
			continue
		}
		switch b.Type {
		case "text":
			text = append(text, b.Text)
		case "tool_use":
			resp.Message.ToolCalls = append(resp.Message.ToolCalls, provider.ToolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		case "thinking", "redacted_thinking":
			resp.Message.Opaque = append(resp.Message.Opaque, rb)
		}
	}
	resp.Message.Text = strings.Join(text, "\n")
	resp.Stop = stopOf(out.StopReason)
	if resp.Stop == provider.StopRefusal && out.StopDetails != nil && resp.Message.Text == "" {
		resp.Message.Text = "[refused: " + out.StopDetails.Category + " — " + out.StopDetails.Explanation + "]"
	}
	if len(resp.Message.ToolCalls) > 0 {
		resp.Stop = provider.StopToolUse
	}
	return resp
}

func stopOf(s string) provider.StopReason {
	switch s {
	case "end_turn", "stop_sequence":
		return provider.StopEnd
	case "tool_use":
		return provider.StopToolUse
	case "max_tokens":
		return provider.StopMaxTokens
	case "refusal":
		return provider.StopRefusal
	case "pause_turn":
		return provider.StopPause
	}
	return provider.StopOther
}

// streamed content, assembled block by block
type sblock struct {
	kind  string
	id    string
	name  string
	text  strings.Builder
	input strings.Builder
	sig   string
}

// readStream assembles a streamed message from server-sent events.
func (c *Client) readStream(r io.Reader, onDelta, onThinking func(string)) (provider.Response, error) {
	resp := provider.Response{Message: provider.Message{Role: provider.Assistant}}
	blocks := map[int]*sblock{}
	var order []int
	stop := ""
	var stopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[6:])
			continue
		case !strings.HasPrefix(line, "data:"):
			continue
		}
		data := strings.TrimSpace(line[5:])
		var ev struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message *struct {
				Model string `json:"model"`
				Usage usage  `json:"usage"`
			} `json:"message"`
			ContentBlock *struct {
				Type string          `json:"type"`
				ID   string          `json:"id"`
				Name string          `json:"name"`
				Text string          `json:"text"`
				In   json.RawMessage `json:"input"`
			} `json:"content_block"`
			Delta *struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				StopReason  string `json:"stop_reason"`
				StopDetails *struct {
					Category    string `json:"category"`
					Explanation string `json:"explanation"`
				} `json:"stop_details"`
			} `json:"delta"`
			Usage *usage `json:"usage"`
			Error *struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		if ev.Type == "" {
			ev.Type = event
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				resp.Model = ev.Message.Model
				resp.Usage = ev.Message.Usage.reading()
			}
		case "content_block_start":
			if ev.ContentBlock != nil {
				b := &sblock{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				b.text.WriteString(ev.ContentBlock.Text)
				blocks[ev.Index] = b
				order = append(order, ev.Index)
			}
		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil || ev.Delta == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				b.text.WriteString(ev.Delta.Text)
				if onDelta != nil {
					onDelta(ev.Delta.Text)
				}
			case "input_json_delta":
				b.input.WriteString(ev.Delta.PartialJSON)
			case "thinking_delta":
				b.text.WriteString(ev.Delta.Thinking)
				if onThinking != nil && ev.Delta.Thinking != "" {
					onThinking(ev.Delta.Thinking)
				}
			case "signature_delta":
				b.sig += ev.Delta.Signature
			}
		case "message_delta":
			if ev.Delta != nil {
				stop = ev.Delta.StopReason
				stopDetails = ev.Delta.StopDetails
			}
			if ev.Usage != nil {
				if ev.Usage.Output > 0 {
					resp.Usage.Output = ev.Usage.Output
				}
				if ev.Usage.Input > 0 {
					resp.Usage.Input = ev.Usage.Input
				}
				if ev.Usage.CacheRead > 0 {
					resp.Usage.CacheRead = ev.Usage.CacheRead
				}
				if ev.Usage.CacheWrite > 0 {
					resp.Usage.CacheWrite = ev.Usage.CacheWrite
				}
			}
		case "error":
			if ev.Error != nil {
				return provider.Response{}, fmt.Errorf("anthropic: stream error %s: %s", ev.Error.Type, ev.Error.Message)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: stream: %w", err)
	}
	var text []string
	for _, i := range order {
		b := blocks[i]
		switch b.kind {
		case "text":
			text = append(text, b.text.String())
		case "tool_use":
			in := json.RawMessage(strings.TrimSpace(b.input.String()))
			if len(in) == 0 || !json.Valid(in) {
				// a streamed input that does not parse is a fault the tool names, never a guess
				in = raw(map[string]string{"_malformed": b.input.String()})
			}
			resp.Message.ToolCalls = append(resp.Message.ToolCalls, provider.ToolCall{ID: b.id, Name: b.name, Input: in})
		case "thinking":
			resp.Message.Opaque = append(resp.Message.Opaque, raw(map[string]string{"type": "thinking", "thinking": b.text.String(), "signature": b.sig}))
		case "redacted_thinking":
			resp.Message.Opaque = append(resp.Message.Opaque, raw(map[string]string{"type": "redacted_thinking", "data": b.text.String()}))
		}
	}
	resp.Message.Text = strings.Join(text, "\n")
	resp.Stop = stopOf(stop)
	if resp.Stop == provider.StopRefusal && stopDetails != nil && resp.Message.Text == "" {
		resp.Message.Text = "[refused: " + stopDetails.Category + " — " + stopDetails.Explanation + "]"
	}
	if len(resp.Message.ToolCalls) > 0 {
		resp.Stop = provider.StopToolUse
	}
	return resp, nil
}
