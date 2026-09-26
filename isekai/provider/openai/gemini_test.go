package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/provider"
)

// TestGeminiCompatibility: the free tier answers 503 under load (retried), errors come as a JSON
// array (read), and a tool call's thought_signature must travel back on the next turn (echoed).
func TestGeminiCompatibility(t *testing.T) {
	var calls atomic.Int32
	var second map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		switch n {
		case 1:
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`[{"error":{"code":503,"message":"The model is overloaded.","status":"UNAVAILABLE"}}]`))
		case 2:
			_, _ = w.Write([]byte(`{"model":"g","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"},"extra_content":{"google":{"thought_signature":"SIG=="}}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`))
		default:
			_ = json.Unmarshal(body, &second)
			_, _ = w.Write([]byte(`{"model":"g","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":12,"completion_tokens":1}}`))
		}
	}))
	defer srv.Close()
	c := &Client{APIKey: "k", Model: "g", BaseURL: srv.URL, HTTP: srv.Client(), ToolCalls: "native"}
	tools := []provider.ToolDef{{Name: "bash", Description: "run", Schema: json.RawMessage(`{"type":"object"}`)}}
	msgs := []provider.Message{{Role: provider.User, Text: "list files"}}
	r1, err := c.Complete(context.Background(), provider.Request{Messages: msgs, Tools: tools})
	if err != nil {
		t.Fatalf("a 503 must be retried, got %v", err)
	}
	if len(r1.Message.ToolCalls) != 1 || !strings.Contains(string(r1.Message.ToolCalls[0].Extra), "SIG==") {
		t.Fatalf("thought_signature not kept on the call: %+v", r1.Message.ToolCalls)
	}
	msgs = append(msgs, r1.Message, provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "c1", Content: "a.txt"}}})
	if _, err := c.Complete(context.Background(), provider.Request{Messages: msgs, Tools: tools}); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(second)
	if !strings.Contains(string(b), `"thought_signature":"SIG=="`) {
		t.Fatalf("thought_signature not echoed back on the next turn: %s", b)
	}
	// an array-form error that is not retryable is read, not called unreadable
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`[{"error":{"code":400,"message":"Function call is missing a thought_signature.","status":"INVALID_ARGUMENT"}}]`))
	}))
	defer srv2.Close()
	c2 := &Client{Model: "g", BaseURL: srv2.URL, HTTP: srv2.Client()}
	_, err = c2.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "INVALID_ARGUMENT: Function call is missing a thought_signature.") {
		t.Fatalf("want the array-form message, got %v", err)
	}
}
