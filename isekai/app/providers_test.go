package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/provider"
)

// loadCfg writes a project config into a fresh world and loads it with a fake environment.
func loadCfg(t *testing.T, yamlText string, env map[string]string) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "world")
	home := filepath.Join(dir, "home")
	_ = os.MkdirAll(filepath.Join(root, ".isekai"), 0o755)
	_ = os.MkdirAll(home, 0o755)
	if yamlText != "" {
		if err := os.WriteFile(filepath.Join(root, ".isekai", "config.yaml"), []byte(yamlText), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := config.LoadWith(config.Options{Dist: "isekai", Root: root, Home: home, Env: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return c, root
}

// anthropicStub answers every Messages call and records the last body.
func anthropicStub(t *testing.T, status int) (*httptest.Server, *map[string]interface{}) {
	t.Helper()
	var last map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		last = nil
		_ = json.Unmarshal(b, &last)
		w.Header().Set("content-type", "application/json")
		if status != 200 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"type":"overloaded_error","message":"busy"}}`)
			return
		}
		model, _ := last["model"].(string)
		_, _ = io.WriteString(w, `{"model":"`+model+`","stop_reason":"end_turn","content":[{"type":"text","text":"@S DONE\n@E 12"}],"usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":0}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

func TestRoutingAndMeter(t *testing.T) {
	srv, last := anthropicStub(t, 200)
	cfg, root := loadCfg(t, `
models:
  default: anthropic/claude-opus-5
  offices:
    great-sage: anthropic/claude-haiku-4-5
    raphael: {model: anthropic/claude-opus-5, effort: high}
  ranks:
    slime: anthropic/claude-sonnet-5
  tasks:
    drain: anthropic/claude-haiku-4-5
providers:
  anthropic:
    baseURL: `+srv.URL+`
    models:
      claude-haiku-4-5: {tier: 1, price: {input: 1, output: 5, cacheRead: 0.1}}
      claude-sonnet-5: {tier: 2, price: {input: 2, output: 10}}
      claude-opus-5: {tier: 3}
budgets:
  session: {tokens: 2000}
`, map[string]string{"ANTHROPIC_API_KEY": "sk-test"})
	ps := NewProviders(cfg, func(k string) string { return map[string]string{"ANTHROPIC_API_KEY": "sk-test"}[k] })
	cases := []struct {
		route Route
		model string
		slot  string
	}{
		{Route{}, "claude-opus-5", "models.default"},
		{Route{Office: "great-sage"}, "claude-haiku-4-5", "models.offices.great-sage"},
		{Route{Office: "analyst"}, "claude-haiku-4-5", "models.offices.great-sage"},
		{Route{Rank: "slime", Office: "raphael"}, "claude-opus-5", "models.offices.raphael"},
		{Route{Rank: "slime"}, "claude-sonnet-5", "models.ranks.slime"},
		{Route{Task: "drain"}, "claude-haiku-4-5", "models.tasks.drain"},
		{Route{Task: "gate"}, "claude-opus-5", "models.default"},
	}
	for _, c := range cases {
		pr, m, err := ps.Route(c.route)
		if err != nil || m.ID != c.model || m.Slot != c.slot {
			t.Errorf("%+v → %s (%s) %v; want %s (%s)", c.route, m.ID, m.Slot, err, c.model, c.slot)
			continue
		}
		if _, err := pr.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err != nil {
			t.Fatal(err)
		}
		if (*last)["model"] != c.model {
			t.Errorf("%+v: body model %v", c.route, (*last)["model"])
		}
	}
	// effort rides the office entry
	pr, _, _ := ps.Route(Route{Office: "raphael"})
	_, _ = pr.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if oc, _ := (*last)["output_config"].(map[string]interface{}); oc["effort"] != "high" {
		t.Errorf("effort: %v", (*last)["output_config"])
	}
	// the meter journals every call, priced from config; unpriced models carry usd null
	j := NewUsageJournal(filepath.Join(root, ".isekai", "instruments", "usage"), "sess-1")
	haiku, m, _ := ps.Route(Route{Office: "great-sage", Creature: "slime-auth", Rank: "slime"})
	price, _ := cfg.PriceFor(m.Ref.Model)
	meter := &Meter{Provider: haiku, Journal: j, Body: "slime-auth", Rank: "slime", Office: "great-sage", Model: m.Ref.Model, Price: price, Session: cfg.Budgets.Session}
	if _, err := meter.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	opus, m2, _ := ps.Route(Route{})
	meter2 := &Meter{Provider: opus, Journal: j, Body: "rimuru", Model: m2.Ref.Model, Session: cfg.Budgets.Session}
	if _, err := meter2.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("journal lines: %q", lines)
	}
	var rec map[string]interface{}
	_ = json.Unmarshal([]byte(lines[0]), &rec)
	for _, k := range []string{"ts", "session", "body", "rank", "office", "model", "provider", "input", "output", "cacheRead", "cacheWrite", "usd"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("record lacks %s: %s", k, lines[0])
		}
	}
	// 100 in × $1/M + 10 out × $5/M + 1000 cache read × $0.1/M = 0.0001 + 0.00005 + 0.0001
	if usd, _ := rec["usd"].(float64); usd < 0.00024 || usd > 0.00026 || rec["body"] != "slime-auth" || rec["office"] != "great-sage" || rec["model"] != "anthropic/claude-haiku-4-5" || rec["cacheRead"].(float64) != 1000 {
		t.Errorf("priced record: %s", lines[0])
	}
	_ = json.Unmarshal([]byte(lines[1]), &rec)
	if rec["usd"] != nil || rec["body"] != "rimuru" {
		t.Errorf("unpriced record: %s", lines[1])
	}
	if tot := j.Total(); tot.Calls != 2 || tot.Unpriced != 1 || tot.Usage.Input != 200 {
		t.Errorf("totals: %+v", tot)
	}
	// the session budget (2000 tokens) is crossed: 2 × (100+10+1000) = 2220
	if why := meter.Over(); !strings.Contains(why, "session token budget") {
		t.Errorf("budget: %q", why)
	}
	if _, err := meter.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Errorf("over budget call: %v", err)
	}
	recs, _ := ReadUsage(j.Dir)
	roll := RollupOf(recs, timeZero(), "")
	if roll.Records != 2 || roll.ByBody["slime-auth"].Calls != 1 || roll.ByModel["anthropic/claude-opus-5"].Unpriced != 1 || len(roll.Lines()) < 3 {
		t.Errorf("rollup: %+v", roll)
	}
}

func TestFallbackAndScript(t *testing.T) {
	srv, _ := anthropicStub(t, 529)
	root := t.TempDir()
	script := filepath.Join(root, "script.json")
	_ = os.WriteFile(script, []byte(`[
  {"when": "list", "calls": [{"name": "bash", "input": {"command": "ls"}}]},
  {"text": "@S DONE first\n@E 16"},
  {"when": "again", "text": "again", "repeat": true}
]`), 0o644)
	cfg, _ := loadCfg(t, `
models:
  default: {model: anthropic/claude-opus-5, fallback: mock/m}
providers:
  anthropic: {baseURL: `+srv.URL+`}
  mock: {enabled: true, script: `+script+`}
`, map[string]string{"ANTHROPIC_API_KEY": "sk-test"})
	ps := NewProviders(cfg, func(k string) string { return map[string]string{"ANTHROPIC_API_KEY": "sk-test"}[k] })
	pr, _, err := ps.Route(Route{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pr.(*fallback); !ok {
		t.Fatalf("fallback wrapper expected, got %T", pr)
	}
	ask := func(text string) provider.Response {
		r, err := pr.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: text}}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	// the primary answers 529 → the mock script answers: `when` first, then in order, repeat kept
	if r := ask("please list"); len(r.Message.ToolCalls) != 1 || r.Message.ToolCalls[0].Name != "bash" || r.Stop != provider.StopToolUse {
		t.Errorf("when step: %+v", r.Message)
	}
	if r := ask("anything"); r.Message.Text != "@S DONE first\n@E 16" {
		t.Errorf("ordered step: %q", r.Message.Text)
	}
	if r := ask("again"); r.Message.Text != "again" {
		t.Errorf("repeat: %q", r.Message.Text)
	}
	if r := ask("again"); r.Message.Text != "again" {
		t.Errorf("repeat twice: %q", r.Message.Text)
	}
	if r := ask("nothing left"); !strings.Contains(r.Message.Text, "@? mock script script.json exhausted") {
		t.Errorf("exhausted: %q", r.Message.Text)
	}
	// a mock with no script says so on the wire
	empty, err := newScriptProvider("mock/x", "", root)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := empty.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); !strings.Contains(r.Message.Text, "@S MOCK") {
		t.Errorf("no script: %q", r.Message.Text)
	}
	// a missing key is a hole, not a panic
	ps2 := NewProviders(cfg, func(string) string { return "" })
	if _, _, err := ps2.Route(Route{}); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") || len(ps2.Holes()) != 1 {
		t.Errorf("missing key: %v %v", err, ps2.Holes())
	}
}

func TestWindowAuto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"data":[{"id":"fake/vllm-coder","max_model_len":32768}]}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	cfg, _ := loadCfg(t, `
models: {default: vllm/fake/vllm-coder}
providers:
  vllm: {type: openai, baseURL: `+srv.URL+`/v1, contextWindow: auto, toolCalls: text, guidedDecoding: true}
  openrouter: {enabled: true, contextWindow: 128000}
`, nil)
	ps := NewProviders(cfg, func(string) string { return "" })
	m, _ := ps.Resolve(Route{})
	if m.Provider != "vllm" || m.ID != "fake/vllm-coder" {
		t.Fatalf("model id with a slash: %+v", m)
	}
	if n := ps.Window(context.Background(), m); n != 32768 {
		t.Errorf("auto window: %d %v", n, ps.Holes())
	}
	m2 := m
	m2.Entry = cfg.Providers["openrouter"]
	m2.Ref.Model, m2.Provider, m2.ID = "openrouter/x", "openrouter", "x"
	if n := ps.Window(context.Background(), m2); n != 128000 {
		t.Errorf("configured window: %d", n)
	}
}
