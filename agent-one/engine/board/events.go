package board

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// event is one server-sent event: a name the pages subscribe to and a JSON payload.
type event struct {
	Name string
	Data string
}

type hub struct {
	mu      sync.Mutex
	clients map[chan event]struct{}
	closed  bool
}

func newHub() *hub { return &hub{clients: map[chan event]struct{}{}} }

func (h *hub) subscribe() chan event {
	ch := make(chan event, 16)
	h.mu.Lock()
	if !h.closed {
		h.clients[ch] = struct{}{}
	} else {
		close(ch)
	}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan event) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *hub) broadcast(e event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- e:
		default: // a slow reader drops an event; the next poll re-sends the state
		}
	}
}

func (h *hub) closeAll() {
	h.mu.Lock()
	h.closed = true
	for ch := range h.clients {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// events streams subagent / usage / loop / log changes; a page patches itself on each.
func (b *Board) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	fl.Flush()
	ch := b.hub.subscribe()
	defer b.hub.unsubscribe(ch)
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-b.stop:
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Name, strings.ReplaceAll(e.Data, "\n", " "))
			fl.Flush()
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// watch polls the feeds and broadcasts when their signature changes. Files are signed by
// (name, size, mtime); the subagent by its JSON. Polling keeps the binary stdlib-only.
func (b *Board) watch() {
	t := time.NewTicker(b.opt.Poll)
	defer t.Stop()
	dir := filepath.Join(b.opt.WorkspaceRoot, b.opt.WorkspaceDir)
	sigs := map[string]string{}
	first := true
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
		}
		if b.hub.count() == 0 && !first {
			continue
		}
		cur := map[string]string{
			"usage":    dirSig(filepath.Join(dir, "instruments", "usage")),
			"loop":     dirSig(filepath.Join(dir, "instruments", "loop")),
			"log":      fileSig(filepath.Join(dir, "log.md")),
			"subagent": subagentSig(b.src.Subagent),
		}
		for name, sig := range cur {
			if !first && sigs[name] != sig {
				b.hub.broadcast(event{Name: name, Data: b.eventPayload(name)})
			}
			sigs[name] = sig
		}
		first = false
	}
}

func (b *Board) eventPayload(name string) string {
	payload := map[string]any{"at": b.opt.Now().Format(time.RFC3339)}
	if name == "subagent" {
		agents := b.src.Subagent()
		if agents == nil {
			agents = []Agent{}
		}
		payload["agents"] = agents
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func dirSig(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	h := sha256.New()
	for _, e := range ents {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "%s|%d|%d\n", e.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fileSig(p string) string {
	info, err := os.Stat(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d|%d", info.Size(), info.ModTime().UnixNano())
}

func subagentSig(subagent func() []Agent) string {
	if subagent == nil {
		return ""
	}
	raw, _ := json.Marshal(subagent())
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func readName(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
