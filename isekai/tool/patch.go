package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ---- multiedit

// MultiEditOptions is tools.multiedit.
type MultiEditOptions struct{ Enabled bool }

// MultiEditTool applies several exact replacements to one file atomically: every edit must
// match (exactly once unless replace_all) against the text as the earlier edits left it, or
// nothing is written.
func MultiEditTool(opt MultiEditOptions) *Tool {
	return &Tool{
		Name:        "multiedit",
		Description: "Apply several exact replacements to one file atomically, in order. Each old must match exactly once (or set replace_all); if any fails, nothing is written.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"old":{"type":"string"},"new":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["old","new"]}}},"required":["path","edits"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			return env.ClassifyPath(env.Resolve(a.Path))
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Path  string
				Edits []struct {
					Old, New   string
					ReplaceAll bool `json:"replace_all"`
				}
			}
			if err := decode(in, &a); err != nil || a.Path == "" || len(a.Edits) == 0 {
				return fail("multiedit: path and a non-empty edits list are required")
			}
			if !opt.Enabled {
				return fail("multiedit is off (tools.multiedit.enabled: false)")
			}
			abs := env.Resolve(a.Path)
			data, err := os.ReadFile(abs)
			if err != nil {
				return fail("multiedit: %v", err)
			}
			text := string(data)
			total := 0
			for i, e := range a.Edits {
				if e.Old == "" {
					return fail("multiedit: edit %d has an empty old — nothing written", i+1)
				}
				n := strings.Count(text, e.Old)
				switch {
				case n == 0:
					return fail("multiedit: edit %d: old string not found in %s — nothing written", i+1, env.Rel(abs))
				case n > 1 && !e.ReplaceAll:
					return fail("multiedit: edit %d: old string matches %d times — make it unique or set replace_all; nothing written", i+1, n)
				}
				count := 1
				if e.ReplaceAll {
					count = -1
				}
				text = strings.Replace(text, e.Old, e.New, count)
				total += n
			}
			if err := writeAtomic(abs, []byte(text)); err != nil {
				return fail("multiedit: %v", err)
			}
			return Result{Output: fmt.Sprintf("edited %s (%d edits, %d replacements)", env.Rel(abs), len(a.Edits), total), Wrote: []string{abs}}
		},
	}
}

// ---- patch

// PatchOptions is tools.patch.
type PatchOptions struct{ Enabled bool }

// Hunk is one @@ block of a unified diff.
type Hunk struct {
	OldStart, OldLines, NewStart, NewLines int
	Lines                                  []string // with their leading ' ', '-', '+'
}

// FilePatch is one file's part of a unified diff.
type FilePatch struct {
	Old, New string // paths after prefix stripping; "/dev/null" for create/delete
	Hunks    []Hunk
}

// Path is the path the patch acts on.
func (f FilePatch) Path() string {
	if f.New != "/dev/null" {
		return f.New
	}
	return f.Old
}

// ParseUnified reads a unified diff into per-file patches. `a/` and `b/` prefixes are stripped
// when both sides carry them; `diff --git` and `index` lines are ignored.
func ParseUnified(text string) ([]FilePatch, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []FilePatch
	var cur *FilePatch
	i := 0
	for i < len(lines) {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "--- "):
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
				return nil, fmt.Errorf("line %d: --- without +++", i+1)
			}
			out = append(out, FilePatch{Old: headerPath(l[4:]), New: headerPath(lines[i+1][4:])})
			cur = &out[len(out)-1]
			i += 2
		case strings.HasPrefix(l, "@@ "):
			if cur == nil {
				return nil, fmt.Errorf("line %d: hunk before any file header", i+1)
			}
			h, err := parseHunkHeader(l)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", i+1, err)
			}
			i++
			old, nw := 0, 0
			for i < len(lines) && (old < h.OldLines || nw < h.NewLines) {
				hl := lines[i]
				if hl == "" && i == len(lines)-1 {
					break
				}
				if hl == "" {
					hl = " " // a blank context line with its space stripped by an editor
				}
				switch hl[0] {
				case ' ':
					old++
					nw++
				case '-':
					old++
				case '+':
					nw++
				case '\\':
					i++
					continue
				default:
					return nil, fmt.Errorf("line %d: unexpected %q inside a hunk", i+1, hl)
				}
				h.Lines = append(h.Lines, hl)
				i++
			}
			// `\ No newline at end of file` right after the hunk.
			for i < len(lines) && strings.HasPrefix(lines[i], "\\") {
				h.Lines = append(h.Lines, lines[i])
				i++
			}
			cur.Hunks = append(cur.Hunks, h)
		default:
			i++
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no file headers (--- / +++) found")
	}
	for k := range out {
		if strings.HasPrefix(out[k].Old, "a/") && (strings.HasPrefix(out[k].New, "b/") || out[k].New == "/dev/null") {
			out[k].Old = out[k].Old[2:]
		}
		if strings.HasPrefix(out[k].New, "b/") && (out[k].Old == "/dev/null" || !strings.HasPrefix(out[k].Old, "a/")) {
			out[k].New = out[k].New[2:]
		} else if strings.HasPrefix(out[k].New, "b/") {
			out[k].New = out[k].New[2:]
		}
	}
	return out, nil
}

func headerPath(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func parseHunkHeader(l string) (Hunk, error) {
	// @@ -a[,b] +c[,d] @@
	f := strings.Fields(l)
	if len(f) < 3 || !strings.HasPrefix(f[1], "-") || !strings.HasPrefix(f[2], "+") {
		return Hunk{}, fmt.Errorf("bad hunk header %q", l)
	}
	rng := func(s string) (int, int, error) {
		s = s[1:]
		start, n := 0, 1
		var err error
		if i := strings.IndexByte(s, ','); i >= 0 {
			if n, err = strconv.Atoi(s[i+1:]); err != nil {
				return 0, 0, err
			}
			s = s[:i]
		}
		if start, err = strconv.Atoi(s); err != nil {
			return 0, 0, err
		}
		return start, n, nil
	}
	os_, ol, err := rng(f[1])
	if err != nil {
		return Hunk{}, err
	}
	ns, nl, err := rng(f[2])
	if err != nil {
		return Hunk{}, err
	}
	return Hunk{OldStart: os_, OldLines: ol, NewStart: ns, NewLines: nl}, nil
}

// Apply applies the file patch to text and returns the new text. A hunk is matched at its
// stated line first, then at growing offsets either side; the first hunk that matches nowhere
// fails the whole patch.
func (f FilePatch) Apply(text string) (string, error) {
	hadNL := strings.HasSuffix(text, "\n")
	lines := splitLines(text)
	offset := 0
	noNewline := false
	for hi, h := range f.Hunks {
		var old, nw []string
		for _, l := range h.Lines {
			switch l[0] {
			case ' ':
				old = append(old, l[1:])
				nw = append(nw, l[1:])
			case '-':
				old = append(old, l[1:])
			case '+':
				nw = append(nw, l[1:])
			case '\\':
				noNewline = true
			}
		}
		want := h.OldStart - 1 + offset
		if h.OldLines == 0 {
			want = h.OldStart + offset // insertion after line OldStart
		}
		at := findHunk(lines, old, want)
		if at < 0 {
			return "", fmt.Errorf("hunk %d (@@ -%d,%d +%d,%d @@) does not apply", hi+1, h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		}
		rebuilt := append(append(append([]string{}, lines[:at]...), nw...), lines[at+len(old):]...)
		offset += (at - want) + len(nw) - len(old)
		lines = rebuilt
	}
	out := strings.Join(lines, "\n")
	if len(lines) > 0 && (hadNL || len(f.Hunks) > 0) && !noNewline {
		out += "\n"
	}
	return out, nil
}

func findHunk(lines, old []string, want int) int {
	matches := func(at int) bool {
		if at < 0 || at+len(old) > len(lines) {
			return false
		}
		for i, l := range old {
			if lines[at+i] != l {
				return false
			}
		}
		return true
	}
	if len(old) == 0 {
		if want < 0 {
			return 0
		}
		if want > len(lines) {
			return len(lines)
		}
		return want
	}
	for d := 0; d <= len(lines); d++ {
		if matches(want - d) {
			return want - d
		}
		if d > 0 && matches(want+d) {
			return want + d
		}
	}
	return -1
}

// PatchTool applies a unified diff to files under the world root, atomically: every file's
// result is computed first; nothing is written unless all apply.
func PatchTool(opt PatchOptions) *Tool {
	paths := func(env Env, in json.RawMessage) ([]string, []FilePatch, error) {
		var a struct{ Patch string }
		if err := decode(in, &a); err != nil {
			return nil, nil, err
		}
		fps, err := ParseUnified(a.Patch)
		if err != nil {
			return nil, nil, err
		}
		var ps []string
		for _, fp := range fps {
			ps = append(ps, env.Resolve(fp.Path()))
		}
		return ps, fps, nil
	}
	return &Tool{
		Name:        "patch",
		Description: "Apply a unified diff (as produced by `diff -u` / `git diff`) to files in the world. All hunks of all files must apply, or nothing is written. Creates (--- /dev/null) and deletes (+++ /dev/null) are supported.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"patch":{"type":"string"}},"required":["patch"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			ps, fps, err := paths(env, in)
			if err != nil {
				return Classification{Class: Write, Why: "patch (unparsed: " + err.Error() + ")"}
			}
			best := Classification{Class: Write, Why: "patches " + strings.Join(rels(env, ps), ", "), Paths: ps}
			for i, p := range ps {
				c := env.ClassifyPath(p)
				if fps[i].New == "/dev/null" && c.Class < Destructive {
					c = Classification{Class: Destructive, Why: "deletes " + env.Rel(p)}
				}
				if c.Class > best.Class {
					best.Class, best.Why = c.Class, c.Why
				}
			}
			return best
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			if !opt.Enabled {
				return fail("patch is off (tools.patch.enabled: false)")
			}
			ps, fps, err := paths(env, in)
			if err != nil {
				return fail("patch: %v", err)
			}
			type pending struct {
				path   string
				text   string
				delete bool
			}
			var todo []pending
			for i, fp := range fps {
				abs := ps[i]
				if fp.New == "/dev/null" {
					if _, err := os.Stat(abs); err != nil {
						return fail("patch: delete %s: %v — nothing written", env.Rel(abs), err)
					}
					todo = append(todo, pending{path: abs, delete: true})
					continue
				}
				text := ""
				if fp.Old != "/dev/null" {
					data, err := os.ReadFile(abs)
					if err != nil {
						return fail("patch: %v — nothing written", err)
					}
					text = string(data)
				} else if _, err := os.Stat(abs); err == nil {
					return fail("patch: create %s: already exists — nothing written", env.Rel(abs))
				}
				out, err := fp.Apply(text)
				if err != nil {
					return fail("patch: %s: %v — nothing written", env.Rel(abs), err)
				}
				todo = append(todo, pending{path: abs, text: out})
			}
			var wrote []string
			for _, p := range todo {
				if p.delete {
					if err := os.Remove(p.path); err != nil {
						return fail("patch: %v (earlier files in this patch were written)", err)
					}
				} else {
					if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
						return fail("patch: %v", err)
					}
					if err := writeAtomic(p.path, []byte(p.text)); err != nil {
						return fail("patch: %v (earlier files in this patch were written)", err)
					}
				}
				wrote = append(wrote, p.path)
			}
			return Result{Output: fmt.Sprintf("patched %s", strings.Join(rels(env, wrote), ", ")), Wrote: wrote}
		},
	}
}

func rels(env Env, ps []string) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = env.Rel(p)
	}
	return out
}
