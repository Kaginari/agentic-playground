package loop

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The gate must see every write, whatever tool made it. A tool that knows its paths reports
// them (write, edit, patch); the shell, a custom tool or an MCP server does not. So a session
// stamps the world tree — size and mtime per file, bounded — at the start of a turn, again after
// every write-class step (the difference is that step's writes when it reported none), and once
// more at the turn's end (a write a read-classified command slipped in). What differs is a write.
//
// The world dir's own instruments and records are not the model's writes and are not stamped:
// instruments/, tmp/, memory/ (the tiers the binary maintains), toolbox/registry.json (derived),
// ontology/graph/unsaid.ttl (where a report's @U lines land) and log.md (the gate's own
// record). `.git` is never walked.

type stamp struct {
	size  int64
	mtime int64
}

// snapshot is the tree as last seen, keyed by root-relative slash path.
type snapshot map[string]stamp

// SnapshotCap bounds one walk; a tree past it is not watched, and the hole is named once.
const SnapshotCap = 200000

func (e *Engine) unwatched(rel string) bool {
	if rel == ".git" || strings.HasSuffix(rel, "/.git") {
		return true
	}
	wd := e.Lexicon.worldDir()
	for _, p := range []string{"instruments", "tmp", "memory", "toolbox/registry.json", "ontology/graph/unsaid.ttl", "log.md"} {
		if rel == wd+"/"+p {
			return true
		}
	}
	return false
}

// stampTree walks the root; ok is false (with why) past the cap or when the root is unreadable.
func (e *Engine) stampTree() (snap snapshot, why string) {
	snap = snapshot{}
	root := e.Root
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // an unreadable entry is not a write
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if e.unwatched(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if n++; n > SnapshotCap {
			return errTreeTooLarge
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		snap[rel] = stamp{size: info.Size(), mtime: info.ModTime().UnixNano()}
		return nil
	})
	switch {
	case err == errTreeTooLarge:
		return nil, fmt.Sprintf("the world tree has more than %d files — writes through the shell are not watched this session; only tools that report their paths reach the gate", SnapshotCap)
	case err != nil:
		return nil, "the world root cannot be walked (" + err.Error() + ") — writes through the shell are not watched"
	}
	return snap, ""
}

var errTreeTooLarge = fmt.Errorf("tree past the snapshot cap")

// diff lists what changed from a to b: new, changed or removed files, sorted.
func (a snapshot) diff(b snapshot) []string {
	var out []string
	for p, sb := range b {
		if sa, ok := a[p]; !ok || sa != sb {
			out = append(out, p)
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// watch takes the turn's baseline; a failed stamp names the hole once and stops watching.
func (s *Session) watch(r *Result) {
	if s.nowatch {
		return
	}
	snap, why := s.Engine.stampTree()
	if why != "" {
		s.nowatch = true
		s.base = nil
		r.hole(why)
		s.Journal.Log(Event{"t": "watch", "off": why})
		return
	}
	s.base = snap
	s.TestsBefore = s.Engine.readTests(snap)
}

// seen re-stamps the tree and returns what changed since the baseline, root-relative; the new
// stamp becomes the baseline. Nothing when the session is not watching.
func (s *Session) seen() []string {
	if s.nowatch || s.base == nil {
		return nil
	}
	snap, why := s.Engine.stampTree()
	if why != "" {
		s.nowatch = true
		s.base = nil
		return nil
	}
	changed := s.base.diff(snap)
	s.base = snap
	return changed
}

// testFile names a test file in the languages the gate reads: Go, JS/TS, Python.
var testFile = regexp.MustCompile(`(_test\.go|\.(test|spec)\.[cm]?[jt]sx?|(^|/)test_[^/]*\.py|_test\.py)$`)

// IsTestFile reports whether a root-relative path is a test file.
func IsTestFile(rel string) bool { return testFile.MatchString(rel) }

// readTests keeps the test files' text as the turn opens, so the gate can see what a turn did to
// the tests it is judged by (bounded: 256 KiB a file, 4 MiB in all).
func (e *Engine) readTests(snap snapshot) map[string]string {
	out := map[string]string{}
	total := 0
	for rel := range snap {
		if !testFile.MatchString(rel) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(rel)))
		if err != nil || len(b) > 256<<10 || total+len(b) > 4<<20 {
			continue
		}
		total += len(b)
		out[rel] = string(b)
	}
	return out
}
