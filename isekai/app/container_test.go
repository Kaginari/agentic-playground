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
