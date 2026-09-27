package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/sandbox"
)

var (
	e2eOnce sync.Once
	e2eDir  string
	e2eErr  error
)

// binaries builds agent-one and the MCP test server once per test binary.
func binaries(t *testing.T) (agentOne, mcp string) {
	t.Helper()
	e2eOnce.Do(func() {
		dir, err := os.MkdirTemp("", "agent-one-e2e")
		if err != nil {
			e2eErr = err
			return
		}
		e2eDir = dir
		for _, b := range []struct{ name, pkg string }{{"agent-one", "../cmd/agent-one"}, {"mcpserver", "./testdata/mcpserver"}} {
			cmd := exec.Command(goBin(), "build", "-o", filepath.Join(dir, b.name), b.pkg)
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
			if out, err := cmd.CombinedOutput(); err != nil {
				e2eErr = fmt.Errorf("%s: %v\n%s", b.name, err, out)
				return
			}
		}
	})
	if e2eErr != nil {
		t.Skip("cannot build the binaries: " + e2eErr.Error())
	}
	return filepath.Join(e2eDir, "agent-one"), filepath.Join(e2eDir, "mcpserver")
}

// exec runs a binary in the workspace with a clean environment (a fake HOME, no keys unless given).
func (w *testWorkspace) exec(bin, stdin string, env []string, args ...string) (int, string, string) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = w.root
	cmd.Env = append([]string{"HOME=" + w.home, "PATH=" + os.Getenv("PATH"), "TERM=dumb"}, env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
		errb.WriteString(err.Error())
	}
	return code, out.String(), errb.String()
}

// journalText joins every loop journal of the workspace (the instrument the e2e reads outputs from).
func (w *testWorkspace) journalText(workspaceDir string) string {
	var b strings.Builder
	ents, _ := os.ReadDir(filepath.Join(w.root, workspaceDir, "instruments", "loop"))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			b.WriteString(w.read(workspaceDir + "/instruments/loop/" + e.Name()))
		}
	}
	return b.String()
}

func (w *testWorkspace) usageText(workspaceDir string) string {
	var b strings.Builder
	ents, _ := os.ReadDir(filepath.Join(w.root, workspaceDir, "instruments", "usage"))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			b.WriteString(w.read(workspaceDir + "/instruments/usage/" + e.Name()))
		}
	}
	return b.String()
}

func mockCfg(script string, extra string) string {
	return "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: " + script + "}}\ntools: {bash: {sandbox: none}}\nui: {board: {autostart: false}}\n" + extra
}

func TestE2ESessionGateAndTools(t *testing.T) {
	agentOne, mcpBin := binaries(t)
	// A. a REPL session over a pipe
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", when("say hi", call("bash", map[string]string{"command": "echo hi"})), when("hi\n", text("@S DONE hi said\n@E 20")))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	code, out, errb := w.exec(agentOne, "say hi\n/status\n/agents\n/quit\n", nil, "--quiet")
	if code != 0 || !strings.Contains(out, "hi said") || !strings.Contains(out, "ranks:") || !strings.Contains(out, "orchestrator") || !strings.Contains(errb, "session ") {
		t.Fatalf("repl: %d\n%s\n%s", code, out, errb)
	}
	// the orchestrator is not an role: its own calls are journaled with no role label
	if u := w.usageText(".agent-one"); !strings.Contains(u, `"agent":"orchestrator"`) || !strings.Contains(u, `"role":""`) || strings.Contains(u, `"role":"drafter"`) {
		t.Fatalf("usage journal labels the session's calls with an role:\n%s", u)
	}
	// B. an outward command denied at the gate (no TTY), then allowed by a rule
	w = newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", when("push", call("bash", map[string]string{"command": "git push origin main"})), text("@S DONE pushed or not\n@E 22"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	code, out, _ = w.exec(agentOne, "", nil, "run", "--quiet", "push")
	if code != 4 || !strings.Contains(out, "denied at the gate") {
		t.Fatalf("denied: %d %q", code, out)
	}
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", "permissions:\n  rules:\n    - {match: \"bash:git push*\", action: allow}\n"))
	w.script(".agent-one/tmp/s.json", when("push", call("bash", map[string]string{"command": "git push origin main"})), text("@S DONE pushed or not\n@E 22"))
	code, out, _ = w.exec(agentOne, "", nil, "run", "--quiet", "push")
	if code != 0 || !strings.Contains(out, "pushed or not") || !strings.Contains(w.journalText(".agent-one"), `"by":"rule"`) {
		t.Fatalf("allowed by rule: %d %q", code, out)
	}
	if code, out, _ := w.exec(agentOne, "", nil, "status"); code != 0 || !strings.Contains(out, "loosening: allow bash:git push*") {
		t.Errorf("status lists the loosening: %d %q", code, out)
	}
	// C. the sandbox: a write outside the workspace does not land, the network is unreachable
	if sandbox.ProbeBwrap("", 5*time.Second).Available {
		outside := t.TempDir()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
		defer srv.Close()
		_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
		w = newTestWorkspace(t, "agent-one", agentOneMembers())
		w.script(".agent-one/tmp/s.json",
			when("contain", map[string]interface{}{"calls": []interface{}{
				map[string]interface{}{"name": "bash", "input": map[string]string{"command": "echo x > " + outside + "/probe; echo probe=$?"}},
				map[string]interface{}{"name": "bash", "input": map[string]string{"command": "exec 3<>/dev/tcp/127.0.0.1/" + port + " && echo NET=YES || echo NET=NO"}},
			}}),
			text("@S DONE contained\n@E 20"))
		w.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: .agent-one/tmp/s.json}}\ntools: {bash: {sandbox: bwrap}}\nui: {board: {autostart: false}}\n")
		code, out, errb = w.exec(agentOne, "", nil, "run", "--quiet", "--approve", "outward", "contain")
		if code != 0 {
			t.Fatalf("sandbox run: %d %q %q", code, out, errb)
		}
		if _, err := os.Stat(filepath.Join(outside, "probe")); err == nil {
			t.Error("a write outside the workspace landed through the sandbox")
		}
		if j := w.journalText(".agent-one"); !strings.Contains(j, "NET=NO\\n\"") || strings.Contains(j, "NET=YES\\n\"") {
			var acts []string
			for _, l := range strings.Split(j, "\n") {
				if strings.Contains(l, `"t":"act"`) || strings.Contains(l, `"t":"gate"`) {
					acts = append(acts, l)
				}
			}
			t.Errorf("the network was reachable from a non-outward command:\n%s", strings.Join(acts, "\n"))
		}
	} else {
		t.Log("bwrap unusable here — the egress control scenario is skipped")
	}
	// D. secret scrubbing: a key in the environment never reaches a command
	w = newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", when("leak", call("bash", map[string]string{"command": "echo \"k=$ANTHROPIC_API_KEY.\" > probe.txt"})), text("@S DONE\n@E 12"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	if code, out, _ := w.exec(agentOne, "", []string{"ANTHROPIC_API_KEY=sk-e2e-secret"}, "run", "--quiet", "leak"); code != 0 || strings.TrimSpace(w.read("probe.txt")) != "k=." {
		t.Fatalf("scrubbing: %d %q probe=%q", code, out, w.read("probe.txt"))
	}
	// E. a custom ls tool; F. a disabled tool: error + alternative, then the doom-loop stop
	w = newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("list", call("lsl", map[string]string{"path": "."})),
		when("README.md", call("webfetch", map[string]string{"url": "https://x"})),
		call("webfetch", map[string]string{"url": "https://x"}),
		text("@S DONE never\n@E 12"))
	w.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: .agent-one/tmp/s.json}}\nui: {board: {autostart: false}}\ntools:\n  webfetch: {enabled: false}\n  bash: {sandbox: none}\n  custom:\n    lsl: {description: list, class: read, params: {path: {type: string, required: true}}, run: [ls, \"-1\", \"{{path}}\"]}\n")
	code, out, _ = w.exec(agentOne, "", nil, "run", "--quiet", "list")
	j := w.journalText(".agent-one")
	if code != 3 || !strings.Contains(out, "doom-loop guard") || !strings.Contains(j, "README.md") || !strings.Contains(j, "disabled by tools.webfetch.enabled in") || !strings.Contains(j, "closest enabled") {
		t.Fatalf("custom tool + doom loop: %d %q\n%s", code, out, clip(j))
	}
	// J. a stdio MCP server written in Go under testdata
	w = newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", when("add", call("mcp__t__add", map[string]int{"a": 2, "b": 3})), when("5", text("@S DONE five\n@E 16")))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", "mcp:\n  servers:\n    t: {command: ["+mcpBin+"], inward: true, sandbox: none}\n"))
	if code, out, errb := w.exec(agentOne, "", nil, "run", "--quiet", "add"); code != 0 || !strings.Contains(out, "five") {
		t.Fatalf("mcp: %d %q %q", code, out, errb)
	}
	if code, out, _ := w.exec(agentOne, "", nil, "status"); code != 0 || !strings.Contains(out, "mcp t (config, inward): 3 tools") {
		t.Errorf("status names the server: %d %q", code, out)
	}
}

func TestE2ECourtDrainRanksAndConfigs(t *testing.T) {
	agentOne, _ := binaries(t)
	// G. a subagent dispatched with role routing to a distinct mock model; H. a drain
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	var bulky []map[string]interface{}
	bulky = append(bulky, when("work", call("dispatch", map[string]string{"agent": "zone-auth", "ask": "findings: look at the login zone", "scope": "src/auth"})))
	for i := 0; i < 4; i++ {
		bulky = append(bulky, call("bash", map[string]string{"command": "seq 1 400"}))
	}
	bulky = append(bulky, text("@S DONE all\n@E 14"))
	w.script(".agent-one/tmp/s.json", bulky...)
	w.script(".agent-one/tmp/c.json",
		when("@ASK findings", text("@S PASS\n@F src/auth/login.go:1 fine\n@U domain login is the door\n@E 60")),
		when("drained messages", text("@S DRAINED\n@U team the e2e drained\n@D goal: work\n@E 50")))
	w.write(".agent-one/config.yaml", `
models: {default: mock/m, roles: {analyst: subagent/c}, tasks: {drain: subagent/c}}
providers:
  mock: {enabled: true, script: .agent-one/tmp/s.json}
  subagent: {type: mock, script: .agent-one/tmp/c.json}
tools: {bash: {sandbox: none}}
policy: {budget: {stressTokens: 3200}}
compaction: {keepRecentTurns: 1}
ui: {board: {autostart: false}}
`)
	code, out, errb := w.exec(agentOne, "", nil, "run", "--quiet", "--session", "e2e-subagent", "work")
	if code != 0 || !strings.Contains(out, "all") {
		t.Fatalf("subagent+drain run: %d %q %q", code, out, errb)
	}
	journal := w.read(".agent-one/instruments/usage/e2e-subagent.jsonl")
	if !strings.Contains(journal, `"agent":"zone-auth"`) || !strings.Contains(journal, `"model":"subagent/c"`) || !strings.Contains(journal, `"role":"analyst"`) || !strings.Contains(journal, `"agent":"drain"`) {
		t.Errorf("usage journal:\n%s", journal)
	}
	notes := w.read(".agent-one/memory/shared/notes.jsonl")
	if !strings.Contains(notes, "login is the door") || !strings.Contains(notes, "the e2e drained") {
		t.Errorf("notes: %s", notes)
	}
	if j := w.journalText(".agent-one"); !strings.Contains(j, `"t":"drain","ok":true`) && !strings.Contains(j, `"ok":true`) {
		t.Errorf("no drain in the loop journal:\n%s", clip(j))
	}
	if code, out, _ := w.exec(agentOne, "", nil, "usage", "--session", "e2e-subagent"); code != 0 || !strings.Contains(out, "zone-auth") || !strings.Contains(out, "unpriced") {
		t.Errorf("usage: %d %q", code, out)
	}
	// I. a 5-rank replace-mode workspace
	w = newTestWorkspace(t, "agent-one", map[string]string{
		".agent-one/AGENT-ONE.md":              testPolicy,
		".agent-one/log.md":                    "# log\n\n---\n",
		".agent-one/lead/core/README.md":       "# lead-core\n\n- **Rank:** lead\n- **Owns:** `src/`\n",
		".agent-one/persistent/gate/README.md": "# persistent-gate\n\n- **Rank:** persistent\n- **Owns:** `src/`\n- **Reports to:** lead-core\n",
		".agent-one/coder/auth/README.md":      "# coder-auth\n\n- **Rank:** coder\n- **Owns:** `src/auth/`\n- **Reports to:** persistent-gate\n",
		".agent-one/tester/qa/README.md":       "# tester-qa\n\n- **Rank:** tester\n- **Owns:** `tests/`\n- **Reports to:** persistent-gate\n",
		".agent-one/scribe/log/README.md":      "# scribe-log\n\n- **Rank:** scribe\n- **Owns:** `docs/`\n- **Reports to:** lead-core\n",
		"src/auth/a.go":                        "package auth\n",
	})
	w.script(".agent-one/tmp/s.json", when("start", call("dispatch", map[string]string{"agent": "coder-auth", "ask": "findings: look"})), when("@S PASS", text("@S DONE five ranks\n@E 20")))
	w.script(".agent-one/tmp/c.json", when("@ASK findings", text("@S PASS\n@U team coders code\n@E 30")))
	w.write(".agent-one/config.yaml", `
models: {default: mock/m, roles: {analyst: subagent/c}}
providers:
  mock: {enabled: true, script: .agent-one/tmp/s.json}
  subagent: {type: mock, script: .agent-one/tmp/c.json}
tools: {bash: {sandbox: none}}
rankSet: replace
ranks:
  lead:   {reportsTo: orchestrator, job: coordinates, agent: ephemeral, role: drafter}
  persistent: {reportsTo: lead, job: holds the gate, holdsGate: true, agent: persistent, role: judge}
  coder:  {reportsTo: persistent, job: writes code, authors: true, agent: ephemeral, role: analyst}
  tester: {reportsTo: persistent, job: writes tests, authors: true, agent: ephemeral, role: analyst}
  scribe: {reportsTo: lead, job: keeps the docs, agent: ephemeral, role: drafter}
ui: {board: {autostart: false}}
`)
	if code, out, errb := w.exec(agentOne, "", nil, "run", "--quiet", "start"); code != 0 || !strings.Contains(out, "five ranks") {
		t.Fatalf("replace-mode workspace: %d %q %q", code, out, errb)
	}
	if code, out, _ := w.exec(agentOne, "", nil, "status"); code != 0 || !strings.Contains(out, "rankSet: replace") || strings.Contains(out, " zone ") {
		t.Errorf("status in replace mode: %d %q", code, out)
	}
	// K. YAML and JSON configs are one config
	yamlW := newTestWorkspace(t, "agent-one", agentOneMembers())
	jsonW := newTestWorkspace(t, "agent-one", agentOneMembers())
	for _, x := range []*testWorkspace{yamlW, jsonW} {
		x.script(".agent-one/tmp/s.json", when("echo", call("bash", map[string]string{"command": "echo same > same.txt"})), text("@S DONE same\n@E 16"))
	}
	yamlW.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: .agent-one/tmp/s.json}}\ntools: {bash: {sandbox: none, timeout: 30s}, webfetch: {enabled: false}}\npolicy: {wire: {cap: 3000}}\nui: {board: {autostart: false}}\n")
	jsonW.write(".agent-one/config.json", `{"models": {"default": "mock/m"}, "providers": {"mock": {"enabled": true, "script": ".agent-one/tmp/s.json"}}, "tools": {"bash": {"sandbox": "none", "timeout": "30s"}, "webfetch": {"enabled": false}}, "policy": {"wire": {"cap": 3000}}, "ui": {"board": {"autostart": false}}}`)
	_, showY, _ := yamlW.exec(agentOne, "", nil, "config", "show")
	_, showJ, _ := jsonW.exec(agentOne, "", nil, "config", "show")
	if showY != showJ || !strings.Contains(showY, `"cap": 3000`) {
		t.Errorf("yaml and json configs differ:\n%s\n---\n%s", clip(showY), clip(showJ))
	}
	for _, x := range []*testWorkspace{yamlW, jsonW} {
		if code, out, _ := x.exec(agentOne, "", nil, "run", "--quiet", "echo"); code != 0 || !strings.Contains(out, "same") || x.read("same.txt") != "same\n" {
			t.Errorf("run on %s: %d %q", x.root, code, out)
		}
	}
}

func TestE2EBothDistributionsAndHarbor(t *testing.T) {
	agentOne, _ := binaries(t)
	for _, c := range []struct{ bin, dist, dir, prefix string }{{agentOne, "agent-one", ".agent-one", "AGENT_ONE_"}} {
		w := newTestWorkspace(t, c.dist, nil)
		_ = os.MkdirAll(w.root, 0o755)
		// 1. init --bench: idempotent, nothing outside the workspace dir
		code, out, errb := w.exec(c.bin, "", nil, "init", "--bench")
		if code != 0 || !strings.Contains(out, "@S OK "+c.dist+" founded") {
			t.Fatalf("%s init: %d %q %q", c.dist, code, out, errb)
		}
		if code, out, _ := w.exec(c.bin, "", nil, "init", "--bench"); code != 0 || !strings.Contains(out, "nothing touched") {
			t.Errorf("%s init twice: %d %q", c.dist, code, out)
		}
		if ents, _ := os.ReadDir(w.root); len(ents) != 1 || ents[0].Name() != c.dir {
			t.Errorf("%s init wrote outside %s: %v", c.dist, c.dir, ents)
		}
		// 4. version
		if code, out, _ := w.exec(c.bin, "", nil, "version"); code != 0 || !strings.HasPrefix(out, c.dist+" dev (none, unknown, ") {
			t.Errorf("%s version: %d %q", c.dist, code, out)
		}
		// 2. run --json with the config from <PREFIX>CONFIG_CONTENT, --model picking the mount
		w.write(c.dir+"/tmp/s.json", `[{"calls":[{"name":"bash","input":{"command":"printf 'hello agentOne\n' > hello.txt && cat hello.txt"}}]},{"text":"@S DONE wrote hello.txt\n@E 30"}]`)
		cfg := "providers: {mock: {enabled: true, script: " + c.dir + "/tmp/s.json}, other: {type: mock}}\nmodels: {default: other/x}\ntools: {profile: minimal, bash: {sandbox: none}}\nui: {board: {autostart: false}}\n"
		code, out, errb = w.exec(c.bin, "", []string{c.prefix + "CONFIG_CONTENT=" + cfg}, "run", "--json", "--quiet", "--model", "mock/m", "Create the file hello.txt")
		if code != 0 {
			t.Fatalf("%s run --json: %d %q %q", c.dist, code, out, errb)
		}
		var res map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil || res["@S"] != "DONE" || w.read("hello.txt") != "hello agentOne\n" {
			t.Fatalf("%s result: %v %q hello=%q", c.dist, err, out, w.read("hello.txt"))
		}
		// 3. the usage journal, one line per call, the Harbor fields
		ents, _ := os.ReadDir(filepath.Join(w.root, c.dir, "instruments", "usage"))
		if len(ents) != 1 {
			t.Fatalf("%s usage journal files: %v", c.dist, ents)
		}
		lines := strings.Split(strings.TrimSpace(w.read(c.dir+"/instruments/usage/"+ents[0].Name())), "\n")
		if len(lines) != 2 {
			t.Errorf("%s usage lines: %d", c.dist, len(lines))
		}
		var rec map[string]interface{}
		_ = json.Unmarshal([]byte(lines[0]), &rec)
		for _, k := range []string{"input", "output", "cacheRead", "cacheWrite"} {
			if _, ok := rec[k].(float64); !ok {
				t.Errorf("%s usage record %s is not an int: %s", c.dist, k, lines[0])
			}
		}
		if v, ok := rec["usd"]; !ok || v != nil {
			t.Errorf("%s unpriced usd must be null: %s", c.dist, lines[0])
		}
		// a checkpointed run exits non-zero
		w.write(c.dir+"/tmp/s2.json", `[{"calls":[{"name":"bash","input":{"command":"echo a"}},{"name":"bash","input":{"command":"echo b"}}]},{"text":"@S DONE\n@E 12"}]`)
		cfg2 := strings.Replace(cfg, "/tmp/s.json", "/tmp/s2.json", 1) + "policy: {budget: {steps: 1}}\n"
		if code, _, _ := w.exec(c.bin, "", []string{c.prefix + "CONFIG_CONTENT=" + cfg2}, "run", "--json", "--quiet", "--model", "mock/m", "two"); code != 5 {
			t.Errorf("%s checkpoint exit: %d", c.dist, code)
		}
		// 5. the gate switched off from the env layer is refused, as config.md says
		if code, _, errb := w.exec(c.bin, "", []string{c.prefix + "CONFIG_CONTENT=" + cfg + "policy: {humanGate: {enabled: false}}\n"}, "run", "--json", "--quiet", "--model", "mock/m", "x"); code != 2 || !strings.Contains(errb, "policy.humanGate.enabled: false is honored only from a config file") {
			t.Errorf("%s gate off from env: %d %q", c.dist, code, errb)
		}
		// selftest on each binary
		if code, out, errb := w.exec(c.bin, "", nil, "selftest"); code != 0 || !strings.HasPrefix(out, "@S PASS ") {
			t.Errorf("%s selftest: %d %q %q", c.dist, code, out, errb)
		}
	}
	// N. the fake vLLM path: an OpenAI-compatible server scripting one bash call, tools.profile
	// minimal offering `bash` with {"command": string}, contextWindow auto from /v1/models
	var calls int
	var offered []string
	fake := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("content-type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(rw, `{"data":[{"id":"fake/vllm-coder","max_model_len":32768}]}`)
			return
		}
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name       string          `json:"name"`
					Parameters json.RawMessage `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls++
		offered = nil
		hasBash := false
		for _, t := range req.Tools {
			offered = append(offered, t.Function.Name)
			if t.Function.Name == "bash" && strings.Contains(string(t.Function.Parameters), `"command":{"type":"string"}`) {
				hasBash = true
			}
		}
		toolResults := 0
		for _, m := range req.Messages {
			if m.Role == "tool" {
				toolResults++
			}
		}
		usage := `"usage":{"prompt_tokens":900,"completion_tokens":40}`
		switch {
		case !hasBash:
			http.Error(rw, "no bash tool offered", 400)
		case toolResults == 0:
			_, _ = io.WriteString(rw, `{"model":"fake/vllm-coder","choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf 'hello agentOne\\n' > hello.txt && cat hello.txt\"}"}}]},"finish_reason":"tool_calls"}],`+usage+`}`)
		default:
			_, _ = io.WriteString(rw, `{"model":"fake/vllm-coder","choices":[{"message":{"role":"assistant","content":"@S DONE wrote hello.txt\n@E 30"},"finish_reason":"stop"}],`+usage+`}`)
		}
	}))
	defer fake.Close()
	w := newTestWorkspace(t, "agent-one", nil)
	_ = os.MkdirAll(w.root, 0o755)
	if code, _, errb := w.exec(agentOne, "", nil, "init", "--bench"); code != 0 {
		t.Fatal(errb)
	}
	cfg := "providers:\n  vllm: {type: openai, baseURL: " + fake.URL + "/v1, apiKeyEnv: VLLM_API_KEY, toolCalls: native, contextWindow: auto}\nmodels: {default: vllm/fake/vllm-coder}\ntools: {profile: minimal, bash: {sandbox: none}}\nbudgets: {session: {tokens: 200000}}\nui: {board: {autostart: false}}\n"
	code, out, errb := w.exec(agentOne, "", []string{"AGENT_ONE_CONFIG_CONTENT=" + cfg, "VLLM_API_KEY=k"}, "run", "--json", "--quiet", "Create the file hello.txt whose only content is the line: hello agentOne")
	if code != 0 || w.read("hello.txt") != "hello agentOne\n" || calls != 2 {
		t.Fatalf("fake vllm: %d calls=%d offered=%v\n%s\n%s", code, calls, offered, out, errb)
	}
	var res map[string]interface{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(out)), &res)
	if u, _ := res["usage"].(map[string]interface{}); u["input"].(float64) != 1800 || u["output"].(float64) != 80 {
		t.Errorf("usage from the fake: %v", res["usage"])
	}
}

// The guard refuses a catastrophic command even when its class was pre-approved: no approval
// can run it. The home the command names is untouched.
func TestE2EGuardRefusesEvenApproved(t *testing.T) {
	agentOne, _ := binaries(t)
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	os.WriteFile(filepath.Join(w.home, "keep.txt"), []byte("mine"), 0o644)
	w.script(".agent-one/tmp/s.json", when("wipe", call("bash", map[string]string{"command": "rm -rf ~"})), text("@S DONE\n@E 5"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	code, out, errb := w.exec(agentOne, "", nil, "run", "--json", "--quiet", "--approve", "destructive,outward", "wipe")
	if _, err := os.Stat(filepath.Join(w.home, "keep.txt")); err != nil {
		t.Fatalf("the home was touched: %v\n%s\n%s", err, out, errb)
	}
	j := w.read(".agent-one/instruments/loop/" + firstJournal(w))
	if !strings.Contains(j, `"by":"guard"`) || !strings.Contains(j, `"decision":"refused"`) {
		t.Fatalf("the journal does not show the guard's refusal (exit %d):\n%s", code, j)
	}
}

// /handoff hands the model the gathered facts and the template; /handoff read gives a fresh
// session the file with the rule to verify it before trusting it.
func TestE2EHandoff(t *testing.T) {
	agentOne, _ := binaries(t)
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("A previous session left this handoff", text("@S DONE read and waiting\n@E 20")),
		when("Write a handoff for a fresh session", call("write", map[string]string{"path": ".agent-one/handoffs/2026-09-27-120000.md", "content": "# HANDOFF: auth\n\n## 3. Current state\nDONE: login\n"})),
		when("2026-09-27-120000.md", text("@S DONE .agent-one/handoffs/2026-09-27-120000.md\n@E 40")))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	code, out, errb := w.exec(agentOne, "/handoff logout next\n/quit\n", nil, "--quiet")
	if code != 0 || !strings.Contains(errb, "handoff → a turn") || !strings.Contains(w.read(".agent-one/handoffs/2026-09-27-120000.md"), "DONE: login") {
		t.Fatalf("write: %d\n%s\n%s", code, out, errb)
	}
	code, out, errb = w.exec(agentOne, "/handoff read\n/quit\n", nil, "--quiet")
	if code != 0 || !strings.Contains(errb, "handoff .agent-one/handoffs/2026-09-27-120000.md → a turn") || !strings.Contains(out, "read and waiting") {
		t.Fatalf("read: %d\n%s\n%s", code, out, errb)
	}
}

// A goal keeps working until its validation passes: the binary runs the command after each turn
// and hands the failure back; `@? human:` pauses it.
func TestE2EGoal(t *testing.T) {
	agentOne, _ := binaries(t)
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("Validation after turn 1", call("write", map[string]string{"path": "done.txt", "content": "ok"})),
		when("You are working to a goal", call("write", map[string]string{"path": "notyet.txt", "content": "x"})),
		when("done.txt", text("@S DONE wrote done.txt\n@E 5")),
		when("notyet.txt", text("@S DONE wrote notyet.txt\n@E 5")))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	code, out, errb := w.exec(agentOne, "", nil, "goal", "--quiet", "--validate", "test -f done.txt", "make done.txt exist")
	if code != 0 || !strings.Contains(out, "goal met after 2 turns") || !strings.Contains(errb, "goal · validation exit 1") {
		t.Fatalf("goal: %d\n%s\n%s", code, out, errb)
	}
	w2 := newTestWorkspace(t, "agent-one", agentOneMembers())
	w2.script(".agent-one/tmp/s.json", when("You are working to a goal", text("I need the product owner.\n@? human: which provider to bill?")))
	w2.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	code, out, errb = w2.exec(agentOne, "", nil, "goal", "--quiet", "--validate", "false", "bill the right provider")
	if code != 3 || !strings.Contains(errb, "paused at turn 1 for the operator: which provider to bill?") {
		t.Fatalf("pause: %d\n%s\n%s", code, out, errb)
	}
}

// /review: two read-only reviewers in parallel (a write is refused, not asked), one merged
// shortlist; nothing fixed.
func TestE2EReview(t *testing.T) {
	agentOne, _ := binaries(t)
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	git := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", w.root, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-qm", "base")
	w.write("src/auth/login.go", "package auth\n\nfunc Login() *int { return nil }\n")
	w.script(".agent-one/tmp/s.json",
		when("Two independent reviewers", text("1. Login returns nil — src/auth/login.go:3 [both]\nDropped 0 as overthinking.\nApprove fixing these, or adjust the list?")),
		when("as a thorough senior developer", call("write", map[string]string{"path": "src/auth/login.go", "content": "fixed"})),
		when("a reviewer reads only", text("Serious: src/auth/login.go:3 Login returns nil. Not ready to merge.")))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	code, out, errb := w.exec(agentOne, "", nil, "review", "--quiet")
	if code != 0 || !strings.Contains(out, "[both]") || !strings.Contains(errb, "review: [judge] reported") || !strings.Contains(errb, "review: [drafter] reported") {
		t.Fatalf("review: %d\n%s\n%s", code, out, errb)
	}
	if strings.Contains(w.read("src/auth/login.go"), "fixed") {
		t.Fatal("a reviewer wrote a file")
	}
}
