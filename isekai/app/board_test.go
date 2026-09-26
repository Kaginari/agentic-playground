package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kaginari/agentic-playground/isekai/board"
	"github.com/Kaginari/agentic-playground/isekai/loop"
)

// TestBoardJunction: the board (rung 5) draws the live court, the config with origins, the
// off-list and the usage the engine journaled, under the isekai and agent-one words.
func TestBoardJunction(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.script(".isekai/tmp/session.json", when("go", call("dispatch", map[string]string{"body": "slime-auth", "ask": "findings: look"})), text("@S DONE\n@E 12"))
	w.script(".isekai/tmp/court.json", when("@ASK findings", text("@S PASS\n@U colony seen on the board\n@E 40")))
	w.write(".isekai/config.yaml", `
models: {default: mock/m, offices: {great-sage: court/c}}
providers:
  mock: {enabled: true, script: .isekai/tmp/session.json}
  court: {type: mock, script: .isekai/tmp/court.json}
tools: {bash: {sandbox: none}}
toolbox: {enabled: false}
ui: {board: {autostart: true, port: 0}}
`)
	a := w.open()
	e, _ := a.Engine()
	if r, err := e.Run(context.Background(), "go"); err != nil || r.Status != loop.Done {
		t.Fatalf("run: %v %+v", err, r)
	}
	b := board.New(a.boardOptions())
	defer b.Close()
	get := func(path string) string {
		rec := httptest.NewRecorder()
		b.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: %d\n%s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	if html := get("/court"); !strings.Contains(html, "slime-auth") || !strings.Contains(html, "great-sage") || !strings.Contains(html, "court/c") {
		t.Errorf("court page lacks the live body:\n%s", clip(html))
	}
	if html := get("/config"); !strings.Contains(html, "config.yaml") || !strings.Contains(html, "toolbox.enabled") {
		t.Errorf("config page lacks origins or the off-list:\n%s", clip(html))
	}
	if html := get("/usage"); !strings.Contains(html, "slime-auth") {
		t.Errorf("usage page lacks the journal:\n%s", clip(html))
	}
	if html := get("/"); strings.Contains(html, "<no value>") {
		t.Error("overview printed <no value>")
	}
	// autostart on an ephemeral port serves it
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Opt.NoBoard = false
	a.startBoard(ctx)
	if a.boardAddr == "" {
		t.Fatalf("board did not start: %v", a.Holes())
	}
	res, err := http.Get("http://" + a.boardAddr + "/court")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("board serving: %v %v", err, res)
	}
	res.Body.Close()
	if !strings.Contains(a.boardLine(), a.boardAddr) {
		t.Errorf("board line: %s", a.boardLine())
	}
	// the agent-one words reach the labels
	az := newTestWorld(t, "", map[string]string{".agent-one/AGENT-ONE.md": "# law\n\n## Core principles\n\n1. one\n", ".agent-one/log.md": "# log\n"})
	az.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true}}\ntools: {bash: {sandbox: none}}\nui: {board: {autostart: false}}\n")
	a2 := az.open()
	if n := a2.boardOptions().Names; n["rank.slime"] != "Zone worker" || n["court_body"] != "Ephemeral subagent" || n["kind.colony"] != "team" {
		t.Errorf("agent-one names: slime=%q court=%q colony=%q", n["rank.slime"], n["court_body"], n["kind.colony"])
	}
}

func clip(s string) string {
	if len(s) > 1500 {
		return s[:1500] + "…"
	}
	return s
}
