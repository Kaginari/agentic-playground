package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EditorOptions is tools.editor: Claude's str_replace_based_edit_tool, paths confined to the
// workspace root.
type EditorOptions struct {
	Enabled bool
	Name    string // default "str_replace_based_edit_tool"
}

const editorName = "str_replace_based_edit_tool"

// Confine resolves p against the env and refuses anything that leaves the workspace root: `..`
// climbs, absolute paths elsewhere, and symlinks (of any existing prefix) pointing out.
func (e Env) Confine(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("path is required")
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(e.Root))
	if err != nil {
		root = filepath.Clean(e.Root)
	}
	abs := e.Resolve(p)
	within := func(q string) bool {
		return q == root || strings.HasPrefix(q, root+string(filepath.Separator)) ||
			q == filepath.Clean(e.Root) || strings.HasPrefix(q, filepath.Clean(e.Root)+string(filepath.Separator))
	}
	if !within(abs) {
		return "", fmt.Errorf("%s is outside the workspace root", p)
	}
	// Walk the longest existing prefix through symlinks.
	probe := abs
	for {
		if real, err := filepath.EvalSymlinks(probe); err == nil {
			if !within(real) {
				return "", fmt.Errorf("%s resolves through a symlink to %s, outside the workspace root", p, real)
			}
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	return abs, nil
}

// EditorTool is str_replace_based_edit_tool: view / create / str_replace / insert. Under an
// anthropic provider it is declared as text_editor_20250728 (schema-less); elsewhere with the
// custom schema below. Errors come back as is_error results.
func EditorTool(opt EditorOptions) *Tool {
	name := opt.Name
	if name == "" {
		name = editorName
	}
	return &Tool{
		Name:        name,
		Description: "View, create and edit files inside the workspace root. command: view [path, view_range] · create [path, file_text] · str_replace [path, old_str, new_str — exactly one match] · insert [path, insert_line, insert_text].",
		Schema:      json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","enum":["view","create","str_replace","insert"]},"path":{"type":"string"},"file_text":{"type":"string"},"old_str":{"type":"string"},"new_str":{"type":"string"},"insert_line":{"type":"integer"},"insert_text":{"type":"string"},"view_range":{"type":"array","items":{"type":"integer"}}},"required":["command","path"]}`),
		Class:       Read,
		Declare:     map[string]json.RawMessage{"anthropic": AnthropicEditor},
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Command, Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if a.Command == "view" || a.Command == "" {
				if !env.Inside(abs) {
					return Classification{Class: Outward, Why: "path outside the workspace: " + abs, Paths: []string{abs}}
				}
				return Classification{Class: Read, Why: "views " + env.Rel(abs), Paths: []string{abs}}
			}
			c := env.ClassifyPath(abs)
			if c.Class == Destructive {
				c.Why = "in-place edit of a record: " + env.Rel(abs)
			}
			return c
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Command, Path, FileText, OldStr, NewStr, InsertText string
				InsertLine                                          *int  `json:"insert_line"`
				ViewRange                                           []int `json:"view_range"`
			}
			// Anthropic's field names are snake_case; decode both spellings.
			var snake struct {
				FileText   string `json:"file_text"`
				OldStr     string `json:"old_str"`
				NewStr     string `json:"new_str"`
				InsertText string `json:"insert_text"`
			}
			if err := decode(in, &a); err != nil {
				return fail("%s: %v", name, err)
			}
			_ = decode(in, &snake)
			if a.FileText == "" {
				a.FileText = snake.FileText
			}
			if a.OldStr == "" {
				a.OldStr = snake.OldStr
			}
			if a.NewStr == "" {
				a.NewStr = snake.NewStr
			}
			if a.InsertText == "" {
				a.InsertText = snake.InsertText
			}
			if !opt.Enabled {
				return fail("%s is off (tools.editor.enabled: false)", name)
			}
			abs, err := env.Confine(a.Path)
			if err != nil {
				return fail("%s: %v", name, err)
			}
			switch a.Command {
			case "view":
				return editorView(env, abs, a.ViewRange)
			case "create":
				if _, err := os.Stat(abs); err == nil {
					return fail("%s: %s exists — view it and use str_replace, or choose another path", name, env.Rel(abs))
				}
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					return fail("%s: %v", name, err)
				}
				if err := os.WriteFile(abs, []byte(a.FileText), 0o644); err != nil {
					return fail("%s: %v", name, err)
				}
				return Result{Output: fmt.Sprintf("created %s (%d bytes)", env.Rel(abs), len(a.FileText)), Wrote: []string{abs}}
			case "str_replace":
				if a.OldStr == "" {
					return fail("%s: old_str is required", name)
				}
				data, err := os.ReadFile(abs)
				if err != nil {
					return fail("%s: %v", name, err)
				}
				n := strings.Count(string(data), a.OldStr)
				if n == 0 {
					return fail("%s: old_str not found in %s", name, env.Rel(abs))
				}
				if n > 1 {
					return fail("%s: old_str matches %d times in %s — include more context so it matches exactly once", name, n, env.Rel(abs))
				}
				out := strings.Replace(string(data), a.OldStr, a.NewStr, 1)
				if err := writeAtomic(abs, []byte(out)); err != nil {
					return fail("%s: %v", name, err)
				}
				line := 1 + strings.Count(string(data)[:strings.Index(string(data), a.OldStr)], "\n")
				return Result{Output: fmt.Sprintf("edited %s at line %d\n%s", env.Rel(abs), line, snippet(out, line, 3)), Wrote: []string{abs}}
			case "insert":
				if a.InsertLine == nil {
					return fail("%s: insert_line is required", name)
				}
				data, err := os.ReadFile(abs)
				if err != nil {
					return fail("%s: %v", name, err)
				}
				lines := splitLines(string(data))
				at := *a.InsertLine
				if at < 0 || at > len(lines) {
					return fail("%s: insert_line %d out of range 0..%d", name, at, len(lines))
				}
				ins := splitLines(a.InsertText)
				out := append(append(append([]string{}, lines[:at]...), ins...), lines[at:]...)
				text := strings.Join(out, "\n")
				if len(out) > 0 {
					text += "\n"
				}
				if err := writeAtomic(abs, []byte(text)); err != nil {
					return fail("%s: %v", name, err)
				}
				return Result{Output: fmt.Sprintf("inserted %d line%s after line %d in %s", len(ins), plural(len(ins)), at, env.Rel(abs)), Wrote: []string{abs}}
			default:
				return fail("%s: unknown command %q (view, create, str_replace, insert)", name, a.Command)
			}
		},
	}
}

func editorView(env Env, abs string, rng []int) Result {
	fi, err := os.Stat(abs)
	if err != nil {
		return fail("%s: %v", editorName, err)
	}
	if fi.IsDir() {
		var b strings.Builder
		fmt.Fprintf(&b, "%s/ (directory)\n", env.Rel(abs))
		n := 0
		_ = filepath.Walk(abs, func(p string, info os.FileInfo, err error) error {
			if err != nil || p == abs {
				return nil
			}
			rel, _ := filepath.Rel(abs, p)
			if strings.Count(rel, string(filepath.Separator)) > 1 {
				return filepath.SkipDir
			}
			if info.IsDir() && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			n++
			if n > globCap {
				return filepath.SkipDir
			}
			if info.IsDir() {
				rel += "/"
			}
			b.WriteString(rel + "\n")
			return nil
		})
		return Result{Output: clip(b.String())}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fail("%s: %v", editorName, err)
	}
	lines := splitLines(string(data))
	start, end := 1, len(lines)
	if len(rng) >= 1 {
		start = rng[0]
	}
	if len(rng) >= 2 && rng[1] != -1 {
		end = rng[1]
	}
	if start < 1 || start > len(lines)+1 || end < start-1 {
		return fail("%s: view_range [%d, %d] out of bounds for %d lines", editorName, start, end, len(lines))
	}
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for i := start - 1; i < end; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
	}
	if b.Len() == 0 {
		b.WriteString("(empty)\n")
	}
	return Result{Output: clip(b.String())}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func snippet(text string, line, around int) string {
	lines := splitLines(text)
	lo, hi := line-1-around, line-1+around
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	var b strings.Builder
	for i := lo; i <= hi; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
	}
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// writeAtomic writes data to a sibling temp file and renames it over path.
func writeAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	_ = os.Chmod(name, mode)
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// ---- ls

// LsOptions is tools.ls.
type LsOptions struct {
	Enabled bool
	Cap     int // entries (default globCap)
}

// LsTool lists a directory: entries with `/` on directories, sizes, `depth` levels deep.
func LsTool(opt LsOptions) *Tool {
	return &Tool{
		Name:        "ls",
		Description: "List a directory (default: the workspace root). depth (default 1) descends; all shows dotfiles.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"depth":{"type":"integer"},"all":{"type":"boolean"}}}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs, Paths: []string{abs}}
			}
			return Classification{Class: Read, Why: "lists " + env.Rel(abs), Paths: []string{abs}}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Path  string
				Depth int
				All   bool
			}
			if err := decode(in, &a); err != nil {
				return fail("ls: %v", err)
			}
			if !opt.Enabled {
				return fail("ls is off (tools.ls.enabled: false)")
			}
			base := env.Resolve(a.Path)
			fi, err := os.Stat(base)
			if err != nil {
				return fail("ls: %v", err)
			}
			if !fi.IsDir() {
				return Result{Output: fmt.Sprintf("%s (%d bytes)\n", env.Rel(base), fi.Size())}
			}
			depth := a.Depth
			if depth <= 0 {
				depth = 1
			}
			cap := opt.Cap
			if cap <= 0 {
				cap = globCap
			}
			var out []string
			var walk func(dir string, level int)
			walk = func(dir string, level int) {
				entries, err := os.ReadDir(dir)
				if err != nil {
					return
				}
				sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
				for _, e := range entries {
					if len(out) >= cap {
						return
					}
					if !a.All && strings.HasPrefix(e.Name(), ".") {
						continue
					}
					p := filepath.Join(dir, e.Name())
					rel, _ := filepath.Rel(base, p)
					if e.IsDir() {
						out = append(out, rel+"/")
						if level < depth && !skipDirs[e.Name()] {
							walk(p, level+1)
						}
						continue
					}
					size := ""
					if info, err := e.Info(); err == nil {
						size = fmt.Sprintf("  %d", info.Size())
					}
					out = append(out, rel+size)
				}
			}
			walk(base, 1)
			if len(out) == 0 {
				return Result{Output: env.Rel(base) + "/ is empty\n"}
			}
			s := strings.Join(out, "\n") + "\n"
			if len(out) >= cap {
				s += fmt.Sprintf("… stopped at %d entries\n", cap)
			}
			return Result{Output: clip(s)}
		},
	}
}
