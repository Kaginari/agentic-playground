package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func world(t *testing.T) (Env, string) {
	t.Helper()
	root := t.TempDir()
	must := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, ".isekai", "isekai.md"), "# law\n")
	must(filepath.Join(root, ".isekai", "log.md"), "# log\n")
	must(filepath.Join(root, "README.md"), "one\ntwo\nthree\n")
	must(filepath.Join(root, "src", "a.go"), "package a\nfunc A() {}\n")
	must(filepath.Join(root, "src", "b.go"), "package a\nfunc B() { A() }\n")
	return Env{Root: root}, root
}

func TestClassifyCommand(t *testing.T) {
	env, _ := world(t)
	cases := map[string]Class{
		"cat README.md": Read, "git status && git log -1": Read, "go test ./...": Read, "echo hi > out.txt": Write,
		"node build.js": Write, "mkdir -p a/b": Write, "git commit -m x": Write, "git push origin main": Outward,
		"curl -s https://x.y": Outward, `bash -c "wget x"`: Outward, "ssh host ls": Outward, "cat /etc/hosts": Read,
		"cat /home/nobody/secret": Outward, "cp a ../../../../../../../../outside": Outward, "cd /tmp && ls": Outward,
		"rm -rf build": Destructive, "git reset --hard": Destructive, "git branch -D x": Destructive, "git push --force": Destructive,
		"find . -name x -delete": Destructive, "echo x > .isekai/log.md": Destructive, "echo x >> .isekai/log.md": Write,
		"sed -i s/a/b/ .isekai/isekai.md": Destructive, "ls | xargs rm": Destructive, "go get x": Outward, "npx foo": Outward,
		"cp a .isekai/canon/x.md": Destructive, "tee -a .isekai/log.md": Write, "tee .isekai/log.md": Destructive,
	}
	for cmd, want := range cases {
		if got := env.ClassifyCommand(cmd); got.Class != want {
			t.Errorf("%q: want %s, got %s (%s)", cmd, want, got.Class, got.Why)
		}
	}
}

func TestSettleOnlyTightens(t *testing.T) {
	env, _ := world(t)
	b := BashTool()
	c, holes := b.Settle(env, []byte(`{"command":"git push","class":"read"}`))
	if c.Class != Outward || len(holes) != 1 {
		t.Fatalf("%+v %v", c, holes)
	}
	c, holes = b.Settle(env, []byte(`{"command":"cat x","class":"destructive"}`))
	if c.Class != Destructive || c.Why != "declared destructive" || len(holes) != 0 {
		t.Fatalf("%+v %v", c, holes)
	}
	_, holes = b.Settle(env, []byte(`{"command":"cat x","class":"network"}`))
	if len(holes) != 1 {
		t.Fatal("an unknown declared class is a hole")
	}
	w := WriteTool()
	if c, _ := w.Settle(env, []byte(`{"path":".isekai/log.md"}`)); c.Class != Destructive {
		t.Fatalf("record %+v", c)
	}
	if c, _ := w.Settle(env, []byte(`{"path":"/home/nobody/x"}`)); c.Class != Outward {
		t.Fatalf("outside %+v", c)
	}
	if c, _ := w.Settle(env, []byte(`{"path":"notes.txt"}`)); c.Class != Write || len(c.Paths) != 1 {
		t.Fatalf("plain %+v", c)
	}
	env.IsRecord = func(rel string) bool { return rel == "notes.txt" }
	if c, _ := w.Settle(env, []byte(`{"path":"notes.txt"}`)); c.Class != Destructive {
		t.Fatalf("injected record test %+v", c)
	}
	env.WorldDir = ".agent-zero"
	if !env.Inside(filepath.Join(os.Getenv("HOME"), ".agent-zero", "x")) {
		t.Fatal("the machine-shared tier follows WorldDir")
	}
}

func run(t *testing.T, tl *Tool, env Env, input string) Result {
	t.Helper()
	return tl.Run(context.Background(), env, json.RawMessage(input))
}

func TestReadWriteEdit(t *testing.T) {
	env, root := world(t)
	r := run(t, ReadTool(), env, `{"path":"README.md"}`)
	if r.Err || !strings.Contains(r.Output, "     2\ttwo") {
		t.Fatalf("read %+v", r)
	}
	r = run(t, ReadTool(), env, `{"path":"README.md","offset":2,"limit":1}`)
	if r.Err || !strings.Contains(r.Output, "two") || strings.Contains(r.Output, "one") || !strings.Contains(r.Output, "1 more lines") {
		t.Fatalf("read window %+v", r)
	}
	if r = run(t, ReadTool(), env, `{"path":"nope"}`); !r.Err {
		t.Fatal("missing file is an error")
	}
	r = run(t, WriteTool(), env, `{"path":"new/dir/f.txt","content":"hello"}`)
	if r.Err || len(r.Wrote) != 1 || r.Wrote[0] != filepath.Join(root, "new/dir/f.txt") {
		t.Fatalf("write %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "new/dir/f.txt")); string(b) != "hello" {
		t.Fatal("content")
	}
	r = run(t, EditTool(), env, `{"path":"src/b.go","old":"A()","new":"C()"}`)
	if r.Err {
		t.Fatalf("edit %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "src/b.go")); !strings.Contains(string(b), "C()") {
		t.Fatal("edit content")
	}
	if r = run(t, EditTool(), env, `{"path":"src/a.go","old":"zzz","new":"y"}`); !r.Err || !strings.Contains(r.Output, "not found") {
		t.Fatalf("edit miss %+v", r)
	}
	if r = run(t, EditTool(), env, `{"path":"src/a.go","old":"a","new":"y"}`); !r.Err || !strings.Contains(r.Output, "matches") {
		t.Fatalf("edit ambiguous %+v", r)
	}
	if r = run(t, EditTool(), env, `{"path":"src/a.go","old":"a","new":"y","replace_all":true}`); r.Err {
		t.Fatalf("replace_all %+v", r)
	}
}

func TestBashGlobGrep(t *testing.T) {
	env, _ := world(t)
	r := run(t, BashTool(), env, `{"command":"echo hi; echo err >&2"}`)
	if r.Err || !strings.Contains(r.Output, "hi") || !strings.Contains(r.Output, "[stderr]\nerr") {
		t.Fatalf("bash %+v", r)
	}
	if r = run(t, BashTool(), env, `{"command":"exit 3"}`); !r.Err || !strings.Contains(r.Output, "[exit 3") {
		t.Fatalf("bash exit %+v", r)
	}
	if r = run(t, BashTool(), env, `{"command":"sleep 5","timeout":1}`); !r.Err || !strings.Contains(r.Output, "killed after 1s") {
		t.Fatalf("bash timeout %+v", r)
	}
	if r = run(t, BashTool(), env, `{"command":"pwd","cwd":"src"}`); r.Err || !strings.HasSuffix(strings.TrimSpace(r.Output), "/src") {
		t.Fatalf("bash cwd %+v", r)
	}
	r = run(t, GlobTool(), env, `{"pattern":"**/*.go"}`)
	if r.Err || !strings.Contains(r.Output, "src/a.go") || !strings.Contains(r.Output, "src/b.go") || strings.Contains(r.Output, "README") {
		t.Fatalf("glob %+v", r)
	}
	if r = run(t, GlobTool(), env, `{"pattern":"*.go","path":"src"}`); r.Err || strings.Count(r.Output, "\n") != 2 {
		t.Fatalf("glob in path %+v", r)
	}
	if r = run(t, GlobTool(), env, `{"pattern":"*.rs"}`); r.Err || !strings.Contains(r.Output, "no files") {
		t.Fatalf("glob none %+v", r)
	}
	r = run(t, GrepTool(), env, `{"pattern":"func [AB]","include":"*.go"}`)
	if r.Err || !strings.Contains(r.Output, "src/a.go:2:func A") || !strings.Contains(r.Output, "src/b.go:2:") {
		t.Fatalf("grep %+v", r)
	}
	if r = run(t, GrepTool(), env, `{"pattern":"("}`); !r.Err {
		t.Fatal("bad regexp is an error")
	}
	if r = run(t, GrepTool(), env, `{"pattern":"zzz"}`); r.Err || !strings.Contains(r.Output, "no matches") {
		t.Fatalf("grep none %+v", r)
	}
}

func TestRegistry(t *testing.T) {
	r := Builtins()
	if got := strings.Join(r.Names(), ","); got != "read,write,edit,bash,glob,grep" {
		t.Fatalf("names %s", got)
	}
	if len(r.Defs()) != 6 || r.Defs()[0].Name != "read" || len(r.Defs()[0].Schema) == 0 {
		t.Fatal("defs")
	}
	cut := r.Only("read", "grep", "nope")
	if strings.Join(cut.Names(), ",") != "read,grep" {
		t.Fatal("only")
	}
	r.Add(&Tool{Name: "dispatch", Class: Write})
	if _, ok := r.Get("dispatch"); !ok || len(r.Names()) != 7 {
		t.Fatal("add")
	}
	r.Remove("dispatch")
	if _, ok := r.Get("dispatch"); ok {
		t.Fatal("remove")
	}
	if c, ok := ParseClass(" Outward "); !ok || c != Outward || Max(Read, Destructive) != Destructive || Class(9).String() != "unknown" {
		t.Fatal("class helpers")
	}
}
