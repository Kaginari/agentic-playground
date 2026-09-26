package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/tool"
)

// MissingPolicy is binary.md §When a tool is missing, as the loop's Missing seam: one error
// naming why and the closest enabled alternatives; the same missing call twice in a turn stops
// the turn (doom-loop guard); a need named twice proposes the config patch that would meet it,
// shown to the human as a diff and applied only on their yes, then reloaded live.
type MissingPolicy struct {
	Cfg      *config.Config
	Disabled func() map[string]string
	// Note lands a shared note of kind team (the first naming); nil skips it.
	Note func(as, text string) error
	// Ask puts the proposal to the human; nil means nobody can answer (run without a TTY).
	Ask tool.Asker
	// Reload is called with the new config after an accepted patch (the shelf rebuilds).
	Reload func(cfg *config.Config, changes []config.Change)

	mu      sync.Mutex
	doom    map[string]*tool.DoomLoop // per run id
	namings map[string]int            // per tool name, this session
}

// Hook is the loop seam.
func (p *MissingPolicy) Hook() func(ctx context.Context, s *loop.Session, call provider.ToolCall) (string, bool) {
	return func(ctx context.Context, s *loop.Session, call provider.ToolCall) (string, bool) {
		p.mu.Lock()
		if p.doom == nil {
			p.doom = map[string]*tool.DoomLoop{}
			p.namings = map[string]int{}
		}
		d := p.doom[s.RunID]
		if d == nil {
			d = &tool.DoomLoop{Limit: p.Cfg.Tools.Missing.DoomLoopRepeats}
			p.doom[s.RunID] = d
		}
		disabled := map[string]string{}
		if p.Disabled != nil {
			disabled = p.Disabled()
		}
		msg := tool.Missing(call.Name, s.Engine.Tools, disabled)
		p.namings[call.Name]++
		n := p.namings[call.Name]
		_, stop := d.Hit(call.Name)
		if stop {
			msg = d.StopMessage(call.Name) + "\n" + msg
			d.Reset()
		}
		p.mu.Unlock()
		as := s.Engine.As
		switch {
		case n == 1:
			// the first naming is a note (Principle 4: one naming is an observation)
			if p.Note != nil {
				_ = p.Note(as, fmt.Sprintf("missing capability named once: %s (%s)", call.Name, firstLine(msg)))
			}
		case n == 2 && p.Cfg.Tools.Missing.ProposeOnSecondNaming:
			msg += "\n" + p.propose(ctx, call.Name)
		}
		return msg, stop
	}
}

// propose drafts the patch for a need named twice and puts it to the human.
func (p *MissingPolicy) propose(ctx context.Context, name string) string {
	diff, err := p.Cfg.ProposeEnable(name)
	if err != nil {
		return "named twice (Provisioning): " + err.Error() + " — the human may add one under tools.custom"
	}
	if strings.TrimSpace(diff) == "" {
		return "named twice (Provisioning): nothing to enable — " + name + " is not a switched-off tool"
	}
	if p.Ask == nil {
		return "named twice (Provisioning): proposed config patch (apply with `agent-one config patch --apply tools." + name + ".enabled=true`):\n" + diff
	}
	ans, err := p.Ask(ctx, tool.Question{Text: "the need for `" + name + "` was named twice — apply this config patch?\n" + diff, Options: []string{"yes", "no"}})
	if err != nil || !strings.EqualFold(strings.TrimSpace(ans), "yes") {
		return "named twice (Provisioning): the config patch was proposed and declined by the human"
	}
	if err := p.Cfg.ApplyPatch(p.Cfg.ProjectFile(), []config.Op{{Path: enablePath(p.Cfg, name), Value: "true"}}); err != nil {
		return "named twice (Provisioning): the human said yes but the patch failed: " + err.Error()
	}
	if !p.Cfg.Tools.Missing.LiveReload {
		return "named twice (Provisioning): config patched (" + p.Cfg.ProjectFile() + "); it applies at the next start (tools.missing.liveReload: false)"
	}
	next, changes, err := p.Cfg.Reload()
	if err != nil {
		return "named twice (Provisioning): config patched but the reload failed: " + err.Error()
	}
	p.mu.Lock()
	p.Cfg = next
	p.mu.Unlock()
	if p.Reload != nil {
		p.Reload(next, changes)
	}
	return fmt.Sprintf("named twice (Provisioning): config patched and reloaded live (%d change%s) — %s is available from the next step", len(changes), plural(len(changes)), name)
}

func enablePath(cfg *config.Config, name string) string {
	if _, ok := cfg.Tools.Custom[name]; ok {
		return "tools.custom." + name + ".enabled"
	}
	return "tools." + name + ".enabled"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
