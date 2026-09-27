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

func TestNew(t *testing.T) {
	old := provider.Getenv
	defer func() { provider.Getenv = old }()
	provider.Getenv = func(k string) string {
		switch k {
		case "OPENAI_BASE_URL":
			return "http://localhost:11434/v1"
		case "AGENT_ONE_MODEL":
			return "llama3"
		}
		return ""
	}
	c, err := New("")
	if err != nil || c.BaseURL != "http://localhost:11434/v1" || c.Model != "llama3" || c.APIKey != "" {
		t.Fatalf("ollama-style config: %v %+v", err, c)
	}
	provider.Getenv = func(string) string { return "" }
	if c, _ := New(""); c.BaseURL != DefaultBaseURL || c.Model != DefaultModel {
		t.Fatalf("defaults %+v", c)
	}
}

func TestComplete(t *testing.T) {
	var got map[string]interface{}
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("authorization")
		agent, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(agent, &got)
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"model":"gpt-t","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}]}}],"usage":{"prompt_tokens":300,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":100}}}`))
	}))
	defer srv.Close()
	c := &Client{APIKey: "k", Model: "gpt-t", BaseURL: srv.URL + "/v1", HTTP: srv.Client()}
	req := provider.Request{
		System: "sys",
		Tools:  []provider.ToolDef{{Name: "bash", Description: "run", Schema: json.RawMessage(`{"type":"object"}`)}},
		Messages: []provider.Message{
			{Role: provider.User, Text: "hi"},
			{Role: provider.Assistant, Text: "ok", ToolCalls: []provider.ToolCall{{ID: "call_0", Name: "bash", Input: json.RawMessage(`{"command":"pwd"}`)}}},
			{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "call_0", Content: "/w", IsError: true}}},
		},
	}
	resp, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer k" {
		t.Fatalf("auth %q", auth)
	}
	msgs := got["messages"].([]interface{})
	if len(msgs) != 4 || msgs[0].(map[string]interface{})["role"] != "system" {
		t.Fatalf("messages %v", msgs)
	}
	am := msgs[2].(map[string]interface{})
	if am["role"] != "assistant" || am["tool_calls"].([]interface{})[0].(map[string]interface{})["function"].(map[string]interface{})["arguments"] != `{"command":"pwd"}` {
		t.Fatalf("assistant encoding %v", am)
	}
	tm := msgs[3].(map[string]interface{})
	if tm["role"] != "tool" || tm["tool_call_id"] != "call_0" || !strings.HasPrefix(tm["content"].(string), "ERROR: ") {
		t.Fatalf("tool message %v", tm)
	}
	if got["tools"].([]interface{})[0].(map[string]interface{})["type"] != "function" {
		t.Fatalf("tools %v", got["tools"])
	}
	if resp.Stop != provider.StopToolUse || resp.Message.ToolCalls[0].Name != "bash" || string(resp.Message.ToolCalls[0].Input) != `{"command":"ls"}` {
		t.Fatalf("response %+v", resp)
	}
	if resp.Usage != (provider.Usage{Input: 200, Output: 20, CacheRead: 100}) {
		t.Fatalf("usage %+v", resp.Usage)
	}
}

func TestErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit"}}`))
	}))
	defer srv.Close()
	c := &Client{Model: "m", BaseURL: srv.URL}
	_, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("want a 429 error, got %v", err)
	}
}

// OpenRouter can end an answer with finish_reason "error" under HTTP 200: a failed call, never a
// finished answer — in a plain response and in a stream alike.
func TestFinishReasonErrorIsAFailure(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"role":"assistant","content":"partial"},"finish_reason":"error"}]}`))
	}))
	defer plain.Close()
	c := &Client{Model: "m", BaseURL: plain.URL}
	if _, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err == nil || !strings.Contains(err.Error(), "finish_reason error") {
		t.Fatalf("plain: want a failed call, got %v", err)
	}
	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"m\",\"choices\":[{\"delta\":{\"content\":\"half an ans\"}}]}\n\n" +
			"data: {\"model\":\"m\",\"choices\":[{\"delta\":{},\"finish_reason\":\"error\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer stream.Close()
	c = &Client{Model: "m", BaseURL: stream.URL, Stream: true}
	_, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}, OnDelta: func(string) {}})
	if err == nil || !strings.Contains(err.Error(), "finish_reason error") {
		t.Fatalf("stream: want a failed call, got %v", err)
	}
}
