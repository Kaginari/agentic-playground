package tool

import (
	"fmt"
	"sort"
	"strings"
)

// The model never meets a silent failure (binary.md §When a tool is missing): a call to an
// unknown or disabled tool gets one error naming why and the closest enabled alternatives, and
// the same missing call twice in a turn stops the turn.

// aliases map the names other harnesses and models reach for to this shelf's tools.
var aliases = map[string][]string{
	"read":      {"cat", "view", "read_file", "open", "file_read", "readfile"},
	"write":     {"write_file", "create", "create_file", "save", "writefile"},
	"edit":      {"edit_file", "replace", "str_replace", "str_replace_editor", "str_replace_based_edit_tool", "substitute"},
	"multiedit": {"multi_edit", "edits", "batch_edit"},
	"patch":     {"apply_patch", "diff", "apply_diff"},
	"bash":      {"sh", "shell", "exec", "run", "command", "terminal", "execute", "run_command", "computer"},
	"ls":        {"dir", "list", "list_dir", "list_files", "tree"},
	"glob":      {"find", "find_files", "files", "search_files"},
	"grep":      {"search", "rg", "ripgrep", "search_content", "ag"},
	"git":       {"vcs", "commit", "git_status"},
	"webfetch":  {"fetch", "curl", "http", "web_fetch", "browse", "get_url", "wget"},
	"websearch": {"search_web", "web_search", "google", "bing", "duckduckgo"},
	"ask":       {"question", "ask_user", "askuserquestion", "ask_human", "prompt_user", "input"},
	"dispatch":  {"agent", "task", "subagent", "spawn", "delegate"},
	"recall":    {"memory_search", "remember_search", "memory"},
	"remember":  {"memory_write", "note", "memorize"},
	"toolbox":   {"skills", "list_tools", "tools"},
	"skill":     {"load", "load_skill", "use_skill"},
}

// Missing explains a call to a name the registry does not hold: why (no such tool, or disabled
// with its origin), and the closest enabled alternatives. disabled maps a tool name to the
// origin of its switch (e.g. "tools.webfetch.enabled in .agent-one/config.yaml:12").
func Missing(name string, reg *Registry, disabled map[string]string) string {
	var b strings.Builder
	if origin, off := disabled[name]; off {
		fmt.Fprintf(&b, "tool %q is disabled by %s", name, origin)
	} else {
		fmt.Fprintf(&b, "no such tool %q", name)
	}
	alts := Closest(name, reg, 3)
	if len(alts) > 0 {
		fmt.Fprintf(&b, " — closest enabled: %s", strings.Join(alts, ", "))
	} else {
		b.WriteString(" — no enabled tool is close; search the toolbox for what exists")
	}
	if _, off := disabled[name]; off {
		b.WriteString(". Compose it from what is on if you can (the same class and gate apply); else name the need with @?")
	}
	return b.String()
}

// Closest ranks the registry's tools by nearness to name: an alias hit, a prefix or substring,
// then a small edit distance. At most n names.
func Closest(name string, reg *Registry, n int) []string {
	if reg == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(name))
	q = strings.TrimPrefix(q, "mcp__")
	type cand struct {
		name  string
		score int
	}
	var cs []cand
	for _, t := range reg.Names() {
		lt := strings.ToLower(t)
		score := 0
		switch {
		case lt == q:
			score = 100
		case aliasHit(t, q):
			score = 90
		case strings.HasPrefix(lt, q) || strings.HasPrefix(q, lt):
			score = 70
		case strings.Contains(lt, q) || strings.Contains(q, lt):
			score = 60
		default:
			if d := editDistance(lt, q); d <= 2 || (len(q) > 5 && d <= 3) {
				score = 50 - d
			}
		}
		if score > 0 {
			cs = append(cs, cand{t, score})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].score > cs[j].score })
	var out []string
	for i, c := range cs {
		if i == n {
			break
		}
		out = append(out, c.name)
	}
	return out
}

func aliasHit(tool, q string) bool {
	for _, a := range aliases[tool] {
		if a == q || strings.ReplaceAll(a, "_", "") == strings.ReplaceAll(q, "_", "") {
			return true
		}
	}
	return false
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// DoomLoop counts repeated missing calls within a turn; past Limit the turn stops.
type DoomLoop struct {
	Limit int // default 2: the same missing call twice stops the turn
	seen  map[string]int
}

// Hit records one missing call and reports its count and whether the turn should stop.
func (d *DoomLoop) Hit(name string) (int, bool) {
	if d.seen == nil {
		d.seen = map[string]int{}
	}
	d.seen[name]++
	limit := d.Limit
	if limit <= 0 {
		limit = 2
	}
	return d.seen[name], d.seen[name] >= limit
}

// Reset clears the counts (a new turn).
func (d *DoomLoop) Reset() { d.seen = nil }

// StopMessage is the text the model reads when the guard fires.
func (d *DoomLoop) StopMessage(name string) string {
	return fmt.Sprintf("doom-loop guard: %q was called %d times this turn and is not available — the turn stops here; name the missing capability with @? (one hop up) instead of calling again", name, d.seen[name])
}
