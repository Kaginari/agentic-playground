package world

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry is one log.md record in the file's own format.
type Entry struct {
	At      time.Time
	Author  string
	Title   string
	Task    string
	Files   []string
	Gate    string
	Result  string
	Learned []string
}

// Render is the entry as log.md keeps it.
func (e Entry) Render() string {
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	files := "none"
	if len(e.Files) > 0 {
		files = strings.Join(e.Files, ", ")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### [%s] %s — %s\n", at.Format(time.RFC3339), or(e.Author, "rimuru"), oneLine(or(e.Title, "untitled")))
	fmt.Fprintf(&b, "- **Task:** %s\n", oneLine(or(e.Task, "—")))
	fmt.Fprintf(&b, "- **Files:** %s\n", oneLine(files))
	fmt.Fprintf(&b, "- **Gate:** %s\n", oneLine(or(e.Gate, "n/a")))
	fmt.Fprintf(&b, "- **Result:** %s\n", oneLine(or(e.Result, "done")))
	if len(e.Learned) == 0 {
		b.WriteString("- **Learned:** —\n")
	} else if len(e.Learned) == 1 {
		fmt.Fprintf(&b, "- **Learned:** %s\n", oneLine(e.Learned[0]))
	} else {
		b.WriteString("- **Learned:**\n")
		for _, l := range e.Learned {
			fmt.Fprintf(&b, "  - %s\n", oneLine(l))
		}
	}
	return b.String()
}

// Log is the append-only writer for log.md (Law 4). It remembers the bytes it last saw and
// refuses to append when that prefix changed underneath it — a rewritten past is never
// silently extended.
type Log struct {
	Path string
	mu   sync.Mutex
	seen int64
	sum  string
}

// OpenLog reads the file's current state (a missing file is an empty record).
func OpenLog(path string) (*Log, error) {
	l := &Log{Path: path}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l.seen, l.sum = int64(len(b)), digest(b)
	return l, nil
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Check reports whether the record's remembered prefix is intact.
func (l *Log) Check() error {
	b, err := os.ReadFile(l.Path)
	if err != nil {
		if os.IsNotExist(err) && l.seen == 0 {
			return nil
		}
		return err
	}
	if int64(len(b)) < l.seen {
		return fmt.Errorf("%s shrank under the writer (%d < %d bytes): the record was rewritten — refusing to append (Law 4)", filepath.Base(l.Path), len(b), l.seen)
	}
	if digest(b[:l.seen]) != l.sum {
		return fmt.Errorf("%s changed under the writer within its first %d bytes: the record was rewritten — refusing to append (Law 4)", filepath.Base(l.Path), l.seen)
	}
	return nil
}

// Append adds one entry. Past bytes are never touched; the append is one write.
func (l *Log) Append(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.Check(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	cur, _ := os.ReadFile(l.Path)
	// a fresh record (or one with no header) gets the file's own header first
	text := e.Render()
	prefix := ""
	if len(cur) == 0 {
		prefix = "# Chronicle — change log\n\nAppend-only. Newest entries at the bottom. One entry per change.\n\n---\n\n"
	} else if !strings.HasSuffix(string(cur), "\n\n") {
		if strings.HasSuffix(string(cur), "\n") {
			prefix = "\n"
		} else {
			prefix = "\n\n"
		}
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(prefix + text)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return cerr
	}
	b, err := os.ReadFile(l.Path)
	if err != nil {
		return err
	}
	l.seen, l.sum = int64(len(b)), digest(b)
	return nil
}

func or(a, b string) string {
	if strings.TrimSpace(a) == "" {
		return b
	}
	return a
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
