package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPatchReload(t *testing.T) {
	w := newWorld(t, map[string]string{".isekai/config.yaml": "# world\nmodel: anthropic/claude-opus-5   # mount\ntools:\n  bash: {timeoutMs: 120000}\n"})
	c := w.load()
	file := c.ProjectFile()
	if file != w.path(".isekai/config.yaml") {
		t.Fatalf("project file: %s", file)
	}
	ops := []Op{
		{Path: "tools.webfetch.enabled", Value: "false"},
		{Path: "tools.custom.lsl", Value: `{class: read, params: {path: {type: string}}, run: [ls, -la, "{{path}}"]}`},
		{Path: "tools.bash.timeoutMs", Value: "30000"},
	}
	diff, err := c.Patch(file, ops)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--- .isekai/config.yaml", "+  webfetch:\n+    enabled: false", "+  custom:\n+    lsl:", "-  bash: {timeoutMs: 120000}", "+  bash:\n+    timeoutMs: 30000"} {
		if !strings.Contains(diff, want) {
			t.Errorf("diff lacks %q:\n%s", want, diff)
		}
	}
	if b, _ := os.ReadFile(file); strings.Contains(string(b), "webfetch") {
		t.Error("Patch wrote the file")
	}
	// a patch the law refuses is refused before the human sees it
	if _, err := c.Patch(file, []Op{{Path: "permissions.rules", Value: `[{match: "bash:*", action: allow}]`}}); err == nil || !strings.Contains(err.Error(), "does not load") {
		t.Errorf("refused patch: %v", err)
	}
	if _, err := c.Patch(file, []Op{{Path: "tools.bash.timeoutMs", Value: "{unterminated"}}); err == nil {
		t.Error("syntax in a value accepted")
	}
	if _, err := c.Patch(file, nil); err == nil {
		t.Error("empty patch accepted")
	}
	// apply, then reload reports what moved, with origins; comments survive
	if err := c.ApplyPatch(file, ops); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	if !strings.HasPrefix(string(b), "# world\nmodel: anthropic/claude-opus-5   # mount\n") {
		t.Errorf("comments lost:\n%s", b)
	}
	next, changes, err := c.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 7 || next.Tools.Bash.Timeout.D() != 30*time.Second || next.Tools.Webfetch.Enabled || next.Tools.Custom["lsl"] == nil {
		var s []string
		for _, ch := range changes {
			s = append(s, ch.String())
		}
		t.Errorf("reload: %d changes\n%s", len(changes), strings.Join(s, "\n"))
	}
	for _, ch := range changes {
		if ch.Origin.File != file || ch.Origin.Line == 0 {
			t.Errorf("change origin: %+v", ch)
		}
	}
	// a failed reload keeps the old config
	w.write(".isekai/config.yaml", "law: {wire: {cap: none}}\n")
	same, changes, err := next.Reload()
	if err == nil || same != next || changes != nil {
		t.Errorf("failed reload: %v %v", err, changes)
	}
	// ProposeEnable drafts the second-naming patch
	w.write(".isekai/config.yaml", "tools: {webfetch: {enabled: false}}\n")
	c = w.load()
	diff, err = c.ProposeEnable("webfetch")
	if err != nil || !strings.Contains(diff, "-tools: {webfetch: {enabled: false}}") || !strings.Contains(diff, "+    enabled: true") {
		t.Errorf("propose: %v\n%s", err, diff)
	}
	if _, err := c.ProposeEnable("teleport"); err == nil {
		t.Error("unknown tool proposed")
	}
	// delete
	diff, err = c.Patch(file, []Op{{Path: "tools.webfetch", Delete: true}})
	if err != nil || !strings.Contains(diff, "-tools: {webfetch: {enabled: false}}") || !strings.Contains(diff, "+tools: {}") {
		t.Errorf("delete: %v\n%s", err, diff)
	}
	// JSON files are re-emitted; a missing file is created
	jf := w.path(".isekai/config.local.json")
	diff, err = c.Patch(jf, []Op{{Path: "sessions.keepDays", Value: "9"}})
	if err != nil || !strings.Contains(diff, "+  \"sessions\": {\n+    \"keepDays\": 9") {
		t.Errorf("json patch: %v\n%s", err, diff)
	}
	if err := c.ApplyPatch(jf, []Op{{Path: "sessions.keepDays", Value: "9"}}); err != nil {
		t.Fatal(err)
	}
	if c2 := w.load(); c2.Sessions.KeepDays != 9 || c2.Where("sessions.keepDays") != jf+":3" {
		t.Errorf("json applied: %d %s", c2.Sessions.KeepDays, c2.Where("sessions.keepDays"))
	}
}
