package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/instrument"
	"github.com/Kaginari/agent-one/loop"
)

// LiveAgent is one agent the subagent can see: rank, role, model, state, elapsed, context, spend.
type LiveAgent struct {
	Name    string
	Rank    string
	Role    string
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

// Subagent is the live subagent (binary.md §Live session): every agent that runs or ran in this
// session, its state, its inbox for /send, and the reports of asynchronous Subagents waiting to
// wake their dispatcher.
type Subagent struct {
	app    *App
	mu     sync.Mutex
	agents map[string]*LiveAgent
	order  []string
	wake   []string // reports of finished background Subagents, for the session's next turn
	onWake func()
}

func newSubagent(a *App) *Subagent {
	c := &Subagent{app: a, agents: map[string]*LiveAgent{}}
	prev := a.OnState
	a.OnState = func(agent, state string) {
		c.setState(agent, state)
		if prev != nil {
			prev(agent, state)
		}
	}
	return c
}

// Track registers an agent's session as it starts.
func (c *Subagent) Track(name, rank, role, model string, s *loop.Session, depth int) *LiveAgent {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[name]
	if b == nil {
		b = &LiveAgent{Name: name, done: make(chan struct{})}
		c.agents[name] = b
		c.order = append(c.order, name)
	}
	b.Rank, b.Role, b.Model, b.Session, b.Depth = rank, role, model, s, depth
	b.Started, b.Ended, b.State = time.Now(), time.Time{}, "queued"
	if b.done == nil {
		b.done = make(chan struct{})
	}
	return b
}

func (c *Subagent) setState(agent, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[agent]
	if b == nil {
		b = &LiveAgent{Name: agent, Started: time.Now(), done: make(chan struct{})}
		c.agents[agent] = b
		c.order = append(c.order, agent)
	}
	b.State = state
	if state == "done" {
		b.Ended = time.Now()
	}
}

// observe is the loop's State seam: the agent's state and its live session.
func (c *Subagent) observe(s *loop.Session, state string) {
	name := agentName(s)
	c.setState(name, state)
	c.mu.Lock()
	if b := c.agents[name]; b != nil && b.Session == nil {
		b.Session = s
	}
	c.mu.Unlock()
}

// Finish marks an agent done with its report.
func (c *Subagent) Finish(name, report string) {
	c.mu.Lock()
	b := c.agents[name]
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

// Send queues a line for a running agent (delivered at its next tool step).
func (c *Subagent) Send(name, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[strings.ToLower(name)]
	if b == nil {
		return fmt.Errorf("no agent named %q in the subagent (%s)", name, strings.Join(c.order, ", "))
	}
	if b.State == "done" {
		return fmt.Errorf("%s is done — its context died with its task; dispatch it again", name)
	}
	b.inbox = append(b.inbox, text)
	return nil
}

// Inbox drains an agent's queued lines.
func (c *Subagent) Inbox(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[name]
	if b == nil || len(b.inbox) == 0 {
		return nil
	}
	lines := b.inbox
	b.inbox = nil
	return lines
}

// Wake queues a finished background Subagent's report for the dispatcher's next turn.
func (c *Subagent) Wake(report string) {
	c.mu.Lock()
	c.wake = append(c.wake, report)
	f := c.onWake
	c.mu.Unlock()
	if f != nil {
		f()
	}
}

// TakeWake returns the queued reports, emptying the queue.
func (c *Subagent) TakeWake() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.wake
	c.wake = nil
	return out
}

// Agents lists the subagent in start order.
func (c *Subagent) Agents() []LiveAgent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []LiveAgent
	for _, n := range c.order {
		out = append(out, *c.agents[n])
	}
	return out
}

// Live counts the agents not done.
func (c *Subagent) Live() int {
	n := 0
	for _, b := range c.Agents() {
		if b.State != "done" && b.Depth > 0 { // the session itself is not a dispatched agent
			n++
		}
	}
	return n
}

// Lines renders /agents: rank, role, model, state, elapsed, context %, tokens, cost.
func (c *Subagent) Lines() []string {
	agents := c.Agents()
	if len(agents) == 0 {
		return []string{"the subagent is empty"}
	}
	sort.SliceStable(agents, func(i, j int) bool { return agents[i].Started.Before(agents[j].Started) })
	var out []string
	for _, b := range agents {
		end := time.Now()
		if !b.Ended.IsZero() {
			end = b.Ended
		}
		ctx := "ctx —"
		if b.Session != nil && b.Session.Reading().Available {
			ctx = fmt.Sprintf("ctx %d%%", b.Session.Reading().Percent())
		}
		sp := c.app.Journal.Agent(b.Name)
		cost := "unpriced"
		if sp.Calls > 0 && sp.Unpriced == 0 {
			cost = fmt.Sprintf("$%.4f", sp.USD)
		}
		out = append(out, fmt.Sprintf("%-20s %-8s %-10s %-32s %-15s %6s  %s  %6d tok  %s", b.Name, orStr(b.Rank, "orchestrator"), orStr(b.Role, "—"), orStr(b.Model, "—"), b.State, end.Sub(b.Started).Round(time.Second), ctx, sp.Tokens(), cost))
	}
	return out
}

// StatusLine is the one line above the prompt.
func (c *Subagent) StatusLine(s *loop.Session) string {
	tot := c.app.Journal.Total()
	cost := "unpriced"
	switch {
	case tot.Calls == 0:
		cost = "no calls yet"
	case tot.Unpriced == 0:
		cost = fmt.Sprintf("$%.4f", tot.USD)
	}
	ctx := "ctx —"
	if r := readingOf(s); r.Available {
		ctx = fmt.Sprintf("ctx %d%% (%s)", r.Percent(), r.Zone)
	}
	live := c.Live()
	subagents := ""
	if live > 0 {
		subagents = fmt.Sprintf(" · %d live", live)
	}
	return fmt.Sprintf("%s · %s · %s · %d tok · %s%s", c.app.Cfg.Dist.Name, c.app.mountModel.Ref.Model, ctx, tot.Tokens(), cost, subagents)
}

// Close marks every agent done.
func (c *Subagent) Close() {
	for _, b := range c.Agents() {
		if b.State != "done" {
			c.Finish(b.Name, "")
		}
	}
}

func readingOf(s *loop.Session) instrument.Context {
	if s == nil {
		return instrument.Context{}
	}
	return s.Reading()
}
