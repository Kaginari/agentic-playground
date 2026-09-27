package tui

import (
	"context"
	"io"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// UI is the running program: the app sends it events; Run blocks until the session ends.
type UI struct {
	Model *Model
	prog  *tea.Program
	mu    sync.Mutex
	done  bool
}

// Start builds the program on a terminal (in, out) but does not run it; events sent before
// Run are queued by Bubble Tea and delivered once the loop starts.
func Start(ctx context.Context, host Host, theme Theme, words Words, in io.Reader, out io.Writer) *UI {
	m := New(host, theme, words)
	opts := []tea.ProgramOption{tea.WithContext(ctx)}
	if in != nil {
		opts = append(opts, tea.WithInput(in))
	}
	if out != nil {
		opts = append(opts, tea.WithOutput(out))
	}
	u := &UI{Model: m}
	u.prog = tea.NewProgram(m, opts...)
	m.Attach(u.prog.Send)
	return u
}

// Run runs the program to its end.
func (u *UI) Run() error {
	_, err := u.prog.Run()
	u.mu.Lock()
	u.done = true
	u.mu.Unlock()
	u.Model.Abort()
	return err
}

// Send delivers an event to the program; after the program ended it is dropped, and a choice
// is answered as aborted so no goroutine waits forever.
func (u *UI) Send(msg tea.Msg) {
	u.mu.Lock()
	done := u.done
	u.mu.Unlock()
	if done {
		if c, ok := msg.(EvChoice); ok {
			select {
			case c.Reply <- ChoiceAnswer{Index: -1, Aborted: true}:
			default:
			}
		}
		return
	}
	u.prog.Send(msg)
}

// Quit ends the program from outside.
func (u *UI) Quit() { u.Send(EvQuit{}) }
