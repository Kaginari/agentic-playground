package app

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/provider"
)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SessionEvent is one line of a session file (config.md §Session file): user | assistant |
// tool | gate | hook | compaction | verdict, with the usage on every assistant line so the
// context instrument can be rebuilt from the file alone.
type SessionEvent struct {
	T       string                 `json:"t"`
	Kind    string                 `json:"kind"`
	Body    string                 `json:"body,omitempty"`
	Text    string                 `json:"text,omitempty"`
	Message *provider.Message      `json:"message,omitempty"`
	Usage   *provider.Usage        `json:"usage,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// SessionStore keeps one JSONL per session under <dir>/<world-hash>/<id>.jsonl. A session is
// never rewritten: compaction appends a compaction event; the drained lines stay.
type SessionStore struct {
	Dir  string
	Root string
	mu   sync.Mutex
	seen map[string]int // messages persisted per run
}

// NewSessionStore names the directory for a world.
func NewSessionStore(dir, root string) *SessionStore {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "isekai-sessions")
	}
	h := sha256.Sum256([]byte(root))
	return &SessionStore{Dir: filepath.Join(dir, hex.EncodeToString(h[:6])), Root: root, seen: map[string]int{}}
}

// Path is the file of a session.
func (s *SessionStore) Path(id string) string { return filepath.Join(s.Dir, id+".jsonl") }

// Append writes one event.
func (s *SessionStore) Append(id string, e SessionEvent) error {
	if s == nil {
		return nil
	}
	if e.T == "" {
		e.T = time.Now().UTC().Format(time.RFC3339Nano)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

// Sync persists the messages of a loop session that are not yet on file (called after each
// turn and before a drain): user lines, assistant lines with usage, tool results.
func (s *SessionStore) Sync(id string, ls *loop.Session, model string) {
	if s == nil || ls == nil {
		return
	}
	s.mu.Lock()
	n := s.seen[id]
	s.mu.Unlock()
	if n > len(ls.Messages) {
		n = 0 // the context was rebuilt (a drain): the new head is a new line
	}
	for i := n; i < len(ls.Messages); i++ {
		m := ls.Messages[i]
		ev := SessionEvent{Body: bodyName(ls), Message: &m}
		switch {
		case m.Role == provider.Assistant:
			ev.Kind = "assistant"
			if i == len(ls.Messages)-1 || i == len(ls.Messages)-2 {
				u := ls.Last
				ev.Usage = &u
			}
			ev.Model = model
		case len(m.ToolResults) > 0:
			ev.Kind = "tool"
		default:
			ev.Kind = "user"
		}
		_ = s.Append(id, ev)
	}
	s.mu.Lock()
	s.seen[id] = len(ls.Messages)
	s.mu.Unlock()
}

// Read loads a session's events.
func (s *SessionStore) Read(id string) ([]SessionEvent, error) {
	f, err := os.Open(s.Path(id))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []SessionEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var e SessionEvent
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// Messages rebuilds the conversation from a session file: the messages after the last
// compaction event (its rebuilt head), in order.
func Messages(events []SessionEvent) ([]provider.Message, string) {
	var msgs []provider.Message
	ask := ""
	for _, e := range events {
		switch e.Kind {
		case "compaction":
			if _, ok := e.Fields["after"]; ok {
				msgs = nil // the lines that follow are the rebuilt context
			}
		case "user", "assistant", "tool":
			if e.Message != nil {
				if ask == "" && e.Kind == "user" {
					ask = e.Message.Text
				}
				msgs = append(msgs, *e.Message)
			}
		}
	}
	return msgs, ask
}

// SessionInfo is one row of `sessions`.
type SessionInfo struct {
	ID      string
	At      time.Time
	Ask     string
	Turns   int
	Tokens  int
	Path    string
	Bodies  int
	LastAt  time.Time
	Verdict string
}

// List reads the directory, newest first.
func (s *SessionStore) List() ([]SessionInfo, error) {
	ents, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []SessionInfo
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		evs, err := s.Read(id)
		if err != nil {
			continue
		}
		info := SessionInfo{ID: id, Path: s.Path(id)}
		bodies := map[string]bool{}
		for _, ev := range evs {
			t, _ := time.Parse(time.RFC3339Nano, ev.T)
			if info.At.IsZero() {
				info.At = t
			}
			info.LastAt = t
			bodies[ev.Body] = true
			switch ev.Kind {
			case "user":
				if ev.Message != nil && ev.Message.Text != "" {
					info.Turns++
					if info.Ask == "" {
						info.Ask = firstLine(ev.Message.Text)
					}
				}
			case "assistant":
				if ev.Usage != nil {
					info.Tokens += ev.Usage.Input + ev.Usage.Output + ev.Usage.CacheRead + ev.Usage.CacheWrite
				}
			case "verdict":
				if v, ok := ev.Fields["verdict"].(string); ok {
					info.Verdict = v
				}
			}
		}
		info.Bodies = len(bodies)
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastAt.After(out[j].LastAt) })
	return out, nil
}

// Line renders a row for the human.
func (i SessionInfo) Line() string {
	when := "—"
	if !i.At.IsZero() {
		when = i.At.Local().Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("%-24s %s  %2d turns  %7d tok  %s", i.ID, when, i.Turns, i.Tokens, i.Ask)
}

// record appends a non-message event (gate, hook, compaction, verdict) to the session file.
func (a *App) record(s *loop.Session, kind string, fields map[string]interface{}) {
	if a.Sessions == nil {
		return
	}
	ev := SessionEvent{Kind: kind, Fields: fields}
	if s != nil {
		ev.Body = bodyName(s)
	}
	_ = a.Sessions.Append(a.SessionID, ev)
}
