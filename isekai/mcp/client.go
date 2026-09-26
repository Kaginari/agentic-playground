// Package mcp is a Model Context Protocol client (JSON-RPC 2.0, stdlib only): stdio servers
// spawned under the sandbox with a scrubbed environment, and streamable-HTTP servers (POST,
// JSON or SSE answers, session id header). A server's death is detected and reported; its tools
// then report the fault instead of vanishing (binary.md §MCP).
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/sandbox"
)

// ProtocolVersion is what the client asks for.
const ProtocolVersion = "2025-06-18"

// ServerConfig is one mcp.servers entry after config decoding.
type ServerConfig struct {
	Name      string
	Transport string // "stdio" | "http" (default: stdio when Command is set, else http)
	// stdio
	Command  string
	Args     []string
	Env      map[string]string // added to the scrubbed environment
	EnvAllow []string          // names that survive scrubbing
	Cwd      string
	Network  bool             // the sandbox lets the server reach the network
	Sandbox  *sandbox.Sandbox // nil: no sandbox (env still scrubbed)
	// http
	URL        string
	Headers    map[string]string
	HTTPClient *http.Client

	Timeout time.Duration // per call (default 60 s)
}

// Info is what the server said at initialize.
type Info struct {
	Name, Version, Protocol string
	Capabilities            json.RawMessage
}

// ToolInfo is one entry of tools/list.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		Title           string `json:"title"`
		ReadOnlyHint    *bool  `json:"readOnlyHint"`
		DestructiveHint *bool  `json:"destructiveHint"`
		IdempotentHint  *bool  `json:"idempotentHint"`
		OpenWorldHint   *bool  `json:"openWorldHint"`
	} `json:"annotations"`
}

// Content is one block of a tool result or a resource.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
	Blob     string `json:"blob,omitempty"`
	Resource *struct {
		URI      string `json:"uri"`
		MimeType string `json:"mimeType"`
		Text     string `json:"text"`
	} `json:"resource,omitempty"`
}

// CallResult is tools/call's answer.
type CallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError"`
}

// Text joins the result's text blocks; other blocks are named.
func (r CallResult) Text() string {
	var b strings.Builder
	for _, c := range r.Content {
		switch c.Type {
		case "text":
			b.WriteString(c.Text)
		case "image", "audio":
			fmt.Fprintf(&b, "[%s %s, %d bytes base64]", c.Type, c.MimeType, len(c.Data))
		case "resource":
			if c.Resource != nil {
				fmt.Fprintf(&b, "[resource %s]\n%s", c.Resource.URI, c.Resource.Text)
			}
		default:
			fmt.Fprintf(&b, "[%s]", c.Type)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Resource is one entry of resources/list.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// Prompt is one entry of prompts/list.
type Prompt struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Arguments   []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Required    bool   `json:"required"`
	} `json:"arguments"`
}

// PromptResult is prompts/get's answer.
type PromptResult struct {
	Description string `json:"description"`
	Messages    []struct {
		Role    string  `json:"role"`
		Content Content `json:"content"`
	} `json:"messages"`
}

// Error is a JSON-RPC error from the server.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("rpc %d: %s", e.Code, e.Message) }

// ErrDead is wrapped by every call on a server that died.
var ErrDead = errors.New("mcp server is down")

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type transport interface {
	call(ctx context.Context, m message) (message, error)
	notify(ctx context.Context, m message) error
	close() error
}

// Client is one connected server.
type Client struct {
	cfg   ServerConfig
	tr    transport
	info  Info
	next  atomic.Int64
	fault atomic.Pointer[string]
}

// Connect starts (or reaches) the server and runs the initialize handshake.
func Connect(ctx context.Context, cfg ServerConfig) (*Client, error) {
	if cfg.Name == "" {
		return nil, errors.New("mcp: server name is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.Transport == "" {
		if cfg.Command != "" {
			cfg.Transport = "stdio"
		} else {
			cfg.Transport = "http"
		}
	}
	c := &Client{cfg: cfg}
	var err error
	switch cfg.Transport {
	case "stdio":
		c.tr, err = newStdio(ctx, cfg, c.setFault)
	case "http", "streamable-http", "sse":
		c.tr, err = newHTTP(cfg, c.setFault)
	default:
		err = fmt.Errorf("transport %q is not stdio or http", cfg.Transport)
	}
	if err != nil {
		return nil, fmt.Errorf("mcp %s: %w", cfg.Name, err)
	}
	params, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "isekai", "version": "1"},
	})
	res, err := c.call(ctx, "initialize", params)
	if err != nil {
		c.tr.close()
		return nil, fmt.Errorf("mcp %s: initialize: %w", cfg.Name, err)
	}
	var init struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"capabilities"`
		ServerInfo      struct {
			Name, Version string
		} `json:"serverInfo"`
	}
	_ = json.Unmarshal(res, &init)
	c.info = Info{Name: init.ServerInfo.Name, Version: init.ServerInfo.Version, Protocol: init.ProtocolVersion, Capabilities: init.Capabilities}
	if err := c.tr.notify(ctx, message{JSONRPC: "2.0", Method: "notifications/initialized"}); err != nil {
		c.tr.close()
		return nil, fmt.Errorf("mcp %s: initialized: %w", cfg.Name, err)
	}
	return c, nil
}

// Name is the configured server name.
func (c *Client) Name() string { return c.cfg.Name }

// Info is what the server declared at initialize.
func (c *Client) Info() Info { return c.info }

// Alive reports whether the server is still answering.
func (c *Client) Alive() bool { return c.fault.Load() == nil }

// Fault is why the server is down ("" while alive).
func (c *Client) Fault() string {
	if f := c.fault.Load(); f != nil {
		return *f
	}
	return ""
}

func (c *Client) setFault(why string) {
	if why == "" {
		why = "server died"
	}
	c.fault.CompareAndSwap(nil, &why)
}

// Close ends the connection (and the stdio process).
func (c *Client) Close() error {
	c.setFault("closed")
	return c.tr.close()
}

func (c *Client) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if f := c.fault.Load(); f != nil {
		return nil, fmt.Errorf("%w: %s", ErrDead, *f)
	}
	id, _ := json.Marshal(c.next.Add(1))
	cctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	resp, err := c.tr.call(cctx, message{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s: no answer within %s", method, c.cfg.Timeout)
		}
		if f := c.fault.Load(); f != nil {
			return nil, fmt.Errorf("%w: %s", ErrDead, *f)
		}
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

// ListTools pages through tools/list.
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	var out []ToolInfo
	cursor := ""
	for page := 0; page < 1000; page++ {
		params := json.RawMessage(`{}`)
		if cursor != "" {
			params, _ = json.Marshal(map[string]string{"cursor": cursor})
		}
		res, err := c.call(ctx, "tools/list", params)
		if err != nil {
			return out, err
		}
		var r struct {
			Tools      []ToolInfo `json:"tools"`
			NextCursor string     `json:"nextCursor"`
		}
		if err := json.Unmarshal(res, &r); err != nil {
			return out, fmt.Errorf("tools/list: %v", err)
		}
		out = append(out, r.Tools...)
		if r.NextCursor == "" || r.NextCursor == cursor {
			return out, nil
		}
		cursor = r.NextCursor
	}
	return out, errors.New("tools/list: pagination did not end")
}

// CallTool runs tools/call.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	params, _ := json.Marshal(map[string]interface{}{"name": name, "arguments": args})
	res, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return CallResult{}, err
	}
	var r CallResult
	if err := json.Unmarshal(res, &r); err != nil {
		return CallResult{}, fmt.Errorf("tools/call %s: %v", name, err)
	}
	return r, nil
}

// ListResources pages through resources/list.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	var out []Resource
	cursor := ""
	for page := 0; page < 1000; page++ {
		params := json.RawMessage(`{}`)
		if cursor != "" {
			params, _ = json.Marshal(map[string]string{"cursor": cursor})
		}
		res, err := c.call(ctx, "resources/list", params)
		if err != nil {
			return out, err
		}
		var r struct {
			Resources  []Resource `json:"resources"`
			NextCursor string     `json:"nextCursor"`
		}
		if err := json.Unmarshal(res, &r); err != nil {
			return out, err
		}
		out = append(out, r.Resources...)
		if r.NextCursor == "" || r.NextCursor == cursor {
			return out, nil
		}
		cursor = r.NextCursor
	}
	return out, errors.New("resources/list: pagination did not end")
}

// ReadResource runs resources/read.
func (c *Client) ReadResource(ctx context.Context, uri string) ([]Content, error) {
	params, _ := json.Marshal(map[string]string{"uri": uri})
	res, err := c.call(ctx, "resources/read", params)
	if err != nil {
		return nil, err
	}
	var r struct {
		Contents []Content `json:"contents"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	return r.Contents, nil
}

// ListPrompts runs prompts/list (paged).
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	var out []Prompt
	cursor := ""
	for page := 0; page < 1000; page++ {
		params := json.RawMessage(`{}`)
		if cursor != "" {
			params, _ = json.Marshal(map[string]string{"cursor": cursor})
		}
		res, err := c.call(ctx, "prompts/list", params)
		if err != nil {
			return out, err
		}
		var r struct {
			Prompts    []Prompt `json:"prompts"`
			NextCursor string   `json:"nextCursor"`
		}
		if err := json.Unmarshal(res, &r); err != nil {
			return out, err
		}
		out = append(out, r.Prompts...)
		if r.NextCursor == "" || r.NextCursor == cursor {
			return out, nil
		}
		cursor = r.NextCursor
	}
	return out, errors.New("prompts/list: pagination did not end")
}

// GetPrompt runs prompts/get.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (PromptResult, error) {
	if args == nil {
		args = map[string]string{}
	}
	params, _ := json.Marshal(map[string]interface{}{"name": name, "arguments": args})
	res, err := c.call(ctx, "prompts/get", params)
	if err != nil {
		return PromptResult{}, err
	}
	var r PromptResult
	if err := json.Unmarshal(res, &r); err != nil {
		return PromptResult{}, err
	}
	return r, nil
}

// Ping checks the server is answering.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "ping", json.RawMessage(`{}`))
	return err
}

// ---- stdio

type stdio struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	wmu     sync.Mutex
	pending map[string]chan message
	pmu     sync.Mutex
	done    chan struct{}
	onFault func(string)
}

func newStdio(ctx context.Context, cfg ServerConfig, onFault func(string)) (*stdio, error) {
	if cfg.Command == "" {
		return nil, errors.New("stdio transport needs a command")
	}
	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	drop, allow := []string(nil), cfg.EnvAllow
	if cfg.Sandbox != nil {
		o := cfg.Sandbox.Options()
		drop, allow = o.EnvDrop, append(append([]string(nil), o.EnvAllow...), cfg.EnvAllow...)
	}
	for k := range cfg.Env {
		allow = append(allow, k) // the server's own env is the human's word
	}
	scrubbed := sandbox.ScrubEnv(env, drop, allow)
	cwd := cfg.Cwd
	call := sandbox.Call{Cwd: cwd, Network: cfg.Network}
	// Not CommandContext: the server outlives the connect call and dies on Close.
	cmd := cfg.Sandbox.Command(context.Background(), call, scrubbed, append([]string{cfg.Command}, cfg.Args...)...)
	cmd.Env = scrubbed
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 8192}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cfg.Command, err)
	}
	t := &stdio{cmd: cmd, stdin: stdin, pending: map[string]chan message{}, done: make(chan struct{}), onFault: onFault}
	go func() {
		r := bufio.NewReaderSize(stdout, 4<<20)
		for {
			line, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(line)) > 0 {
				t.dispatch(line)
			}
			if err != nil {
				break
			}
		}
		werr := cmd.Wait()
		why := "exited"
		if werr != nil {
			why = werr.Error()
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			why += ": " + lastLine(s)
		}
		onFault("server " + why)
		close(t.done)
		t.pmu.Lock()
		for id, ch := range t.pending {
			close(ch)
			delete(t.pending, id)
		}
		t.pmu.Unlock()
	}()
	return t, nil
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(strings.TrimRight(s, "\n"), '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	k := len(p)
	if k > l.n {
		k = l.n
	}
	l.n -= k
	_, err := l.w.Write(p[:k])
	return len(p), err
}

func (t *stdio) dispatch(line []byte) {
	var m message
	if err := json.Unmarshal(line, &m); err != nil {
		return
	}
	if m.Method != "" && len(m.ID) > 0 {
		// A request from the server: answer ping, decline the rest.
		var resp message
		if m.Method == "ping" {
			resp = message{JSONRPC: "2.0", ID: m.ID, Result: json.RawMessage(`{}`)}
		} else {
			resp = message{JSONRPC: "2.0", ID: m.ID, Error: &Error{Code: -32601, Message: "client does not support " + m.Method}}
		}
		_ = t.write(resp)
		return
	}
	if m.Method != "" {
		return // notification
	}
	t.pmu.Lock()
	ch, ok := t.pending[string(m.ID)]
	if ok {
		delete(t.pending, string(m.ID))
	}
	t.pmu.Unlock()
	if ok {
		ch <- m
	}
}

func (t *stdio) write(m message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	t.wmu.Lock()
	defer t.wmu.Unlock()
	_, err = t.stdin.Write(append(b, '\n'))
	return err
}

func (t *stdio) call(ctx context.Context, m message) (message, error) {
	select {
	case <-t.done:
		return message{}, ErrDead
	default:
	}
	ch := make(chan message, 1)
	t.pmu.Lock()
	t.pending[string(m.ID)] = ch
	t.pmu.Unlock()
	if err := t.write(m); err != nil {
		t.pmu.Lock()
		delete(t.pending, string(m.ID))
		t.pmu.Unlock()
		return message{}, fmt.Errorf("%w: write: %v", ErrDead, err)
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return message{}, ErrDead
		}
		return resp, nil
	case <-t.done:
		return message{}, ErrDead
	case <-ctx.Done():
		t.pmu.Lock()
		delete(t.pending, string(m.ID))
		t.pmu.Unlock()
		return message{}, ctx.Err()
	}
}

func (t *stdio) notify(ctx context.Context, m message) error { return t.write(m) }

func (t *stdio) close() error {
	_ = t.stdin.Close()
	select {
	case <-t.done:
		return nil
	case <-time.After(500 * time.Millisecond):
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	select {
	case <-t.done:
	case <-time.After(2 * time.Second):
	}
	return nil
}

// ---- streamable HTTP

type httpT struct {
	cfg     ServerConfig
	client  *http.Client
	session atomic.Pointer[string]
	onFault func(string)
}

func newHTTP(cfg ServerConfig, onFault func(string)) (*httpT, error) {
	if cfg.URL == "" {
		return nil, errors.New("http transport needs a url")
	}
	cl := cfg.HTTPClient
	if cl == nil {
		cl = &http.Client{Timeout: cfg.Timeout + 5*time.Second}
	}
	return &httpT{cfg: cfg, client: cl, onFault: onFault}, nil
}

func (t *httpT) post(ctx context.Context, m message) (*http.Response, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	if s := t.session.Load(); s != nil {
		req.Header.Set("Mcp-Session-Id", *s)
	}
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			t.onFault("unreachable: " + err.Error())
		}
		return nil, err
	}
	if s := resp.Header.Get("Mcp-Session-Id"); s != "" {
		t.session.Store(&s)
	}
	return resp, nil
}

func (t *httpT) call(ctx context.Context, m message) (message, error) {
	resp, err := t.post(ctx, m)
	if err != nil {
		return message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && t.session.Load() != nil {
		t.onFault("session expired (404)")
		return message{}, ErrDead
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return message{}, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSSE(resp.Body, m.ID)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return message{}, err
	}
	var r message
	if err := json.Unmarshal(body, &r); err != nil {
		return message{}, fmt.Errorf("bad json answer: %v", err)
	}
	return r, nil
}

// readSSE reads events until the response with our id arrives.
func readSSE(body io.Reader, id json.RawMessage) (message, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64<<10), 64<<20)
	var data strings.Builder
	flush := func() (message, bool) {
		if data.Len() == 0 {
			return message{}, false
		}
		var m message
		err := json.Unmarshal([]byte(data.String()), &m)
		data.Reset()
		if err != nil || m.Method != "" || !bytes.Equal(bytes.TrimSpace(m.ID), bytes.TrimSpace(id)) {
			return message{}, false
		}
		return m, true
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if m, ok := flush(); ok {
				return m, nil
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if m, ok := flush(); ok {
		return m, nil
	}
	if err := sc.Err(); err != nil {
		return message{}, err
	}
	return message{}, errors.New("sse stream ended without the response")
}

func (t *httpT) notify(ctx context.Context, m message) error {
	resp, err := t.post(ctx, m)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("http %d on notification", resp.StatusCode)
	}
	return nil
}

func (t *httpT) close() error { return nil }
