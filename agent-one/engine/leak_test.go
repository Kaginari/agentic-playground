package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The vocabulary this engine was forked from must not appear anywhere in the tree — sources,
// tests, testdata, embedded assets, templates, docs. The words are spelled in pieces so this
// file passes its own scan. Two exclusions: vendored third-party assets, and FORKED.md, whose
// rename table records the old words by design.
var leak = regexp.MustCompile(`(?i)` + strings.Join([]string{
	"ise" + "kai", "rim" + "uru", "veld" + "ora", "sli" + "me", "kij" + "in",
	"dark[ -_]?" + "e" + "lf", "high[ -_]?(" + "o" + "rc|" + "e" + "lf)", `\b` + "o" + `rcs?\b`, `\b` + "e" + `lf\b`, `\b` + "e" + `lves\b`,
	"great[ -_]?" + "sa" + "ge", "raph" + "ael", `\b` + "ci" + `el\b`, "temp" + "est", "reinc" + "arnat", "ju" + "ra",
	"crea" + "ture", "cre" + "st", "gen" + "ome", "col" + "ony", "court " + "body", "keeper " + "body", `\b` + "ani" + `ma\b`,
}, "|"))

func TestNoForkedVocabularyLeaks(t *testing.T) {
	var hits []string
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(p)
		if d.IsDir() {
			if rel == ".git" || rel == "board/assets" || rel == "bin" { // bin/: a built binary is bytes, not words
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "FORKED.md" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if m := leak.FindString(line); m != "" {
				hits = append(hits, rel+":"+itoa(i+1)+": "+m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Errorf("%d leak(s) of the forked vocabulary:\n%s", len(hits), strings.Join(hits, "\n"))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
