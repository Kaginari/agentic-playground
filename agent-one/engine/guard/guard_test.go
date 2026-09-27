package guard

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Corpus reads testdata/corpus.txt: kind ("block" | "allow") and command, \n escapes a newline.
func Corpus(t testing.TB, path string) [][2]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][2]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, cmd, _ := strings.Cut(line, " ")
		out = append(out, [2]string{kind, strings.ReplaceAll(cmd, `\n`, "\n")})
	}
	return out
}

func TestTheCorpus(t *testing.T) {
	g := Default()
	if len(g.Notes) > 0 {
		t.Fatalf("the built-in list does not compile: %v", g.Notes)
	}
	fails := 0
	for _, c := range Corpus(t, "corpus.txt") {
		r := g.Match(c[1])
		switch {
		case c[0] == "block" && r == nil:
			t.Errorf("not blocked: %q", c[1])
			fails++
		case c[0] == "allow" && r != nil:
			t.Errorf("blocked (%s): %q", r.Pattern, c[1])
			fails++
		}
	}
	if fails > 0 {
		t.Logf("%d of the corpus wrong", fails)
	}
}

func TestSharedFileAddsNeverRemoves(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, SharedPath)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("# mine\n(^|[[:space:]])terraform[[:space:]]+destroy\n(unclosed\n"), 0o644)
	g := ForHome(home)
	if g.Match("terraform destroy -auto-approve") == nil {
		t.Fatal("the machine-wide file adds its patterns")
	}
	if g.Match("rm -rf ~") == nil {
		t.Fatal("the built-in list stays")
	}
	if len(g.Notes) != 1 || !strings.Contains(g.Notes[0], "dangerous-patterns.txt:3") {
		t.Fatalf("a bad pattern is a note with its line: %v", g.Notes)
	}
}

func TestTheHomeByItsPath(t *testing.T) {
	g := ForHome("/home/you")
	for _, c := range []string{"rm -rf /home/you", "rm -rf /home/you/", `rm -rf "/home/you"`, "rm -r -f /home/you/*", "cd / && rm -rf /home/you"} {
		if g.Match(c) == nil {
			t.Errorf("not blocked: %q", c)
		}
	}
	for _, c := range []string{"rm -rf /home/you/project/tmp", "rm -rf /home/you2", "ls /home/you"} {
		if r := g.Match(c); r != nil {
			t.Errorf("blocked (%s): %q", r.Source, c)
		}
	}
}
