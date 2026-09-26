package config

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// Argv fills a custom tool's argv: each `{{param}}` becomes the parameter's value inside its
// own element, so a value like `; rm -rf /` stays one argument and never meets a shell.
func (t *CustomTool) Argv(params map[string]string) ([]string, error) {
	if len(t.Run) == 0 {
		return nil, fmt.Errorf("custom tool has no run argv")
	}
	if err := t.checkParams(params); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(t.Run))
	for _, el := range t.Run {
		out = append(out, placeholder.ReplaceAllStringFunc(el, func(m string) string {
			name := placeholder.FindStringSubmatch(m)[1]
			if v, ok := params[name]; ok {
				return v
			}
			return t.Params[name].Default
		}))
	}
	return out, nil
}

// ShellEnv is the shell form's environment: every parameter as P_<name>=<value>, so the
// template reads "$P_path" and the shell never interpolates the value into its own text.
func (t *CustomTool) ShellEnv(params map[string]string) ([]string, error) {
	if t.Shell == "" {
		return nil, fmt.Errorf("custom tool has no shell template")
	}
	if err := t.checkParams(params); err != nil {
		return nil, err
	}
	var env []string
	for _, name := range sortedKeys(t.Params) {
		v, ok := params[name]
		if !ok {
			v = t.Params[name].Default
		}
		env = append(env, "P_"+name+"="+v)
	}
	return env, nil
}

func (t *CustomTool) checkParams(params map[string]string) error {
	for name, p := range t.Params {
		v, ok := params[name]
		if !ok {
			if p.Required && p.Default == "" {
				return fmt.Errorf("parameter %q is required", name)
			}
			continue
		}
		if len(p.Enum) > 0 && !contains(p.Enum, v) {
			return fmt.Errorf("parameter %q: %q is not one of %s", name, v, strings.Join(p.Enum, ", "))
		}
	}
	for name := range params {
		if _, ok := t.Params[name]; !ok {
			return fmt.Errorf("unknown parameter %q", name)
		}
	}
	return nil
}

// Schema renders the tool's flat JSON-schema object for a provider's tool definition.
func (t *CustomTool) Schema() string {
	var props []string
	var required []string
	for _, name := range sortedKeys(t.Params) {
		p := t.Params[name]
		typ := p.Type
		if typ == "" {
			typ = "string"
		}
		prop := fmt.Sprintf("%q:{\"type\":%q", name, typ)
		if p.Description != "" {
			prop += fmt.Sprintf(",\"description\":%q", p.Description)
		}
		if len(p.Enum) > 0 {
			var es []string
			for _, e := range p.Enum {
				es = append(es, fmt.Sprintf("%q", e))
			}
			prop += ",\"enum\":[" + strings.Join(es, ",") + "]"
		}
		props = append(props, prop+"}")
		if p.Required {
			required = append(required, fmt.Sprintf("%q", name))
		}
	}
	s := "{\"type\":\"object\",\"properties\":{" + strings.Join(props, ",") + "}"
	if len(required) > 0 {
		s += ",\"required\":[" + strings.Join(required, ",") + "]"
	}
	return s + "}"
}

// builtinFields maps a builtin tool name to its Tools struct field.
func builtinFields() map[string]int {
	m := map[string]int{}
	t := reflect.TypeOf(Tools{})
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if _, ok := Builtins[tag]; ok {
			m[tag] = i
		}
	}
	return m
}

// Builtin returns a builtin tool's settings.
func (c *Config) Builtin(name string) (*BuiltinTool, bool) {
	i, ok := builtinFields()[name]
	if !ok {
		return nil, false
	}
	return reflect.ValueOf(&c.Tools).Elem().Field(i).Addr().Interface().(*BuiltinTool), true
}

// ToolEnabled says whether a tool (builtin or custom) is on: the profile offers it and its
// own switch is on. The reason names the origin when it is off.
func (c *Config) ToolEnabled(name string) (bool, string) {
	if t, ok := c.Tools.Custom[name]; ok {
		if !t.Enabled {
			return false, fmt.Sprintf("disabled by tools.custom.%s.enabled in %s", name, c.Where("tools.custom."+name+".enabled"))
		}
		if len(t.Profiles) > 0 && !contains(t.Profiles, c.Tools.Profile) && c.Tools.Profile != "max" {
			return false, fmt.Sprintf("not offered under tools.profile %s (%s)", c.Tools.Profile, c.Where("tools.profile"))
		}
		return true, ""
	}
	b, ok := c.Builtin(name)
	if !ok {
		return false, "no such tool"
	}
	if !b.Enabled {
		return false, fmt.Sprintf("disabled by tools.%s.enabled in %s", name, c.Where("tools."+name+".enabled"))
	}
	if set := Profiles[c.Tools.Profile]; set != nil && !contains(set, name) {
		return false, fmt.Sprintf("not offered under tools.profile %s (%s)", c.Tools.Profile, c.Where("tools.profile"))
	}
	return true, ""
}

// EnabledTools lists every tool that is on, builtins first in policy order, then custom.
func (c *Config) EnabledTools() []string {
	var out []string
	for _, n := range builtinOrder() {
		if ok, _ := c.ToolEnabled(n); ok {
			out = append(out, n)
		}
	}
	for _, n := range sortedKeys(c.Tools.Custom) {
		if ok, _ := c.ToolEnabled(n); ok {
			out = append(out, n)
		}
	}
	return out
}

// ToolsOff lists switched-off tools for status's plain `tools off:` line.
func (c *Config) ToolsOff() []string {
	var out []string
	for _, n := range builtinOrder() {
		if ok, why := c.ToolEnabled(n); !ok {
			out = append(out, n+" — "+why)
		}
	}
	for _, n := range sortedKeys(c.Tools.Custom) {
		if ok, why := c.ToolEnabled(n); !ok {
			out = append(out, n+" — "+why)
		}
	}
	return out
}

func builtinOrder() []string {
	names := make([]string, 0, len(Builtins))
	for n := range Builtins {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ToolClass is a builtin's effective floor: the declared class may only tighten.
func (c *Config) ToolClass(name string) string {
	if b, ok := c.Builtin(name); ok && b.Class != "" {
		return b.Class
	}
	if t, ok := c.Tools.Custom[name]; ok {
		return t.Class
	}
	return Builtins[name]
}

// checkTools validates the profile, builtin overrides and every custom tool.
func (c *Config) checkTools() error {
	switch c.Tools.Profile {
	case "max", "anthropic", "openai", "minimal":
	default:
		return fmt.Errorf("%s: tools.profile: %q is not max, anthropic, openai or minimal", c.Where("tools.profile"), c.Tools.Profile)
	}
	for _, n := range builtinOrder() {
		b, _ := c.Builtin(n)
		if b.Class != "" {
			if _, ok := classOrder[b.Class]; !ok {
				return fmt.Errorf("%s: tools.%s.class: %q is not read, write, outward or destructive", c.Where("tools."+n+".class"), n, b.Class)
			}
			if floor := Builtins[n]; floor != "" && classOrder[b.Class] < classOrder[floor] {
				return fmt.Errorf("%s: tools.%s.class: %s loosens the floor %s — a declared class only tightens", c.Where("tools."+n+".class"), n, b.Class, floor)
			}
		}
		if n == "bash" && b.Sandbox != "" && b.Sandbox != "bwrap" && b.Sandbox != "none" {
			return fmt.Errorf("%s: tools.bash.sandbox: %q is not bwrap or none", c.Where("tools.bash.sandbox"), b.Sandbox)
		}
	}
	for _, name := range sortedKeys(c.Tools.Custom) {
		t := c.Tools.Custom[name]
		at := func(f string) string { return c.Where("tools.custom." + name + "." + f) }
		here := c.Where("tools.custom." + name)
		if _, builtin := Builtins[name]; builtin && !t.Override {
			return fmt.Errorf("%s: tools.custom.%s collides with the builtin of that name — set override: true to replace it", here, name)
		}
		if t.Class == "" {
			return fmt.Errorf("%s: tools.custom.%s.class is required (read | write | outward | destructive)", here, name)
		}
		if _, ok := classOrder[t.Class]; !ok {
			return fmt.Errorf("%s: tools.custom.%s.class: %q is not read, write, outward or destructive", at("class"), name, t.Class)
		}
		if len(t.Run) == 0 && t.Shell == "" {
			return fmt.Errorf("%s: tools.custom.%s needs run (argv) or shell (template)", here, name)
		}
		if len(t.Run) > 0 && t.Shell != "" {
			return fmt.Errorf("%s: tools.custom.%s: run and shell are one or the other", here, name)
		}
		if t.Shell != "" && placeholder.MatchString(t.Shell) {
			return fmt.Errorf("%s: tools.custom.%s.shell: no {{param}} in a shell template — parameters arrive as $P_<name>", at("shell"), name)
		}
		for _, el := range t.Run {
			for _, m := range placeholder.FindAllStringSubmatch(el, -1) {
				if _, ok := t.Params[m[1]]; !ok {
					return fmt.Errorf("%s: tools.custom.%s.run: placeholder {{%s}} is not in params", at("run"), name, m[1])
				}
			}
		}
		for pn, p := range t.Params {
			switch p.Type {
			case "", "string", "integer", "number", "boolean":
			default:
				return fmt.Errorf("%s: tools.custom.%s.params.%s.type: %q is not string, integer, number or boolean", at("params."+pn+".type"), name, pn, p.Type)
			}
		}
		switch t.Sandbox {
		case "", "inherit", "bwrap", "none":
		default:
			return fmt.Errorf("%s: tools.custom.%s.sandbox: %q is not inherit, bwrap or none", at("sandbox"), name, t.Sandbox)
		}
		for _, p := range t.Profiles {
			if _, ok := Profiles[p]; !ok || p == "max" {
				return fmt.Errorf("%s: tools.custom.%s.profiles: %q is not anthropic, openai or minimal", at("profiles"), name, p)
			}
		}
	}
	return nil
}

// checkMCP validates servers: transport, credentials only via env, tool classes.
func (c *Config) checkMCP() error {
	if c.tree != nil {
		if servers := c.tree.Get("mcp").Get("servers"); servers != nil {
			for i, name := range servers.Keys {
				if o := servers.Vals[i].Get("oauth"); o != nil {
					return fmt.Errorf("%s: mcp.servers.%s.oauth: later — a remote server authenticates by a header from env (headersEnv) for now", o.Where(), name)
				}
			}
		}
	}
	for _, name := range sortedKeys(c.MCP.Servers) {
		s := c.MCP.Servers[name]
		here := c.Where("mcp.servers." + name)
		at := func(f string) string { return c.Where("mcp.servers." + name + "." + f) }
		if !s.Enabled && s.Type == "" && len(s.Command) == 0 && s.URL == "" {
			continue // `{enabled: false}` alone switches a server off without its definition
		}
		if s.Type == "" {
			switch {
			case len(s.Command) > 0:
				s.Type = "stdio"
			case s.URL != "":
				s.Type = "http"
			}
		}
		switch s.Type {
		case "stdio":
			if len(s.Command) == 0 {
				return fmt.Errorf("%s: mcp.servers.%s: a stdio server needs command", here, name)
			}
		case "http":
			if s.URL == "" {
				return fmt.Errorf("%s: mcp.servers.%s: an http server needs url", here, name)
			}
		default:
			return fmt.Errorf("%s: mcp.servers.%s.type: %q is not stdio or http", at("type"), name, s.Type)
		}
		for _, h := range sortedKeys(s.Headers) {
			lh := strings.ToLower(h)
			if lh == "authorization" || lh == "x-api-key" || lh == "api-key" || looksSecret(s.Headers[h]) {
				return fmt.Errorf("%s: mcp.servers.%s.headers.%s carries a credential — put the env var name under headersEnv", at("headers."+h), name, h)
			}
		}
		for h, v := range s.HeadersEnv {
			if !isEnvName(v) {
				return fmt.Errorf("%s: mcp.servers.%s.headersEnv.%s must name an environment variable", at("headersEnv."+h), name, h)
			}
		}
		for k, v := range s.Env {
			if looksSecret(v) {
				return fmt.Errorf("%s: mcp.servers.%s.env.%s carries a credential — list the variable under envAllow instead", at("env."+k), name, k)
			}
		}
		switch s.Sandbox {
		case "", "inherit", "bwrap", "none":
		default:
			return fmt.Errorf("%s: mcp.servers.%s.sandbox: %q is not inherit, bwrap or none", at("sandbox"), name, s.Sandbox)
		}
		for tn, t := range s.Tools {
			if t.Class == "" {
				continue
			}
			if _, ok := classOrder[t.Class]; !ok {
				return fmt.Errorf("%s: mcp.servers.%s.tools.%s.class: %q is not a class", at("tools."+tn+".class"), name, tn, t.Class)
			}
			if !s.Inward && classOrder[t.Class] < classOrder["outward"] {
				return fmt.Errorf("%s: mcp.servers.%s.tools.%s.class: %s is below outward on a server not marked inward (Principle 7)", at("tools."+tn+".class"), name, tn, t.Class)
			}
		}
	}
	return nil
}

// MCPClass is a server tool's class: outward unless the server is inward (then write at
// most); the tool's own config class tightens an outward server and is taken as written on
// an inward one — the human's claim about a mouth that never leaves the machine.
func (c *Config) MCPClass(server, tool string) string {
	s := c.MCP.Servers[server]
	if s == nil {
		return "outward"
	}
	floor := "outward"
	if s.Inward {
		floor = "write"
	}
	t := s.Tools[tool]
	if t == nil || t.Class == "" {
		return floor
	}
	if s.Inward || classOrder[t.Class] > classOrder[floor] {
		return t.Class
	}
	return floor
}
