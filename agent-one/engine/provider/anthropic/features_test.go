package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
)

// TestFeatures: declared tools go out schema-less; adaptive thinking, effort, fallbacks and
// cache_control ride the agent and headers; thinking blocks come back opaque and are replayed.
func TestFeatures(t *testing.T) {
	var got map[string]interface{}
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		agent, _ := io.ReadAll(r.Body)
		got = nil
		_ = json.Unmarshal(agent, &got)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"claude-opus-5","stop_reason":"tool_use","content":[{"type":"thinking","thinking":"hm","signature":"sig1"},{"type":"text","text":"ok"},{"type":"tool_use","id":"tu_1","name":"bash","input":{"command":"ls"}}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":900}}`))
	}))
	defer srv.Close()
	c := &Client{APIKey: "sk-test", Model: "claude-opus-5", BaseURL: srv.URL, HTTP: srv.Client(), Thinking: true, Effort: "high", Fallbacks: true, Cache: true}
	req := provider.Request{
		System:     "principles",
		SystemTail: "@RECALL a#1",
		Tools: []provider.ToolDef{
			{Name: "bash", Description: "custom", Schema: json.RawMessage(`{"type":"object"}`), Declare: map[string]json.RawMessage{"anthropic": json.RawMessage(`{"type":"bash_20250124","name":"bash"}`)}},
			{Name: "read", Description: "read a file", Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		},
		Messages: []provider.Message{{Role: provider.User, Text: "hi"}},
	}
	resp, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Get("anthropic-beta") != FallbackBeta {
		t.Errorf("fallback beta header: %q", hdr.Get("anthropic-beta"))
	}
	if got["fallbacks"] != "default" {
		t.Errorf("fallbacks agent: %v", got["fallbacks"])
	}
	if th, _ := got["thinking"].(map[string]interface{}); th["type"] != "adaptive" {
		t.Errorf("thinking: %v", got["thinking"])
	}
	if oc, _ := got["output_config"].(map[string]interface{}); oc["effort"] != "high" {
		t.Errorf("effort: %v", got["output_config"])
	}
	for _, k := range []string{"budget_tokens", "temperature"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s must never be sent", k)
		}
	}
	sys := got["system"].([]interface{})
	first := sys[0].(map[string]interface{})
	if len(sys) != 2 || first["text"] != "principles" || first["cache_control"].(map[string]interface{})["type"] != "ephemeral" {
		t.Errorf("system blocks: %v", sys)
	}
	if _, cached := sys[1].(map[string]interface{})["cache_control"]; cached {
		t.Error("the tail must not be cached")
	}
	tools := got["tools"].([]interface{})
	declared := tools[0].(map[string]interface{})
	if declared["type"] != "bash_20250124" || declared["name"] != "bash" || declared["input_schema"] != nil || declared["description"] != nil {
		t.Errorf("declared tool: %v", declared)
	}
	if custom := tools[1].(map[string]interface{}); custom["input_schema"] == nil || custom["type"] != nil {
		t.Errorf("custom tool: %v", custom)
	}
	if resp.Usage.CacheRead != 900 || resp.Stop != provider.StopToolUse || resp.Message.Text != "ok" || len(resp.Message.Opaque) != 1 {
		t.Fatalf("response: %+v", resp)
	}
	// replay: the thinking block leads the assistant message
	req.Messages = append(req.Messages, resp.Message, provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "tu_1", Content: "a b"}}})
	if _, err := c.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	asst := got["messages"].([]interface{})[1].(map[string]interface{})["content"].([]interface{})
	if asst[0].(map[string]interface{})["type"] != "thinking" || asst[0].(map[string]interface{})["signature"] != "sig1" {
		t.Errorf("thinking replay: %v", asst)
	}
	// switches off
	c.Thinking, c.Fallbacks, c.Cache, c.Effort = false, false, false, ""
	_, _ = c.Complete(context.Background(), provider.Request{System: "s", Messages: req.Messages[:1]})
	if got["thinking"] != nil || got["fallbacks"] != nil || got["output_config"] != nil || hdr.Get("anthropic-beta") != "" {
		t.Errorf("switches off: %v %v", got, hdr)
	}
	if _, cached := got["system"].([]interface{})[0].(map[string]interface{})["cache_control"]; cached {
		t.Error("cache off")
	}
}

func TestStopReasons(t *testing.T) {
	for want, agent := range map[provider.StopReason]string{
		provider.StopRefusal:   `{"stop_reason":"refusal","stop_details":{"category":"x","explanation":"y"},"content":[]}`,
		provider.StopPause:     `{"stop_reason":"pause_turn","content":[{"type":"text","text":"…"}]}`,
		provider.StopMaxTokens: `{"stop_reason":"max_tokens","content":[{"type":"text","text":"cut"}]}`,
		provider.StopEnd:       `{"stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(agent))
		}))
		c := &Client{APIKey: "k", Model: "m", BaseURL: srv.URL, HTTP: srv.Client()}
		resp, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
		srv.Close()
		if err != nil || resp.Stop != want {
			t.Errorf("%s: %v %v", want, resp.Stop, err)
		}
		if want == provider.StopRefusal && !strings.Contains(resp.Message.Text, "[refused: x — y]") {
			t.Errorf("refusal text: %q", resp.Message.Text)
		}
	}
}

// TestStream: SSE events assemble into one response; text deltas reach the callback as they
// arrive; custom tools ask for eager input streaming; a tool input is parsed whole.
func TestStream(t *testing.T) {
	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent, _ := io.ReadAll(r.Body)
		got = nil
		_ = json.Unmarshal(agent, &got)
		w.Header().Set("content-type", "text/event-stream")
		for _, ev := range []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"model":"claude-opus-5","usage":{"input_tokens":7,"output_tokens":1}}}`,
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"s1"}}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"hel"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"lo"}}`,
			`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tu_9","name":"read","input":{}}}`,
			`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"pa"}}`,
			`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"th\":\"a.md\"}"}}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":12}}`,
			`data: {"type":"message_stop"}`,
		} {
			_, _ = io.WriteString(w, ev+"\n\n")
		}
	}))
	defer srv.Close()
	c := &Client{APIKey: "k", Model: "m", BaseURL: srv.URL, HTTP: srv.Client(), Stream: true, Thinking: true}
	var deltas []string
	resp, err := c.Complete(context.Background(), provider.Request{
		Tools:    []provider.ToolDef{{Name: "read", Description: "r", Schema: json.RawMessage(`{"type":"object"}`)}},
		Messages: []provider.Message{{Role: provider.User, Text: "hi"}},
		OnDelta:  func(s string) { deltas = append(deltas, s) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["stream"] != true || got["tools"].([]interface{})[0].(map[string]interface{})["eager_input_streaming"] != true {
		t.Errorf("stream request: %v", got)
	}
	if strings.Join(deltas, "|") != "hel|lo" || resp.Message.Text != "hello" {
		t.Errorf("deltas %v text %q", deltas, resp.Message.Text)
	}
	if len(resp.Message.ToolCalls) != 1 || string(resp.Message.ToolCalls[0].Input) != `{"path":"a.md"}` || resp.Message.ToolCalls[0].ID != "tu_9" {
		t.Errorf("tool call: %+v", resp.Message.ToolCalls)
	}
	if resp.Usage.Input != 7 || resp.Usage.Output != 12 || resp.Stop != provider.StopToolUse || resp.Model != "claude-opus-5" {
		t.Errorf("usage/stop: %+v", resp)
	}
	if len(resp.Message.Opaque) != 1 || !strings.Contains(string(resp.Message.Opaque[0]), `"signature":"s1"`) {
		t.Errorf("thinking kept opaque: %s", resp.Message.Opaque)
	}
	// no callback: no streaming asked
	_, _ = c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if got["stream"] == true {
		t.Error("stream only with a delta callback")
	}
}
