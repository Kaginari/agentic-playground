package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/tool"
)

// subjectOf is what a permission rule's glob is matched against (config.md §Permission
// rules): the command for bash and git, the world-relative path for the path tools, the URL
// for webfetch, the query for websearch, the body for dispatch, the Mind for skill, the JSON
// input for anything else.
func subjectOf(name string, in json.RawMessage, env tool.Env) string {
	var m map[string]interface{}
	_ = json.Unmarshal(in, &m)
	str := func(k string) string {
		v, _ := m[k].(string)
		return v
	}
	switch name {
	case "bash":
		return str("command")
	case "git":
		if c := str("command"); c != "" {
			return c
		}
		var parts []string
		if args, ok := m["args"].([]interface{}); ok {
			for _, a := range args {
				parts = append(parts, fmt.Sprint(a))
			}
		}
		return "git " + strings.Join(parts, " ")
	case "read", "ls", "glob", "grep", "write", "edit", "multiedit", "patch", "str_replace_based_edit_tool":
		p := str("path")
		if p == "" {
			return ""
		}
		return strings.TrimPrefix(env.Rel(env.Resolve(p)), "./")
	case "webfetch":
		return str("url")
	case "websearch":
		return str("query")
	case "dispatch":
		return str("body")
	case "skill":
		return str("name")
	}
	return string(in)
}

// decideHook is the loop's Decide seam: config.Decide before the gate.
func decideHook(cfg *config.Config, env func() tool.Env) func(s *loop.Session, st *loop.StepRecord, cls tool.Classification) loop.Decision {
	return func(s *loop.Session, st *loop.StepRecord, cls tool.Classification) loop.Decision {
		name := st.Tool
		if name == "str_replace_based_edit_tool" {
			name = "edit"
		}
		action, rule := cfg.Decide(name, subjectOf(st.Tool, st.Input, env()), cls.Class.String())
		if rule == nil {
			return loop.Decision{}
		}
		return loop.Decision{Action: string(action), Why: fmt.Sprintf("%s → %s (%s)", rule.Match, rule.Action, rule.Origin)}
	}
}
