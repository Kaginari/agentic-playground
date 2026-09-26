// Package shell is one persistent bash per agent: cwd and exported variables survive between
// calls, every command is framed with a unique sentinel that carries its exit code and the
// resulting cwd, a timeout kills the whole tree a command started, and `restart` throws the
// session away. Background jobs are named, logged to a file, listed, killed on close. The
// session runs inside the sandbox; a command the gate let reach the network runs in a
// one-shot net-enabled shell seeded with the session's exports and cwd.
package shell

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Kaginari/agent-one/sandbox"
)

// Options configures a session.
type Options struct {
	Root      string           // the workspace root; the default cwd
	Cwd       string           // starting cwd (default Root)
	Sandbox   *sandbox.Sandbox // nil: no sandbox, env still scrubbed with the default rules
	Env       []string         // base environment (default os.Environ()); scrubbed before use
	OutputCap int              // bytes kept per command (default 64 KiB); the rest is counted
	JobsDir   string           // background job logs (default <Root>/.agent-one/instruments/jobs)
	Timeout   time.Duration    // default per command (default 10 min)
	Shell     string           // the shell binary (default "bash")
}

// RunOptions varies per command.
type RunOptions struct {
	Timeout time.Duration
	Network bool // the gate passed this command as outward: it may reach the network
}

// Result is one command's reading.
type Result struct {
	Output    string
	Exit      int
	Cwd       string
	Duration  time.Duration
	TimedOut  bool
	Clipped   int  // bytes dropped past OutputCap
	Restarted bool // the session was restarted (timeout that would not yield, or death)
}

// Job is a background job.
type Job struct {
	Name    string
	Command string
	PID     int
	Log     string
	Started time.Time
	Done    bool
	Exit    int
	Ended   time.Time
}

// Session is the persistent shell.
type Session struct {
	opt  Options
	mu   sync.Mutex
	proc *proc
	cwd  string
	jobs map[string]*jobState
	dead string
}

type jobState struct {
	Job
	cmd    *exec.Cmd
	cancel context.CancelFunc
	mu     sync.Mutex
}

// New starts a session.
func New(opt Options) (*Session, error) {
	if opt.Root == "" {
		opt.Root, _ = os.Getwd()
	}
	if opt.Cwd == "" {
		opt.Cwd = opt.Root
	}
	if opt.OutputCap <= 0 {
		opt.OutputCap = 64 << 10
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 10 * time.Minute
	}
	if opt.Shell == "" {
		opt.Shell = "bash"
	}
	if opt.JobsDir == "" {
		opt.JobsDir = filepath.Join(opt.Root, ".agent-one", "instruments", "jobs")
	}
	if opt.Env == nil {
		opt.Env = os.Environ()
	}
	s := &Session{opt: opt, cwd: opt.Cwd, jobs: map[string]*jobState{}}
	if err := s.start(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) env() []string {
	env := s.opt.Sandbox.Scrub(s.opt.Env)
	return append(env, "AGENT_ONE_ROOT="+s.opt.Root, "TERM=dumb", "PS1=", "PS2=")
}

func (s *Session) start() error {
	p, err := startProc(s.opt.Sandbox, sandbox.Call{Cwd: s.cwd}, s.env(), s.opt.Shell)
	if err != nil {
		return err
	}
	s.proc = p
	s.dead = ""
	return nil
}

// Cwd is the session's current directory.
func (s *Session) Cwd() string { s.mu.Lock(); defer s.mu.Unlock(); return s.cwd }

// Sandboxed reports the sandbox mode the session runs under.
func (s *Session) Sandboxed() sandbox.Mode { return s.opt.Sandbox.Mode() }

// Restart throws the shell away (cwd back to the start, exports lost) and starts a fresh one.
func (s *Session) Restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restartLocked()
}

func (s *Session) restartLocked() error {
	if s.proc != nil {
		s.proc.kill()
	}
	s.cwd = s.opt.Cwd
	return s.start()
}

// Run executes cmd in the session and returns its reading. A timeout kills every process the
// command started; the shell then finishes the rest of the command's list within a short grace
// (reported in the same reading) or, if it will not yield (a builtin loop), is restarted.
func (s *Session) Run(ctx context.Context, cmd string, ro RunOptions) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ro.Timeout <= 0 {
		ro.Timeout = s.opt.Timeout
	}
	restarted := false
	if s.proc == nil || !s.proc.alive() {
		if err := s.restartLocked(); err != nil {
			return Result{}, err
		}
		restarted = true
	}
	if ro.Network && s.opt.Sandbox.Mode() == sandbox.Bwrap {
		r, err := s.oneShot(ctx, cmd, ro)
		r.Restarted = restarted
		return r, err
	}
	r, err := s.proc.run(ctx, cmd, ro.Timeout, s.opt.OutputCap)
	r.Restarted = restarted
	if err != nil {
		if errors.Is(err, errDead) || errors.Is(err, errStuck) {
			// The shell is gone or would not yield: start over, keep the reading.
			why := err.Error()
			if rerr := s.restartLocked(); rerr != nil {
				return r, fmt.Errorf("%s; restart failed: %v", why, rerr)
			}
			r.Restarted = true
			r.Output += "\n[" + why + " — session restarted; cwd and exports reset]"
			return r, nil
		}
		return r, err
	}
	if r.Cwd != "" {
		s.cwd = r.Cwd
	}
	return r, nil
}

// oneShot runs cmd in a fresh net-enabled shell seeded with the session's exports and cwd,
// then carries the resulting cwd back.
func (s *Session) oneShot(ctx context.Context, cmd string, ro RunOptions) (Result, error) {
	snap, err := s.proc.run(ctx, "export -p", 30*time.Second, 1<<20)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot of the session's exports: %w", err)
	}
	p, err := startProc(s.opt.Sandbox, sandbox.Call{Cwd: s.cwd, Network: true}, s.env(), s.opt.Shell)
	if err != nil {
		return Result{}, err
	}
	defer p.kill()
	if _, err := p.run(ctx, snap.Output, 30*time.Second, 4096); err != nil {
		return Result{}, fmt.Errorf("seeding the net-enabled shell: %w", err)
	}
	r, err := p.run(ctx, cmd, ro.Timeout, s.opt.OutputCap)
	if err != nil && !errors.Is(err, errDead) && !errors.Is(err, errStuck) {
		return r, err
	}
	if r.Cwd != "" && r.Cwd != s.cwd {
		if _, cerr := s.proc.run(ctx, "cd "+shellQuote(r.Cwd), 10*time.Second, 4096); cerr == nil {
			s.cwd = r.Cwd
		}
	}
	return r, nil
}

// Background starts cmd as a named job; its output goes to <JobsDir>/<name>.log.
func (s *Session) Background(name, cmd string, network bool) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		name = "job-" + nonce()[:6]
	}
	if !jobName.MatchString(name) {
		return Job{}, fmt.Errorf("job name %q: letters, digits, - _ . only", name)
	}
	if j, ok := s.jobs[name]; ok && !j.done() {
		return Job{}, fmt.Errorf("job %q is still running (pid %d)", name, j.PID)
	}
	if err := os.MkdirAll(s.opt.JobsDir, 0o755); err != nil {
		return Job{}, err
	}
	log := filepath.Join(s.opt.JobsDir, name+".log")
	f, err := os.Create(log)
	if err != nil {
		return Job{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := s.opt.Sandbox.Command(ctx, sandbox.Call{Cwd: s.cwd, Network: network}, s.env(), s.opt.Shell, "-c", cmd)
	c.Stdout, c.Stderr = f, f
	c.Stdin = nil
	if err := c.Start(); err != nil {
		cancel()
		f.Close()
		return Job{}, err
	}
	j := &jobState{Job: Job{Name: name, Command: cmd, PID: c.Process.Pid, Log: log, Started: time.Now()}, cmd: c, cancel: cancel}
	s.jobs[name] = j
	go func() {
		err := c.Wait()
		f.Close()
		j.mu.Lock()
		j.Done, j.Ended = true, time.Now()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				j.Exit = ee.ExitCode()
			} else {
				j.Exit = 1
			}
		}
		j.mu.Unlock()
	}()
	return j.snapshot(), nil
}

var jobName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func (j *jobState) done() bool { j.mu.Lock(); defer j.mu.Unlock(); return j.Done }

func (j *jobState) snapshot() Job { j.mu.Lock(); defer j.mu.Unlock(); return j.Job }

// Jobs lists background jobs, oldest first.
func (s *Session) Jobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j.snapshot())
	}
	for i := 1; i < len(out); i++ {
		for k := i; k > 0 && out[k].Started.Before(out[k-1].Started); k-- {
			out[k], out[k-1] = out[k-1], out[k]
		}
	}
	return out
}

// Kill ends a job (the whole tree it started).
func (s *Session) Kill(name string) error {
	s.mu.Lock()
	j, ok := s.jobs[name]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no job %q", name)
	}
	if j.done() {
		return nil
	}
	killTree(j.PID)
	j.cancel()
	return nil
}

// Close kills every job and the shell.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if !j.done() {
			killTree(j.PID)
			j.cancel()
		}
	}
	if s.proc != nil {
		s.proc.kill()
		s.proc = nil
	}
}

// ---- the framed process

var (
	errDead  = errors.New("shell session died")
	errStuck = errors.New("command timed out and the shell would not yield")
)

type proc struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	lines    chan string
	eof      chan struct{}
	shellPid int // the shell itself (a grandchild under bwrap); its group is spared by killChildren
	mu       sync.Mutex
	gone     bool // the shell exited (its EXIT trap spoke); EOF follows
}

// exitTrap prints the sentinel of the running frame when the shell exits (an `exit N` inside a
// command), so the reading still carries the code; the caller then sees EOF and restarts.
const exitTrap = "trap 'printf \"%s+exit %d %s\\n\" \"$__isk_end\" \"$?\" \"$PWD\"' EXIT\n"

func startProc(sb *sandbox.Sandbox, call sandbox.Call, env []string, shell string) (*proc, error) {
	cmd := sb.Command(context.Background(), call, env, shell, "--noprofile", "--norc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", shell, err)
	}
	p := &proc{cmd: cmd, stdin: stdin, lines: make(chan string, 1024), eof: make(chan struct{})}
	p.shellPid = cmd.Process.Pid
	go func() {
		r := bufio.NewReaderSize(stdout, 64<<10)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				p.lines <- line
			}
			if err != nil {
				break
			}
		}
		close(p.eof)
		_ = cmd.Wait()
		p.mu.Lock()
		p.gone = true
		p.mu.Unlock()
	}()
	if _, err := io.WriteString(stdin, exitTrap); err != nil {
		p.kill()
		return nil, fmt.Errorf("init %s: %w", shell, err)
	}
	r, err := p.run(context.Background(), "echo $$", 10*time.Second, 64)
	if err != nil {
		p.kill()
		return nil, fmt.Errorf("init %s: %w", shell, err)
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(r.Output)); err == nil && pid > 0 {
		p.shellPid = pid
	}
	return p, nil
}

func (p *proc) alive() bool {
	p.mu.Lock()
	gone := p.gone
	p.mu.Unlock()
	if gone {
		return false
	}
	select {
	case <-p.eof:
		return false
	default:
		return true
	}
}

// sentinel finds the frame's end marker in a line: the output before it, the exit code, the
// cwd, and whether the shell itself exited (the EXIT trap spoke instead of the frame).
func sentinel(line, end string) (before string, code int, cwd string, exited, ok bool) {
	i := strings.Index(line, end)
	if i < 0 {
		return "", 0, "", false, false
	}
	rest := line[i+len(end):]
	if strings.HasPrefix(rest, "+exit ") {
		exited, rest = true, rest[len("+exit "):]
	} else if strings.HasPrefix(rest, " ") {
		rest = rest[1:]
	} else {
		return "", 0, "", false, false
	}
	rest = strings.TrimRight(rest, "\n")
	sp := strings.SplitN(rest, " ", 2)
	code, _ = strconv.Atoi(sp[0])
	if len(sp) == 2 {
		cwd = sp[1]
	}
	return line[:i], code, cwd, exited, true
}

func (p *proc) kill() {
	killTree(p.cmd.Process.Pid)
	_ = p.stdin.Close()
	select {
	case <-p.eof:
	case <-time.After(2 * time.Second):
	}
}

func nonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// run frames cmd and reads until its sentinel. Output is merged stdout+stderr.
func (p *proc) run(ctx context.Context, cmd string, timeout time.Duration, capBytes int) (Result, error) {
	if !p.alive() {
		return Result{}, errDead
	}
	id := nonce()
	hd, end := "__ISK_HD_"+id, "__ISK_END_"+id
	if strings.Contains(cmd, hd) {
		return Result{}, errors.New("command contains the frame delimiter")
	}
	frame := fmt.Sprintf("IFS= read -r -d '' __isk_cmd <<'%s'\n%s\n%s\n__isk_end=%s\neval \"$__isk_cmd\" < /dev/null 2>&1\n__isk_rc=$?\nprintf '%s %%d %%s\\n' \"$__isk_rc\" \"$PWD\"\n", hd, cmd, hd, end, end)
	t0 := time.Now()
	for drained := false; !drained; { // stray lines from an earlier `&` process
		select {
		case <-p.lines:
		default:
			drained = true
		}
	}
	if _, err := io.WriteString(p.stdin, frame); err != nil {
		return Result{}, errDead
	}
	var out strings.Builder
	clipped := 0
	keep := func(s string) {
		room := capBytes - out.Len()
		if room <= 0 {
			clipped += len(s)
			return
		}
		if len(s) > room {
			clipped += len(s) - room
			s = s[:room]
		}
		out.WriteString(s)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	timedOut := false
	for {
		select {
		case line := <-p.lines:
			if before, code, cwd, exited, ok := sentinel(line, end); ok {
				keep(before)
				if exited {
					p.mu.Lock()
					p.gone = true
					p.mu.Unlock()
				}
				r := Result{Output: out.String(), Exit: code, Cwd: cwd, Duration: time.Since(t0), Clipped: clipped, TimedOut: timedOut}
				if timedOut {
					r.Exit = 124
				}
				return r, nil
			}
			keep(line)
		case <-p.eof:
			return Result{Output: out.String(), Exit: -1, Duration: time.Since(t0), Clipped: clipped}, errDead
		case <-ctx.Done():
			timedOut = true
			p.killChildren()
			if r, ok := p.drain(&out, keep, end); ok {
				r.TimedOut, r.Exit, r.Clipped = true, 124, clipped
				return r, nil
			}
			return Result{Output: out.String(), Exit: 124, TimedOut: true, Duration: time.Since(t0), Clipped: clipped}, errStuck
		case <-timer.C:
			timedOut = true
			p.killChildren()
			if r, ok := p.drain(&out, keep, end); ok {
				r.TimedOut, r.Exit, r.Clipped = true, 124, clipped
				r.Duration = time.Since(t0)
				return r, nil
			}
			return Result{Output: out.String(), Exit: 124, TimedOut: true, Duration: time.Since(t0), Clipped: clipped}, errStuck
		}
	}
}

// drain waits briefly for the sentinel after the children were killed; a builtin loop never
// yields it, and the caller then restarts the shell.
func (p *proc) drain(out *strings.Builder, keep func(string), end string) (Result, bool) {
	grace := time.After(700 * time.Millisecond)
	for {
		select {
		case line := <-p.lines:
			if before, _, cwd, exited, ok := sentinel(line, end); ok {
				keep(before)
				if exited {
					p.mu.Lock()
					p.gone = true
					p.mu.Unlock()
				}
				return Result{Output: out.String(), Cwd: cwd}, true
			}
			keep(line)
		case <-p.eof:
			return Result{}, false
		case <-grace:
			return Result{}, false
		}
	}
}

// killChildren kills everything under the shell but the shell itself: every descendant by
// pid, and every process group they started that is not the shell's own.
func (p *proc) killChildren() {
	spare, _ := syscall.Getpgid(p.shellPid)
	for _, pid := range descendants(p.shellPid) {
		killPid(pid, spare)
	}
}

// killTree kills pid and every process under it, by process group and by pid.
func killTree(pid int) {
	pids := append(descendants(pid), pid)
	for _, id := range pids {
		killPid(id, 0)
	}
}

func killPid(pid, sparePgid int) {
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid > 1 && pgid != sparePgid && pgid != os.Getpid() && pgid != syscall.Getpgrp() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// descendants lists every process under pid, deepest last, from /proc.
func descendants(pid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	children := map[int][]int{}
	for _, e := range entries {
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		s := string(stat)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(s[i+1:])
		if len(f) < 2 {
			continue
		}
		ppid, _ := strconv.Atoi(f[1])
		children[ppid] = append(children[ppid], id)
	}
	var out []int
	queue := []int{pid}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range children[cur] {
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
