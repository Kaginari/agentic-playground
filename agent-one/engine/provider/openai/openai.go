// Package openai speaks chat-completions over net/http. The base URL is configurable
// (OPENAI_BASE_URL) so OpenRouter, Ollama, vLLM, SGLang and any compatible endpoint answer the
// same call. Four adaptations for open models, each a switch on the Client (binary.md §Open
// models): tool calls as tagged text where native calling is broken (ToolCalls "text");
// guided decoding of a Subagent's report to the wire's JSON schema (Guided, on a Request that
// says Wire); the served context window read from /v1/models (ContextWindow); streaming.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/wire"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-4o"
)

// Client is one chat-completions endpoint. An empty APIKey is allowed (Ollama).
type Client struct {
	APIKey    string
	Model     string
	BaseURL   string
	HTTP      *http.Client
	Headers   map[string]string
	ToolCalls string // "native" (default) | "text"
	Guided    bool   // response_format with the wire's JSON schema on a Wire request
	Stream    bool   // stream when the request carries OnDelta
	MaxTokens int

	winOnce sync.Once
	window  int
	winErr  error
}

// New reads OPENAI_API_KEY (optional), OPENAI_BASE_URL and AGENT_ONE_MODEL.
func New(model string) (*Client, error) {
	if model == "" {
		model = provider.DefaultModel(DefaultModel)
	}
	base := provider.Getenv("OPENAI_BASE_URL")
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{APIKey: provider.Getenv("OPENAI_API_KEY"), Model: model, BaseURL: base, HTTP: &http.Client{Timeout: 10 * time.Minute}, ToolCalls: "native", Stream: true}, nil
}

func (c *Client) Name() string { return "openai/" + c.Model }

type function struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
}

type toolCall struct {
	Index    *int     `json:"index,omitempty"`
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
	Model          string          `json:"model"`
	Messages       []message       `json:"messages"`
	Tools          []toolDef       `json:"tools,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat json.RawMessage `json:"response_format,omitempty"`
	Stream         bool            `json:"stream,omitempty"`
	StreamOptions  json.RawMessage `json:"stream_options,omitempty"`
}

type usage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
	Details    struct {
		Cached int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u usage) reading() provider.Usage {
	return provider.Usage{Input: u.Prompt - u.Details.Cached, Output: u.Completion, CacheRead: u.Details.Cached}
}

type response struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// WireSchema is the envelope as a JSON schema, for guided decoding: a small model cannot drop
// @S or @U when the grammar demands them.
const WireSchema = `{"type":"object","properties":{"S":{"type":"string","description":"the status word (@S)"},"F":{"type":"array","items":{"type":"string"},"description":"findings, file:line fact (@F)"},"V":{"type":"array","items":{"type":"string"},"description":"verdicts with evidence (@V)"},"Q":{"type":"array","items":{"type":"string"},"description":"holes, named never guessed (@?)"},"U":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string","enum":["policy","team","ownership"]},"text":{"type":"string"}},"required":["kind","text"]},"description":"the unsaid (@U)"}},"required":["S","F","Q","U"]}`

// textToolIntro is what the model reads when tool calls travel as tagged text.
const textToolIntro = "\n\nTools are called as tagged text. To call one, write exactly:\n<tool_call>{\"name\": \"<tool>\", \"input\": {…}}</tool_call>\nOne call per tag; several tags run in order; results come back as <tool_result id=\"…\">…</tool_result>. Tools:\n"

var tagRe = regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>`)

// Encode turns a Request into the chat-completions agent: the system prompt becomes the first
// message, each tool result its own `tool` message.
func Encode(model string, req provider.Request) request {
	return (&Client{Model: model, ToolCalls: "native"}).encode(req, false)
}

func (c *Client) text() bool { return c.ToolCalls == "text" }

func (c *Client) encode(req provider.Request, stream bool) request {
	r := request{Model: c.Model, MaxTokens: req.MaxTokens, Stream: stream}
	if r.MaxTokens <= 0 {
		r.MaxTokens = c.MaxTokens
	}
	system := req.SystemText()
	if c.text() && len(req.Tools) > 0 {
		var b strings.Builder
		b.WriteString(textToolIntro)
		for _, t := range req.Tools {
			s := t.Schema
			if len(s) == 0 {
				s = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			fmt.Fprintf(&b, "- %s: %s — input schema %s\n", t.Name, t.Description, string(s))
		}
		system += b.String()
	}
	if system != "" {
		r.Messages = append(r.Messages, message{Role: "system", Content: system})
	}
	for _, m := range req.Messages {
		if c.text() {
			// no native tool messages: calls and results ride as text
			if m.Role == provider.Assistant {
				var b strings.Builder
				b.WriteString(m.Text)
				for _, tc := range m.ToolCalls {
					fmt.Fprintf(&b, "\n<tool_call>{\"name\": %q, \"input\": %s}</tool_call>", tc.Name, orEmpty(tc.Input))
				}
				r.Messages = append(r.Messages, message{Role: "assistant", Content: strings.TrimSpace(b.String())})
				continue
			}
			var b strings.Builder
			for _, tr := range m.ToolResults {
				content := tr.Content
				if tr.IsError {
					content = "ERROR: " + content
				}
				fmt.Fprintf(&b, "<tool_result id=%q>\n%s\n</tool_result>\n", tr.ID, content)
			}
			b.WriteString(m.Text)
			r.Messages = append(r.Messages, message{Role: "user", Content: strings.TrimSpace(b.String())})
			continue
		}
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
				am.ToolCalls = append(am.ToolCalls, toolCall{ID: tc.ID, Type: "function", Function: function{Name: tc.Name, Arguments: string(orEmpty(tc.Input))}})
			}
			r.Messages = append(r.Messages, am)
		} else if m.Text != "" {
			r.Messages = append(r.Messages, message{Role: "user", Content: m.Text})
		}
	}
	if !c.text() {
		for _, t := range req.Tools {
			s := t.Schema
			if len(s) == 0 {
				s = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			r.Tools = append(r.Tools, toolDef{Type: "function", Function: function{Name: t.Name, Description: t.Description, Parameters: s}})
		}
	}
	if c.Guided && req.Wire {
		r.ResponseFormat = json.RawMessage(`{"type":"json_schema","json_schema":{"name":"wire","schema":` + WireSchema + `}}`)
	}
	if stream {
		r.StreamOptions = json.RawMessage(`{"include_usage":true}`)
	}
	return r
}

func orEmpty(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return json.RawMessage(`{}`)
	}
	return in
}

func (c *Client) do(ctx context.Context, method, path string, agent []byte) (*http.Response, error) {
	var rd io.Reader
	if agent != nil {
		rd = bytes.NewReader(agent)
	}
	hr, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, rd)
	if err != nil {
		return nil, err
	}
	hr.Header.Set("content-type", "application/json")
	if c.APIKey != "" {
		hr.Header.Set("authorization", "Bearer "+c.APIKey)
	}
	for k, v := range c.Headers {
		hr.Header.Set(k, v)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(hr)
}

// Complete performs one chat-completions call.
func (c *Client) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if err := provider.Validate(req); err != nil {
		return provider.Response{}, err
	}
	stream := c.Stream && req.OnDelta != nil
	agent, err := json.Marshal(c.encode(req, stream))
	if err != nil {
		return provider.Response{}, err
	}
	res, err := c.do(ctx, "POST", "/chat/completions", agent)
	if err != nil {
		return provider.Response{}, fmt.Errorf("openai: %w", err)
	}
	defer res.Body.Close()
	var resp provider.Response
	if stream && res.StatusCode == 200 && strings.HasPrefix(res.Header.Get("content-type"), "text/event-stream") {
		resp, err = c.readStream(res.Body, req.OnDelta)
		if err != nil {
			return provider.Response{}, err
		}
	} else {
		rawBody, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		if err != nil {
			return provider.Response{}, err
		}
		var out response
		if err := json.Unmarshal(rawBody, &out); err != nil {
			return provider.Response{}, fmt.Errorf("openai: HTTP %d, unreadable agent: %w", res.StatusCode, err)
		}
		if res.StatusCode != 200 || out.Error != nil {
			msg := strings.TrimSpace(string(rawBody))
			if out.Error != nil {
				msg = out.Error.Type + ": " + out.Error.Message
			}
			return provider.Response{}, fmt.Errorf("openai: HTTP %d: %s", res.StatusCode, msg)
		}
		if len(out.Choices) == 0 {
			return provider.Response{}, fmt.Errorf("openai: no choices in the response")
		}
		ch := out.Choices[0]
		resp = provider.Response{Model: out.Model, Message: provider.Message{Role: provider.Assistant, Text: ch.Message.Content}, Usage: out.Usage.reading()}
		for _, tc := range ch.Message.ToolCalls {
			resp.Message.ToolCalls = append(resp.Message.ToolCalls, callOf(tc.ID, tc.Function.Name, tc.Function.Arguments))
		}
		resp.Stop = finishOf(ch.FinishReason)
		if req.OnDelta != nil && resp.Message.Text != "" && len(tagRe.FindAllString(resp.Message.Text, -1)) == 0 {
			req.OnDelta(resp.Message.Text)
		}
	}
	if c.text() {
		c.parseTextCalls(&resp)
	}
	if c.Guided && req.Wire && len(resp.Message.ToolCalls) == 0 {
		resp.Message.Text = RenderWire(resp.Message.Text)
	}
	if len(resp.Message.ToolCalls) > 0 {
		resp.Stop = provider.StopToolUse
	}
	return resp, nil
}

// callOf reads a native tool call; arguments that do not parse are a named fault.
func callOf(id, name, args string) provider.ToolCall {
	in := json.RawMessage(args)
	if strings.TrimSpace(args) == "" {
		in = json.RawMessage(`{}`)
	}
	if !json.Valid(in) {
		b, _ := json.Marshal(map[string]string{"_malformed": args})
		in = b
	}
	return provider.ToolCall{ID: id, Name: name, Input: in}
}

func finishOf(s string) provider.StopReason {
	switch s {
	case "stop":
		return provider.StopEnd
	case "tool_calls", "function_call":
		return provider.StopToolUse
	case "length":
		return provider.StopMaxTokens
	case "content_filter":
		return provider.StopRefusal
	}
	return provider.StopOther
}

// parseTextCalls lifts <tool_call> tags out of the text into ToolCalls. A tag whose JSON
// does not parse becomes a call named `_malformed` whose input names the fault, so the model
// meets one error result, never a guess.
func (c *Client) parseTextCalls(resp *provider.Response) {
	text := resp.Message.Text
	ms := tagRe.FindAllStringSubmatch(text, -1)
	if len(ms) == 0 {
		return
	}
	for i, m := range ms {
		id := fmt.Sprintf("call_%d_%d", len(resp.Message.ToolCalls), i+1)
		var call struct {
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
			Args  json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(m[1]), &call); err != nil || call.Name == "" {
			why := "tag is not {\"name\", \"input\"} JSON"
			if err != nil {
				why = err.Error()
			}
			b, _ := json.Marshal(map[string]string{"fault": why, "raw": m[1]})
			resp.Message.ToolCalls = append(resp.Message.ToolCalls, provider.ToolCall{ID: id, Name: "_malformed", Input: b})
			continue
		}
		in := call.Input
		if len(in) == 0 {
			in = call.Args
		}
		if len(in) == 0 {
			in = json.RawMessage(`{}`)
		}
		resp.Message.ToolCalls = append(resp.Message.ToolCalls, provider.ToolCall{ID: id, Name: call.Name, Input: in})
	}
	resp.Message.Text = strings.TrimSpace(tagRe.ReplaceAllString(text, ""))
}

// RenderWire turns a guided-decoding JSON answer into the envelope. Text that is not the
// schema's JSON is returned as is.
func RenderWire(text string) string {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "{") {
		return text
	}
	var w struct {
		S string   `json:"S"`
		F []string `json:"F"`
		V []string `json:"V"`
		Q []string `json:"Q"`
		U []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"U"`
	}
	if err := json.Unmarshal([]byte(t), &w); err != nil || w.S == "" {
		return text
	}
	rep := wire.Report{Status: w.S, Findings: w.F, Verdicts: w.V, Holes: w.Q}
	for _, u := range w.U {
		rep.Unsaid = append(rep.Unsaid, wire.Unsaid{Kind: u.Kind, Text: u.Text})
	}
	return rep.Emit(0, "")
}

// readStream assembles a streamed chat completion.
func (c *Client) readStream(r io.Reader, onDelta func(string)) (provider.Response, error) {
	resp := provider.Response{Message: provider.Message{Role: provider.Assistant}}
	var text strings.Builder
	calls := map[int]*toolCall{}
	var order []int
	finish := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			break
		}
		var ev struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta        message `json:"delta"`
				FinishReason string  `json:"finish_reason"`
			} `json:"choices"`
			Usage *usage `json:"usage"`
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		if ev.Error != nil {
			return provider.Response{}, fmt.Errorf("openai: stream error %s: %s", ev.Error.Type, ev.Error.Message)
		}
		if ev.Model != "" {
			resp.Model = ev.Model
		}
		if ev.Usage != nil {
			resp.Usage = ev.Usage.reading()
		}
		for _, ch := range ev.Choices {
			if ch.Delta.Content != "" {
				text.WriteString(ch.Delta.Content)
				if onDelta != nil && !c.text() {
					onDelta(ch.Delta.Content)
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				cur := calls[idx]
				if cur == nil {
					cur = &toolCall{}
					calls[idx] = cur
					order = append(order, idx)
				}
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				if tc.Function.Name != "" {
					cur.Function.Name = tc.Function.Name
				}
				cur.Function.Arguments += tc.Function.Arguments
			}
			if ch.FinishReason != "" {
				finish = ch.FinishReason
			}
		}
	}
	if err := sc.Err(); err != nil {
		return provider.Response{}, fmt.Errorf("openai: stream: %w", err)
	}
	resp.Message.Text = text.String()
	for _, i := range order {
		tc := calls[i]
		resp.Message.ToolCalls = append(resp.Message.ToolCalls, callOf(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}
	resp.Stop = finishOf(finish)
	if c.text() && onDelta != nil {
		if t := strings.TrimSpace(tagRe.ReplaceAllString(resp.Message.Text, "")); t != "" {
			onDelta(t)
		}
	}
	return resp, nil
}

// ContextWindow reads the served maximum for the model from /v1/models (vLLM's
// max_model_len, OpenRouter's context_length, or context_window); 0 with an error when the
// endpoint does not say. The reading is cached for the client's life.
func (c *Client) ContextWindow(ctx context.Context) (int, error) {
	c.winOnce.Do(func() {
		res, err := c.do(ctx, "GET", "/models", nil)
		if err != nil {
			c.winErr = fmt.Errorf("openai: /models: %w", err)
			return
		}
		defer res.Body.Close()
		rawBody, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		if res.StatusCode != 200 {
			c.winErr = fmt.Errorf("openai: /models: HTTP %d", res.StatusCode)
			return
		}
		var out struct {
			Data []struct {
				ID            string `json:"id"`
				MaxModelLen   int    `json:"max_model_len"`
				ContextLength int    `json:"context_length"`
				ContextWindow int    `json:"context_window"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rawBody, &out); err != nil {
			c.winErr = fmt.Errorf("openai: /models: %w", err)
			return
		}
		for _, m := range out.Data {
			if m.ID != c.Model {
				continue
			}
			for _, n := range []int{m.MaxModelLen, m.ContextLength, m.ContextWindow} {
				if n > 0 {
					c.window = n
					return
				}
			}
			c.winErr = fmt.Errorf("openai: /models lists %s without a context length", c.Model)
			return
		}
		c.winErr = fmt.Errorf("openai: /models does not list %s", c.Model)
	})
	return c.window, c.winErr
}
