package mock

import (
	"context"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/provider"
)

func TestScript(t *testing.T) {
	m := New(Call("1", "read", map[string]string{"path": "a"}), Text("done"))
	req := provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}
	r1, err := m.Complete(context.Background(), req)
	if err != nil || r1.Stop != provider.StopToolUse || r1.Message.ToolCalls[0].Name != "read" || string(r1.Message.ToolCalls[0].Input) != `{"path":"a"}` {
		t.Fatalf("%v %+v", err, r1)
	}
	if r1.Usage.Context() == 0 {
		t.Fatal("usage should rise like a real provider's")
	}
	req.Messages = append(req.Messages, r1.Message, provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "1", Content: "x"}}})
	r2, err := m.Complete(context.Background(), req)
	if err != nil || r2.Stop != provider.StopEnd || r2.Message.Text != "done" || r2.Usage.Context() <= r1.Usage.Context() {
		t.Fatalf("%v %+v", err, r2)
	}
	if _, err := m.Complete(context.Background(), req); err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("want exhaustion, got %v", err)
	}
	if len(m.Requests) != 3 {
		t.Fatalf("requests recorded: %d", len(m.Requests))
	}
	if _, err := m.Complete(context.Background(), provider.Request{}); err == nil {
		t.Fatal("validation on")
	}
	m.Fallback = &provider.Response{Message: provider.Message{Text: "fallback"}}
	if r, err := m.Complete(context.Background(), req); err != nil || r.Message.Text != "fallback" || r.Message.Role != provider.Assistant {
		t.Fatalf("fallback %v %+v", err, r)
	}
}
