// Package testserver is a tiny MCP server for the client's tests: a handful of tools with
// annotations, paginated tools/list, one resource, one prompt, and ways to fail on purpose
// (an error result, a slow call, a server that exits). It serves over stdio (main under
// mcp/testdata/server) and as an http.Handler (JSON or SSE responses, session id header).
package testserver

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const Protocol = "2025-06-18"

type msg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server holds the scripted behaviour.
type Server struct {
	PageSize int            // tools/list page size (default 2)
	Exit     func(code int) // what `die` does (default os.Exit)
	mu       sync.Mutex
	calls    map[string]int
}

func New() *Server { return &Server{PageSize: 2, Exit: os.Exit, calls: map[string]int{}} }

var tools = []map[string]interface{}{
	{"name": "echo", "description": "echo text back", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"text": map[string]string{"type": "string"}}, "required": []string{"text"}}},
	{"name": "boom", "description": "always fails", "inputSchema": map[string]interface{}{"type": "object"}},
	{"name": "slow", "description": "sleeps ms", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"ms": map[string]string{"type": "integer"}}}},
	{"name": "die", "description": "exits the server", "inputSchema": map[string]interface{}{"type": "object"}},
	{"name": "env", "description": "reads an env var", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"name": map[string]string{"type": "string"}}}},
	{"name": "wipe", "description": "destructive by annotation", "inputSchema": map[string]interface{}{"type": "object"}, "annotations": map[string]interface{}{"destructiveHint": true, "readOnlyHint": false}},
	{"name": "fetch", "description": "open workspace by annotation", "inputSchema": map[string]interface{}{"type": "object"}, "annotations": map[string]interface{}{"openWorkspaceHint": true}},
	{"name": "ro", "description": "read-only by annotation", "inputSchema": map[string]interface{}{"type": "object"}, "annotations": map[string]interface{}{"readOnlyHint": true}},
	{"name": "count", "description": "how many calls so far", "inputSchema": map[string]interface{}{"type": "object"}},
}

// Handle answers one message; nil for a notification.
func (s *Server) Handle(raw []byte) []byte {
	var m msg
	if err := json.Unmarshal(raw, &m); err != nil {
		return s.reply(nil, nil, &rpcErr{-32700, "parse error"})
	}
	if len(m.ID) == 0 {
		return nil // notification
	}
	s.mu.Lock()
	s.calls[m.Method]++
	s.mu.Unlock()
	switch m.Method {
	case "initialize":
		return s.reply(m.ID, map[string]interface{}{
			"protocolVersion": Protocol,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}, "resources": map[string]interface{}{}, "prompts": map[string]interface{}{}},
			"serverInfo":      map[string]string{"name": "testserver", "version": "0.1"},
		}, nil)
	case "ping":
		return s.reply(m.ID, map[string]interface{}{}, nil)
	case "tools/list":
		var p struct{ Cursor string }
		_ = json.Unmarshal(m.Params, &p)
		start := 0
		if p.Cursor != "" {
			start, _ = strconv.Atoi(p.Cursor)
		}
		size := s.PageSize
		if size <= 0 {
			size = len(tools)
		}
		end := start + size
		if end > len(tools) {
			end = len(tools)
		}
		res := map[string]interface{}{"tools": tools[start:end]}
		if end < len(tools) {
			res["nextCursor"] = strconv.Itoa(end)
		}
		return s.reply(m.ID, res, nil)
	case "tools/call":
		var p struct {
			Name      string
			Arguments map[string]interface{}
		}
		_ = json.Unmarshal(m.Params, &p)
		return s.reply(m.ID, s.call(p.Name, p.Arguments), nil)
	case "resources/list":
		return s.reply(m.ID, map[string]interface{}{"resources": []map[string]string{{"uri": "mem://hello", "name": "hello", "mimeType": "text/plain"}}}, nil)
	case "resources/read":
		var p struct{ URI string }
		_ = json.Unmarshal(m.Params, &p)
		if p.URI != "mem://hello" {
			return s.reply(m.ID, nil, &rpcErr{-32002, "resource not found: " + p.URI})
		}
		return s.reply(m.ID, map[string]interface{}{"contents": []map[string]string{{"uri": p.URI, "mimeType": "text/plain", "text": "hello from a resource"}}}, nil)
	case "prompts/list":
		return s.reply(m.ID, map[string]interface{}{"prompts": []map[string]interface{}{{"name": "greet", "description": "greets", "arguments": []map[string]interface{}{{"name": "name", "required": true}}}}}, nil)
	case "prompts/get":
		var p struct {
			Name      string
			Arguments map[string]string
		}
		_ = json.Unmarshal(m.Params, &p)
		if p.Name != "greet" {
			return s.reply(m.ID, nil, &rpcErr{-32602, "unknown prompt"})
		}
		return s.reply(m.ID, map[string]interface{}{"description": "a greeting", "messages": []map[string]interface{}{{"role": "user", "content": map[string]string{"type": "text", "text": "Hello, " + p.Arguments["name"] + "!"}}}}, nil)
	}
	return s.reply(m.ID, nil, &rpcErr{-32601, "method not found: " + m.Method})
}

func text(s string, isErr bool) map[string]interface{} {
	return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": s}}, "isError": isErr}
}

func (s *Server) call(name string, args map[string]interface{}) map[string]interface{} {
	switch name {
	case "echo":
		t, _ := args["text"].(string)
		return text(t, false)
	case "boom":
		return text("boom failed as designed", true)
	case "slow":
		ms, _ := args["ms"].(float64)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return text("slept", false)
	case "die":
		go func() { time.Sleep(50 * time.Millisecond); s.Exit(3) }()
		return text("dying", false)
	case "env":
		n, _ := args["name"].(string)
		v, ok := os.LookupEnv(n)
		if !ok {
			return text("(unset)", false)
		}
		return text(v, false)
	case "wipe", "fetch", "ro":
		return text(name+" ran", false)
	case "count":
		s.mu.Lock()
		n := s.calls["tools/call"]
		s.mu.Unlock()
		return text(strconv.Itoa(n), false)
	}
	return text("unknown tool "+name, true)
}

func (s *Server) reply(id json.RawMessage, result interface{}, e *rpcErr) []byte {
	if id == nil {
		id = json.RawMessage("null")
	}
	b, _ := json.Marshal(msg{JSONRPC: "2.0", ID: id, Result: result, Error: e})
	return b
}

// ServeStdio reads newline-delimited messages from in and writes replies to out.
func (s *Server) ServeStdio(in io.Reader, out io.Writer) {
	r := bufio.NewReaderSize(in, 1<<20)
	var mu sync.Mutex
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 1 {
			if resp := s.Handle(line); resp != nil {
				mu.Lock()
				out.Write(append(resp, '\n'))
				mu.Unlock()
			}
		}
		if err != nil {
			return
		}
	}
}

// Handler serves the streamable HTTP transport. sse true answers every request as an SSE
// stream (a stray notification event first, then the response); false answers plain JSON.
func (s *Server) Handler(sse bool) http.Handler {
	var mu sync.Mutex
	sessions := map[string]bool{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var m msg
		_ = json.Unmarshal(body, &m)
		sid := r.Header.Get("Mcp-Session-Id")
		mu.Lock()
		known := sessions[sid]
		mu.Unlock()
		if m.Method == "initialize" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			sid = hex.EncodeToString(b)
			mu.Lock()
			sessions[sid] = true
			mu.Unlock()
			w.Header().Set("Mcp-Session-Id", sid)
		} else if sid == "" || !known {
			if sid != "" && !known {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Mcp-Session-Id required", http.StatusBadRequest)
			return
		}
		resp := s.Handle(body)
		if resp == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if !sse {
			w.Header().Set("Content-Type", "application/json")
			w.Write(resp)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", `{"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"stray"}}`)
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", resp)
	})
}
