package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/discover"
	"github.com/Kaginari/agent-one/mcp"
	"github.com/Kaginari/agent-one/sandbox"
	"github.com/Kaginari/agent-one/tool"
)

// MCPServer is one connected (or failed) server.
type MCPServer struct {
	Name   string
	Source string // config | claude:.mcp.json | opencode:opencode.jsonc
	Inward bool
	Client *mcp.Client
	Fault  string // why it is not serving
	Tools  []*tool.Tool
}

// MCPPrompt is a server prompt, offered as the slash command /<server>:<name>.
type MCPPrompt struct {
	Server, Name, Description string
	Args                      []string
}

// MCPSet is every MCP server the session talks to (binary.md §MCP): config servers plus the
// imports from .mcp.json and opencode.json(c), each behind its switch; tools named
// mcp__<server>__<tool>, classes outward unless inward; stdio servers under the sandbox.
type MCPSet struct {
	Servers map[string]*MCPServer
	Prompts []MCPPrompt
	holes   []string
	mu      sync.Mutex
}

// ConnectMCP connects every enabled server; a server that fails to start is a hole and its
// entry keeps the fault (Principle 9: reported, never routed around).
func ConnectMCP(ctx context.Context, cfg *config.Config, root string, sb *sandbox.Sandbox, env func(string) string) *MCPSet {
	set := &MCPSet{Servers: map[string]*MCPServer{}}
	if !cfg.MCP.Enabled {
		return set
	}
	type spec struct {
		cfg    mcp.ServerConfig
		opt    mcp.ServerOptions
		source string
	}
	specs := map[string]spec{}
	for name, s := range cfg.MCP.Servers {
		if !s.Enabled {
			continue
		}
		sc := mcp.ServerConfig{Name: name, Transport: s.Type, Env: s.Env, EnvAllow: s.EnvAllow, Cwd: s.Cwd, Network: s.Network, URL: s.URL, Headers: map[string]string{}, Timeout: orDur(s.Timeout.D(), orDur(cfg.MCP.Timeout.D(), 5*time.Second))}
		if len(s.Command) > 0 {
			sc.Command, sc.Args = s.Command[0], append(append([]string(nil), s.Command[1:]...), s.Args...)
		}
		if sc.Cwd == "" {
			sc.Cwd = root
		} else if !filepath.IsAbs(sc.Cwd) {
			sc.Cwd = filepath.Join(root, sc.Cwd)
		}
		if strings.Contains(sc.Command, "/") && !filepath.IsAbs(sc.Command) {
			sc.Command = filepath.Join(root, sc.Command) // a workspace-relative server binary
		}
		for k, v := range s.Headers {
			sc.Headers[k] = v
		}
		for h, v := range s.HeadersEnv {
			if val := env(v); val != "" {
				sc.Headers[h] = val
			} else {
				set.hole(fmt.Sprintf("mcp.servers.%s.headersEnv.%s: %s is not set", name, h, v))
			}
		}
		switch s.Sandbox {
		case "none":
			sc.Sandbox = nil
		default:
			sc.Sandbox = sb
		}
		opt := mcp.ServerOptions{Enabled: true, Inward: s.Inward, Classes: map[string]string{}}
		for tn, t := range s.Tools {
			if !t.Enabled {
				opt.Disable = append(opt.Disable, tn)
			}
			opt.Classes[tn] = cfg.MCPClass(name, tn)
		}
		specs[name] = spec{sc, opt, "config"}
	}
	if cfg.MCP.Import.ClaudeCode.Enabled || cfg.MCP.Import.Opencode.Enabled {
		claude, oc := cfg.MCP.Import.ClaudeCode.Enabled, cfg.MCP.Import.Opencode.Enabled
		found, errs := discover.MCP(discover.Options{Root: root, MCP: true, Sources: discover.Sources{Claude: &claude, OpenCode: &oc}})
		for _, e := range errs {
			set.hole("mcp import: " + e.Error())
		}
		for _, f := range found {
			if _, ok := specs[f.Name]; ok || !f.Enabled {
				continue // config wins over an import of the same name
			}
			sc := mcp.ServerConfig{Name: f.Name, Transport: f.Transport, Command: f.Command, Args: f.Args, Env: f.Env, URL: f.URL, Headers: f.Headers, Network: true, Sandbox: sb, Timeout: orDur(cfg.MCP.Timeout.D(), 5*time.Second)}
			specs[f.Name] = spec{sc, mcp.ServerOptions{Enabled: true}, f.Source}
		}
	}
	names := make([]string, 0, len(specs))
	for n := range specs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		sp := specs[name]
		srv := &MCPServer{Name: name, Source: sp.source, Inward: sp.opt.Inward}
		set.Servers[name] = srv
		cctx, cancel := context.WithTimeout(ctx, orDur(sp.cfg.Timeout, 5*time.Second)*3)
		c, err := mcp.Connect(cctx, sp.cfg)
		cancel()
		if err != nil {
			srv.Fault = err.Error()
			set.hole(fmt.Sprintf("mcp server %s did not start: %v", name, err))
			continue
		}
		srv.Client = c
		tools, err := mcp.Tools(ctx, c, sp.opt)
		if err != nil {
			srv.Fault = err.Error()
			set.hole(fmt.Sprintf("mcp server %s: %v", name, err))
		}
		srv.Tools = tools
		if ps, err := c.ListPrompts(ctx); err == nil {
			for _, p := range ps {
				var args []string
				for _, a := range p.Arguments {
					args = append(args, a.Name)
				}
				set.Prompts = append(set.Prompts, MCPPrompt{Server: name, Name: p.Name, Description: p.Description, Args: args})
			}
		}
	}
	return set
}

func (m *MCPSet) hole(h string) {
	m.mu.Lock()
	m.holes = append(m.holes, h)
	m.mu.Unlock()
}

// Holes lists faults met while connecting.
func (m *MCPSet) Holes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.holes...)
}

// Tools is every server's tools, servers in name order.
func (m *MCPSet) Tools() []*tool.Tool {
	var out []*tool.Tool
	for _, n := range m.names() {
		out = append(out, m.Servers[n].Tools...)
	}
	return out
}

func (m *MCPSet) names() []string {
	names := make([]string, 0, len(m.Servers))
	for n := range m.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Status renders one line per server: alive with its tool count, or its fault.
func (m *MCPSet) Status() []string {
	var out []string
	for _, n := range m.names() {
		s := m.Servers[n]
		kind := "outward"
		if s.Inward {
			kind = "inward"
		}
		switch {
		case s.Client == nil:
			out = append(out, fmt.Sprintf("@? mcp %s (%s): down — %s", n, s.Source, s.Fault))
		case !s.Client.Alive():
			out = append(out, fmt.Sprintf("@? mcp %s (%s): died — %s", n, s.Source, s.Client.Fault()))
		default:
			out = append(out, fmt.Sprintf("mcp %s (%s, %s): %d tools", n, s.Source, kind, len(s.Tools)))
		}
	}
	return out
}

// ReadResource answers `read mcp://<server>/<uri>`.
func (m *MCPSet) ReadResource(ctx context.Context, ref string) (string, error) {
	rest := strings.TrimPrefix(ref, "mcp://")
	server, uri, ok := strings.Cut(rest, "/")
	if !ok || server == "" {
		return "", fmt.Errorf("%s is not mcp://<server>/<uri>", ref)
	}
	s := m.Servers[server]
	if s == nil {
		return "", fmt.Errorf("no MCP server %q (%s)", server, strings.Join(m.names(), ", "))
	}
	if s.Client == nil || !s.Client.Alive() {
		return "", fmt.Errorf("MCP server %s is down: %s", server, orStr(s.Fault, s.Client.Fault()))
	}
	blocks, err := s.Client.ReadResource(ctx, uri)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range blocks {
		switch {
		case c.Text != "":
			b.WriteString(c.Text)
		case c.Blob != "":
			fmt.Fprintf(&b, "[blob %s, %d bytes base64]", c.MimeType, len(c.Blob))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// Prompt renders a server prompt as text for a turn (the slash command /<server>:<name>).
func (m *MCPSet) Prompt(ctx context.Context, server, name string, args map[string]string) (string, error) {
	s := m.Servers[server]
	if s == nil || s.Client == nil {
		return "", fmt.Errorf("no MCP server %q is up", server)
	}
	r, err := s.Client.GetPrompt(ctx, name, args)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, msg := range r.Messages {
		if msg.Content.Text != "" {
			fmt.Fprintf(&b, "%s\n", msg.Content.Text)
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// IsInward reports the human's word on a server: it never leaves the machine.
func (m *MCPSet) IsInward(server string) bool {
	s := m.Servers[server]
	return s != nil && s.Inward
}

// Close ends every server.
func (m *MCPSet) Close() {
	for _, s := range m.Servers {
		if s.Client != nil {
			_ = s.Client.Close()
		}
	}
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
