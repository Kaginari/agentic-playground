// Package sandbox runs a command under bubblewrap: the filesystem read-only except the workspace
// root and a private /tmp, no network unless the call passed the human gate as outward, the
// child dying with its parent. Where bwrap is missing or cannot run (some kernels and
// containers refuse user namespaces) the mode is None and the reason is kept for `status`.
// Secrets never cross: the environment is scrubbed before any child sees it.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Mode is how a command is contained.
type Mode string

const (
	Bwrap Mode = "bwrap"
	None  Mode = "none"
)

// Options is the sandbox configuration; the integrator fills it from config.
type Options struct {
	Enabled  bool     // false: Mode None, why "disabled by config"
	Root     string   // the workspace root, bound read-write
	RWPaths  []string // more read-write binds (the machine-shared tier, a build cache)
	Bwrap    string   // the bwrap binary; "" looks it up on PATH
	EnvDrop  []string // more env names to drop; `*` globs allowed (e.g. AWS_*)
	EnvAllow []string // names that survive scrubbing; wins over every drop rule
	Timeout  time.Duration
}

// Call is what varies per command: whether the gate let it reach the network, and its cwd.
type Call struct {
	Network bool
	Cwd     string
}

// Probe is the reading from trying bwrap once.
type Probe struct {
	Available bool
	Path      string
	Version   string
	Why       string // why not, when unavailable
}

// Sandbox is the probed policy. Zero value: Mode None.
type Sandbox struct {
	opt   Options
	probe Probe
	once  sync.Once
}

// New builds a sandbox; the probe runs lazily on first use.
func New(opt Options) *Sandbox { return &Sandbox{opt: opt} }

// Options returns the configuration the sandbox was built with.
func (s *Sandbox) Options() Options {
	if s == nil {
		return Options{}
	}
	return s.opt
}

// Mode reports bwrap or none.
func (s *Sandbox) Mode() Mode {
	if s == nil || !s.opt.Enabled {
		return None
	}
	if s.Probe().Available {
		return Bwrap
	}
	return None
}

// Why explains a None mode; "" under bwrap.
func (s *Sandbox) Why() string {
	if s == nil {
		return "no sandbox configured"
	}
	if !s.opt.Enabled {
		return "disabled by config"
	}
	return s.Probe().Why
}

// Probe runs bwrap once with the options the sandbox will use and caches the result.
func (s *Sandbox) Probe() Probe {
	if s == nil {
		return Probe{Why: "no sandbox configured"}
	}
	s.once.Do(func() { s.probe = ProbeBwrap(s.opt.Bwrap, s.opt.Timeout) })
	return s.probe
}

// ProbeBwrap tries to run /bin/true under bwrap with the flags this package uses and reports
// whether it works, and if not, why (the kernel or container refusing namespaces is common).
func ProbeBwrap(bin string, timeout time.Duration) Probe {
	if bin == "" {
		p, err := exec.LookPath("bwrap")
		if err != nil {
			return Probe{Why: "bwrap not on PATH"}
		}
		bin = p
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ver, _ := exec.CommandContext(ctx, bin, "--version").Output()
	args := append(baseArgs(Options{}, Call{}), "--", "/bin/true")
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		why := strings.TrimSpace(string(out))
		if why == "" {
			why = err.Error()
		}
		return Probe{Path: bin, Version: strings.TrimSpace(string(ver)), Why: "bwrap cannot run here: " + firstLine(why)}
	}
	return Probe{Available: true, Path: bin, Version: strings.TrimSpace(string(ver))}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func baseArgs(opt Options, call Call) []string {
	args := []string{
		"--ro-bind", "/", "/",
		"--tmpfs", "/tmp",
		"--dev", "/dev",
		"--proc", "/proc",
		"--die-with-parent",
		"--new-session",
	}
	if opt.Root != "" {
		args = append(args, "--bind", opt.Root, opt.Root)
	}
	for _, p := range opt.RWPaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				args = append(args, "--bind", p, p)
			}
		}
	}
	if !call.Network {
		args = append(args, "--unshare-net")
	}
	if call.Cwd != "" {
		args = append(args, "--chdir", call.Cwd)
	}
	return args
}

// Wrap prefixes argv with the bwrap invocation for this call; under None it returns argv as is.
func (s *Sandbox) Wrap(call Call, argv []string) []string {
	if s.Mode() != Bwrap {
		return argv
	}
	out := append([]string{s.probe.Path}, baseArgs(s.opt, call)...)
	out = append(out, "--")
	return append(out, argv...)
}

// Command builds an exec.Cmd for argv under this sandbox with a scrubbed environment and its own
// process group (so a timeout can kill everything it started). env nil means os.Environ().
func (s *Sandbox) Command(ctx context.Context, call Call, env []string, argv ...string) *exec.Cmd {
	if env == nil {
		env = os.Environ()
	}
	w := s.Wrap(call, argv)
	cmd := exec.CommandContext(ctx, w[0], w[1:]...)
	cmd.Env = s.Scrub(env)
	cmd.Dir = call.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// Scrub applies this sandbox's env rules (nil-safe: the default rules alone).
func (s *Sandbox) Scrub(env []string) []string {
	if s == nil {
		return ScrubEnv(env, nil, nil)
	}
	return ScrubEnv(env, s.opt.EnvDrop, s.opt.EnvAllow)
}

var secretName = regexp.MustCompile(`(?i)(^|_)(API_KEY|TOKEN|SECRET|PASSWORD|PASSWD)$`)

// ScrubEnv drops secrets from an environment: any name ending in _API_KEY, _TOKEN, _SECRET,
// _PASSWORD (case-insensitive), plus the caller's names (globs allowed). allow wins.
func ScrubEnv(env, drop, allow []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if Dropped(name, drop, allow) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Dropped reports whether an env name is scrubbed under the rules.
func Dropped(name string, drop, allow []string) bool {
	for _, a := range allow {
		if match(a, name) {
			return false
		}
	}
	if secretName.MatchString(name) {
		return true
	}
	for _, d := range drop {
		if match(d, name) {
			return true
		}
	}
	return false
}

func match(pattern, name string) bool {
	if pattern == name {
		return true
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// Status renders the sandbox for the status board.
func (s *Sandbox) Status() string {
	if s.Mode() == Bwrap {
		return fmt.Sprintf("sandbox: bwrap (%s)", s.probe.Version)
	}
	return "sandbox: none — " + s.Why()
}
