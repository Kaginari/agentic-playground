package yaml

import (
	"strings"
	"testing"
)

func TestEmitRoundTrip(t *testing.T) {
	src := `model: anthropic/claude-opus-5
providers:
  vllm:
    type: openai
    headers: {X-Team: platform}
    models: {}
rules:
  - id: a
    text: |
      Never force-push.
    scope: all
  - [1, 2]
  - - x
    - y
flags: [true, "42", null, 1.5]
odd: "a: b"
empty: ""
`
	n := mustParse(t, src)
	out := Emit(n)
	back, err := Parse([]byte(out), "emit.yaml")
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, out)
	}
	if !Equal(n, back) {
		t.Fatalf("round trip differs:\n%s\n---\n%s", out, back.Pretty())
	}
}

func TestEditInPlace(t *testing.T) {
	src := `# workspace config
model: anthropic/claude-opus-5   # the mount
tools:
  webfetch: {enabled: true}
  bash:
    timeoutMs: 120000
policy:
  wire:
    cap: 2048
`
	// replace a scalar, keep the comment
	out, err := Edit([]byte(src), "c.yaml", []string{"model"}, StringNode("vllm/x", "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "model: vllm/x   # the mount") || !strings.Contains(string(out), "# workspace config") {
		t.Errorf("scalar edit:\n%s", out)
	}
	// insert a nested new key under an existing block map
	out, err = Edit(out, "c.yaml", []string{"tools", "bash", "sandbox"}, StringNode("none", "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "    timeoutMs: 120000\n    sandbox: none\n") {
		t.Errorf("insert:\n%s", out)
	}
	// insert into a flow map rewrites it as a block
	out, err = Edit(out, "c.yaml", []string{"tools", "webfetch", "enabled"}, BoolNode(false, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  webfetch:\n    enabled: false\n") {
		t.Errorf("flow rewrite:\n%s", out)
	}
	// a brand-new top-level path
	out, err = Edit(out, "c.yaml", []string{"tools", "custom", "ls", "run"}, mustParse(t, "[ls, -la, \"{{path}}\"]"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  custom:\n    ls:\n      run:\n        - ls\n        - -la\n        - \"{{path}}\"\n") {
		t.Errorf("deep insert:\n%s", out)
	}
	// delete
	out, err = Edit(out, "c.yaml", []string{"policy"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "policy:") || strings.Contains(string(out), "cap:") {
		t.Errorf("delete:\n%s", out)
	}
	n, err := Parse(out, "c.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if n.Get("tools").Get("bash").Get("sandbox").Str != "none" || n.Get("tools").Get("custom").Get("ls").Get("run").Items[2].Str != "{{path}}" {
		t.Errorf("final tree: %s", n.JSON())
	}
	// empty document
	out, err = Edit(nil, "e.yaml", []string{"a", "b"}, IntNode(1, "", 0))
	if err != nil || string(out) != "a:\n  b: 1\n" {
		t.Errorf("empty doc: %q %v", out, err)
	}
}

func TestDiff(t *testing.T) {
	d := Diff("c.yaml", []byte("a: 1\nb: 2\nc: 3\n"), []byte("a: 1\nb: 5\nc: 3\nd: 4\n"))
	for _, want := range []string{"--- c.yaml", "-b: 2", "+b: 5", "+d: 4", "@@ -1,3 +1,4 @@"} {
		if !strings.Contains(d, want) {
			t.Errorf("diff lacks %q:\n%s", want, d)
		}
	}
	if Diff("x", []byte("same\n"), []byte("same\n")) != "" {
		t.Error("identical texts should diff empty")
	}
}
