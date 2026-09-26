package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/toolbox"
)

// workspaceTools are the tools that read the workspace itself (binary.md §The default toolset):
// recall · remember (the memory tiers) · toolbox (search the registry, load level 2) · onto
// (who owns a path, what a member may see) · skill (load a Skill) · workingNotes (working memory) ·
// drained (what left the context). Each honours its switch; a switched-off one is absent and
// the missing-tool policy names why.
func (a *App) workspaceTools(agent string) []*tool.Tool {
	var out []*tool.Tool
	on := func(name string) bool { ok, _ := a.Cfg.ToolEnabled(name); return ok }
	if on("recall") {
		out = append(out, a.recallTool(agent))
	}
	if on("remember") {
		out = append(out, a.rememberTool(agent))
	}
	if on("toolbox") {
		out = append(out, a.toolboxTool(agent))
	}
	if on("onto") {
		out = append(out, a.ontoTool())
	}
	if on("skill") {
		out = append(out, a.skillTool(agent))
	}
	if on("workingNotes") {
		out = append(out, a.workingNotesTool(agent))
	}
	if a.Drainer != nil && a.Cfg.Compaction.Enabled {
		out = append(out, a.Drainer.RecallTool())
	}
	return out
}

func failResult(format string, args ...interface{}) tool.Result {
	return tool.Result{Output: fmt.Sprintf(format, args...), Err: true}
}

func (a *App) recallTool(agent string) *tool.Tool {
	kinds := strings.Join([]string{a.Lex.Token("policy"), a.Lex.Token("team"), a.Lex.Token("domain")}, "|")
	return &tool.Tool{
		Name:        "recall",
		Description: "Recall from the memory tiers by meaning (long: the workspace's docs and log; shared: notes). Returns anchors (path#section) and snippets, never whole files. kind filters shared notes by " + kinds + ".",
		Schema:      json.RawMessage(`{"type":"object","properties":{"question":{"type":"string"},"k":{"type":"integer"},"tier":{"type":"string","enum":["long","shared","all"]},"kind":{"type":"string"}},"required":["question"]}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Question, Tier, Kind string
				K                    int
			}
			if err := json.Unmarshal(in, &q); err != nil || strings.TrimSpace(q.Question) == "" {
				return failResult("recall: question is required")
			}
			m, err := a.Workspace.Memory()
			if err != nil {
				return failResult("recall: %v", err)
			}
			if _, why := m.LoadIndex(); why != "" && a.Cfg.Memory.Long.RebuildOnStale {
				_, _ = m.Index()
			}
			kind := q.Kind
			if k, ok := a.Lex.Canonical(kind); ok {
				kind = k
			}
			k := q.K
			if k <= 0 {
				k = a.Cfg.Memory.Recall.K
			}
			res, err := m.Recall(q.Question, memory.RecallOpts{As: agent, Tier: q.Tier, Kind: kind, K: k, NoCache: !a.Cfg.Memory.Short.Enabled})
			if err != nil {
				return failResult("recall: %v", err)
			}
			var b strings.Builder
			for _, r := range res.Results {
				b.WriteString(memory.FormatResult(r) + "\n")
				if snip := strings.TrimSpace(r.Snippet); snip != "" {
					b.WriteString("   " + firstLine(snip) + "\n")
				}
			}
			for _, h := range res.Holes {
				b.WriteString("@? " + h + "\n")
			}
			if b.Len() == 0 {
				return tool.Result{Output: "no memory matches " + q.Question}
			}
			return tool.Result{Output: strings.TrimRight(b.String(), "\n")}
		},
	}
}

func (a *App) rememberTool(agent string) *tool.Tool {
	kinds := strings.Join([]string{a.Lex.Token("policy"), a.Lex.Token("team"), a.Lex.Token("domain")}, "|")
	return &tool.Tool{
		Name:        "remember",
		Description: "Append a shared note (workspace tier; machine: true for every workspace on this machine). kind marks the unsaid: " + kinds + ". Notes are append-only.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"kind":{"type":"string"},"tag":{"type":"string"},"machine":{"type":"boolean"}},"required":["text"]}`),
		Class:       tool.Write,
		Classify: func(env tool.Env, in json.RawMessage) tool.Classification {
			var q struct{ Machine bool }
			_ = json.Unmarshal(in, &q)
			if q.Machine {
				return tool.Classification{Class: tool.Write, Why: "appends a machine-shared note (~/" + a.Cfg.Dist.WorkspaceDir + ")"}
			}
			return tool.Classification{Class: tool.Write, Why: "appends a workspace note"}
		},
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Text, Kind, Tag string
				Machine         bool
			}
			if err := json.Unmarshal(in, &q); err != nil || strings.TrimSpace(q.Text) == "" {
				return failResult("remember: text is required")
			}
			if q.Machine && !a.Cfg.Memory.Shared.Machine.Enabled {
				return failResult("remember: the machine tier is off (memory.shared.machine.enabled)")
			}
			if !q.Machine && !a.Cfg.Memory.Shared.Workspace.Enabled {
				return failResult("remember: the workspace tier is off (memory.shared.workspace.enabled)")
			}
			kind := q.Kind
			if kind != "" {
				k, ok := a.Lex.Canonical(kind)
				if !ok {
					return failResult("remember: kind %q is not %s", kind, kinds)
				}
				kind = k
			}
			m, err := a.Workspace.Memory()
			if err != nil {
				return failResult("remember: %v", err)
			}
			r, err := m.Remember(q.Text, memory.RememberOpts{As: agent, Kind: kind, Tag: q.Tag, Machine: q.Machine})
			if err != nil {
				return failResult("remember: %v", err)
			}
			return tool.Result{Output: fmt.Sprintf("noted (%s, %s)", r.Scope, r.Src), Wrote: nil}
		},
	}
}

func (a *App) toolboxTool(agent string) *tool.Tool {
	return &tool.Tool{
		Name:        "toolbox",
		Description: "Search the toolbox registry (Skills, commands, tools, Agents, externals, MCP tools, switched-off tools) by question — level 1: names, one line, cost; or load one entry's agent by name (level 2, journaled). Loading is your own decision, never pre-emptive.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"load":{"type":"string","description":"entry name to load (level 2)"},"section":{"type":"integer","description":"one section of the loaded entry"},"k":{"type":"integer"}},"required":[]}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Query, Load string
				Section     *int
				K           int
			}
			if err := json.Unmarshal(in, &q); err != nil {
				return failResult("toolbox: %v", err)
			}
			tb, err := a.Workspace.Toolbox()
			if err != nil {
				return failResult("toolbox: %v", err)
			}
			if q.Load != "" {
				if r, err := tb.Load(q.Load, toolbox.LoadOpts{As: agent, Sec: q.Section}); err == nil {
					return tool.Result{Output: r.Text}
				}
				if s, ok := a.Found.Skill(q.Load); ok {
					text, err := s.Load()
					if err != nil {
						return failResult("toolbox: %v", err)
					}
					return tool.Result{Output: text}
				}
				if t, ok := a.mcpTool(q.Load); ok {
					return tool.Result{Output: fmt.Sprintf("%s — %s\nclass %s\nschema %s", t.Name, t.Description, t.Class, string(t.Schema))}
				}
				return failResult("toolbox: nothing named %q — search first", q.Load)
			}
			if strings.TrimSpace(q.Query) == "" {
				return failResult("toolbox: query or load is required")
			}
			if L := tb.LoadRegistry(); L.Live || tb.IsStale(L) {
				_, _ = tb.Index()
			}
			var b strings.Builder
			if br, err := tb.Brief(q.Query, toolbox.PickOpts{As: agent, Budget: a.Cfg.Toolbox.BudgetTokens, K: q.K}); err == nil {
				for _, l := range br.Lines {
					b.WriteString(l + "\n")
				}
			} else {
				b.WriteString("@? registry: " + err.Error() + "\n")
			}
			// the registry the files hold, plus what only the session knows: mcp tools, off tools
			words := strings.Fields(strings.ToLower(q.Query))
			hit := func(s string) bool {
				s = strings.ToLower(s)
				for _, w := range words {
					if strings.Contains(s, w) {
						return true
					}
				}
				return false
			}
			if a.MCP != nil {
				for _, t := range a.MCP.Tools() {
					if hit(t.Name + " " + t.Description) {
						fmt.Fprintf(&b, "@T mcp %s · %s · class %s\n", t.Name, firstLine(t.Description), t.Class)
					}
				}
			}
			for name, why := range a.Shelf.Disabled() {
				if hit(name) {
					fmt.Fprintf(&b, "@T off %s · %s\n", name, why)
				}
			}
			if b.Len() == 0 {
				return tool.Result{Output: "nothing in the toolbox matches " + q.Query}
			}
			return tool.Result{Output: strings.TrimRight(b.String(), "\n")}
		},
	}
}

func (a *App) mcpTool(name string) (*tool.Tool, bool) {
	if a.MCP == nil {
		return nil, false
	}
	for _, t := range a.MCP.Tools() {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

func (a *App) ontoTool() *tool.Tool {
	return &tool.Tool{
		Name:        "onto",
		Description: "Query the member graph: who owns a path (owner), what a member may see (member: its projection), or the roster (neither).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"member":{"type":"string"},"budget":{"type":"integer"}}}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Path, Member string
				Budget       int
			}
			_ = json.Unmarshal(in, &q)
			switch {
			case q.Path != "":
				rel := strings.TrimPrefix(env.Rel(env.Resolve(q.Path)), "./")
				if c := a.Workspace.Owner(rel); c != nil {
					return tool.Result{Output: fmt.Sprintf("%s is owned by %s (%s) — doc %s; one hop up: %s", rel, c.Name, c.Rank, c.Doc, orStr(c.Parent, "orchestrator"))}
				}
				return tool.Result{Output: rel + " has no owner on the roster (an unrouted ownership — a Provisioning trigger when it recurs)"}
			case q.Member != "":
				if a.Workspace.Onto == nil {
					return failResult("onto: the ontology is off (ontology.enabled)")
				}
				budget := q.Budget
				if budget <= 0 {
					budget = a.Cfg.Ontology.ProjectBudgetTokens
				}
				lines, err := a.Workspace.Onto.Project(strings.ToLower(q.Member), budget)
				if err != nil {
					return failResult("onto: %v", err)
				}
				if len(lines) == 0 {
					return tool.Result{Output: q.Member + " sees nothing yet"}
				}
				return tool.Result{Output: strings.Join(lines, "\n")}
			}
			var b strings.Builder
			for _, c := range a.Workspace.Members {
				fmt.Fprintf(&b, "%s (%s) owns %s", c.Name, c.Rank, orStr(strings.Join(c.Ownership, ", "), "nothing"))
				if c.Parent != "" {
					fmt.Fprintf(&b, " → %s", c.Parent)
				}
				b.WriteString("\n")
			}
			if b.Len() == 0 {
				return tool.Result{Output: "the roster is empty: no member under " + a.Cfg.Dist.WorkspaceDir + "/"}
			}
			return tool.Result{Output: strings.TrimRight(b.String(), "\n")}
		},
	}
}

func (a *App) skillTool(agent string) *tool.Tool {
	return &tool.Tool{
		Name:        "skill",
		Description: "Import a skill by name: load its full agent (level 2). Names come from the toolbox manifest (@T skill …) and the skill dirs.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"section":{"type":"integer"}},"required":["name"]}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Name    string
				Section *int
			}
			if err := json.Unmarshal(in, &q); err != nil || q.Name == "" {
				return failResult("skill: name is required")
			}
			if tb, err := a.Workspace.Toolbox(); err == nil {
				if r, err := tb.Load(q.Name, toolbox.LoadOpts{As: agent, Kind: "skill", Sec: q.Section}); err == nil {
					return tool.Result{Output: r.Text}
				}
			}
			s, ok := a.Found.Skill(q.Name)
			if !ok {
				var names []string
				for _, sk := range a.Found.Skills {
					names = append(names, sk.Name)
				}
				return failResult("skill: no Skill named %q (known: %s)", q.Name, orStr(strings.Join(names, ", "), "none"))
			}
			text, err := s.Load()
			if err != nil {
				return failResult("skill: %v", err)
			}
			return tool.Result{Output: text}
		},
	}
}

// workingNotesTool is the working memory: at most ~5 dated notes in
// <workspace>/instruments/working-notes/<session>.md; over the limit is a stress reading, said out loud.
func (a *App) workingNotesTool(agent string) *tool.Tool {
	limit := a.Cfg.Tools.WorkingNotes.Limit
	if limit <= 0 {
		limit = memory.WorkingNotesLimit
	}
	return &tool.Tool{
		Name:        "workingNotes",
		Description: fmt.Sprintf("Your working memory: the goal, where the plan stands, decisions and why, open holes, the next step — at most %d notes. `notes` replaces the working notes; `add` appends one; neither reads it.", limit),
		Schema:      json.RawMessage(`{"type":"object","properties":{"notes":{"type":"array","items":{"type":"string"}},"add":{"type":"string"}}}`),
		Class:       tool.Write,
		Classify: func(env tool.Env, in json.RawMessage) tool.Classification {
			return tool.Classification{Class: tool.Write, Why: "writes the workingNotes"}
		},
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Notes []string
				Add   string
			}
			_ = json.Unmarshal(in, &q)
			p := a.workingNotesPath(a.SessionID)
			cur := readWorkingNotes(p)
			switch {
			case len(q.Notes) > 0:
				cur = q.Notes
			case strings.TrimSpace(q.Add) != "":
				cur = append(cur, strings.TrimSpace(q.Add))
			default:
				if len(cur) == 0 {
					return tool.Result{Output: "the working notes is empty"}
				}
				return tool.Result{Output: renderWorkingNotes(cur)}
			}
			if err := writeWorkingNotes(p, agent, cur); err != nil {
				return failResult("workingNotes: %v", err)
			}
			out := fmt.Sprintf("workingNotes: %d note%s → %s", len(cur), plural(len(cur)), relOrAbs(a.Root, p))
			if len(cur) > limit {
				out += fmt.Sprintf("\n@? the working notes holds %d notes, over the limit of %d — a stress reading (Principle 5): consolidate to a rule, a invariant or a wrap-up, then let go", len(cur), limit)
			}
			return tool.Result{Output: out}
		},
	}
}

func readWorkingNotes(p string) []string {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "- ") {
			out = append(out, strings.TrimPrefix(l, "- "))
		}
	}
	return out
}

func renderWorkingNotes(notes []string) string {
	var b strings.Builder
	for _, t := range notes {
		b.WriteString("- " + t + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeWorkingNotes(p, agent string, notes []string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	head := fmt.Sprintf("# workingNotes — %s — %s\n\n", agent, time.Now().UTC().Format("2006-01-02"))
	return os.WriteFile(p, []byte(head+renderWorkingNotes(notes)+"\n"), 0o644)
}

func sortedNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
