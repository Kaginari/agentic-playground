package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) Options {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	root := filepath.Join(base, "proj", "sub")
	write(t, filepath.Join(base, "proj", "CLAUDE.md"), "parent claude\n")
	write(t, filepath.Join(root, "AGENTS.md"), "root agents\n")
	write(t, filepath.Join(root, "CLAUDE.md"), "root claude\n")
	write(t, filepath.Join(home, ".claude", "CLAUDE.md"), "global claude\n")
	write(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), "global agents\n")
	write(t, filepath.Join(root, ".claude", "skills", "tdd", "SKILL.md"), "---\nname: tdd\ndescription: \"Test-driven: red, green, refactor\"\nlicense: MIT\n---\n\n# TDD body\n\nsteps\n")
	write(t, filepath.Join(root, ".opencode", "skill", "don", "SKILL.md"), "---\nname: don\ndescription: Don a Mind\n---\nbody don\n")
	write(t, filepath.Join(root, ".opencode", "skills", "genesis", "SKILL.md"), "no frontmatter body\n")
	write(t, filepath.Join(home, ".claude", "skills", "tdd", "SKILL.md"), "---\nname: tdd\ndescription: global copy loses\n---\nx\n")
	write(t, filepath.Join(home, ".config", "opencode", "skills", "mint", "SKILL.md"), "---\nname: mint\ndescription: Mint a Body\n---\nx\n")
	write(t, filepath.Join(root, ".claude", "commands", "mint.md"), "---\ndescription: Mint a Body\nargument-hint: \"[name]\"\n---\n\nMint $ARGUMENTS now; first $1 second $2 third $3\n")
	write(t, filepath.Join(root, ".claude", "commands", "isekai", "check.md"), "check body\n")
	write(t, filepath.Join(root, ".opencode", "command", "genesis.md"), "---\ndescription: Genesis\n---\nbirth\n")
	write(t, filepath.Join(home, ".claude", "commands", "mint.md"), "global mint loses\n")
	write(t, filepath.Join(root, ".claude", "agents", "orc-security.md"), "---\nname: orc-security\ndescription: Gate for security\nmodel: sonnet\ntools: Read, Grep, Bash\n---\n\nYou are the orc of security.\n")
	write(t, filepath.Join(root, ".opencode", "agents", "rimuru.md"), "---\ndescription: The throne\nmode: all\ntools:\n  - read\n  - bash\n---\nthrone prompt\n")
	write(t, filepath.Join(root, ".mcp.json"), `{"mcpServers": {"fs": {"command": "npx", "args": ["-y", "server-fs"], "env": {"A": "1"}}, "remote": {"type": "http", "url": "https://x/mcp", "headers": {"Authorization": "Bearer t"}}}}`)
	write(t, filepath.Join(root, "opencode.jsonc"), "{\n  // a comment\n  \"$schema\": \"https://opencode.ai/config.json\", /* block */\n  \"mcp\": {\n    \"local\": {\"type\": \"local\", \"command\": [\"bun\", \"x\", \"srv\"], \"environment\": {\"B\": \"2\"}, \"enabled\": false,},\n    \"web\": {\"type\": \"remote\", \"url\": \"https://y/mcp\"},\n  },\n}\n")
	return Options{Root: root, Home: home, Instructions: true, Skills: true, Commands: true, Agents: true, MCP: true}
}

func TestInstructions(t *testing.T) {
	opt := fixture(t)
	ins := Instructions(opt)
	var got []string
	for _, i := range ins {
		got = append(got, i.Source+":"+strings.TrimSpace(i.Content))
	}
	want := "opencode:root agents claude:root claude claude:parent claude claude-global:global claude opencode-global:global agents"
	if strings.Join(got, " ") != want {
		t.Fatalf("%v", got)
	}
	off := false
	opt.Sources.Claude = &off
	if ins := Instructions(opt); len(ins) != 2 || ins[0].Source != "opencode" {
		t.Fatalf("claude off: %+v", ins)
	}
}

func TestSkills(t *testing.T) {
	opt := fixture(t)
	sk := Skills(opt)
	names := map[string]Skill{}
	for _, s := range sk {
		names[s.Name] = s
	}
	if len(sk) != 4 {
		t.Fatalf("%d %+v", len(sk), sk)
	}
	if names["tdd"].Source != "claude" || names["tdd"].Description != "Test-driven: red, green, refactor" || names["tdd"].Frontmatter["license"] != "MIT" {
		t.Fatalf("%+v", names["tdd"])
	}
	body, err := names["tdd"].Load()
	if err != nil || !strings.HasPrefix(body, "# TDD body") || strings.Contains(body, "name: tdd") {
		t.Fatalf("%q %v", body, err)
	}
	if names["genesis"].Description != "" || names["genesis"].Source != "opencode" {
		t.Fatalf("%+v", names["genesis"])
	}
	if names["mint"].Source != "opencode-global" || names["don"].Source != "opencode" {
		t.Fatalf("%+v %+v", names["mint"], names["don"])
	}
}

func TestCommands(t *testing.T) {
	opt := fixture(t)
	cs := Commands(opt)
	byName := map[string]Command{}
	for _, c := range cs {
		byName[c.Name] = c
	}
	if len(cs) != 3 {
		t.Fatalf("%+v", cs)
	}
	m := byName["mint"]
	if m.Source != "claude" || m.Description != "Mint a Body" || m.Frontmatter["argument-hint"] != "[name]" {
		t.Fatalf("%+v", m)
	}
	if got := m.Expand("slime-x --as y"); got != "Mint slime-x --as y now; first slime-x second --as third y\n" {
		t.Fatalf("%q", got)
	}
	if byName["isekai:check"].Template != "check body\n" || byName["genesis"].Source != "opencode" {
		t.Fatalf("%+v", cs)
	}
}

func TestAgents(t *testing.T) {
	opt := fixture(t)
	as := Agents(opt)
	if len(as) != 2 {
		t.Fatalf("%+v", as)
	}
	orc, rim := as[0], as[1]
	if orc.Name != "orc-security" || orc.Model != "sonnet" || orc.Mode != "subagent" || strings.Join(orc.Tools, "|") != "Read|Grep|Bash" || orc.Prompt != "You are the orc of security." {
		t.Fatalf("%+v", orc)
	}
	if rim.Name != "rimuru" || rim.Mode != "all" || strings.Join(rim.Tools, "|") != "read|bash" || rim.Description != "The throne" {
		t.Fatalf("%+v", rim)
	}
}

func TestMCP(t *testing.T) {
	opt := fixture(t)
	ss, errs := MCP(opt)
	if len(errs) != 0 || len(ss) != 4 {
		t.Fatalf("%+v %v", ss, errs)
	}
	byName := map[string]MCPServer{}
	for _, s := range ss {
		byName[s.Name] = s
	}
	fs := byName["fs"]
	if fs.Transport != "stdio" || fs.Command != "npx" || len(fs.Args) != 2 || fs.Env["A"] != "1" || !fs.Enabled || fs.Source != "claude:.mcp.json" {
		t.Fatalf("%+v", fs)
	}
	if r := byName["remote"]; r.Transport != "http" || r.URL != "https://x/mcp" || r.Headers["Authorization"] == "" {
		t.Fatalf("%+v", r)
	}
	if l := byName["local"]; l.Transport != "stdio" || l.Command != "bun" || strings.Join(l.Args, " ") != "x srv" || l.Enabled || l.Env["B"] != "2" || l.Source != "opencode:opencode.jsonc" {
		t.Fatalf("%+v", l)
	}
	if w := byName["web"]; w.Transport != "http" || w.URL != "https://y/mcp" || !w.Enabled {
		t.Fatalf("%+v", w)
	}
	write(t, filepath.Join(opt.Root, ".mcp.json"), "{not json")
	if _, errs := MCP(opt); len(errs) != 1 || !strings.Contains(errs[0].Error(), ".mcp.json") {
		t.Fatalf("malformed must be reported: %v", errs)
	}
	r := All(opt)
	if len(r.Errors) != 1 || len(r.Skills) != 4 || len(r.Instructions) != 5 {
		t.Fatalf("%+v", r)
	}
}

func TestHelpers(t *testing.T) {
	fm, body := Frontmatter("---\nname: x\ndescription: 'quoted: colon'\ntools: [a, \"b\"]\nmulti:\n  - one\n  - two\n---\nbody here\n")
	if fm["name"] != "x" || fm["description"] != "quoted: colon" || body != "body here\n" {
		t.Fatalf("%v %q", fm, body)
	}
	if l := List(fm["tools"]); strings.Join(l, "|") != "a|b" {
		t.Fatal(l)
	}
	if l := List(fm["multi"]); strings.Join(l, "|") != "one|two" {
		t.Fatal(l)
	}
	if fm, body := Frontmatter("no header\n"); len(fm) != 0 || body != "no header\n" {
		t.Fatal(fm, body)
	}
	if fm, _ := Frontmatter("---\nunterminated: yes\n"); len(fm) != 0 {
		t.Fatal("unterminated frontmatter is body")
	}
	got := string(StripJSONC([]byte("{\"a\": \"http://x // not a comment\", // c\n /* b */ \"b\": [1,2,],}")))
	if got != "{\"a\": \"http://x // not a comment\", \n  \"b\": [1,2]}" {
		t.Fatalf("%q", got)
	}
}
