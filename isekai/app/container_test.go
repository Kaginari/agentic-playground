package app

import (
	"strings"
	"testing"
)

func TestContainerArgsMountTheWorldReadWriteAndTheRestReadOnly(t *testing.T) {
	s := ContainerSpec{Dist: "isekai", Image: "isekai-runtime:x", Exe: "/opt/isekai", Root: "/w", Cwd: "/w/src", Home: "/home/u", UID: 1000, GID: 1000,
		RO: []string{"/home/u/.config/isekai"}, RW: []string{"/home/u/.local/share/isekai"}, EnvKeys: []string{"OPENROUTER_API_KEY"}, Argv: []string{"run", "hi"}}
	got := strings.Join(s.Args(), " ")
	for _, want := range []string{
		"-v /w:/w ", "-v /opt/isekai:/usr/local/bin/isekai:ro", "-v /home/u/.config/isekai:/home/u/.config/isekai:ro",
		"-v /home/u/.local/share/isekai:/home/u/.local/share/isekai ", "--user 1000:1000", "-e OPENROUTER_API_KEY ",
		"-e ISEKAI_CONTAINERED=1", "-w /w/src isekai-runtime:x /usr/local/bin/isekai run hi", "--network host",
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
	keys := strings.Join(passEnv("isekai", []string{"PATH=/bin", "HOME=/h", "OPENROUTER_API_KEY=k", "ISEKAI_THEME=light", "ISEKAI_CONTAINERED=1", "TERM=xterm", "AWS_SECRET=s"}), " ")
	if keys != "ISEKAI_THEME OPENROUTER_API_KEY TERM" {
		t.Fatalf("passed env %q", keys)
	}
	if !strings.HasPrefix(DefaultRuntimeImage("isekai"), "isekai-runtime:") {
		t.Fatal(DefaultRuntimeImage("isekai"))
	}
}

func TestContaineredIsANoOpInside(t *testing.T) {
	io := IO{Env: func(k string) string {
		if k == "ISEKAI_CONTAINERED" {
			return "1"
		}
		return ""
	}}
	if _, handled := Containered("isekai", []string{"--containered", "status"}, io); handled {
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
		w := newTestWorld(t, "isekai", isekaiCreatures())
		w.write(".isekai/config.yaml", setupYAML(c))
		a := w.open()
		if got := a.Cfg.Models.Default.Model; got != c.model {
			t.Fatalf("%s: the world runs on %q, want %q", name, got, c.model)
		}
		if p := a.Cfg.Providers[c.provider]; p == nil || !p.Enabled {
			t.Fatalf("%s: provider %s not enabled", name, c.provider)
		}
	}
}

func TestGuardPatternsFromConfig(t *testing.T) {
	w := newTestWorld(t, "isekai", isekaiCreatures())
	w.write(".isekai/guards.yaml", "- '(^|[[:space:]])terraform[[:space:]]+destroy'\n")
	a := w.open()
	g := a.theGuard()
	if r := g.Match("terraform destroy -auto-approve"); r == nil || !strings.Contains(r.Source, "guards.yaml") {
		t.Fatalf("a pattern in guards.yaml blocks, named by its file: %+v", r)
	}
	if g.Match("rm -rf ~") == nil {
		t.Fatal("the built-in list stays")
	}
	if !strings.Contains(a.guardLine(), "3 sources") {
		t.Fatalf("status counts the config as a source: %s", a.guardLine())
	}
}
