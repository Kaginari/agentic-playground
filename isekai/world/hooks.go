package world

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/instrument"
	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/memory"
	"github.com/Kaginari/agentic-playground/isekai/toolbox"
	"github.com/Kaginari/agentic-playground/isekai/wire"
)

// RecallOptions is the recall beat: memory hits and the toolbox brief for the current ask,
// anchors only.
type RecallOptions struct {
	Enabled bool
	Memory  bool
	Toolbox bool
	K       int // memory hits (0 = 5)
	Budget  int // toolbox manifest budget in tokens (0 = 1500)
}

// RecordOptions is the record beat: what a done step lands.
type RecordOptions struct {
	Enabled bool
	Unsaid  bool // `@U` lines in a tool result (a Court's report) go home: onto + shared note
	Landed  bool // a step that wrote files lands a shared note naming them
}

// HookOptions gathers every hook's switch.
type HookOptions struct {
	Recall RecallOptions
	Record RecordOptions
	Gate   GateOptions
	Prompt PromptOptions
}

// DefaultHooks is everything on, the four checks included.
func DefaultHooks() HookOptions {
	return HookOptions{
		Recall: RecallOptions{Enabled: true, Memory: true, Toolbox: true},
		Record: RecordOptions{Enabled: true, Unsaid: true},
		Gate:   GateOptions{Enabled: true, RightAuthor: true, TraitsHold: true, DutiesDone: true, DocTruthful: true, Log: true},
		Prompt: DefaultPrompt(),
	}
}

// Hooks wires the world into a loop engine: System, Perceive (session tracking for the shared
// budget), Recall, Record and EndGate. Compaction's Drain is merged in by the integrator.
func (w *World) Hooks(opt HookOptions) loop.Hooks {
	h := loop.Hooks{
		System:   w.System(opt.Prompt),
		Perceive: func(s *loop.Session) *instrument.Context { w.track(s); return nil },
		EndGate:  w.EndGate(opt.Gate),
	}
	if opt.Recall.Enabled {
		h.Recall = func(ctx context.Context, s *loop.Session, ask string) loop.Recall {
			return w.Recall(bodyOf(s), ask, opt.Recall)
		}
	}
	if opt.Record.Enabled {
		h.Record = func(ctx context.Context, s *loop.Session, st *loop.StepRecord) []string {
			return w.Record(bodyOf(s), st, opt.Record)
		}
	}
	return h
}

func bodyOf(s *loop.Session) string {
	if s.Engine.As != "" {
		return s.Engine.As
	}
	return s.Engine.Lexicon.Body
}

// MergeHooks lays over's hooks on base: where both set one, both run (over's Perceive and
// Drain readings win; System is over's; holes concatenate).
func MergeHooks(base, over loop.Hooks) loop.Hooks {
	out := base
	if over.System != nil {
		out.System = over.System
	}
	if over.Perceive != nil {
		if base.Perceive == nil {
			out.Perceive = over.Perceive
		} else {
			out.Perceive = func(s *loop.Session) *instrument.Context {
				c := base.Perceive(s)
				if o := over.Perceive(s); o != nil {
					return o
				}
				return c
			}
		}
	}
	if over.Recall != nil {
		if base.Recall == nil {
			out.Recall = over.Recall
		} else {
			out.Recall = func(ctx context.Context, s *loop.Session, ask string) loop.Recall {
				a, b := base.Recall(ctx, s, ask), over.Recall(ctx, s, ask)
				return loop.Recall{Anchors: append(a.Anchors, b.Anchors...), Tools: append(a.Tools, b.Tools...), Holes: append(a.Holes, b.Holes...), Rebuilt: a.Rebuilt || b.Rebuilt}
			}
		}
	}
	if over.Record != nil {
		if base.Record == nil {
			out.Record = over.Record
		} else {
			out.Record = func(ctx context.Context, s *loop.Session, st *loop.StepRecord) []string {
				return append(base.Record(ctx, s, st), over.Record(ctx, s, st)...)
			}
		}
	}
	if over.EndGate != nil {
		if base.EndGate == nil {
			out.EndGate = over.EndGate
		} else {
			out.EndGate = func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
				v, h, err := base.EndGate(ctx, s, r)
				v2, h2, err2 := over.EndGate(ctx, s, r)
				if err == nil {
					err = err2
				}
				if v2 != "" {
					v = strings.TrimSpace(v + " · " + v2)
				}
				return v, append(h, h2...), err
			}
		}
	}
	if over.Drain != nil {
		out.Drain = over.Drain
	}
	return out
}

func (w *World) track(s *loop.Session) {
	w.mu.Lock()
	w.sessions[s.Engine] = s
	w.mu.Unlock()
}

// SessionOf is the live session of an engine the hooks have seen, or nil.
func (w *World) SessionOf(e *loop.Engine) *loop.Session {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sessions[e]
}

// Memory opens the memory instrument on this world (the JS port reads `.isekai/` by name).
func (w *World) Memory() (*memory.World, error) { return memory.Open(w.Root) }

// Toolbox opens the toolbox instrument on this world.
func (w *World) Toolbox() (*toolbox.World, error) { return toolbox.Open(w.Root) }

// Recall runs the recall beat: memory hits as `src#sec` anchors, toolbox @T lines. Never a body.
func (w *World) Recall(as, ask string, opt RecallOptions) loop.Recall {
	var rc loop.Recall
	if !opt.Enabled || strings.TrimSpace(ask) == "" {
		return rc
	}
	if opt.Memory {
		m, err := w.Memory()
		if err != nil {
			rc.Holes = append(rc.Holes, "memory: "+err.Error())
		} else {
			if _, why := m.LoadIndex(); why != "" {
				if _, err := m.Index(); err == nil {
					rc.Rebuilt = true
				}
			}
			res, err := m.Recall(ask, memory.RecallOpts{As: as, K: opt.K})
			if err != nil {
				rc.Holes = append(rc.Holes, "memory: "+err.Error())
			} else {
				for _, r := range res.Results {
					a := r.Src
					if r.Sec > 0 {
						a += fmt.Sprintf("#%d", r.Sec)
					}
					rc.Anchors = append(rc.Anchors, a)
				}
				for _, h := range res.Holes {
					if !strings.HasPrefix(h, "no shared notes yet") {
						rc.Holes = append(rc.Holes, "memory: "+h)
					}
				}
			}
		}
	}
	if opt.Toolbox {
		t, err := w.Toolbox()
		if err != nil {
			rc.Holes = append(rc.Holes, "toolbox: "+err.Error())
		} else {
			if L := t.LoadRegistry(); L.Live || t.IsStale(L) {
				if _, err := t.Index(); err == nil {
					rc.Rebuilt = true
				}
			}
			b, err := t.Brief(ask, toolbox.PickOpts{As: as, Budget: opt.Budget})
			if err != nil {
				rc.Holes = append(rc.Holes, "toolbox: "+err.Error())
			} else {
				rc.Tools = append(rc.Tools, b.Lines...)
				for _, h := range b.Holes {
					if strings.HasPrefix(h, "registry older") || strings.HasPrefix(h, "no registry") {
						continue
					}
					rc.Holes = append(rc.Holes, "toolbox: "+h)
				}
			}
		}
	}
	return rc
}

// Record runs the record beat on a done step: `@U` lines in the tool's output go home, and a
// step that landed files may leave a shared note.
func (w *World) Record(as string, st *loop.StepRecord, opt RecordOptions) []string {
	var holes []string
	if !opt.Enabled {
		return nil
	}
	if opt.Unsaid && strings.Contains(st.Result.Output, "@U") {
		rep, ok := wire.ParseReport(st.Result.Output)
		if ok && len(rep.Unsaid) > 0 {
			by := as
			if st.Tool == "dispatch" {
				if b := dispatchedBody(st); b != "" {
					by = b
				}
			}
			holes = append(holes, w.Land(by, rep)...)
		}
	}
	if opt.Landed && len(st.Wrote) > 0 {
		if err := w.Remember(as, "", fmt.Sprintf("%s landed %s (%s %s)", as, strings.Join(st.Wrote, " "), st.Tool, st.ID), "landed"); err != nil {
			holes = append(holes, "remember: "+err.Error())
		}
	}
	return holes
}

func dispatchedBody(st *loop.StepRecord) string {
	var a struct{ Body string }
	if err := jsonUnmarshal(st.Input, &a); err == nil {
		return a.Body
	}
	return ""
}

// Land sends a report's `@U` lines home: an ontology Fact on the reporting body and a shared
// note of that kind. Kind tokens of either vocabulary are accepted; disk keeps the canonical.
func (w *World) Land(by string, rep wire.Report) []string {
	var holes []string
	for _, u := range rep.Unsaid {
		kind, text := u.Kind, u.Text
		if kind == "" {
			f := strings.Fields(text)
			if len(f) > 1 {
				if k, ok := w.Lex.Canonical(f[0]); ok {
					kind, text = k, strings.TrimSpace(strings.TrimPrefix(text, f[0]))
				}
			}
		}
		if kind == "" {
			holes = append(holes, "@U without a kind not landed: "+oneLine(text))
			continue
		}
		if w.Onto != nil {
			if _, err := w.Onto.AssertUnsaid(by, kind, text); err != nil {
				if _, err2 := w.Onto.AssertUnsaid("rimuru", kind, text); err2 != nil {
					holes = append(holes, "onto: "+err.Error())
				}
			}
		}
		if err := w.Remember(by, kind, text, ""); err != nil {
			holes = append(holes, "remember: "+err.Error())
		}
	}
	return holes
}

// Remember appends a shared note (kind "" for an untyped one).
func (w *World) Remember(as, kind, text, tag string) error {
	m, err := w.Memory()
	if err != nil {
		return err
	}
	_, err = m.Remember(text, memory.RememberOpts{As: as, Kind: kind, Tag: tag})
	return err
}

func jsonUnmarshal(b []byte, v interface{}) error { return json.Unmarshal(b, v) }
