package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/tool"
)

// ServerOptions is the per-server policy from config (mcp.servers.<name>).
type ServerOptions struct {
	Enabled bool
	Inward  bool              // the human marked this server inside the workspace: floor write, not outward
	Classes map[string]string // per-tool class override (mcp.servers.<s>.tools.<t>.class): the floor
	Disable []string          // tool names not to expose
}

// ToolName is the registry name of a server's tool.
func ToolName(server, tool string) string { return "mcp__" + server + "__" + tool }

// Classify settles an MCP tool's class: the floor is outward unless the server is inward
// (write), or the config's per-tool word; annotations only tighten — destructiveHint →
// destructive, openWorkspaceHint → outward; readOnlyHint lowers nothing.
func Classify(t ToolInfo, opt ServerOptions) (tool.Class, string) {
	floor, why := tool.Outward, "mcp server (a mouth outside the workspace)"
	if opt.Inward {
		floor, why = tool.Write, "mcp server marked inward"
	}
	if s, ok := opt.Classes[t.Name]; ok {
		if c, ok := tool.ParseClass(s); ok {
			floor, why = c, "class set in config for this tool"
		}
	}
	c := floor
	if t.Annotations.DestructiveHint != nil && *t.Annotations.DestructiveHint && c < tool.Destructive {
		c, why = tool.Destructive, "destructiveHint"
	}
	if t.Annotations.OpenWorkspaceHint != nil && *t.Annotations.OpenWorkspaceHint && c < tool.Outward {
		c, why = tool.Outward, "openWorkspaceHint"
	}
	return c, why
}

// Tools lists the server's tools as registry tools named mcp__<server>__<tool>. A call on a
// server that died reports the fault instead of vanishing.
func Tools(ctx context.Context, c *Client, opt ServerOptions) ([]*tool.Tool, error) {
	if !opt.Enabled {
		return nil, nil
	}
	infos, err := c.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcp %s: %w", c.Name(), err)
	}
	var out []*tool.Tool
	for _, ti := range infos {
		if contains(opt.Disable, ti.Name) {
			continue
		}
		out = append(out, Adapt(c, ti, opt))
	}
	return out, nil
}

// Adapt wraps one MCP tool.
func Adapt(c *Client, ti ToolInfo, opt ServerOptions) *tool.Tool {
	class, why := Classify(ti, opt)
	name := ToolName(c.Name(), ti.Name)
	schema := ti.InputSchema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	desc := strings.TrimSpace(ti.Description)
	if desc == "" {
		desc = ti.Name + " on MCP server " + c.Name()
	}
	remote := ti.Name
	return &tool.Tool{
		Name:        name,
		Description: desc,
		Schema:      schema,
		Class:       class,
		Classify: func(env tool.Env, in json.RawMessage) tool.Classification {
			return tool.Classification{Class: class, Why: why}
		},
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			if !c.Alive() {
				return tool.Result{Output: fmt.Sprintf("%s: mcp server %s is down (%s) — its tools cannot answer; name the need with @? or ask for the server to be restarted", name, c.Name(), c.Fault()), Err: true}
			}
			r, err := c.CallTool(ctx, remote, in)
			if err != nil {
				return tool.Result{Output: fmt.Sprintf("%s: %v", name, err), Err: true}
			}
			text := r.Text()
			if text == "" && r.IsError {
				text = "tool reported an error with no message"
			}
			return tool.Result{Output: text, Err: r.IsError}
		},
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
