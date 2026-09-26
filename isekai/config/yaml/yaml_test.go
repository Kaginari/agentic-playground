package yaml

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Node {
	t.Helper()
	n, err := Parse([]byte(src), "t.yaml")
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
	return n
}

func TestScalars(t *testing.T) {
	n := mustParse(t, `
s: hello world
q1: 'it''s'
q2: "a\tb\u00e9"
b1: true
b2: False
i1: 42
i2: -7
i3: 0x1F
f1: 3.5
f2: 1e3
n1: null
n2: ~
n3:
url: http://x.y/z#frag
apos: it's fine
num_str: "42"
`)
	cases := map[string]struct {
		kind Kind
		text string
	}{
		"s": {String, "hello world"}, "q1": {String, "it's"}, "q2": {String, "a\tbé"},
		"b1": {Bool, "true"}, "b2": {Bool, "false"}, "i1": {Int, "42"}, "i2": {Int, "-7"},
		"i3": {Int, "31"}, "f1": {Float, "3.5"}, "f2": {Float, "1000"}, "n1": {Null, "null"},
		"n2": {Null, "null"}, "n3": {Null, "null"}, "url": {String, "http://x.y/z#frag"},
		"apos": {String, "it's fine"}, "num_str": {String, "42"},
	}
	for k, want := range cases {
		got := n.Get(k)
		if got == nil {
			t.Fatalf("%s missing", k)
		}
		if got.Kind != want.kind || got.Text() != want.text {
			t.Errorf("%s: got %s %q, want %s %q", k, got.Kind, got.Text(), want.kind, want.text)
		}
	}
}

func TestStructure(t *testing.T) {
	n := mustParse(t, `# comment
top:
  nested:
    deep: 1   # trailing comment
  list:
  - a
  - b: 1
    c: 2
  -
    d: 3
  - - x
    - y
  flow: {a: 1, b: [1, 2, "three"], c: {}}
  empty: []
`)
	top := n.Get("top")
	if top.Get("nested").Get("deep").Int != 1 {
		t.Fatal("deep")
	}
	l := top.Get("list")
	if l.Kind != List || len(l.Items) != 4 {
		t.Fatalf("list: %s", l.JSON())
	}
	if l.Items[0].Str != "a" || l.Items[1].Get("c").Int != 2 || l.Items[2].Get("d").Int != 3 {
		t.Fatalf("list items: %s", l.JSON())
	}
	if l.Items[3].Kind != List || l.Items[3].Items[1].Str != "y" {
		t.Fatalf("nested list: %s", l.Items[3].JSON())
	}
	f := top.Get("flow")
	if f.Get("b").Items[2].Str != "three" || f.Get("c").Kind != Map {
		t.Fatalf("flow: %s", f.JSON())
	}
	if top.Get("empty").Kind != List || len(top.Get("empty").Items) != 0 {
		t.Fatal("empty list")
	}
	if top.Line != 2 || top.Get("nested").Get("deep").Line != 4 || l.Items[1].Get("c").Line != 8 {
		t.Errorf("lines: top %d deep %d c %d", top.Line, top.Get("nested").Get("deep").Line, l.Items[1].Get("c").Line)
	}
}

func TestBlockScalars(t *testing.T) {
	n := mustParse(t, `
lit: |
  line one
    indented
  line three

fold: >
  a b
  c

  d
strip: |-
  x
keep: |+
  y

after: 1
`)
	if got := n.Get("lit").Str; got != "line one\n  indented\nline three\n" {
		t.Errorf("literal: %q", got)
	}
	if got := n.Get("fold").Str; got != "a b c\nd\n" {
		t.Errorf("folded: %q", got)
	}
	if got := n.Get("strip").Str; got != "x" {
		t.Errorf("strip: %q", got)
	}
	if got := n.Get("keep").Str; got != "y\n\n" {
		t.Errorf("keep: %q", got)
	}
	if n.Get("after").Int != 1 {
		t.Error("after")
	}
}

func TestRejections(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a: &x 1\nb: *x\n", "anchors"},
		{"a: 1\nb: *x\n", "aliases"},
		{"a: !!str 1\n", "tags"},
		{"%YAML 1.2\n---\na: 1\n", "directives"},
		{"---\na: 1\n---\nb: 2\n", "multiple documents"},
		{"? [a, b]\n: 1\n", "complex keys"},
		{"a: {[x]: 1}\n", "complex keys"},
		{"a:\n\tb: 1\n", "tab"},
		{"a: 1\na: 2\n", "duplicate key"},
		{"a: {x: 1, x: 2}\n", "duplicate key"},
		{"a: {x: 1\n", "unterminated"},
		{"a: [1, 2\n", "unterminated"},
		{"a: 'x\n", "unterminated"},
		{"a: \"x\n", "unterminated"},
		{"a: b: c\n", "nested mapping"},
		{"a: - x\n", "own line"},
		{"a:\n  b: 1\n c: 2\n", "unexpected indentation"},
		{"a: 1\n  b: 2\n", "unexpected indentation"},
		{"- a\nb: 1\n", "expected a `- ` list item"},
		{"a:\n  - x\n  y: 1\n", "expected a `- ` list item"},
		{"a: \"x\" junk\n", "unexpected"},
		{"a: @x\n", "reserved"},
		{"a: |\n  x\n y\n", "block scalar"},
		{"a: 1\n...\n", "document end"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.src), "t.yaml")
		if err == nil {
			t.Errorf("accepted %q", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %q lacks %q", c.src, err, c.want)
		}
		if !strings.HasPrefix(err.Error(), "t.yaml:") {
			t.Errorf("%q: error lacks file:line: %q", c.src, err)
		}
	}
}

func TestJSONEquivalence(t *testing.T) {
	y := mustParse(t, `
model: vllm/meta-llama/Llama-3.1-70B-Instruct
providers:
  vllm:
    type: openai
    baseURL: "http://gpu-box:8000/v1"
    apiKeyEnv: VLLM_API_KEY
    headers: {X-Team: platform}
    models:
      meta-llama/Llama-3.1-70B-Instruct: {contextWindow: 131072}
permissions:
  rules:
    - {match: "bash:git push*", action: allow}
    - match: "edit:**/*.lock"
      action: deny
rules:
  - id: no-force
    text: |
      Never force-push.
    scope: all
  - check: go vet ./...
    scope: rank:slime
    timeout: 30
compaction:
  trigger: {tokens: 150000, fraction: 0.8}
  enabled: false
`)
	j, err := ParseJSON([]byte(`{
  // a comment
  "model": "vllm/meta-llama/Llama-3.1-70B-Instruct",
  "providers": {
    "vllm": {
      "type": "openai", "baseURL": "http://gpu-box:8000/v1", "apiKeyEnv": "VLLM_API_KEY",
      "headers": {"X-Team": "platform"},
      "models": {"meta-llama/Llama-3.1-70B-Instruct": {"contextWindow": 131072}}
    }
  },
  "permissions": {"rules": [
    {"match": "bash:git push*", "action": "allow"},
    {"match": "edit:**/*.lock", "action": "deny"},
  ]},
  "rules": [
    {"id": "no-force", "text": "Never force-push.\n", "scope": "all"},
    {"check": "go vet ./...", "scope": "rank:slime", "timeout": 30}
  ],
  /* block */
  "compaction": {"trigger": {"tokens": 150000, "fraction": 0.8}, "enabled": false}
}`), "t.json")
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(y, j) {
		t.Fatalf("not equal:\n%s\n%s", y.Pretty(), j.Pretty())
	}
	if j.Get("compaction").Get("enabled").Line != 20 || y.Get("compaction").Get("enabled").Line != 26 {
		t.Errorf("lines: json %d yaml %d", j.Get("compaction").Get("enabled").Line, y.Get("compaction").Get("enabled").Line)
	}
}

func TestJSONRejections(t *testing.T) {
	cases := []struct{ src, want string }{
		{`{"a": 1, "a": 2}`, "duplicate key"},
		{`{"a": 1`, "unterminated"},
		{`{a: 1}`, "quoted key"},
		{`[1, 2`, "unterminated"},
		{`{"a": tru}`, "unexpected"},
		{`{"a": 1} x`, "after the document"},
		{"{\"a\": /* x }", "unterminated /*"},
		{"{\"a\": \"x\ny\"}", "newline"},
	}
	for _, c := range cases {
		_, err := ParseJSON([]byte(c.src), "t.json")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.src, err, c.want)
		}
	}
}

func TestEmptyAndTopLevel(t *testing.T) {
	if n := mustParse(t, ""); n.Kind != Null {
		t.Error("empty")
	}
	if n := mustParse(t, "---\n# only comments\n"); n.Kind != Null {
		t.Error("marker only")
	}
	if n := mustParse(t, "- a\n- b\n"); n.Kind != List || len(n.Items) != 2 {
		t.Error("top list")
	}
	if n := mustParse(t, "just a string\n"); n.Kind != String {
		t.Error("top scalar")
	}
	n := mustParse(t, "\"quoted key\": 1\n'k2': [a, 'b c', \"d,e\"]\n")
	if n.Get("quoted key").Int != 1 || n.Get("k2").Items[2].Str != "d,e" {
		t.Errorf("quoted keys: %s", n.JSON())
	}
}

func TestCoerce(t *testing.T) {
	if Coerce("true", "env", 0).Kind != Bool || Coerce("12", "env", 0).Kind != Int || Coerce("x", "env", 0).Kind != String {
		t.Error("coerce")
	}
}
