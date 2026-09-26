package openai

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

func serve(t *testing.T, handler func(path string, agent map[string]interface{}) (string, string)) (*httptest.Server, *map[string]interface{}) {
	t.Helper()
	var last map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var agent map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &agent)
		if agent != nil {
			last = agent
		}
		ct, out := handler(r.URL.Path, agent)
		w.Header().Set("content-type", ct)
		_, _ = io.WriteString(w, out)
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

// TestTextToolCalls: with toolCalls text the tools ride the system prompt, calls come back as
// tags, results go out as text, and a malformed tag is a named fault, never a guess.
func TestTextToolCalls(t *testing.T) {
	srv, last := serve(t, func(path string, agent map[string]interface{}) (string, string) {
		return "application/json", `{"model":"llama","choices":[{"message":{"role":"assistant","content":"I will look.\n<tool_call>{\"name\":\"read\",\"input\":{\"path\":\"a.md\"}}</tool_call>\n<tool_call>{not json}</tool_call>"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4}}`
	})
	c := &Client{Model: "llama", BaseURL: srv.URL, HTTP: srv.Client(), ToolCalls: "text"}
	req := provider.Request{System: "sys", Tools: []provider.ToolDef{{Name: "read", Description: "read a file", Schema: json.RawMessage(`{"type":"object"}`)}}, Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}
	resp, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, hasTools := (*last)["tools"]; hasTools {
		t.Error("no native tools in text mode")
	}
	sys := (*last)["messages"].([]interface{})[0].(map[string]interface{})["content"].(string)
	if !strings.Contains(sys, "<tool_call>") || !strings.Contains(sys, "- read: read a file") {
		t.Errorf("system: %q", sys)
	}
	if len(resp.Message.ToolCalls) != 2 || resp.Message.ToolCalls[0].Name != "read" || string(resp.Message.ToolCalls[0].Input) != `{"path":"a.md"}` || resp.Stop != provider.StopToolUse || resp.Message.Text != "I will look." {
		t.Fatalf("calls: %+v text %q", resp.Message.ToolCalls, resp.Message.Text)
	}
	if bad := resp.Message.ToolCalls[1]; bad.Name != "_malformed" || !strings.Contains(string(bad.Input), "fault") {
		t.Errorf("malformed: %+v", bad)
	}
	// the results ride back as text on a user message
	req.Messages = append(req.Messages, resp.Message, provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: resp.Message.ToolCalls[0].ID, Content: "A"}, {ID: resp.Message.ToolCalls[1].ID, Content: "bad tag", IsError: true}}})
	if _, err := c.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	msgs := (*last)["messages"].([]interface{})
	asst := msgs[2].(map[string]interface{})
	user := msgs[3].(map[string]interface{})
	if asst["tool_calls"] != nil || !strings.Contains(asst["content"].(string), `<tool_call>{"name": "read"`) || user["role"] != "user" || !strings.Contains(user["content"].(string), `<tool_result id="call_0_1">`) || !strings.Contains(user["content"].(string), "ERROR: bad tag") {
		t.Errorf("replay: %v", msgs)
	}
}

func TestGuidedWire(t *testing.T) {
	srv, last := serve(t, func(path string, agent map[string]interface{}) (string, string) {
		return "application/json", `{"model":"llama","choices":[{"message":{"role":"assistant","content":"{\"S\":\"PASS\",\"F\":[\"a.go:1 one\"],\"Q\":[],\"U\":[{\"kind\":\"team\",\"text\":\"the fact\"}]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	})
	c := &Client{Model: "llama", BaseURL: srv.URL, HTTP: srv.Client(), Guided: true}
	resp, err := c.Complete(context.Background(), provider.Request{Wire: true, Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	rf, _ := (*last)["response_format"].(map[string]interface{})
	if rf["type"] != "json_schema" {
		t.Errorf("response_format: %v", (*last)["response_format"])
	}
	if !strings.HasPrefix(resp.Message.Text, "@S PASS\n") || !strings.Contains(resp.Message.Text, "@F a.go:1 one\n") || !strings.Contains(resp.Message.Text, "@U team the fact\n") || !strings.Contains(resp.Message.Text, "\n@E ") {
		t.Errorf("rendered: %q", resp.Message.Text)
	}
	// not a wire request: no grammar
	_, _ = c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if (*last)["response_format"] != nil {
		t.Error("grammar only on a wire request")
	}
	if RenderWire("plain text") != "plain text" {
		t.Error("non-JSON passes through")
	}
}

func TestContextWindow(t *testing.T) {
	srv, _ := serve(t, func(path string, agent map[string]interface{}) (string, string) {
		if path == "/models" {
			return "application/json", `{"data":[{"id":"other","context_length":8000},{"id":"llama","max_model_len":32768}]}`
		}
		return "application/json", `{}`
	})
	c := &Client{Model: "llama", BaseURL: srv.URL, HTTP: srv.Client()}
	if n, err := c.ContextWindow(context.Background()); err != nil || n != 32768 {
		t.Fatalf("%d %v", n, err)
	}
	c2 := &Client{Model: "nope", BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c2.ContextWindow(context.Background()); err == nil || !strings.Contains(err.Error(), "does not list") {
		t.Errorf("unknown model: %v", err)
	}
}

func TestStreaming(t *testing.T) {
	srv, last := serve(t, func(path string, agent map[string]interface{}) (string, string) {
		return "text/event-stream", strings.Join([]string{
			`data: {"model":"llama","choices":[{"delta":{"role":"assistant","content":"hel"}}]}`,
			`data: {"choices":[{"delta":{"content":"lo"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read","arguments":"{\"pa"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"x\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
			`data: [DONE]`,
		}, "\n\n") + "\n"
	})
	c := &Client{Model: "llama", BaseURL: srv.URL, HTTP: srv.Client(), Stream: true}
	var deltas []string
	resp, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}, OnDelta: func(s string) { deltas = append(deltas, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if (*last)["stream"] != true || strings.Join(deltas, "|") != "hel|lo" || resp.Message.Text != "hello" {
		t.Errorf("stream: %v %v %q", (*last)["stream"], deltas, resp.Message.Text)
	}
	if len(resp.Message.ToolCalls) != 1 || string(resp.Message.ToolCalls[0].Input) != `{"path":"x"}` || resp.Stop != provider.StopToolUse || resp.Usage.Input != 3 {
		t.Errorf("calls: %+v %v", resp.Message.ToolCalls, resp.Usage)
	}
}
