package mcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/mcp/internal/testserver"
	"github.com/Kaginari/agent-one/sandbox"
	"github.com/Kaginari/agent-one/tool"
)

var (
	buildOnce sync.Once
	serverBin string
	buildErr  error
)

// server builds testdata/server once per test binary.
func server(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
		if _, err := os.Stat(goBin); err != nil {
			goBin, err = exec.LookPath("go")
			if err != nil {
				buildErr = err
				return
			}
		}
		dir, err := os.MkdirTemp("", "mcp-testserver")
		if err != nil {
			buildErr = err
			return
		}
		serverBin = filepath.Join(dir, "server")
		cmd := exec.Command(goBin, "build", "-o", serverBin, "./testdata/server")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = err
			t.Log(string(out))
		}
	})
	if buildErr != nil {
		t.Skip("cannot build the test server: " + buildErr.Error())
	}
	return serverBin
}

func TestMain(m *testing.M) {
	code := m.Run()
	if serverBin != "" {
		os.RemoveAll(filepath.Dir(serverBin))
	}
	os.Exit(code)
}

func connectStdio(t *testing.T, sb *sandbox.Sandbox, timeout time.Duration) *Client {
	t.Helper()
	bin := server(t)
	cwd := t.TempDir()
	if sb != nil {
		// Under bwrap /tmp is private: the binary and the cwd must live in the workspace root.
		root := sb.Options().Root
		data, err := os.ReadFile(bin)
		if err != nil {
			t.Fatal(err)
		}
		bin = filepath.Join(root, "server")
		if err := os.WriteFile(bin, data, 0o755); err != nil {
			t.Fatal(err)
		}
		cwd = root
	}
	os.Setenv("MCP_TEST_TOKEN", "leak")
	os.Setenv("MCP_TEST_PLAIN", "visible")
	t.Cleanup(func() { os.Unsetenv("MCP_TEST_TOKEN"); os.Unsetenv("MCP_TEST_PLAIN") })
	c, err := Connect(context.Background(), ServerConfig{Name: "ts", Command: bin, Sandbox: sb, Timeout: timeout, Env: map[string]string{"MCP_OWN_TOKEN": "mine"}, Cwd: cwd})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func exercise(t *testing.T, c *Client) {
	t.Helper()
	ctx := context.Background()
	if c.Info().Name != "testserver" || c.Info().Protocol != testserver.Protocol {
		t.Fatalf("%+v", c.Info())
	}
	tools, err := c.ListTools(ctx)
	if err != nil || len(tools) != 9 {
		t.Fatalf("pagination should gather every tool: %d %v", len(tools), err)
	}
	if tools[0].Name != "echo" || tools[8].Name != "count" {
		t.Fatalf("%+v", tools)
	}
	r, err := c.CallTool(ctx, "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil || r.IsError || r.Text() != "hi" {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = c.CallTool(ctx, "boom", nil)
	if err != nil || !r.IsError || !strings.Contains(r.Text(), "boom failed") {
		t.Fatalf("%+v %v", r, err)
	}
	res, err := c.ListResources(ctx)
	if err != nil || len(res) != 1 || res[0].URI != "mem://hello" {
		t.Fatalf("%+v %v", res, err)
	}
	contents, err := c.ReadResource(ctx, "mem://hello")
	if err != nil || len(contents) != 1 || contents[0].Text != "hello from a resource" {
		t.Fatalf("%+v %v", contents, err)
	}
	if _, err := c.ReadResource(ctx, "mem://nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("rpc error must surface: %v", err)
	}
	ps, err := c.ListPrompts(ctx)
	if err != nil || len(ps) != 1 || ps[0].Name != "greet" || !ps[0].Arguments[0].Required {
		t.Fatalf("%+v %v", ps, err)
	}
	pr, err := c.GetPrompt(ctx, "greet", map[string]string{"name": "Orchestrator"})
	if err != nil || len(pr.Messages) != 1 || pr.Messages[0].Content.Text != "Hello, Orchestrator!" {
		t.Fatalf("%+v %v", pr, err)
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStdio(t *testing.T) {
	c := connectStdio(t, nil, 5*time.Second)
	exercise(t, c)
	ctx := context.Background()
	// Env scrubbing: *_TOKEN dropped, plain kept, the server's own env allowed.
	for name, want := range map[string]string{"MCP_TEST_TOKEN": "(unset)", "MCP_TEST_PLAIN": "visible", "MCP_OWN_TOKEN": "mine"} {
		r, err := c.CallTool(ctx, "env", json.RawMessage(`{"name":"`+name+`"}`))
		if err != nil || r.Text() != want {
			t.Fatalf("%s: %q %v", name, r.Text(), err)
		}
	}
	// Timeout: a slow call is reported, the server stays alive.
	short := connectStdio(t, nil, 300*time.Millisecond)
	if _, err := short.CallTool(ctx, "slow", json.RawMessage(`{"ms":2000}`)); err == nil || !strings.Contains(err.Error(), "no answer within") {
		t.Fatalf("%v", err)
	}
	if !short.Alive() {
		t.Fatal("a timeout is not death")
	}
	// Death: the server exits; the client reports it and stays dead.
	r, err := c.CallTool(ctx, "die", nil)
	if err != nil || r.Text() != "dying" {
		t.Fatalf("%+v %v", r, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for c.Alive() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if c.Alive() || !strings.Contains(c.Fault(), "exit status 3") {
		t.Fatalf("death not detected: alive=%v fault=%q", c.Alive(), c.Fault())
	}
	if _, err := c.CallTool(ctx, "echo", nil); err == nil || !strings.Contains(err.Error(), "down") {
		t.Fatalf("calls after death: %v", err)
	}
}

func TestStdioUnderSandbox(t *testing.T) {
	root := t.TempDir()
	sb := sandbox.New(sandbox.Options{Enabled: true, Root: root, EnvDrop: []string{"MCP_TEST_PLAIN"}})
	if sb.Mode() != sandbox.Bwrap {
		t.Skip("bwrap unavailable: " + sb.Why())
	}
	c := connectStdio(t, sb, 5*time.Second)
	exercise(t, c)
	r, err := c.CallTool(context.Background(), "env", json.RawMessage(`{"name":"MCP_TEST_PLAIN"}`))
	if err != nil || r.Text() != "(unset)" {
		t.Fatalf("sandbox EnvDrop should apply to the server: %q %v", r.Text(), err)
	}
}

func TestHTTP(t *testing.T) {
	for _, sse := range []bool{false, true} {
		srv := httptest.NewServer(testserver.New().Handler(sse))
		c, err := Connect(context.Background(), ServerConfig{Name: "web", URL: srv.URL, Timeout: 5 * time.Second, Headers: map[string]string{"Authorization": "Bearer x"}})
		if err != nil {
			t.Fatalf("sse=%v: %v", sse, err)
		}
		exercise(t, c)
		if c.tr.(*httpT).session.Load() == nil {
			t.Fatal("session id not kept")
		}
		srv.Close()
		if _, err := c.ListTools(context.Background()); err == nil {
			t.Fatal("closed server must fail")
		}
		if c.Alive() {
			t.Fatal("unreachable server is a fault")
		}
		c.Close()
	}
	// A wrong session is a 404 → dead.
	srv := httptest.NewServer(testserver.New().Handler(false))
	defer srv.Close()
	c, err := Connect(context.Background(), ServerConfig{Name: "web", URL: srv.URL, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	bogus := "bogus"
	c.tr.(*httpT).session.Store(&bogus)
	if _, err := c.ListTools(context.Background()); err == nil || c.Alive() || !strings.Contains(c.Fault(), "session expired") {
		t.Fatalf("%v %q", err, c.Fault())
	}
}

func TestAdapter(t *testing.T) {
	c := connectStdio(t, nil, 5*time.Second)
	ctx := context.Background()
	if ts, err := Tools(ctx, c, ServerOptions{}); err != nil || ts != nil {
		t.Fatal("disabled server exposes nothing")
	}
	ts, err := Tools(ctx, c, ServerOptions{Enabled: true, Disable: []string{"die"}})
	if err != nil || len(ts) != 8 {
		t.Fatalf("%d %v", len(ts), err)
	}
	byName := map[string]*tool.Tool{}
	for _, tl := range ts {
		byName[tl.Name] = tl
	}
	env := tool.Env{Root: t.TempDir()}
	echo := byName["mcp__ts__echo"]
	if echo == nil || echo.Class != tool.Outward || !strings.Contains(string(echo.Schema), `"text"`) {
		t.Fatalf("%+v", echo)
	}
	r := echo.Run(ctx, env, json.RawMessage(`{"text":"via adapter"}`))
	if r.Err || r.Output != "via adapter" {
		t.Fatalf("%+v", r)
	}
	if r := byName["mcp__ts__boom"].Run(ctx, env, nil); !r.Err || !strings.Contains(r.Output, "boom failed") {
		t.Fatalf("%+v", r)
	}
	if byName["mcp__ts__wipe"].Class != tool.Destructive || byName["mcp__ts__fetch"].Class != tool.Outward || byName["mcp__ts__ro"].Class != tool.Outward {
		t.Fatal("annotations: destructive tightens, readOnly lowers nothing")
	}
	// Inward server: floor write; config may set a tool's class; annotations still tighten.
	in := ServerOptions{Enabled: true, Inward: true, Classes: map[string]string{"ro": "read", "wipe": "read"}}
	var wipe, ro, fetch, echoI ToolInfo
	infos, _ := c.ListTools(ctx)
	for _, ti := range infos {
		switch ti.Name {
		case "wipe":
			wipe = ti
		case "ro":
			ro = ti
		case "fetch":
			fetch = ti
		case "echo":
			echoI = ti
		}
	}
	if cl, _ := Classify(echoI, in); cl != tool.Write {
		t.Fatal(cl)
	}
	if cl, _ := Classify(ro, in); cl != tool.Read {
		t.Fatal(cl)
	}
	if cl, why := Classify(wipe, in); cl != tool.Destructive || why != "destructiveHint" {
		t.Fatal(cl, why)
	}
	if cl, _ := Classify(fetch, in); cl != tool.Outward {
		t.Fatal(cl)
	}
	// The tool's own Settle keeps the class (declared class only tightens).
	if cl, _ := echo.Settle(env, json.RawMessage(`{"text":"x","class":"read"}`)); cl.Class != tool.Outward {
		t.Fatal(cl)
	}
	// Death reported by the tool, not a vanished tool.
	c.Close()
	if r := echo.Run(ctx, env, json.RawMessage(`{"text":"x"}`)); !r.Err || !strings.Contains(r.Output, "is down") {
		t.Fatalf("%+v", r)
	}
}

func TestConnectErrors(t *testing.T) {
	if _, err := Connect(context.Background(), ServerConfig{Name: "x", Command: "/nonexistent/mcp"}); err == nil {
		t.Fatal("missing command")
	}
	if _, err := Connect(context.Background(), ServerConfig{Name: "x", Transport: "carrier-pigeon"}); err == nil {
		t.Fatal("transport")
	}
	if _, err := Connect(context.Background(), ServerConfig{Command: "true"}); err == nil {
		t.Fatal("name")
	}
	bin := server(t)
	// A server that never answers initialize: a timeout, not a hang.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Connect(ctx, ServerConfig{Name: "sleeper", Command: "sleep", Args: []string{"30"}, Timeout: 300 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "initialize") {
		t.Fatalf("%v", err)
	}
	_ = bin
}
