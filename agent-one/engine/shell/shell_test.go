package shell

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/sandbox"
)

func newSession(t *testing.T, sb *sandbox.Sandbox) (*Session, string) {
	t.Helper()
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	s, err := New(Options{Root: root, Sandbox: sb, Timeout: 5 * time.Second, OutputCap: 200, Env: []string{"PATH=" + os.Getenv("PATH"), "MY_API_KEY=leak", "HOME=" + root}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, root
}

func TestPersistenceAndFraming(t *testing.T) {
	s, root := newSession(t, nil)
	ctx := context.Background()
	r, err := s.Run(ctx, "export FOO=bar; cd sub; printf 'no newline'", RunOptions{})
	if err != nil || r.Exit != 0 || r.Output != "no newline" || r.Cwd != filepath.Join(root, "sub") {
		t.Fatalf("%+v %v", r, err)
	}
	r, _ = s.Run(ctx, "echo $FOO; pwd; echo err >&2; (exit 3)", RunOptions{})
	if r.Exit != 3 || !strings.Contains(r.Output, "bar\n"+filepath.Join(root, "sub")+"\nerr\n") {
		t.Fatalf("%+v", r)
	}
	if s.Cwd() != filepath.Join(root, "sub") {
		t.Fatal("cwd not kept")
	}
	// Broken syntax does not eat the frame.
	r, err = s.Run(ctx, "echo 'unbalanced ) quote", RunOptions{})
	if err != nil || r.Exit == 0 {
		t.Fatalf("syntax error should be a non-zero reading: %+v %v", r, err)
	}
	r, _ = s.Run(ctx, "echo still-alive", RunOptions{})
	if r.Output != "still-alive\n" {
		t.Fatalf("%+v", r)
	}
	// Env scrubbed; the root is known.
	r, _ = s.Run(ctx, `echo "${MY_API_KEY:-scrubbed} $AGENT_ONE_ROOT"`, RunOptions{})
	if r.Output != "scrubbed "+root+"\n" {
		t.Fatalf("%+v", r)
	}
	// Output cap.
	r, _ = s.Run(ctx, "seq 1 1000", RunOptions{})
	if len(r.Output) != 200 || r.Clipped == 0 {
		t.Fatalf("cap: %d %d", len(r.Output), r.Clipped)
	}
	// stdin does not swallow the frame.
	r, _ = s.Run(ctx, "cat; echo after", RunOptions{})
	if r.Output != "after\n" {
		t.Fatalf("%+v", r)
	}
}

func TestTimeoutKillsTree(t *testing.T) {
	s, _ := newSession(t, nil)
	ctx := context.Background()
	t0 := time.Now()
	r, err := s.Run(ctx, "export KEEP=1; bash -c 'sleep 30' & sleep 30; echo rest-of-list", RunOptions{Timeout: 400 * time.Millisecond})
	if err != nil || !r.TimedOut || r.Exit != 124 || !strings.Contains(r.Output, "rest-of-list") {
		t.Fatalf("%+v %v", r, err)
	}
	if left := descendants(s.proc.shellPid); len(left) != 0 {
		t.Fatalf("processes survived the kill under the shell: %v", left)
	}
	if time.Since(t0) > 5*time.Second {
		t.Fatal("kill was slow")
	}
	if r.Restarted {
		t.Fatalf("a yielding timeout must not restart: %+v", r)
	}
	r, _ = s.Run(ctx, "echo ${KEEP:-lost}", RunOptions{})
	if r.Output != "1\n" {
		t.Fatalf("exports should survive a yielding timeout: %+v", r)
	}
	// A builtin loop never yields: the session restarts.
	r, err = s.Run(ctx, "while :; do :; done", RunOptions{Timeout: 300 * time.Millisecond})
	if err != nil || !r.Restarted || !r.TimedOut {
		t.Fatalf("%+v %v", r, err)
	}
	r, _ = s.Run(ctx, "echo ${KEEP:-lost}", RunOptions{})
	if r.Output != "lost\n" {
		t.Fatalf("restart should reset exports: %+v", r)
	}
}

func TestDeathAndRestart(t *testing.T) {
	s, root := newSession(t, nil)
	ctx := context.Background()
	r, err := s.Run(ctx, "cd sub; exit 7", RunOptions{})
	if err != nil || r.Exit != 7 || r.Cwd != filepath.Join(root, "sub") {
		t.Fatalf("exit should still be read: %+v %v", r, err)
	}
	r, _ = s.Run(ctx, "pwd", RunOptions{})
	if r.Output != root+"\n" || !r.Restarted {
		t.Fatalf("the next call restarts: %+v", r)
	}
	_, _ = s.Run(ctx, "cd sub", RunOptions{})
	if err := s.Restart(); err != nil {
		t.Fatal(err)
	}
	if s.Cwd() != root {
		t.Fatal("restart resets cwd")
	}
}

func TestBackgroundJobs(t *testing.T) {
	s, root := newSession(t, nil)
	j, err := s.Background("ticker", "echo start; sleep 30; echo end", false)
	if err != nil || j.PID == 0 || !strings.HasPrefix(j.Log, filepath.Join(root, ".agent-one", "instruments", "jobs")) {
		t.Fatalf("%+v %v", j, err)
	}
	if _, err := s.Background("ticker", "true", false); err == nil {
		t.Fatal("duplicate running name must be refused")
	}
	if _, err := s.Background("bad name", "true", false); err == nil {
		t.Fatal("bad name refused")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, _ := os.ReadFile(j.Log)
		if strings.Contains(string(b), "start") || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if b, _ := os.ReadFile(j.Log); !strings.Contains(string(b), "start") {
		t.Fatal("job output not logged")
	}
	quick, _ := s.Background("", "exit 5", false)
	time.Sleep(300 * time.Millisecond)
	jobs := s.Jobs()
	if len(jobs) != 2 || jobs[0].Name != "ticker" || jobs[0].Done {
		t.Fatalf("%+v", jobs)
	}
	if jobs[1].Name != quick.Name || !jobs[1].Done || jobs[1].Exit != 5 {
		t.Fatalf("%+v", jobs[1])
	}
	if err := s.Kill("ticker"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if !s.Jobs()[0].Done {
		t.Fatal("killed job should be done")
	}
	if err := s.Kill("nope"); err == nil {
		t.Fatal("unknown job")
	}
	j2, _ := s.Background("closer", "sleep 30", false)
	s.Close()
	time.Sleep(200 * time.Millisecond)
	if err := checkPid(j2.PID); err == nil {
		t.Fatal("close must kill jobs")
	}
}

func checkPid(pid int) error {
	_, err := os.Stat(filepath.Join("/proc", itoa(pid), "cmdline"))
	if err != nil {
		return err
	}
	st, err := os.ReadFile(filepath.Join("/proc", itoa(pid), "stat"))
	if err != nil {
		return err
	}
	if strings.Contains(string(st), ") Z ") {
		return os.ErrNotExist
	}
	return nil
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestSandboxedNetworkOneShot(t *testing.T) {
	root := t.TempDir()
	sb := sandbox.New(sandbox.Options{Enabled: true, Root: root})
	if sb.Mode() != sandbox.Bwrap {
		t.Skip("bwrap unavailable: " + sb.Why())
	}
	_ = os.MkdirAll(filepath.Join(root, "sub", "deeper"), 0o755)
	s, err := New(Options{Root: root, Sandbox: sb, Timeout: 5 * time.Second, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if s.Sandboxed() != sandbox.Bwrap {
		t.Fatal("session should be sandboxed")
	}
	if r, _ := s.Run(ctx, "touch /usr/nope 2>&1; echo rc=$?", RunOptions{}); !strings.Contains(r.Output, "rc=1") {
		t.Fatalf("read-only /: %+v", r)
	}
	if r, _ := s.Run(ctx, "echo ok > sub/w && cat sub/w", RunOptions{}); r.Output != "ok\n" {
		t.Fatalf("rw root: %+v", r)
	}
	ifaces := "tail -n +3 /proc/net/dev | grep -v lo: | wc -l"
	if r, _ := s.Run(ctx, ifaces, RunOptions{}); strings.TrimSpace(r.Output) != "0" {
		t.Fatalf("session must have no network: %+v", r)
	}
	_, _ = s.Run(ctx, "export FOO=seeded; cd sub", RunOptions{})
	r, err := s.Run(ctx, "echo $FOO; pwd; cd deeper", RunOptions{Network: true})
	if err != nil || !strings.HasPrefix(r.Output, "seeded\n"+filepath.Join(root, "sub")+"\n") {
		t.Fatalf("net one-shot should carry exports and cwd: %+v %v", r, err)
	}
	if s.Cwd() != filepath.Join(root, "sub", "deeper") {
		t.Fatalf("cwd from the net shell should carry back: %s", s.Cwd())
	}
	if r, _ := s.Run(ctx, "pwd", RunOptions{}); r.Output != filepath.Join(root, "sub", "deeper")+"\n" {
		t.Fatalf("%+v", r)
	}
}
