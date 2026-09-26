// Package instrument is perception: readings, never feelings. The context window occupancy
// comes from the provider's own usage report, against a conservative, model-independent
// budget (200k, stress past 180k — context-check.sh's numbers).
package instrument

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/provider"
)

const (
	DefaultBudget = 200000
	DefaultStress = 180000
)

// Zone names where a reading sits.
type Zone string

const (
	OK     Zone = "OK"
	NEAR   Zone = "NEAR"   // past 75% of the stress line
	STRESS Zone = "STRESS" // past the stress line: write, dispatch, stop
	UNREAD Zone = "UNREAD" // no reading yet — a finding, not a zero
)

// Context is one occupancy reading.
type Context struct {
	Available bool   `json:"available"`
	Tokens    int    `json:"tokens"`
	Limit     int    `json:"limit"`
	Stress    int    `json:"stress"`
	Zone      Zone   `json:"zone"`
	Why       string `json:"why,omitempty"`
}

// Budget is the two lines a reading is judged against. Zero fields take the defaults; a
// Limit below zero disables the reading (a distribution that measures elsewhere).
type Budget struct {
	Limit  int
	Stress int
}

func (b Budget) norm() Budget {
	if b.Limit == 0 {
		b.Limit = DefaultBudget
	}
	if b.Stress == 0 {
		b.Stress = DefaultStress
	}
	return b
}

// Reading judges a usage report against the budget.
func (b Budget) Reading(u provider.Usage) Context {
	b = b.norm()
	c := Context{Available: true, Tokens: u.Context(), Limit: b.Limit, Stress: b.Stress}
	switch {
	case c.Tokens >= c.Stress:
		c.Zone = STRESS
	case float64(c.Tokens) >= float64(c.Stress)*0.75:
		c.Zone = NEAR
	default:
		c.Zone = OK
	}
	return c
}

// Unread is the reading before any provider call answered.
func (b Budget) Unread(why string) Context {
	b = b.norm()
	return Context{Available: false, Limit: b.Limit, Stress: b.Stress, Zone: UNREAD, Why: why}
}

// Stressed reports whether the reading is in the stress zone.
func (c Context) Stressed() bool { return c.Available && c.Tokens >= c.Stress }

// Percent of the budget in use.
func (c Context) Percent() int {
	if c.Limit == 0 {
		return 0
	}
	return 100 * c.Tokens / c.Limit
}

// String is the human-facing line, context-check.sh's wording.
func (c Context) String() string {
	if !c.Available {
		return "context: unmeasured — " + c.Why
	}
	s := fmt.Sprintf("context %s / %s tokens (%d%%)", group(c.Tokens), group(c.Limit), c.Percent())
	switch c.Zone {
	case STRESS:
		return s + " — STRESS ZONE: write what is not durable, dispatch the rest, stop here"
	case NEAR:
		return s + " — approaching the stress zone"
	}
	return s + " — within budget"
}

// Status is the status line: the readings a session shows on every turn.
type Status struct {
	Provider string
	Body     string
	Turns    int
	Steps    int
	Elapsed  time.Duration
	Spend    provider.Usage
	Context  Context
	Journal  string
}

// Line renders the status board on one line, terse but human.
func (s Status) Line() string {
	parts := []string{}
	if s.Body != "" {
		parts = append(parts, s.Body)
	}
	if s.Provider != "" {
		parts = append(parts, s.Provider)
	}
	parts = append(parts, fmt.Sprintf("turns %d", s.Turns), fmt.Sprintf("steps %d", s.Steps))
	if s.Elapsed > 0 {
		parts = append(parts, s.Elapsed.Round(time.Second).String())
	}
	if s.Spend != (provider.Usage{}) {
		parts = append(parts, fmt.Sprintf("spend in %s out %s", group(s.Spend.Input+s.Spend.CacheRead+s.Spend.CacheWrite), group(s.Spend.Output)))
	}
	parts = append(parts, s.Context.String())
	if s.Journal != "" {
		parts = append(parts, "journal "+s.Journal)
	}
	return strings.Join(parts, " · ")
}

func group(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
