package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	wishtea "charm.land/wish/v2/bubbletea"

	"github.com/Kaginari/agentic-playground/isekai/tui"
)

// `board --ssh [addr]` serves the terminal board over SSH (Wish): every connection gets its own
// full-screen board over the world's files. It listens on 127.0.0.1:2222 unless told otherwise,
// and admits only the keys in ~/.ssh/authorized_keys — no file, no server (it fails closed).

const defaultSSHAddr = "127.0.0.1:2222"

var stdLogWarn = log.StandardLogOptions{ForceLevel: log.WarnLevel}

// newLogger is the human-facing log of a long-running command: levelled, coloured, timed.
func newLogger(w io.Writer, prefix string) *log.Logger {
	return log.NewWithOptions(w, log.Options{Prefix: prefix, ReportTimestamp: true, TimeFormat: "15:04:05"})
}

func (a *App) serveSSH(ctx context.Context, addr string, lg *log.Logger) error {
	home := a.Opt.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	keys := filepath.Join(home, ".ssh", "authorized_keys")
	if _, err := os.Stat(keys); err != nil {
		return fmt.Errorf("no %s: the SSH board admits only the keys listed there — add yours, or serve without --ssh", tilde(keys, home))
	}
	hostKey := filepath.Join(home, ".local", "share", a.Cfg.Dist.Name, "ssh", "host_ed25519")
	if err := os.MkdirAll(filepath.Dir(hostKey), 0o700); err != nil {
		return err
	}
	if host, _, err := net.SplitHostPort(addr); err == nil && host != "127.0.0.1" && host != "localhost" && host != "::1" {
		lg.Warn("the SSH board listens beyond this machine", "addr", addr)
	}
	srv, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKey),
		wish.WithAuthorizedKeys(keys),
		wish.WithIdleTimeout(30*time.Minute),
		wish.WithMiddleware(
			wishtea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				lg.Info("ssh session", "user", s.User(), "from", s.RemoteAddr().String())
				return tui.NewBoardOnly(boardHost{a}, tui.NewTheme(true), boardWords(a)), nil
			}),
			activeterm.Middleware(),
		),
	)
	if err != nil {
		return err
	}
	lg.Info("serving over ssh", "addr", addr, "keys", tilde(keys, home))
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		return err
	}
	return nil
}

func boardWords(a *App) tui.Words {
	w := tui.DefaultWords()
	w.Dist = a.Cfg.Dist.Name
	if a.Lex.Court != "" {
		w.Court, w.Courts = a.Lex.Court, a.Lex.Court+"s"
	}
	return w
}

// boardHost is a tui.Host for the board alone: no session behind it.
type boardHost struct{ a *App }

func (h boardHost) Welcome() tui.Welcome           { return tui.Welcome{Dist: h.a.Cfg.Dist.Name} }
func (h boardHost) Footer() tui.FooterView         { return tui.FooterView{World: h.a.Root} }
func (h boardHost) Submit(string) string           { return "the SSH board has no session" }
func (h boardHost) Queue(string) string            { return "" }
func (h boardHost) Interrupt()                     {}
func (h boardHost) Slash(string) ([]string, bool)  { return nil, false }
func (h boardHost) Commands() []tui.MenuItem       { return nil }
func (h boardHost) Complete(string) []string       { return nil }
func (h boardHost) Board(rng string) tui.BoardView { return h.a.BoardView(rng) }
