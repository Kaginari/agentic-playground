package provider

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	u := func(s string) Message { return Message{Role: User, Text: s} }
	a := func(calls ...ToolCall) Message { return Message{Role: Assistant, Text: "x", ToolCalls: calls} }
	res := func(ids ...string) Message {
		m := Message{Role: User}
		for _, id := range ids {
			m.ToolResults = append(m.ToolResults, ToolResult{ID: id, Content: "ok"})
		}
		return m
	}
	cases := []struct {
		name string
		msgs []Message
		want string
	}{
		{"empty", nil, "no messages"},
		{"ok", []Message{u("hi")}, ""},
		{"tool round trip", []Message{u("hi"), a(ToolCall{ID: "1", Name: "read"}), res("1")}, ""},
		{"two users", []Message{u("a"), u("b")}, "two user messages"},
		{"assistant first", []Message{a()}, "must follow"},
		{"ends on assistant", []Message{u("a"), a()}, "end with a user"},
		{"missing result", []Message{u("a"), a(ToolCall{ID: "1", Name: "x"}, ToolCall{ID: "2", Name: "y"}), res("1")}, "1 tool results for 2"},
		{"stray result", []Message{u("a"), a(), res("9")}, "no preceding"},
		{"wrong id", []Message{u("a"), a(ToolCall{ID: "1", Name: "x"}), res("2")}, "answers no call"},
		{"empty user", []Message{u("   ")}, "empty user"},
	}
	for _, c := range cases {
		err := Validate(Request{Messages: c.msgs})
		if c.want == "" && err != nil {
			t.Errorf("%s: unexpected %v", c.name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: want %q, got %v", c.name, c.want, err)
		}
	}
}

func TestUsage(t *testing.T) {
	u := Usage{Input: 100, Output: 5, CacheRead: 50, CacheWrite: 25}
	if u.Context() != 175 {
		t.Fatalf("context %d", u.Context())
	}
	if s := u.Add(u); s.Input != 200 || s.Output != 10 {
		t.Fatalf("add %+v", s)
	}
}

func TestDefaultModel(t *testing.T) {
	old := Getenv
	defer func() { Getenv = old }()
	Getenv = func(k string) string { return "" }
	if DefaultModel("m") != "m" {
		t.Fatal("fallback")
	}
	Getenv = func(k string) string {
		if k == "AGENT_ONE_MODEL" {
			return "env-model"
		}
		return ""
	}
	if DefaultModel("m") != "env-model" {
		t.Fatal("AGENT_ONE_MODEL wins")
	}
}
