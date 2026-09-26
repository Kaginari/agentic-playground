package tui

import (
	"fmt"
	"strings"
)

// Diff is a line diff between two texts, as the write/edit/patch blocks show it.
type Diff struct {
	Lines []DiffLine
	Add   int
	Del   int
	// Truncated is set when the inputs were too large to diff line by line (only the counts
	// are honest then).
	Truncated bool
}

// DiffLine is one line of the diff: ' ' context, '+' added, '-' removed, '~' a hunk gap.
type DiffLine struct {
	Kind byte
	Old  int // old line number (0 for added)
	New  int // new line number (0 for removed)
	Text string
}

const diffMaxLines = 4000

// DiffText diffs two texts by line (an LCS table, quadratic — bounded by diffMaxLines per
// side; past that only the counts are reported) and keeps `context` lines around each change.
func DiffText(before, after string, context int) Diff {
	a := splitLines(before)
	b := splitLines(after)
	if len(a) > diffMaxLines || len(b) > diffMaxLines {
		return Diff{Add: len(b), Del: len(a), Truncated: true}
	}
	// trim the common prefix and suffix first: most edits touch a few lines of a long file
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	// LCS over the middle
	n, m := len(ma), len(mb)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if ma[i] == mb[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var all []DiffLine
	oi, ni := 1, 1
	for i := 0; i < pre; i++ {
		all = append(all, DiffLine{' ', oi, ni, a[i]})
		oi++
		ni++
	}
	i, j := 0, 0
	d := Diff{}
	for i < n || j < m {
		switch {
		case i < n && j < m && ma[i] == mb[j]:
			all = append(all, DiffLine{' ', oi, ni, ma[i]})
			i++
			j++
			oi++
			ni++
		case i < n && (j >= m || dp[i+1][j] >= dp[i][j+1]):
			all = append(all, DiffLine{'-', oi, 0, ma[i]})
			i++
			oi++
			d.Del++
		default:
			all = append(all, DiffLine{'+', 0, ni, mb[j]})
			j++
			ni++
			d.Add++
		}
	}
	for k := len(a) - suf; k < len(a); k++ {
		all = append(all, DiffLine{' ', oi, ni, a[k]})
		oi++
		ni++
	}
	// keep context lines around changes, a gap marker between hunks
	keep := make([]bool, len(all))
	for k, l := range all {
		if l.Kind != ' ' {
			for c := k - context; c <= k+context; c++ {
				if c >= 0 && c < len(all) {
					keep[c] = true
				}
			}
		}
	}
	gap := false
	for k, l := range all {
		if keep[k] {
			if gap && len(d.Lines) > 0 {
				d.Lines = append(d.Lines, DiffLine{Kind: '~'})
			}
			gap = false
			d.Lines = append(d.Lines, l)
		} else {
			gap = true
		}
	}
	return d
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// Stat is the "+3 −1" summary.
func (d Diff) Stat() string {
	return fmt.Sprintf("+%d −%d", d.Add, d.Del)
}
