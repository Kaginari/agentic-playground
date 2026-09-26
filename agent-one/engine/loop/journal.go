package loop

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Event is one journal line. The field names follow loop.js's journal so one reader serves both:
// t (run|turn|budget|recall|gate|act|verify|record|remember|end|resume|drain), at, id.
type Event map[string]interface{}

// Journal appends one JSON line per beat to <dir>/<run-id>.jsonl. A nil Journal writes nothing
// (dry runs, a distribution that journals elsewhere).
type Journal struct {
	Path string
}

func (j *Journal) Log(e Event) {
	if j == nil || j.Path == "" {
		return
	}
	if _, ok := e["at"]; !ok {
		e["at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_ = os.MkdirAll(filepath.Dir(j.Path), 0o755)
	f, err := os.OpenFile(j.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
}

// ReadJournal reads every line of a journal back; unreadable lines are skipped.
func ReadJournal(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil && e != nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// StepState is what the journal says happened to one step.
type StepState struct {
	ID         string `json:"id"`
	Tool       string `json:"tool,omitempty"`
	Class      string `json:"class,omitempty"`
	Status     string `json:"status"` // done | failed | denied | refused | interrupted mid-act | pending
	Attempts   int    `json:"attempts"`
	actStarted bool
}

// RunState is a journal read back honestly.
type RunState struct {
	ID       string      `json:"id"`
	Status   string      `json:"status"` // the end line's status, else INTERRUPTED
	As       string      `json:"as"`
	Ask      string      `json:"ask,omitempty"`
	Provider string      `json:"provider,omitempty"`
	At       string      `json:"at,omitempty"`
	Turns    int         `json:"turns"`
	Events   int         `json:"events"`
	Steps    []StepState `json:"steps"`
	Context  Event       `json:"context,omitempty"`
	Path     string      `json:"journal"`
}

func str(e Event, k string) string {
	if v, ok := e[k].(string); ok {
		return v
	}
	return ""
}

// State folds a journal's events into a RunState.
func State(path string, events []Event) RunState {
	st := RunState{Path: path, Status: "INTERRUPTED", Events: len(events)}
	idx := map[string]int{}
	step := func(id string) *StepState {
		if i, ok := idx[id]; ok {
			return &st.Steps[i]
		}
		st.Steps = append(st.Steps, StepState{ID: id, Status: "pending"})
		idx[id] = len(st.Steps) - 1
		return &st.Steps[len(st.Steps)-1]
	}
	for _, e := range events {
		id := str(e, "id")
		switch str(e, "t") {
		case "run":
			st.ID, st.As, st.Ask, st.Provider, st.At = id, str(e, "as"), str(e, "ask"), str(e, "provider"), str(e, "at")
		case "turn":
			st.Turns++
			if c, ok := e["context"].(map[string]interface{}); ok {
				st.Context = c
			}
		case "end":
			st.Status = str(e, "status")
		case "gate":
			s := step(id)
			s.Tool, s.Class = str(e, "tool"), str(e, "effective")
			if str(e, "decision") == "denied" {
				s.Status = "denied"
			}
		case "act":
			s := step(id)
			if b, _ := e["start"].(bool); b {
				s.actStarted = true
				s.Status = "interrupted mid-act"
				if n, ok := e["attempt"].(float64); ok {
					s.Attempts = int(n)
				}
			} else {
				s.actStarted = false
			}
		case "record":
			s := step(id)
			s.Status = str(e, "status")
			s.actStarted = false
			if n, ok := e["attempts"].(float64); ok {
				s.Attempts = int(n)
			}
		}
	}
	return st
}

// Runs lists every journal in dir, newest first.
func Runs(dir string) ([]RunState, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type f struct {
		name string
		mod  time.Time
	}
	var files []f
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, f{e.Name(), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	var out []RunState
	for _, x := range files {
		p := filepath.Join(dir, x.name)
		ev, _ := ReadJournal(p)
		s := State(p, ev)
		if s.ID == "" {
			s.ID = strings.TrimSuffix(x.name, ".jsonl")
		}
		out = append(out, s)
	}
	return out, nil
}
