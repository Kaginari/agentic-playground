package app

import (
	"strings"
	"testing"
)

func TestContainerArgsMountTheWorldReadWriteAndTheRestReadOnly(t *testing.T) {
	s := ContainerSpec{Dist: "agent-one", Image: "agent-one-runtime:x", Exe: "/opt/agent-one", Root: "/w", Cwd: "/w/src", Home: "/home/u", UID: 1000, GID: 1000,
		RO: []string{"/home/u/.config/agent-one"}, RW: []string{"/home/u/.local/share/agent-one"}, EnvKeys: []string{"OPENROUTER_API_KEY"}, Argv: []string{"run", "hi"}}
	got := strings.Join(s.Args(), " ")
	for _, want := range []string{
		"-v /w:/w ", "-v /opt/agent-one:/usr/local/bin/agent-one:ro", "-v /home/u/.config/agent-one:/home/u/.config/agent-one:ro",
		"-v /home/u/.local/share/agent-one:/home/u/.local/share/agent-one ", "--user 1000:1000", "-e OPENROUTER_API_KEY ",
		"-e AGENT_ONE_CONTAINERED=1", "-w /w/src agent-one-runtime:x /usr/local/bin/agent-one run hi", "--network host",
	} {
		if !strings.Contains(got+" ", want) {
			t.Fatalf("docker argv misses %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "=sk-") || strings.Contains(got, " -t ") {
		t.Fatalf("a key value or a TTY without one: %s", got)
	}
}

func TestContainerFlagsAndEnv(t *testing.T) {
	rest, on, img := stripContainerFlags([]string{"--containered", "--image", "my:img", "run", "x", "--image=ignored-later"})
	if !on || img != "ignored-later" || strings.Join(rest, " ") != "run x" {
		t.Fatalf("strip: %v %v %q", rest, on, img)
	}
	keys := strings.Join(passEnv("agent-one", []string{"PATH=/bin", "HOME=/h", "OPENROUTER_API_KEY=k", "AGENT_ONE_THEME=light", "AGENT_ONE_CONTAINERED=1", "TERM=xterm", "AWS_SECRET=s"}), " ")
	if keys != "AGENT_ONE_THEME OPENROUTER_API_KEY TERM" {
		t.Fatalf("passed env %q", keys)
	}
	if !strings.HasPrefix(DefaultRuntimeImage("agent-one"), "agent-one-runtime:") {
		t.Fatal(DefaultRuntimeImage("agent-one"))
	}
}

func TestContaineredIsANoOpInside(t *testing.T) {
	io := IO{Env: func(k string) string {
		if k == "AGENT_ONE_CONTAINERED" {
			return "1"
		}
		return ""
	}}
	if _, handled := Containered("agent-one", []string{"--containered", "status"}, io); handled {
		t.Fatal("inside the container the flag must not start another one")
	}
}

func TestSetupYAMLLoads(t *testing.T) {
	for name, c := range setupPresets {
		if c.model == "openai/" || c.model == "ollama/" || c.model == "vllm/" {
			c.model += "some-model"
		}
		y := setupYAML(c)
		if !strings.Contains(y, "default:") || !strings.Contains(y, c.provider+":") {
			t.Fatalf("%s: %s", name, y)
		}
		if c.keyEnv != "" && !strings.Contains(y, "apiKeyEnv: "+c.keyEnv) {
			t.Fatalf("%s: the key's name is missing: %s", name, y)
		}
	}
}

func TestSetupConfigOpensTheWorld(t *testing.T) {
	for name, c := range setupPresets {
		if strings.HasSuffix(c.model, "/") {
			c.model += "some-model"
		}
		w := newTestWorkspace(t, "agent-one", agentOneMembers())
		w.write(".agent-one/config.yaml", setupYAML(c))
		a := w.open()
		if got := a.Cfg.Models.Default.Model; got != c.model {
			t.Fatalf("%s: the workspace runs on %q, want %q", name, got, c.model)
		}
		if p := a.Cfg.Providers[c.provider]; p == nil || !p.Enabled {
			t.Fatalf("%s: provider %s not enabled", name, c.provider)
		}
	}
}
