package config

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, s string
		path   bool
		want   bool
	}{
		{"git push*", "git push origin main", false, true},
		{"git push*", "git pushx", false, true},
		{"git push*", "git pull", false, false},
		{"*", "anything; rm -rf /", false, true},
		{"git ?ush", "git push", false, true},
		{"**/*.lock", "go.lock", true, true},
		{"**/*.lock", "vendor/a/b.lock", true, true},
		{"*.lock", "vendor/go.lock", true, false},
		{"src/*", "src/a.go", true, true},
		{"src/*", "src/a/b.go", true, false},
		{"src/**", "src/a/b.go", true, true},
		{"**/.env*", ".env.local", true, true},
		{"**/.env*", "cfg/.env", true, true},
		{"https://example.com/*", "https://example.com/x/y", false, true},
		{"https://example.com/*", "https://evil.com/", false, false},
	}
	for _, c := range cases {
		if got := Match(c.pat, c.s, c.path); got != c.want {
			t.Errorf("Match(%q, %q, path=%v) = %v", c.pat, c.s, c.path, got)
		}
	}
}

func TestSpecificity(t *testing.T) {
	spec := func(m string) Specificity {
		tool, pat, _ := strings.Cut(m, ":")
		return (&PermRule{Tool: tool, Pattern: pat}).Specificity()
	}
	// exact tool beats *, more literals beat fewer, fewer wildcards beat more
	order := []string{"bash:git push origin main", "bash:git push --force*", "bash:git push*", "bash:git *", "bash:*", "*:git push*", "*:*"}
	for i := 0; i+1 < len(order); i++ {
		if !spec(order[i]).beats(spec(order[i+1])) {
			t.Errorf("%s should beat %s (%+v vs %+v)", order[i], order[i+1], spec(order[i]), spec(order[i+1]))
		}
	}
	if spec("bash:git *").beats(spec("bash:git ?")) || !spec("bash:git ?").beats(spec("bash:git *")) {
		t.Error("? is not more specific than * with equal literals")
	}
	if s := spec("bash:a**b*?"); s.Literal != 2 || s.Wildcards != 2 || s.Singles != 1 {
		t.Errorf("counting: %+v", s)
	}
}

func TestDecide(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
permissions:
  rules:
    - {match: "bash:git push*", action: allow}
    - {match: "bash:git push --force*", action: deny}
    - {match: "bash:git push --force-with-lease*", action: allow}
    - {match: "*:git push --tags*", action: deny}
    - {match: "bash:rm -rf *", action: ask}
    - {match: "edit:**/*.lock", action: deny}
    - {match: "read:**/.env*", action: ask}
    - {match: "webfetch:https://docs.example.com/*", action: allow}
    - {match: "bash:make test", action: ask}
    - {match: "bash:make test", action: allow}
`})
	c := w.load()
	cases := []struct {
		tool, input, class string
		want               Action
		rule               string
	}{
		{"bash", "git push origin main", "outward", Allow, "bash:git push*"},
		{"bash", "git push --force origin", "outward", Deny, "bash:git push --force*"},
		{"bash", "git push --force-with-lease origin", "outward", Allow, "bash:git push --force-with-lease*"},
		{"bash", "git push --tags", "outward", Allow, "bash:git push*"}, // exact tool beats * even with more literals
		{"bash", "rm -rf build", "destructive", Ask, "bash:rm -rf *"},
		{"bash", "rm -rf /", "destructive", Ask, "bash:rm -rf *"},
		{"bash", "ls", "read", Allow, ""},
		{"bash", "curl https://x", "outward", Ask, ""},
		{"bash", "make test", "read", Deny, ""}, // placeholder, checked below
		{"edit", "go.lock", "write", Deny, "edit:**/*.lock"},
		{"edit", "main.go", "write", Allow, ""},
		{"read", ".env.local", "read", Ask, "read:**/.env*"},
		{"read", "README.md", "read", Allow, ""},
		{"webfetch", "https://docs.example.com/a", "outward", Allow, "webfetch:https://docs.example.com/*"},
		{"webfetch", "https://other.example.com/", "outward", Ask, ""},
		{"dispatch", "domain-security", "write", Allow, ""},
	}
	for _, cs := range cases {
		if cs.input == "make test" {
			// equal specificity, equal literals: deny > ask > allow — ask beats allow here
			got, r := c.Decide("bash", "make test", "read")
			if got != Ask || r == nil || r.Match != "bash:make test" || r.Action != "ask" {
				t.Errorf("tie: got %s %+v", got, r)
			}
			continue
		}
		got, r := c.Decide(cs.tool, cs.input, cs.class)
		if got != cs.want {
			t.Errorf("Decide(%s, %q, %s) = %s, want %s", cs.tool, cs.input, cs.class, got, cs.want)
		}
		if cs.rule == "" && r != nil || cs.rule != "" && (r == nil || r.Match != cs.rule) {
			t.Errorf("Decide(%s, %q): rule %+v, want %q", cs.tool, cs.input, r, cs.rule)
		}
	}
	lo := c.Loosenings()
	if len(lo) != 4 {
		t.Errorf("loosenings: %v", lo)
	}
	// permissions off: only the class floor
	w.write(".agent-one/config.local.yaml", "permissions: {enabled: false}\n")
	c = w.load()
	if got, r := c.Decide("bash", "git push --force x", "outward"); got != Ask || r != nil {
		t.Errorf("permissions off: %s %+v", got, r)
	}
	// gate off from a file: everything allowed but deny
	w.write(".agent-one/config.local.yaml", "policy: {humanGate: {enabled: false}}\n")
	c = w.load()
	if got, _ := c.Decide("bash", "curl x", "outward"); got != Allow {
		t.Errorf("gate off: %s", got)
	}
	if got, _ := c.Decide("bash", "git push --force x", "outward"); got != Deny {
		t.Errorf("gate off, deny still wins: %s", got)
	}
	if off := c.Off(); len(off) != 1 || off[0].Key != "policy.humanGate.enabled" || off[0].Origin.Layer != "local" {
		t.Errorf("gate off is in the off-list: %v", off)
	}
	if lo := c.Loosenings(); len(lo) != 5 || !strings.HasPrefix(lo[0], "policy.humanGate.enabled: false") {
		t.Errorf("gate off is a loosening: %v", lo)
	}
}

func TestPermissionRefusals(t *testing.T) {
	cases := []struct{ name, yaml, want string }{
		{"bare * on bash", `permissions: {rules: [{match: "bash:*", action: allow}]}`, "auto-approve"},
		{"** on bash", `permissions: {rules: [{match: "bash:**", action: allow}]}`, "auto-approve"},
		{"* on webfetch", `permissions: {rules: [{match: "webfetch:*", action: allow}]}`, "auto-approve"},
		{"* on git", `permissions: {rules: [{match: "git:*", action: allow}]}`, "auto-approve"},
		{"*:* allow", `permissions: {rules: [{match: "*:*", action: allow}]}`, "auto-approve"},
		{"bad action", `permissions: {rules: [{match: "bash:ls", action: maybe}]}`, "not allow, ask or deny"},
		{"bad match", `permissions: {rules: [{match: "bash", action: allow}]}`, "<tool>:<glob>"},
		{"gate off via env-file", "", "honored only from a config file"},
		{"approve via env", "", "never env"},
	}
	for _, cs := range cases {
		w := newWorkspace(t, map[string]string{".agent-one/config.yaml": cs.yaml + "\n"})
		switch cs.name {
		case "gate off via env-file":
			w.env["AGENT_ONE_CONFIG_CONTENT"] = "policy: {humanGate: {enabled: false}}"
		case "approve via env":
			w.env["AGENT_ONE_CONFIG_CONTENT"] = "policy: {humanGate: {approve: [outward]}}"
		}
		err := w.loadErr()
		if err == nil || !strings.Contains(err.Error(), cs.want) {
			t.Errorf("%s: got %v, want %q", cs.name, err, cs.want)
		}
	}
	// allowed: a bare * on read, deny on anything, ask on anything, `*` allow from --approve
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `permissions: {rules: [{match: "read:*", action: allow}, {match: "*:*", action: deny}, {match: "bash:*", action: ask}]}` + "\n"})
	c := w.load(Override{Path: "policy.humanGate.approve", Value: "[outward]", Flag: "--approve"})
	if c.Policy.HumanGate.Approve[0] != "outward" || c.Where("policy.humanGate.approve[0]") != "flag:--approve" {
		t.Errorf("approve from flag: %+v", c.Policy.HumanGate)
	}
	// the {tool, pattern} spelling is folded to match
	w = newWorkspace(t, map[string]string{".agent-one/config.json": `{"permissions": {"rules": [{"tool": "bash", "pattern": "git push *", "action": "allow"}]}}`})
	c = w.load()
	if r := c.Permissions.Rules[0]; r.Match != "bash:git push *" || r.Tool != "bash" || len(c.Holes) != 0 {
		t.Errorf("tool/pattern alias: %+v holes %v", r, c.Holes)
	}
	// an unknown tool name is a hole, not an error
	w = newWorkspace(t, map[string]string{".agent-one/config.yaml": `permissions: {rules: [{match: "teleport:*", action: deny}]}` + "\n"})
	c = w.load()
	if len(c.Holes) != 1 || !strings.Contains(c.Holes[0], "names no tool") {
		t.Errorf("unknown tool: %v", c.Holes)
	}
	// approve destructive is accepted but sits in the off-list
	w = newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy: {humanGate: {approve: [destructive]}}\n"})
	c = w.load()
	if off := c.Off(); len(off) != 1 || off[0].Key != "policy.humanGate.approve[0]" {
		t.Errorf("approve destructive: %v", off)
	}
}
