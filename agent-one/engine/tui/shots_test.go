package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestShots writes the screens as ANSI files when TUI_SHOTS names a directory, for a human (or
// charm's freeze) to look at in colour: the intro's frames, the welcome, a busy turn, the board.
func TestShots(t *testing.T) {
	dir := os.Getenv("TUI_SHOTS")
	if dir == "" {
		t.Skip("TUI_SHOTS unset")
	}
	shot := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(s+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := &fakeHost{}
	th := NewTheme(true)
	th.Color = true // a test's stdout is no terminal; the shots show what one draws
	m := New(h, th, DefaultWords())
	m.SetIntro(true)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	var frames []string
	for i := 0; i < 90 && m.intro != nil; i++ {
		if i%6 == 0 {
			frames = append(frames, m.introView())
		}
		if !m.stepIntro() {
			break
		}
		time.Sleep(time.Second / introFPS)
	}
	shot("intro", strings.Join(frames, "\n"+strings.Repeat("─", 30)+"\n"))
	shot("welcome", m.theme.Welcome(h.Welcome(), 100))
	before := "package loop\n\nfunc (s *Session) drive() {\n\t// the gate at the turn's end\n\treturn s.finish(ctx, r, resp, end)\n}\n"
	after := "package loop\n\nfunc (s *Session) drive() {\n\t// the gate at the turn's end\n\tres, err := s.finish(ctx, r, resp, end)\n\tif err == errGateRetry {\n\t\tcontinue // sent back once\n\t}\n\treturn res, err\n}\n"
	d := DiffText(before, after, 2)
	d.Path = "loop/loop.go"
	shot("tools", strings.Join([]string{
		m.theme.Tool(ToolView{Name: "bash", Summary: "go test ./...", Class: "read", Status: "done", Ms: 2840, Output: "ok  agent-one/loop   0.05s\nok  agent-one/workspace  0.21s\nok  agent-one/tui    2.87s"}, 100),
		m.theme.Tool(ToolView{Name: "edit", Summary: "loop/loop.go", Class: "write", Status: "done", Ms: 3, Diff: &d, Link: "file:///w/loop/loop.go"}, 100),
		m.theme.Tool(ToolView{Name: "bash", Summary: "git push origin main", Class: "outward", Status: "denied", Why: "not now"}, 100),
	}, "\n\n"))
	m.intro = nil
	m.Update(EvTurnStart{Text: "explain the gate"})
	m.busy, m.verb, m.turnStart = true, "Thinking…", time.Now().Add(-12*time.Second)
	var busy []string
	for i := 0; i < 16; i += 4 {
		m.shimmer = i
		busy = append(busy, m.Render())
	}
	shot("busy", strings.Join(busy, "\n\n"))
	m.busy = false
	// the cast: thinking as it arrives, the gate at work, the agent view, every rank's face
	m.Update(EvTurnStart{Text: "shorten the token TTL"})
	for len(m.prints) > 0 {
		<-m.prints
	}
	m.Update(EvStream{Agent: m.words.Session(), Kind: "thinking", Text: "The TTL lives in src/auth/token.go and the zone owns that zone. Its doc says one hour; the ask says fifteen minutes. I should dispatch the zone rather than write it myself."})
	shot("thinking", m.Render())
	m.think.Reset()
	m.Update(EvState{Agent: m.words.Session(), State: "gating"})
	shot("gating", m.Render())
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Ask: "@ASK draft\nshorten the TTL to 15 minutes", State: "tool"}})
	m.Update(EvStream{Agent: "zone-auth", Kind: "thinking", Text: "The constant is in token.go; the doc must change in the same turn."})
	m.Update(EvBodyStep{Agent: "zone-auth", Tool: ToolView{ID: "s1", Name: "read", Summary: "src/auth/token.go", Class: "read", Status: "done", Ms: 2, Output: "package auth\n\nconst ttl = time.Hour"}, End: true})
	m.Update(EvBodyStep{Agent: "zone-auth", Tool: ToolView{ID: "s2", Name: "edit", Summary: "src/auth/token.go", Class: "write", Status: "running"}})
	m.Update(EvState{Agent: m.words.Session(), State: "thinking"})
	shot("subagent-live", m.Render())
	m.openBodies()
	shot("agents", m.Render())
	m.bview = nil
	var faces []string
	for _, r := range []string{m.words.Session(), "zone", "domain", "coord", "service", "auditor"} {
		ic := m.theme.Icon(m.words.Dist, r, 0)
		faces = append(faces, ic[0]+"  "+r+"\n"+ic[1])
	}
	shot("faces", strings.Join(faces, "\n\n"))
	m.busy = false
	m.subagents = map[string]*SubagentView{}
	m.openPalette()
	m.palette.query = "se"
	m.Toast("✓ zone-auth done", m.theme.pill("ok"))
	for i := 0; i < 120 && m.stepToasts(); i++ {
	}
	shot("overlay", m.Render())
	m.palette, m.toasts = nil, nil
	m.Update(evBoard{h.Board("24h")})
	for i, p := range []string{"agents", "graph", "roles", "usage"} {
		m.openBoard()
		m.Update(evBoard{h.Board("24h")})
		m.board.page = i
		shot("board-"+p, m.Render())
		m.board = nil
	}
}
