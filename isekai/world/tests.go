package world

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Tests intact: a turn may not pass by making the tests easier. A test file the turn touched is
// compared with its text as the turn opened: fewer tests, more skips or `.only`s, or the file gone
// is a failed gate — the reasons go back to the model (law.gate.retries) and then to the human,
// who alone may decide a test should go.

var (
	testDecl = map[string]*regexp.Regexp{
		"go": regexp.MustCompile(`(?m)^func (Test|Benchmark|Fuzz|Example)\w*\(`),
		"js": regexp.MustCompile(`(?m)(^|[^.\w])(it|test)(\.each\([^)]*\))?\s*\(`),
		"py": regexp.MustCompile(`(?m)^\s*(async\s+)?def test_\w*\s*\(`),
	}
	testSkip = map[string]*regexp.Regexp{
		"go": regexp.MustCompile(`\bt\.(Skip|SkipNow|Skipf)\(`),
		"js": regexp.MustCompile(`\b(it|test|describe)\.(skip|only|todo)\s*\(|(^|[^.\w])x(it|describe|test)\s*\(|(^|[^.\w])f(it|describe)\s*\(`),
		"py": regexp.MustCompile(`@pytest\.mark\.(skip|skipif|xfail)|@unittest\.skip|\bpytest\.skip\(|\bself\.skipTest\(`),
	}
)

func testLang(rel string) string {
	switch {
	case strings.HasSuffix(rel, "_test.go"):
		return "go"
	case strings.HasSuffix(rel, ".py"):
		return "py"
	}
	return "js"
}

// testsIntact compares each touched test file (and each one that vanished) with the turn's start.
func (w *World) testsIntact(before map[string]string, wrote []string) []string {
	var why []string
	touched := map[string]bool{}
	for _, rel := range wrote {
		touched[filepath.ToSlash(rel)] = true
	}
	var paths []string
	for rel := range before {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		old := before[rel]
		b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
		if os.IsNotExist(err) {
			if n := len(testDecl[testLang(rel)].FindAllString(old, -1)); n > 0 {
				why = append(why, fmt.Sprintf("%s was deleted (%d test%s)", rel, n, plural(n)))
			}
			continue
		}
		if err != nil || !touched[rel] {
			continue
		}
		cur := string(b)
		lang := testLang(rel)
		if a, z := len(testDecl[lang].FindAllString(old, -1)), len(testDecl[lang].FindAllString(cur, -1)); z < a {
			why = append(why, fmt.Sprintf("%s lost %d test%s (%d → %d)", rel, a-z, plural(a-z), a, z))
		}
		if a, z := len(testSkip[lang].FindAllString(old, -1)), len(testSkip[lang].FindAllString(cur, -1)); z > a {
			why = append(why, fmt.Sprintf("%s gained %d skip/only marker%s (%d → %d)", rel, z-a, plural(z-a), a, z))
		}
	}
	return why
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
