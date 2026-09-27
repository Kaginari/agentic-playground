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
		m.theme.Tool(ToolView{Name: "bash", Summary: "go test ./...", Class: "read", Status: "done", Ms: 2840, Output: "ok  isekai/loop   0.05s\nok  isekai/world  0.21s\nok  isekai/tui    2.87s"}, 100),
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
	m.Update(evBoard{h.Board("24h")})
	for i, p := range []string{"agents", "graph", "offices", "usage"} {
		m.openBoard()
		m.Update(evBoard{h.Board("24h")})
		m.board.page = i
		shot("board-"+p, m.Render())
		m.board = nil
	}
}
