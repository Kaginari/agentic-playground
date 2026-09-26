package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelftest(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"selftest"}, IO{In: strings.NewReader(""), Out: &out, Err: &errb, Env: func(string) string { return "" }})
	if code != 0 || !strings.HasPrefix(out.String(), "@S PASS ") {
		t.Fatalf("code %d\n%s%s", code, out.String(), errb.String())
	}
}

func TestFindRootAndRegistry(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".isekai"), 0o755)
	deep := filepath.Join(root, "a", "b")
	_ = os.MkdirAll(deep, 0o755)
	if r, ok := FindRoot(deep); !ok || r != root {
		t.Fatalf("%q %v", r, ok)
	}
	if _, ok := FindRoot(t.TempDir()); ok {
		t.Fatal("no world")
	}
	Register(Command{Name: "extra", Summary: "later package", Run: func(args []string, io IO) int { return 7 }})
	var out bytes.Buffer
	if Main([]string{"extra"}, IO{Out: &out, Err: &out}) != 7 {
		t.Fatal("registered command runs")
	}
	out.Reset()
	Main([]string{"help"}, IO{Out: &out, Err: &out})
	if !strings.Contains(out.String(), "extra") || !strings.Contains(out.String(), "selftest") {
		t.Fatalf("help: %s", out.String())
	}
	out.Reset()
	code := Main([]string{"run", "-root", root, "hello"}, IO{In: strings.NewReader(""), Out: &out, Err: &out, Env: func(string) string { return "" }})
	if code != 2 || !strings.Contains(out.String(), "no provider") {
		t.Fatalf("no keys → FAIL naming the env: %d %s", code, out.String())
	}
}
