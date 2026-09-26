package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/loop"
)

// LiveBody is one body the court can see: rank, office, model, state, elapsed, context, spend.
type LiveBody struct {
	Name    string
	Rank    string
	Office  string
	Model   string
	State   string // thinking · tool · waiting on gate · done · queued
	Started time.Time
	Ended   time.Time
	Session *loop.Session
	Depth   int
	inbox   []string
	report  string
	done    chan struct{}
}

// Court is the live court (binary.md §Live session): every body that runs or ran in this
// session, its state, its inbox for /send, and the reports of asynchronous Courts waiting to
// wake their dispatcher.
type Court struct {
	app    *App
	mu     sync.Mutex
	bodies map[string]*LiveBody
	order  []string
	wake   []string // reports of finished background Courts, for the session's next turn
	onWake func()
}

func newCourt(a *App) *Court {
	c := &Court{app: a, bodies: map[string]*LiveBody{}}
	prev := a.OnState
	a.OnState = func(body, state string) {
		c.setState(body, state)
		if prev != nil {
			prev(body, state)
		}
	}
	return c
}

// Track registers a body's session as it starts.
func (c *Court) Track(name, rank, office, model string, s *loop.Session, depth int) *LiveBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.bodies[name]
	if b == nil {
		b = &LiveBody{Name: name, done: make(chan struct{})}
		c.bodies[name] = b
		c.order = append(c.order, name)
	}
	b.Rank, b.Office, b.Model, b.Session, b.Depth = rank, office, model, s, depth
	b.Started, b.Ended, b.State = time.Now(), time.Time{}, "queued"
	if b.done == nil {
		b.done = make(chan struct{})
	}
	return b
}

func (c *Court) setState(body, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.bodies[body]
	if b == nil {
		b = &LiveBody{Name: body, Started: time.Now(), done: make(chan struct{})}
		c.bodies[body] = b
		c.order = append(c.order, body)
	}
	b.State = state
	if state == "done" {
		b.Ended = time.Now()
	}
}

// observe is the loop's State seam: the body's state and its live session.
func (c *Court) observe(s *loop.Session, state string) {
	name := bodyName(s)
	c.setState(name, state)
	c.mu.Lock()
	if b := c.bodies[name]; b != nil && b.Session == nil {
		b.Session = s
	}
	c.mu.Unlock()
}

// Finish marks a body done with its report.
func (c *Court) Finish(name, report string) {
	c.mu.Lock()
	b := c.bodies[name]
	if b != nil {
		b.State, b.Ended, b.report = "done", time.Now(), report
		select {
		case <-b.done:
		default:
			close(b.done)
		}
	}
	c.mu.Unlock()
}

// Send queues a line for a running body (delivered at its next tool step).
func (c *Court) Send(name, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.bodies[strings.ToLower(name)]
	if b == nil {
		return fmt.Errorf("no body named %q in the court (%s)", name, strings.Join(c.order, ", "))
	}
	if b.State == "done" {
		return fmt.Errorf("%s is done — its context died with its task; dispatch it again", name)
	}
	b.inbox = append(b.inbox, text)
	return nil
}

// Inbox drains a body's queued lines.
func (c *Court) Inbox(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.bodies[name]
	if b == nil || len(b.inbox) == 0 {
		return nil
	}
	lines := b.inbox
	b.inbox = nil
	return lines
}

// Wake queues a finished background Court's report for the dispatcher's next turn.
func (c *Court) Wake(report string) {
	c.mu.Lock()
	c.wake = append(c.wake, report)
	f := c.onWake
	c.mu.Unlock()
	if f != nil {
		f()
	}
}

// TakeWake returns the queued reports, emptying the queue.
func (c *Court) TakeWake() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.wake
	c.wake = nil
	return out
}

// Bodies lists the court in start order.
func (c *Court) Bodies() []LiveBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []LiveBody
	for _, n := range c.order {
		out = append(out, *c.bodies[n])
	}
	return out
}

// Live counts the bodies not done.
func (c *Court) Live() int {
	n := 0
	for _, b := range c.Bodies() {
		if b.State != "done" {
			n++
		}
	}
	return n
}

// Lines renders /agents: rank, office, model, state, elapsed, context %, tokens, cost.
func (c *Court) Lines() []string {
	bodies := c.Bodies()
	if len(bodies) == 0 {
		return []string{"the court is empty"}
	}
	sort.SliceStable(bodies, func(i, j int) bool { return bodies[i].Started.Before(bodies[j].Started) })
	var out []string
	for _, b := range bodies {
		end := time.Now()
		if !b.Ended.IsZero() {
			end = b.Ended
		}
		ctx := "ctx —"
		if b.Session != nil && b.Session.Context.Available {
			ctx = fmt.Sprintf("ctx %d%%", b.Session.Context.Percent())
		}
		sp := c.app.Journal.Body(b.Name)
		cost := "unpriced"
		if sp.Calls > 0 && sp.Unpriced == 0 {
			cost = fmt.Sprintf("$%.4f", sp.USD)
		}
		out = append(out, fmt.Sprintf("%-20s %-8s %-10s %-32s %-15s %6s  %s  %6d tok  %s", b.Name, orStr(b.Rank, "rimuru"), orStr(b.Office, "—"), orStr(b.Model, "—"), b.State, end.Sub(b.Started).Round(time.Second), ctx, sp.Tokens(), cost))
	}
	return out
}

// StatusLine is the one line above the prompt.
func (c *Court) StatusLine(s *loop.Session) string {
	tot := c.app.Journal.Total()
	cost := "unpriced"
	if tot.Calls > 0 && tot.Unpriced == 0 {
		cost = fmt.Sprintf("$%.4f", tot.USD)
	}
	ctx := "ctx —"
	if s != nil && s.Context.Available {
		ctx = fmt.Sprintf("ctx %d%% (%s)", s.Context.Percent(), s.Context.Zone)
	}
	live := c.Live()
	courts := ""
	if live > 0 {
		courts = fmt.Sprintf(" · %d live", live)
	}
	return fmt.Sprintf("%s · %s · %s · %d tok · %s%s", c.app.Cfg.Dist.Name, c.app.mountModel.Ref.Model, ctx, tot.Tokens(), cost, courts)
}

// Close marks every body done.
func (c *Court) Close() {
	for _, b := range c.Bodies() {
		if b.State != "done" {
			c.Finish(b.Name, "")
		}
	}
}
