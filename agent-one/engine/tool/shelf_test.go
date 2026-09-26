package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/shell"
)

func call(t *testing.T, tl *Tool, env Env, input string) Result {
	t.Helper()
	return tl.Run(context.Background(), env, json.RawMessage(input))
}

func TestDeclareCarriesProviderShape(t *testing.T) {
	b, _ := NewBashTool(BashOptions{Enabled: true})
	d := b.Def()
	raw, ok := d.DeclareFor("anthropic")
	if !ok || !strings.Contains(string(raw), `"bash_20250124"`) {
		t.Fatalf("%s %v", raw, ok)
	}
	if _, ok := d.DeclareFor("openai"); ok {
		t.Fatal("openai has no special shape")
	}
	if _, ok := ReadTool().Def().DeclareFor("anthropic"); ok {
		t.Fatal("plain tools declare nothing")
	}
	e := EditorTool(EditorOptions{Enabled: true}).Def()
	if raw, _ := e.DeclareFor("anthropic"); !strings.Contains(string(raw), "text_editor_20250728") || e.Name != "str_replace_based_edit_tool" {
		t.Fatalf("%s %s", raw, e.Name)
	}
}

func TestBashOnShell(t *testing.T) {
	env, root := workspace(t)
	b, closeB := NewBashTool(BashOptions{Enabled: true, Timeout: 5 * time.Second, Shell: shell.Options{Env: []string{"PATH=" + os.Getenv("PATH"), "X_TOKEN=leak"}}})
	defer closeB()
	if r := call(t, b, env, `{"command":"export FOO=1; cd src"}`); r.Err || !strings.Contains(r.Output, "[cwd → src]") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, b, env, `{"command":"echo $FOO $(basename $PWD) ${X_TOKEN:-scrubbed}"}`); r.Output != "1 src scrubbed\n" {
		t.Fatalf("%+v", r)
	}
	if r := call(t, b, env, `{"command":"exit 4"}`); !r.Err || !strings.Contains(r.Output, "[exit 4") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, b, env, `{"restart":true}`); r.Err || !strings.Contains(r.Output, "restarted") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, b, env, `{"command":"echo ${FOO:-gone}; pwd"}`); r.Output != "gone\n"+root+"\n" {
		t.Fatalf("%+v", r)
	}
	if r := call(t, b, env, `{"command":"echo start; sleep 20","background":"bg1"}`); r.Err || !strings.Contains(r.Output, "job bg1 started") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, b, env, `{"command":"sleep 30","timeout":1}`); !r.Err || !strings.Contains(r.Output, "timed out") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, b, env, `{"command":"pwd","cwd":"src"}`); r.Output != filepath.Join(root, "src")+"\n[cwd → src]\n" {
		t.Fatalf("%+v", r)
	}
	// The classifier path is the old one.
	if c, _ := b.Settle(env, []byte(`{"command":"git push"}`)); c.Class != Outward {
		t.Fatal(c)
	}
	if c, _ := b.Settle(env, []byte(`{"restart":true}`)); c.Class != Read {
		t.Fatal(c)
	}
	off, _ := NewBashTool(BashOptions{})
	if r := call(t, off, env, `{"command":"true"}`); !r.Err || !strings.Contains(r.Output, "tools.bash.enabled") {
		t.Fatalf("%+v", r)
	}
	// One session per tool instance: the second tool sees its own shell.
	other, closeOther := NewBashTool(BashOptions{Enabled: true})
	defer closeOther()
	if r := call(t, other, env, `{"command":"echo ${FOO:-own}"}`); r.Output != "own\n" {
		t.Fatalf("%+v", r)
	}
	// A caller-owned session survives the tool's close.
	own, err := shell.New(shell.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	owned, closeOwned := NewBashTool(BashOptions{Enabled: true, Session: own})
	closeOwned()
	if r := call(t, owned, env, `{"command":"echo alive"}`); r.Output != "alive\n" {
		t.Fatalf("%+v", r)
	}
}

func TestEditorTool(t *testing.T) {
	env, root := workspace(t)
	ed := EditorTool(EditorOptions{Enabled: true})
	if r := call(t, ed, env, `{"command":"view","path":"README.md"}`); r.Err || !strings.Contains(r.Output, "     2\ttwo") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, ed, env, `{"command":"view","path":"README.md","view_range":[2,-1]}`); r.Err || strings.Contains(r.Output, "one") || !strings.Contains(r.Output, "three") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, ed, env, `{"command":"view","path":"src"}`); r.Err || !strings.Contains(r.Output, "a.go") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, ed, env, `{"command":"create","path":"new/x.txt","file_text":"alpha\nbeta\n"}`); r.Err || len(r.Wrote) != 1 {
		t.Fatalf("%+v", r)
	}
	if r := call(t, ed, env, `{"command":"create","path":"new/x.txt","file_text":"again"}`); !r.Err || !strings.Contains(r.Output, "exists") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, ed, env, `{"command":"str_replace","path":"new/x.txt","old_str":"beta","new_str":"gamma"}`); r.Err {
		t.Fatalf("%+v", r)
	}
	if r := call(t, ed, env, `{"command":"str_replace","path":"new/x.txt","old_str":"a","new_str":"b"}`); !r.Err || !strings.Contains(r.Output, "matches") {
		t.Fatalf("ambiguous must fail: %+v", r)
	}
	if r := call(t, ed, env, `{"command":"str_replace","path":"new/x.txt","old_str":"zzz","new_str":"b"}`); !r.Err || !strings.Contains(r.Output, "not found") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, ed, env, `{"command":"insert","path":"new/x.txt","insert_line":1,"insert_text":"mid"}`); r.Err {
		t.Fatalf("%+v", r)
	}
	got, _ := os.ReadFile(filepath.Join(root, "new", "x.txt"))
	if string(got) != "alpha\nmid\ngamma\n" {
		t.Fatalf("%q", got)
	}
	if r := call(t, ed, env, `{"command":"insert","path":"new/x.txt","insert_line":9,"insert_text":"x"}`); !r.Err {
		t.Fatal("out of range")
	}
	if r := call(t, ed, env, `{"command":"frobnicate","path":"new/x.txt"}`); !r.Err {
		t.Fatal("unknown command")
	}
	// Confinement.
	for _, p := range []string{"../outside.txt", "/etc/passwd", filepath.Join(filepath.Dir(root), "sibling"), "src/../../escape"} {
		if r := call(t, ed, env, fmt.Sprintf(`{"command":"view","path":%q}`, p)); !r.Err || !strings.Contains(r.Output, "outside the workspace root") {
			t.Fatalf("%s: %+v", p, r)
		}
	}
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		if r := call(t, ed, env, `{"command":"view","path":"link/secret"}`); !r.Err || !strings.Contains(r.Output, "symlink") {
			t.Fatalf("symlink escape: %+v", r)
		}
		if r := call(t, ed, env, `{"command":"create","path":"link/new","file_text":"x"}`); !r.Err {
			t.Fatalf("symlink escape on create: %+v", r)
		}
	}
	if c, _ := ed.Settle(env, []byte(`{"command":"str_replace","path":".agent-one/log.md"}`)); c.Class != Destructive {
		t.Fatal(c)
	}
	if c, _ := ed.Settle(env, []byte(`{"command":"view","path":".agent-one/log.md"}`)); c.Class != Read {
		t.Fatal(c)
	}
	if r := call(t, EditorTool(EditorOptions{}), env, `{"command":"view","path":"README.md"}`); !r.Err {
		t.Fatal("disabled")
	}
}

func TestLsTool(t *testing.T) {
	env, root := workspace(t)
	_ = os.WriteFile(filepath.Join(root, ".hidden"), nil, 0o644)
	ls := LsTool(LsOptions{Enabled: true})
	r := call(t, ls, env, `{}`)
	if r.Err || !strings.Contains(r.Output, "src/\n") || strings.Contains(r.Output, "a.go") || strings.Contains(r.Output, ".hidden") {
		t.Fatalf("%+v", r)
	}
	r = call(t, ls, env, `{"depth":2,"all":true}`)
	if !strings.Contains(r.Output, "src/a.go") || !strings.Contains(r.Output, ".hidden") {
		t.Fatalf("%+v", r)
	}
	if c, _ := ls.Settle(env, []byte(`{"path":"/home/nobody"}`)); c.Class != Outward {
		t.Fatal(c)
	}
}

func TestMultiEditAtomic(t *testing.T) {
	env, root := workspace(t)
	me := MultiEditTool(MultiEditOptions{Enabled: true})
	r := call(t, me, env, `{"path":"README.md","edits":[{"old":"one","new":"uno"},{"old":"nope","new":"x"}]}`)
	if !r.Err || !strings.Contains(r.Output, "nothing written") {
		t.Fatalf("%+v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "README.md")); string(got) != "one\ntwo\nthree\n" {
		t.Fatalf("file changed on failure: %q", got)
	}
	r = call(t, me, env, `{"path":"README.md","edits":[{"old":"one","new":"uno"},{"old":"t","new":"T","replace_all":true}]}`)
	if r.Err || len(r.Wrote) != 1 {
		t.Fatalf("%+v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "README.md")); string(got) != "uno\nTwo\nThree\n" {
		t.Fatalf("%q", got)
	}
}

const samplePatch = `diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1,3 +1,4 @@
 one
-two
+deux
 three
+four
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+hello
+workspace
--- a/src/b.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package a
-func B() { A() }
`

func TestPatchTool(t *testing.T) {
	env, root := workspace(t)
	fps, err := ParseUnified(samplePatch)
	if err != nil || len(fps) != 3 || fps[0].Path() != "README.md" || fps[1].Old != "/dev/null" || fps[2].New != "/dev/null" {
		t.Fatalf("%+v %v", fps, err)
	}
	p := PatchTool(PatchOptions{Enabled: true})
	in, _ := json.Marshal(map[string]string{"patch": samplePatch})
	if c, _ := p.Settle(env, in); c.Class != Destructive || !strings.Contains(c.Why, "deletes") || len(c.Paths) != 3 {
		t.Fatalf("%+v", c)
	}
	r := p.Run(context.Background(), env, in)
	if r.Err || len(r.Wrote) != 3 {
		t.Fatalf("%+v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "README.md")); string(got) != "one\ndeux\nthree\nfour\n" {
		t.Fatalf("%q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "new.txt")); string(got) != "hello\nworkspace\n" {
		t.Fatalf("%q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "b.go")); err == nil {
		t.Fatal("b.go should be deleted")
	}
	// A hunk stated at the wrong line still applies by offset; a hunk that matches nowhere
	// fails the whole patch and writes nothing.
	_ = os.WriteFile(filepath.Join(root, "off.txt"), []byte("x\ny\nz\nw\n"), 0o644)
	ok := "--- a/off.txt\n+++ b/off.txt\n@@ -1,2 +1,2 @@\n z\n-w\n+W\n"
	in, _ = json.Marshal(map[string]string{"patch": ok})
	if r := p.Run(context.Background(), env, in); r.Err {
		t.Fatalf("%+v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "off.txt")); string(got) != "x\ny\nz\nW\n" {
		t.Fatalf("%q", got)
	}
	bad := "--- a/off.txt\n+++ b/off.txt\n@@ -1,1 +1,1 @@\n-x\n+X\n--- a/README.md\n+++ b/README.md\n@@ -1,1 +1,1 @@\n-nothing\n+here\n"
	in, _ = json.Marshal(map[string]string{"patch": bad})
	if r := p.Run(context.Background(), env, in); !r.Err || !strings.Contains(r.Output, "does not apply") {
		t.Fatalf("%+v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "off.txt")); string(got) != "x\ny\nz\nW\n" {
		t.Fatalf("atomicity: %q", got)
	}
	if _, err := ParseUnified("just text"); err == nil {
		t.Fatal("no headers")
	}
}

func TestClassifyGit(t *testing.T) {
	env, _ := workspace(t)
	cases := map[string]Class{
		"status": Read, "log -3 --oneline": Read, "diff --stat": Read, "branch -a": Read, "remote -v": Read, "stash list": Read, "config --get user.name": Read,
		"add .": Write, "commit -m x": Write, "checkout -b feat": Write, "stash": Write, "tag v1": Write, "branch feat": Write, "config user.name x": Write,
		"push origin main": Outward, "fetch": Outward, "pull --rebase": Outward, "clone https://x": Outward, "remote add o https://x": Outward,
		"reset --hard": Destructive, "clean -fd": Destructive, "rebase -i HEAD~3": Destructive, "push --force": Destructive, "push -f": Destructive, "push origin +main": Destructive,
		"branch -D x": Destructive, "tag -d v1": Destructive, "stash drop": Destructive, "checkout -- .": Destructive, "rm -r x": Destructive, "frob": Write,
	}
	for line, want := range cases {
		args, _ := SplitArgs(line)
		if got := env.ClassifyGit(args); got.Class != want {
			t.Errorf("git %s: want %s got %s (%s)", line, want, got.Class, got.Why)
		}
	}
	if a, err := SplitArgs(`commit -m "a b" 'c d' e\ f`); err != nil || len(a) != 5 || a[2] != "a b" || a[3] != "c d" || a[4] != "e f" {
		t.Fatalf("%q %v", a, err)
	}
	if _, err := SplitArgs(`"open`); err == nil {
		t.Fatal("unterminated")
	}
}

func TestGitTool(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	env, root := workspace(t)
	g := GitTool(GitOptions{Enabled: true})
	if r := call(t, g, env, `{"args":["init","-q"]}`); r.Err {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatal("init did not land in the root")
	}
	if r := call(t, g, env, `{"command":"status --porcelain"}`); r.Err || !strings.Contains(r.Output, "README.md") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, g, env, `{"args":["nonsense-sub"]}`); !r.Err || !strings.Contains(r.Output, "[exit") {
		t.Fatalf("%+v", r)
	}
	if c, _ := g.Settle(env, []byte(`{"command":"git push --force"}`)); c.Class != Destructive {
		t.Fatal(c)
	}
	if r := call(t, GitTool(GitOptions{}), env, `{"args":["status"]}`); !r.Err {
		t.Fatal("disabled")
	}
}

func TestWebFetch(t *testing.T) {
	env, _ := workspace(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<html><head><title>T &amp; T</title><style>x{}</style><script>var a=1;</script></head><body><h1>Hello</h1><p>Some <b>bold</b> text &lt;here&gt;.</p><a href="/x">link</a><!-- c --></body></html>`)
		case "/big":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(strings.Repeat("a", 5000)))
		case "/bin":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte{0x89, 'P', 'N', 'G', 0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	wf := WebFetchTool(WebFetchOptions{Enabled: true, MaxBytes: 1000})
	r := call(t, wf, env, fmt.Sprintf(`{"url":"%s/page"}`, srv.URL))
	if r.Err {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{"# T & T", "Hello", "Some bold text <here>.", "link (/x)"} {
		if !strings.Contains(r.Output, want) {
			t.Errorf("missing %q in %s", want, r.Output)
		}
	}
	if strings.Contains(r.Output, "var a=1") || strings.Contains(r.Output, "<b>") {
		t.Fatalf("scripts/tags leaked: %s", r.Output)
	}
	if r := call(t, wf, env, fmt.Sprintf(`{"url":"%s/page","raw":true}`, srv.URL)); !strings.Contains(r.Output, "<h1>") {
		t.Fatal("raw keeps html")
	}
	if r := call(t, wf, env, fmt.Sprintf(`{"url":"%s/big"}`, srv.URL)); !strings.Contains(r.Output, "truncated at the cap") || !strings.Contains(r.Output, "(1000 bytes") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, wf, env, fmt.Sprintf(`{"url":"%s/bin"}`, srv.URL)); !r.Err || !strings.Contains(r.Output, "not text") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, wf, env, fmt.Sprintf(`{"url":"%s/missing"}`, srv.URL)); !r.Err || !strings.Contains(r.Output, " 404 ") {
		t.Fatalf("%+v", r)
	}
	if r := call(t, wf, env, `{"url":"ftp://x"}`); !r.Err {
		t.Fatal("scheme")
	}
	if c, _ := wf.Settle(env, []byte(`{"url":"https://example.com/a"}`)); c.Class != Outward || c.Why != "fetches example.com" {
		t.Fatal(c)
	}
	if r := call(t, WebFetchTool(WebFetchOptions{}), env, `{"url":"http://x"}`); !r.Err || !strings.Contains(r.Output, "tools.webfetch.enabled") {
		t.Fatal(r)
	}
}

func TestWebSearch(t *testing.T) {
	env, _ := workspace(t)
	none := WebSearchOptions{Enabled: true}
	if none.Available() {
		t.Fatal("no backend → unavailable")
	}
	if r := call(t, WebSearchTool(none), env, `{"query":"x"}`); !r.Err || !strings.Contains(r.Output, "no backend") {
		t.Fatalf("%+v", r)
	}
	var gotN int
	be := SearcherFunc(func(ctx context.Context, q string, n int) ([]Hit, error) {
		gotN = n
		return []Hit{{Title: "A", URL: "https://a", Snippet: "sa"}, {Title: "B", URL: "https://b"}}, nil
	})
	ws := WebSearchTool(WebSearchOptions{Enabled: true, Backend: be, MaxResults: 5})
	r := call(t, ws, env, `{"query":"agent-one","n":50}`)
	if r.Err || gotN != 5 || !strings.Contains(r.Output, "1. A\n   https://a\n   sa\n2. B") {
		t.Fatalf("%d %+v", gotN, r)
	}
	if c, _ := ws.Settle(env, []byte(`{"query":"q"}`)); c.Class != Outward {
		t.Fatal(c)
	}
}

func TestAskTool(t *testing.T) {
	env, _ := workspace(t)
	var got Question
	ask := AskTool(AskOptions{Enabled: true, Asker: func(ctx context.Context, q Question) (string, error) { got = q; return "  yes  ", nil }})
	r := call(t, ask, env, `{"question":"go on?","options":["yes","no"]}`)
	if r.Err || r.Output != "yes" || got.Text != "go on?" || len(got.Options) != 2 {
		t.Fatalf("%+v %+v", r, got)
	}
	if r := call(t, AskTool(AskOptions{Enabled: true}), env, `{"question":"q"}`); !r.Err || !strings.Contains(r.Output, "@?") {
		t.Fatalf("%+v", r)
	}
	if c, _ := ask.Settle(env, []byte(`{"question":"q"}`)); c.Class != Read {
		t.Fatal(c)
	}
	var out strings.Builder
	tty := TTYAsker(strings.NewReader("2\nmaybe\nfree text\n\n"), &out)
	q := Question{Text: "pick", Options: []string{"a", "b"}}
	if a, err := tty(context.Background(), q); err != nil || a != "b" {
		t.Fatalf("%q %v", a, err)
	}
	if _, err := tty(context.Background(), q); err == nil {
		t.Fatal("not an option")
	}
	if a, err := tty(context.Background(), Question{Text: "say", Options: []string{"a"}, Free: true}); err != nil || a != "free text" {
		t.Fatalf("%q %v", a, err)
	}
	if !strings.Contains(out.String(), "1) a") {
		t.Fatal(out.String())
	}
}

func TestCustomTool(t *testing.T) {
	env, _ := workspace(t)
	c, err := CustomTool(Custom{Name: "say", Description: "print args", Params: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`), Run: []string{"printf", "[%s]\n", "{{x}}", "pre-{{x}}-post"}, Class: "read"}, CustomOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := call(t, c, env, `{"x":"; rm -rf /"}`)
	if r.Err || r.Output != "[; rm -rf /]\n[pre-; rm -rf /-post]\n" {
		t.Fatalf("placeholder must stay one argv element: %+v", r)
	}
	if cl, _ := c.Settle(env, []byte(`{"x":"hello"}`)); cl.Class != Read {
		t.Fatalf("floor read for a printf: %+v", cl)
	}
	if cl, _ := c.Settle(env, []byte(`{"x":"; rm -rf /"}`)); cl.Class != Destructive {
		t.Fatalf("a destructive-looking value still tightens (errs toward asking): %+v", cl)
	}
	sh, err := CustomTool(Custom{Name: "shellish", Shell: `echo "n=$P_n s=$P_s"`, Class: "read", Timeout: 5}, CustomOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := call(t, sh, env, `{"n":42,"s":"a b"}`); r.Err || r.Output != "n=42 s=a b\n" {
		t.Fatalf("%+v", r)
	}
	if _, err := CustomTool(Custom{Name: "Bad Name", Run: []string{"true"}}, CustomOptions{}); err == nil {
		t.Fatal("name")
	}
	if _, err := CustomTool(Custom{Name: "both", Run: []string{"true"}, Shell: "true"}, CustomOptions{}); err == nil {
		t.Fatal("run xor shell")
	}
	if _, err := CustomTool(Custom{Name: "cls", Run: []string{"true"}, Class: "network"}, CustomOptions{}); err == nil {
		t.Fatal("class")
	}
	deflt, _ := CustomTool(Custom{Name: "w", Run: []string{"true"}}, CustomOptions{})
	if deflt.Class != Write {
		t.Fatal("default floor is write")
	}
	slow, _ := CustomTool(Custom{Name: "slow", Run: []string{"sleep", "5"}, Timeout: 1}, CustomOptions{})
	if r := call(t, slow, env, `{}`); !r.Err || !strings.Contains(r.Output, "killed after") {
		t.Fatalf("%+v", r)
	}
}

func TestMissingAndDoomLoop(t *testing.T) {
	bt, _ := NewBashTool(BashOptions{Enabled: true})
	reg := NewRegistry(ReadTool(), WriteTool(), EditTool(), GrepTool(), GlobTool(), bt)
	disabled := map[string]string{"webfetch": "tools.webfetch.enabled in .agent-one/config.yaml:12"}
	msg := Missing("webfetch", reg, disabled)
	if !strings.Contains(msg, "disabled by tools.webfetch.enabled in .agent-one/config.yaml:12") || !strings.Contains(msg, "@?") {
		t.Fatal(msg)
	}
	msg = Missing("cat", reg, disabled)
	if !strings.HasPrefix(msg, `no such tool "cat"`) || !strings.Contains(msg, "closest enabled: read") {
		t.Fatal(msg)
	}
	if got := Closest("shell", reg, 3); len(got) == 0 || got[0] != "bash" {
		t.Fatal(got)
	}
	if got := Closest("gerp", reg, 3); len(got) == 0 || got[0] != "grep" {
		t.Fatal(got)
	}
	if got := Closest("edit_file", reg, 1); len(got) != 1 || got[0] != "edit" {
		t.Fatal(got)
	}
	if got := Closest("zzzzzzzz", reg, 3); len(got) != 0 {
		t.Fatal(got)
	}
	var d DoomLoop
	if n, stop := d.Hit("x"); n != 1 || stop {
		t.Fatal(n, stop)
	}
	if n, stop := d.Hit("x"); n != 2 || !stop || !strings.Contains(d.StopMessage("x"), "2 times") {
		t.Fatal(n, stop)
	}
	d.Reset()
	if _, stop := d.Hit("x"); stop {
		t.Fatal("reset")
	}
}
