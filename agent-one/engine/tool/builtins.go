package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	outputCap   = 24 << 10 // bytes of tool output handed to the model; the rest is a pointer
	bashTimeout = 600      // seconds, loop.js's default per-step time
	grepCap     = 200
	globCap     = 500
)

func fail(format string, a ...interface{}) Result {
	return Result{Output: fmt.Sprintf(format, a...), Err: true}
}

func decode(input json.RawMessage, v interface{}) error {
	if len(input) == 0 {
		return nil
	}
	return json.Unmarshal(input, v)
}

func clip(s string) string {
	if len(s) <= outputCap {
		return s
	}
	return s[:outputCap] + fmt.Sprintf("\n… [%d bytes clipped — read the file or narrow the ask]", len(s)-outputCap)
}

// ---- read

func ReadTool() *Tool {
	return &Tool{
		Name:        "read",
		Description: "Read a file. Lines are numbered. offset (1-based) and limit narrow the window.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"}},"required":["path"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs, Paths: []string{abs}}
			}
			return Classification{Class: Read, Why: "reads " + env.Rel(abs), Paths: []string{abs}}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Path   string
				Offset int
				Limit  int
			}
			if err := decode(in, &a); err != nil || a.Path == "" {
				return fail("read: path is required")
			}
			abs := env.Resolve(a.Path)
			data, err := os.ReadFile(abs)
			if err != nil {
				return fail("read: %v", err)
			}
			if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
				return fail("read: %s is binary (%d bytes)", env.Rel(abs), len(data))
			}
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			if len(data) == 0 {
				lines = nil
			}
			start := a.Offset
			if start < 1 {
				start = 1
			}
			if start > len(lines) {
				return Result{Output: fmt.Sprintf("(%s has %d lines; offset %d is past the end)", env.Rel(abs), len(lines), start)}
			}
			end := len(lines)
			if a.Limit > 0 && start-1+a.Limit < end {
				end = start - 1 + a.Limit
			}
			var b strings.Builder
			for i := start - 1; i < end; i++ {
				fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
			}
			if end < len(lines) {
				fmt.Fprintf(&b, "… %d more lines (offset %d)\n", len(lines)-end, end+1)
			}
			return Result{Output: clip(b.String())}
		},
	}
}

// ---- write

func WriteTool() *Tool {
	return &Tool{
		Name:        "write",
		Description: "Write a whole file (creating directories). Overwriting a record (AGENT-ONE.md, log.md, design, notes.jsonl) is destructive and gated.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			return env.ClassifyPath(env.Resolve(a.Path))
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct{ Path, Content string }
			if err := decode(in, &a); err != nil || a.Path == "" {
				return fail("write: path is required")
			}
			abs := env.Resolve(a.Path)
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return fail("write: %v", err)
			}
			if err := os.WriteFile(abs, []byte(a.Content), 0o644); err != nil {
				return fail("write: %v", err)
			}
			return Result{Output: fmt.Sprintf("wrote %s (%d bytes)", env.Rel(abs), len(a.Content)), Wrote: []string{abs}}
		},
	}
}

// ---- edit

func EditTool() *Tool {
	return &Tool{
		Name:        "edit",
		Description: "Replace one exact string in a file. old must match exactly once (or set replace_all).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old","new"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			c := env.ClassifyPath(env.Resolve(a.Path))
			if c.Class == Destructive {
				c.Why = "in-place edit of a record: " + env.Rel(c.Paths[0])
			}
			return c
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Path, Old, New string
				ReplaceAll     bool `json:"replace_all"`
			}
			if err := decode(in, &a); err != nil || a.Path == "" || a.Old == "" {
				return fail("edit: path and a non-empty old are required")
			}
			abs := env.Resolve(a.Path)
			data, err := os.ReadFile(abs)
			if err != nil {
				return fail("edit: %v", err)
			}
			n := strings.Count(string(data), a.Old)
			switch {
			case n == 0:
				return fail("edit: old string not found in %s", env.Rel(abs))
			case n > 1 && !a.ReplaceAll:
				return fail("edit: old string matches %d times in %s — make it unique or set replace_all", n, env.Rel(abs))
			}
			out := strings.Replace(string(data), a.Old, a.New, map[bool]int{true: -1, false: 1}[a.ReplaceAll])
			if err := os.WriteFile(abs, []byte(out), 0o644); err != nil {
				return fail("edit: %v", err)
			}
			return Result{Output: fmt.Sprintf("edited %s (%d replacement%s)", env.Rel(abs), n, map[bool]string{true: "s", false: ""}[n != 1]), Wrote: []string{abs}}
		},
	}
}

// ---- bash

func BashTool() *Tool {
	return &Tool{
		Name:        "bash",
		Description: "Run a shell command in the workspace root. Its class is read from the command (git push, curl are outward; rm, git reset are destructive); `class` may declare a tighter one.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"cwd":{"type":"string"},"timeout":{"type":"integer","description":"seconds"},"class":{"type":"string","enum":["read","write","outward","destructive"]}},"required":["command"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Command, Cwd string }
			_ = decode(in, &a)
			if a.Cwd != "" {
				env.Cwd = env.Resolve(a.Cwd)
			}
			c := env.ClassifyCommand(a.Command)
			if a.Cwd != "" && !env.Inside(env.Cwd) && c.Class < Outward {
				c = Classification{Class: Outward, Why: "cwd outside the workspace: " + a.Cwd}
			}
			return c
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Command, Cwd string
				Timeout      int
			}
			if err := decode(in, &a); err != nil || strings.TrimSpace(a.Command) == "" {
				return fail("bash: command is required")
			}
			dir := env.Dir()
			if a.Cwd != "" {
				dir = env.Resolve(a.Cwd)
			}
			t := a.Timeout
			if t <= 0 {
				t = env.Timeout
			}
			if t <= 0 {
				t = bashTimeout
			}
			cctx, cancel := context.WithTimeout(ctx, time.Duration(t)*time.Second)
			defer cancel()
			cmd := exec.CommandContext(cctx, "bash", "-c", a.Command)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "AGENT_ONE_ROOT="+env.Root)
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			t0 := time.Now()
			err := cmd.Run()
			code := 0
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else {
					code = 1
				}
			}
			var b strings.Builder
			b.WriteString(out.String())
			if errb.Len() > 0 {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteString("\n")
				}
				b.WriteString("[stderr]\n" + errb.String())
			}
			if cctx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(&b, "\n[killed after %ds]", t)
				code = 124
			}
			if code != 0 {
				fmt.Fprintf(&b, "\n[exit %d, %s]", code, time.Since(t0).Round(time.Millisecond))
			}
			return Result{Output: clip(b.String()), Err: code != 0}
		},
	}
}

// ---- glob

func GlobTool() *Tool {
	return &Tool{
		Name:        "glob",
		Description: "Find files by pattern (** allowed), relative to the workspace root or `path`. Sorted, newest first.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs}
			}
			return Classification{Class: Read, Why: "lists " + env.Rel(abs)}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct{ Pattern, Path string }
			if err := decode(in, &a); err != nil || a.Pattern == "" {
				return fail("glob: pattern is required")
			}
			base := env.Resolve(a.Path)
			hits, err := Glob(base, a.Pattern)
			if err != nil {
				return fail("glob: %v", err)
			}
			if len(hits) == 0 {
				return Result{Output: "no files match " + a.Pattern}
			}
			var b strings.Builder
			for i, h := range hits {
				if i == globCap {
					fmt.Fprintf(&b, "… %d more\n", len(hits)-globCap)
					break
				}
				b.WriteString(env.Rel(h) + "\n")
			}
			return Result{Output: b.String()}
		},
	}
}

var skipDirs = map[string]bool{".git": true, "node_modules": true}

// Glob walks base and returns the absolute paths matching pattern; `**` spans directories.
// Results are newest-modified first.
func Glob(base, pattern string) ([]string, error) {
	pattern = filepath.ToSlash(filepath.Clean(pattern))
	parts := strings.Split(pattern, "/")
	type hit struct {
		p string
		t time.Time
	}
	var hits []hit
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if p != base && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if matchParts(parts, strings.Split(filepath.ToSlash(rel), "/")) {
			hits = append(hits, hit{p, info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].t.After(hits[j].t) })
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.p
	}
	return out, nil
}

func matchParts(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchParts(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if ok, _ := filepath.Match(pat[0], segs[0]); !ok {
		return false
	}
	return matchParts(pat[1:], segs[1:])
}

// ---- grep

func GrepTool() *Tool {
	return &Tool{
		Name:        "grep",
		Description: "Search file contents with a Go regexp under `path` (default: the workspace root). `include` filters by file name glob. Output: path:line:text.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"include":{"type":"string"},"ignore_case":{"type":"boolean"}},"required":["pattern"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs}
			}
			return Classification{Class: Read, Why: "searches " + env.Rel(abs)}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Pattern, Path, Include string
				IgnoreCase             bool `json:"ignore_case"`
			}
			if err := decode(in, &a); err != nil || a.Pattern == "" {
				return fail("grep: pattern is required")
			}
			pat := a.Pattern
			if a.IgnoreCase {
				pat = "(?i)" + pat
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return fail("grep: %v", err)
			}
			base := env.Resolve(a.Path)
			var b strings.Builder
			n, files := 0, 0
			_ = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
				if err != nil || n >= grepCap {
					return nil
				}
				if info.IsDir() {
					if p != base && skipDirs[info.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				if a.Include != "" {
					if ok, _ := filepath.Match(a.Include, info.Name()); !ok {
						return nil
					}
				}
				if info.Size() > 8<<20 {
					return nil
				}
				data, err := os.ReadFile(p)
				if err != nil || bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
					return nil
				}
				files++
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						n++
						if len(line) > 300 {
							line = line[:300] + "…"
						}
						fmt.Fprintf(&b, "%s:%d:%s\n", env.Rel(p), i+1, line)
						if n >= grepCap {
							fmt.Fprintf(&b, "… stopped at %d matches\n", grepCap)
							return filepath.SkipDir
						}
					}
				}
				return nil
			})
			if n == 0 {
				return Result{Output: fmt.Sprintf("no matches for %q in %d files", a.Pattern, files)}
			}
			return Result{Output: clip(b.String())}
		},
	}
}
