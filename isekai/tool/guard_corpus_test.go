package tool

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// Without the guard, the classifier alone must still put every command of the guard's block
// corpus before the human: outward or destructive, never a silent write.
func TestGuardCorpusNeedsTheHuman(t *testing.T) {
	f, err := os.Open("../guard/corpus.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	e := Env{Root: t.TempDir()}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		kind, cmd, _ := strings.Cut(sc.Text(), " ")
		if kind != "block" {
			continue
		}
		cmd = strings.ReplaceAll(cmd, `\n`, "\n")
		if c := e.ClassifyCommand(cmd); c.Class < Outward {
			t.Errorf("%s (%s): %q", c.Class, c.Why, cmd)
		}
	}
}
