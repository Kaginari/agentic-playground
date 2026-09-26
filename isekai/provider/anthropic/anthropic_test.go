package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/provider"
)

func TestNewNeedsKey(t *testing.T) {
	old := provider.Getenv
	defer func() { provider.Getenv = old }()
	provider.Getenv = func(string) string { return "" }
	if _, err := New(""); err == nil {
		t.Fatal("expected an error without ANTHROPIC_API_KEY")
	}
	provider.Getenv = func(k string) string {
		switch k {
		case "ANTHROPIC_API_KEY":
			return "sk-test"
		case "ISEKAI_MODEL":
			return "claude-x"
		}
		return ""
	}
	c, err := New("")
	if err != nil || c.Model != "claude-x" || c.BaseURL != DefaultBaseURL {
		t.Fatalf("%v %+v", err, c)
	}
	if c, _ := New("flag-model"); c.Model != "flag-model" {
		t.Fatal("the flag wins over ISEKAI_MODEL")
	}
}

func TestComplete(t *testing.T) {
	var got map[string]interface{}
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		if r.URL.Path != "/v1/messages" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"claude-t","stop_reason":"tool_use","content":[{"type":"text","text":"let me look"},{"type":"tool_use","id":"tu_1","name":"read","input":{"path":"a.md"}}],"usage":{"input_tokens":120,"output_tokens":30,"cache_read_input_tokens":1000,"cache_creation_input_tokens":10}}`))
	}))
	defer srv.Close()
	c := &Client{APIKey: "sk-test", Model: "claude-t", BaseURL: srv.URL, HTTP: srv.Client()}
	req := provider.Request{
		System: "sys",
		Tools:  []provider.ToolDef{{Name: "read", Description: "read a file", Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}},
		Messages: []provider.Message{
			{Role: provider.User, Text: "hi"},
			{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "tu_0", Name: "read", Input: json.RawMessage(`{"path":"x"}`)}}},
			{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "tu_0", Content: "contents", IsError: true}}, Text: "and then"},
		},
	}
	resp, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Get("x-api-key") != "sk-test" || hdr.Get("anthropic-version") != Version {
		t.Fatalf("headers %v", hdr)
	}
	if got["model"] != "claude-t" || got["system"] != "sys" || got["max_tokens"].(float64) != defaultMax {
		t.Fatalf("body %v", got)
	}
	msgs := got["messages"].([]interface{})
	if len(msgs) != 3 {
		t.Fatalf("messages %v", msgs)
	}
	last := msgs[2].(map[string]interface{})["content"].([]interface{})
	tr := last[0].(map[string]interface{})
	if tr["type"] != "tool_result" || tr["tool_use_id"] != "tu_0" || tr["is_error"] != true || last[1].(map[string]interface{})["type"] != "text" {
		t.Fatalf("tool result encoding %v", last)
	}
	tools := got["tools"].([]interface{})
	if tools[0].(map[string]interface{})["input_schema"] == nil {
		t.Fatalf("tools %v", tools)
	}
	if resp.Stop != provider.StopToolUse || resp.Message.Text != "let me look" || len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Name != "read" || string(resp.Message.ToolCalls[0].Input) != `{"path":"a.md"}` {
		t.Fatalf("response %+v", resp)
	}
	if resp.Usage != (provider.Usage{Input: 120, Output: 30, CacheRead: 1000, CacheWrite: 10}) || resp.Usage.Context() != 1130 {
		t.Fatalf("usage %+v", resp.Usage)
	}
}

func TestErrorsAndRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("x-api-key"), "bad") {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","stop_reason":"refusal","stop_details":{"category":"cyber","explanation":"no"},"content":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	c := &Client{APIKey: "bad", Model: "m", BaseURL: srv.URL}
	_, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "authentication_error") {
		t.Fatalf("want a 401 error, got %v", err)
	}
	c.APIKey = "ok"
	resp, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if err != nil || resp.Stop != provider.StopRefusal || !strings.Contains(resp.Message.Text, "cyber") {
		t.Fatalf("refusal: %v %+v", err, resp)
	}
	if _, err := c.Complete(context.Background(), provider.Request{}); err == nil {
		t.Fatal("an invalid request is refused before any HTTP")
	}
}
