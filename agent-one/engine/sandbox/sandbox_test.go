package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScrubEnv(t *testing.T) {
	env := []string{"HOME=/h", "ANTHROPIC_API_KEY=sk", "GITHUB_TOKEN=x", "DB_SECRET=y", "PG_PASSWORD=z", "api_key=lower", "TOKEN=bare", "AWS_ACCESS_KEY_ID=a", "KEEP_TOKEN=k", "PATH=/bin", "NOTASECRET=1"}
	got := ScrubEnv(env, []string{"AWS_*"}, []string{"KEEP_TOKEN"})
	want := "HOME=/h PATH=/bin NOTASECRET=1 KEEP_TOKEN=k"
	for _, w := range strings.Fields(want) {
		if !contains(got, w) {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	if len(got) != 4 {
		t.Errorf("want 4 survivors, got %v", got)
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestNoneMode(t *testing.T) {
	s := New(Options{Enabled: false})
	if s.Mode() != None || s.Why() != "disabled by config" {
		t.Fatalf("%s %s", s.Mode(), s.Why())
	}
	if got := s.Wrap(Call{}, []string{"ls"}); len(got) != 1 {
		t.Fatal("none mode wraps nothing")
	}
	var nilS *Sandbox
	if nilS.Mode() != None || nilS.Status() == "" {
		t.Fatal("nil sandbox is None")
	}
	bad := New(Options{Enabled: true, Bwrap: "/nonexistent/bwrap"})
	if bad.Mode() != None || bad.Why() == "" {
		t.Fatalf("missing binary: %s %q", bad.Mode(), bad.Why())
	}
}

func requireBwrap(t *testing.T, root string) *Sandbox {
	t.Helper()
	s := New(Options{Enabled: true, Root: root})
	if s.Mode() != Bwrap {
		t.Skip("bwrap unavailable: " + s.Why())
	}
	return s
}

func TestBwrapConfinement(t *testing.T) {
	root := t.TempDir()
	s := requireBwrap(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	run := func(call Call, script string) (string, error) {
		cmd := s.Command(ctx, call, []string{"PATH=/usr/bin:/bin", "MY_TOKEN=leak", "SAFE=1"}, "bash", "-c", script)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := run(Call{Cwd: root}, "echo hi > inside.txt && cat inside.txt"); err != nil || out != "hi" {
		t.Fatalf("root is rw: %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, "inside.txt")); err != nil {
		t.Fatal("write did not land on the host")
	}
	if _, err := run(Call{Cwd: root}, "touch /usr/agent-one-should-fail"); err == nil {
		t.Fatal("/ must be read-only")
	}
	if out, err := run(Call{Cwd: root}, "echo x > /tmp/priv && cat /tmp/priv"); err != nil || out != "x" {
		t.Fatalf("private /tmp: %q %v", out, err)
	}
	if _, err := os.Stat("/tmp/priv"); err == nil {
		t.Fatal("/tmp must be private")
	}
	if out, _ := run(Call{Cwd: root}, "echo \"${MY_TOKEN:-scrubbed}\" \"$SAFE\""); out != "scrubbed 1" {
		t.Fatalf("env scrub: %q", out)
	}
	ifaces := "tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d ' ' | grep -v '^lo$' | wc -l"
	if out, _ := run(Call{Cwd: root}, ifaces); out != "0" {
		t.Fatalf("network should be unshared, saw interfaces: %q", out)
	}
	if out, _ := run(Call{Cwd: root, Network: true}, ifaces); out == "0" {
		t.Log("network call sees no interface beyond lo (host may have none)")
	}
	if !strings.Contains(s.Status(), "bwrap") {
		t.Fatal(s.Status())
	}
}
