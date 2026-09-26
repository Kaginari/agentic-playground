package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/board"
	"github.com/Kaginari/agentic-playground/isekai/config"
)

// boardOptions feeds the board what files cannot know (board/README.md §Feeds): the live
// court, the effective config with origins, the off-list, the running session; the labels
// come from the lexicon so an agent-one board speaks its own words.
func (a *App) boardOptions() board.Options {
	names := board.DefaultNames()
	for race, disp := range a.Lex.Display {
		if disp != "" {
			names["rank."+strings.ReplaceAll(race, "_", "")] = disp
			names["rank."+race] = disp
		}
	}
	set := func(k, v string) {
		if v != "" {
			names[k] = v
		}
	}
	set("rank.rimuru", a.Lex.Session)
	set("rank.veldora", a.Lex.Human)
	set("court_body", a.Lex.Court)
	set("territory", strings.ToLower(strings.TrimSuffix(a.Lex.Territory, ":")))
	set("thoughts", strings.ToLower(strings.TrimPrefix(a.Lex.Thoughts, "## ")))
	set("kind.law", a.Lex.Token("law"))
	set("kind.colony", a.Lex.Token("colony"))
	set("kind.territory", a.Lex.Token("territory"))
	src := board.Sources{
		Court:  a.liveBodies,
		Config: func() any { return configTree(a.Cfg) },
		Off:    a.offList,
		Session: func() board.Session {
			return board.Session{ID: a.SessionID, Model: a.mountModel.Ref.Model, Provider: a.mountModel.Provider, Started: a.started, State: a.sessionState()}
		},
	}
	return board.Options{WorldRoot: a.Root, WorldDir: a.Cfg.Dist.WorldDir, Layout: a.World.Ranks.Layout(a.Lex), Names: names, Sources: src}
}

func (a *App) sessionState() string {
	for _, b := range a.Court.Bodies() {
		if b.Name == "rimuru" {
			return b.State
		}
	}
	return "idle"
}

// liveBodies is the court as the board draws it.
func (a *App) liveBodies() []board.Body {
	var out []board.Body
	for _, b := range a.Court.Bodies() {
		sp := a.Journal.Body(b.Name)
		body := board.Body{Name: b.Name, Rank: orStr(b.Rank, "rimuru"), Office: b.Office, Model: b.Model, State: b.State, Started: b.Started,
			Input: sp.Usage.Input, Output: sp.Usage.Output, CacheRead: sp.Usage.CacheRead, CacheWrite: sp.Usage.CacheWrite}
		if p, _, err := config.SplitModel(b.Model); err == nil {
			body.Provider = p
		}
		if b.Session != nil && b.Session.Reading().Available {
			r := b.Session.Reading()
			body.ContextTokens, body.ContextLimit = r.Tokens, r.Limit
		}
		if sp.Calls > 0 && sp.Unpriced == 0 {
			usd := sp.USD
			body.USD = &usd
		}
		out = append(out, body)
	}
	return out
}

func (a *App) offList() []board.Off {
	var out []board.Off
	for _, f := range a.Cfg.Off() {
		out = append(out, board.Off{Feature: f.Key, Origin: f.Origin.String()})
	}
	return out
}

// configTree is the effective config as a tree whose leaves are {value, origin}.
func configTree(cfg *config.Config) any {
	var root map[string]any
	if err := json.Unmarshal([]byte(cfg.Show(false)), &root); err != nil {
		return map[string]any{"error": err.Error()}
	}
	var walk func(v any, path string) any
	walk = func(v any, path string) any {
		switch x := v.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, val := range x {
				out[k] = walk(val, joinPath(path, k))
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, val := range x {
				out[i] = walk(val, fmt.Sprintf("%s[%d]", path, i))
			}
			return out
		default:
			return map[string]any{"value": v, "origin": cfg.Where(path)}
		}
	}
	return walk(root, "")
}

func joinPath(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

// startBoard lights the board when ui.board.autostart says so (a one-shot run never does).
func (a *App) startBoard(ctx context.Context) {
	if a.Opt.NoBoard || a.World == nil || !a.Cfg.UI.Board.Autostart {
		return
	}
	addr := fmt.Sprintf("127.0.0.1:%d", a.Cfg.UI.Board.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		a.hole(fmt.Sprintf("board: %v (ui.board.port %s)", err, a.Cfg.Where("ui.board.port")))
		return
	}
	addr = ln.Addr().String()
	b := board.New(a.boardOptions())
	srv := &http.Server{Handler: b, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		b.Close()
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed && ctx.Err() == nil {
			a.hole("board: " + err.Error())
		}
	}()
	a.mu.Lock()
	a.boardAddr = addr
	a.mu.Unlock()
	fmt.Fprintf(a.Opt.Err, "board: http://%s/\n", addr)
}

// cmdBoard serves the board alone, without a session, until interrupted.
func (a *App) cmdBoard(ctx context.Context, io IO) int {
	if a.World == nil {
		fmt.Fprintln(io.Err, "@S FAIL\n@? no world to draw")
		return 2
	}
	addr := fmt.Sprintf("127.0.0.1:%d", a.Cfg.UI.Board.Port)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		<-sig
		cancel()
	}()
	fmt.Fprintf(io.Err, "board: http://%s/ (Ctrl-C to stop)\n", addr)
	if err := board.Serve(ctx, addr, a.boardOptions()); err != nil && ctx.Err() == nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? board: %v\n", err)
		return 2
	}
	return 0
}

// boardLine is the board's status line.
func (a *App) boardLine() string {
	a.mu.Lock()
	addr := a.boardAddr
	a.mu.Unlock()
	if addr != "" {
		return "board: http://" + addr + "/"
	}
	if !a.Cfg.UI.Board.Autostart {
		return "board: off (ui.board.autostart " + a.Cfg.Where("ui.board.autostart") + ")"
	}
	return fmt.Sprintf("board: not running here — `%s board` serves it on 127.0.0.1:%d", a.Cfg.Dist.Name, a.Cfg.UI.Board.Port)
}
