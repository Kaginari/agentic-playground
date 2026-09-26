package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/sandbox"
	"github.com/Kaginari/agent-one/shell"
	"github.com/Kaginari/agent-one/tool"
)

// shelfOrder is the policy's order for the builtin table (binary.md §The default toolset).
var shelfOrder = []string{"bash", "read", "ls", "glob", "grep", "write", "edit", "multiedit", "patch", "git", "webfetch", "websearch", "ask"}

// workspaceToolNames are the tools the workspace package supplies (rung 3); the shelf leaves a seat
// for each and reports it disabled with its origin when config switches it off.
var workspaceToolNames = []string{"dispatch", "recall", "remember", "toolbox", "onto", "skill", "workingNotes"}

// Shelf builds the tool registry for an agent from config: every builtin honours its enabled
// switch, the profile and its overrides; custom tools join on the same terms; MCP tools and
// the workspace's own tools are added by their packages through Extra. Build is called once per
// agent so each gets its own persistent shell (bash) — a Subagent's dies with its task.
type Shelf struct {
	Cfg          *config.Config
	Root         string
	WorkspaceDir string
	Sandbox      *sandbox.Sandbox
	Asker        tool.Asker
	Env          func(string) string
	// Extra adds tools a later rung supplies (MCP, the workspace's own); called per Build with the
	// agent's name; a nil result adds nothing.
	Extra []func(agent string) []*tool.Tool
	// Resource answers `read mcp://…` (set by the MCP set); nil: such a read is a fault.
	Resource func(ctx context.Context, uri string) (string, error)
	// Inward says whether an MCP server was marked inward (its resource reads are class read).
	Inward func(server string) bool

	holes []string
}

// NewShelf reads the switchboard once.
func NewShelf(cfg *config.Config, root string, sb *sandbox.Sandbox, asker tool.Asker, env func(string) string) *Shelf {
	return &Shelf{Cfg: cfg, Root: root, WorkspaceDir: cfg.Dist.WorkspaceDir, Sandbox: sb, Asker: asker, Env: env}
}

// Holes lists what could not be built.
func (s *Shelf) Holes() []string { return append([]string(nil), s.holes...) }

func (s *Shelf) hole(h string) {
	for _, x := range s.holes {
		if x == h {
			return
		}
	}
	s.holes = append(s.holes, h)
}

// Disabled maps every tool that is off to the reason (binary.md §When a tool is missing:
// the model reads why and the closest enabled alternatives).
func (s *Shelf) Disabled() map[string]string {
	out := map[string]string{}
	names := append(append([]string(nil), shelfOrder...), workspaceToolNames...)
	for _, n := range names {
		if ok, why := s.Cfg.ToolEnabled(n); !ok {
			out[n] = strings.TrimPrefix(why, "disabled by ")
		}
	}
	for n := range s.Cfg.Tools.Custom {
		if ok, why := s.Cfg.ToolEnabled(n); !ok {
			out[n] = strings.TrimPrefix(why, "disabled by ")
		}
	}
	return out
}

func (s *Shelf) on(name string) bool {
	ok, _ := s.Cfg.ToolEnabled(name)
	return ok
}

func (s *Shelf) builtin(name string) *config.BuiltinTool {
	b, _ := s.Cfg.Builtin(name)
	if b == nil {
		return &config.BuiltinTool{}
	}
	return b
}

func (s *Shelf) sandboxFor(mode string) *sandbox.Sandbox {
	switch mode {
	case "none":
		return nil
	case "bwrap":
		if s.Sandbox != nil && s.Sandbox.Mode() == sandbox.Bwrap {
			return s.Sandbox
		}
		return s.Sandbox
	}
	return s.Sandbox
}

func (s *Shelf) shellSandbox() *sandbox.Sandbox {
	if s.Cfg.Tools.Bash.Sandbox == "none" {
		return nil
	}
	return s.Sandbox
}

// Build makes a fresh registry for an agent. The closer ends the agent's shell and its jobs.
func (s *Shelf) Build(agent string) (*tool.Registry, func()) {
	reg := tool.NewRegistry()
	var closers []func()
	add := func(t *tool.Tool) {
		if t == nil {
			return
		}
		s.override(t)
		reg.Add(t)
	}
	profile := s.Cfg.Tools.Profile
	for _, name := range shelfOrder {
		if !s.on(name) {
			continue
		}
		b := s.builtin(name)
		switch name {
		case "bash":
			t, closer := tool.NewBashTool(tool.BashOptions{
				Enabled: true,
				Shell: shell.Options{Root: s.Root, Sandbox: s.shellSandbox(), JobsDir: s.abs(b.JobsDir),
					Timeout: orDur(b.MaxTimeout.D(), 10*time.Minute), OutputCap: s.Cfg.Tools.Output.MaxBytes},
				Timeout: orDur(b.Timeout.D(), 2*time.Minute),
			})
			if !b.Background {
				t = withoutBackground(t)
			}
			closers = append(closers, closer)
			add(t)
		case "read":
			add(s.readTool())
		case "ls":
			add(tool.LsTool(tool.LsOptions{Enabled: true, Cap: b.Limit}))
		case "glob":
			add(tool.GlobTool())
		case "grep":
			add(tool.GrepTool())
		case "write":
			add(tool.WriteTool())
		case "edit":
			add(tool.EditTool())
			if profile != "minimal" {
				add(tool.EditorTool(tool.EditorOptions{Enabled: true}))
			}
		case "multiedit":
			add(tool.MultiEditTool(tool.MultiEditOptions{Enabled: true}))
		case "patch":
			add(tool.PatchTool(tool.PatchOptions{Enabled: true}))
		case "git":
			add(tool.GitTool(tool.GitOptions{Enabled: true, Timeout: orDur(b.Timeout.D(), 2*time.Minute), Sandbox: s.shellSandbox()}))
		case "webfetch":
			add(tool.WebFetchTool(tool.WebFetchOptions{Enabled: true, MaxBytes: int64(b.MaxBytes), Timeout: orDur(b.Timeout.D(), time.Minute)}))
		case "websearch":
			add(tool.WebSearchTool(tool.WebSearchOptions{Enabled: true, Backend: s.searchBackend(b)}))
		case "ask":
			add(tool.AskTool(tool.AskOptions{Enabled: true, Asker: s.Asker}))
		}
	}
	for _, name := range sortedKeys(s.Cfg.Tools.Custom) {
		c := s.Cfg.Tools.Custom[name]
		if !s.on(name) {
			continue
		}
		t, err := s.customTool(name, c)
		if err != nil {
			s.hole(fmt.Sprintf("tools.custom.%s: %v", name, err))
			continue
		}
		if _, builtin := config.Builtins[name]; builtin && c.Override {
			reg.Remove(name)
		}
		reg.Add(t)
	}
	for _, extra := range s.Extra {
		for _, t := range extra(agent) {
			if t != nil {
				reg.Add(t)
			}
		}
	}
	if profile == "openai" {
		for _, n := range reg.Names() {
			if t, ok := reg.Get(n); ok && t.Declare != nil {
				cp := *t
				cp.Declare = nil
				reg.Add(&cp)
			}
		}
	}
	return reg, func() {
		for _, c := range closers {
			c()
		}
	}
}

// override applies a builtin's config overrides: description, a tighter class, a timeout.
func (s *Shelf) override(t *tool.Tool) {
	b, ok := s.Cfg.Builtin(t.Name)
	if !ok {
		return
	}
	if b.Description != "" {
		t.Description = b.Description
	}
	if b.Class != "" {
		if c, ok := tool.ParseClass(b.Class); ok && c > t.Class {
			t.Class = c
		}
	}
}

func (s *Shelf) abs(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(s.Root, p)
}

// customTool converts a config entry to a tool.
func (s *Shelf) customTool(name string, c *config.CustomTool) (*tool.Tool, error) {
	sb := c.Sandbox != "none"
	return tool.CustomTool(tool.Custom{
		Name: name, Description: c.Description, Params: json.RawMessage(c.Schema()),
		Run: c.Run, Shell: c.Shell, Class: c.Class, Timeout: int(c.Timeout.D() / time.Second), Cwd: c.Cwd, Sandbox: sb,
	}, tool.CustomOptions{Sandbox: s.sandboxFor(c.Sandbox), Timeout: 2 * time.Minute})
}

// readTool is `read` with `mcp://<server>/<uri>` routed to the server's resource.
func (s *Shelf) readTool() *tool.Tool {
	base := tool.ReadTool()
	run := base.Run
	classify := base.Classify
	t := *base
	t.Description = base.Description + " A path mcp://<server>/<uri> reads that MCP server's resource."
	t.Classify = func(env tool.Env, in json.RawMessage) tool.Classification {
		var a struct{ Path string }
		_ = json.Unmarshal(in, &a)
		if strings.HasPrefix(a.Path, "mcp://") {
			server := strings.SplitN(strings.TrimPrefix(a.Path, "mcp://"), "/", 2)[0]
			if s.Inward != nil && s.Inward(server) {
				return tool.Classification{Class: tool.Read, Why: "reads a resource on inward MCP server " + server}
			}
			return tool.Classification{Class: tool.Outward, Why: "reads a resource on MCP server " + server}
		}
		return classify(env, in)
	}
	t.Run = func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
		var a struct{ Path string }
		_ = json.Unmarshal(in, &a)
		if strings.HasPrefix(a.Path, "mcp://") {
			if s.Resource == nil {
				return tool.Result{Output: "read: no MCP server is connected (mcp.servers, mcp.import) — " + a.Path + " cannot be read", Err: true}
			}
			text, err := s.Resource(ctx, a.Path)
			if err != nil {
				return tool.Result{Output: "read: " + err.Error(), Err: true}
			}
			return tool.Result{Output: text}
		}
		return run(ctx, env, in)
	}
	return &t
}

// withoutBackground refuses `background` when tools.bash.background is off.
func withoutBackground(t *tool.Tool) *tool.Tool {
	run := t.Run
	cp := *t
	cp.Run = func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
		var a struct{ Background json.RawMessage }
		_ = json.Unmarshal(in, &a)
		if len(a.Background) > 0 && string(a.Background) != "false" && string(a.Background) != "null" {
			return tool.Result{Output: "bash: background jobs are off (tools.bash.background: false)", Err: true}
		}
		return run(ctx, env, in)
	}
	return &cp
}

// searchBackend is a SearXNG-style JSON endpoint named by tools.websearch.backend; nil when
// none is configured (the tool then says so).
func (s *Shelf) searchBackend(b *config.BuiltinTool) tool.Searcher {
	if b.Backend == "" {
		return nil
	}
	timeout := orDur(b.Timeout.D(), time.Minute)
	return tool.SearcherFunc(func(ctx context.Context, q string, n int) ([]tool.Hit, error) {
		u := strings.ReplaceAll(b.Backend, "{query}", url.QueryEscape(q))
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(cctx, "GET", u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		res, err := (&http.Client{Timeout: timeout}).Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		agent, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("websearch backend: HTTP %d", res.StatusCode)
		}
		var out struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
				Snippet string `json:"snippet"`
			} `json:"results"`
		}
		if err := json.Unmarshal(agent, &out); err != nil {
			return nil, fmt.Errorf("websearch backend: %v", err)
		}
		var hits []tool.Hit
		for i, r := range out.Results {
			if n > 0 && i >= n {
				break
			}
			snip := r.Content
			if snip == "" {
				snip = r.Snippet
			}
			hits = append(hits, tool.Hit{Title: r.Title, URL: r.URL, Snippet: snip})
		}
		return hits, nil
	})
}

func orDur(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func sortedKeys[T any](m map[string]T) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
