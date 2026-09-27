package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/workspace"
)

// --containered runs the binary itself inside a Docker container: the
// workspace mounted read-write at its own path, what the session needs from the host mounted
// read-only, the session store read-write, the host network (the model API and the board), the
// caller's uid. The container is the boundary, so the tool sandbox does not nest inside it.

//go:embed container.Dockerfile
var runtimeDockerfile []byte

// ContainerSpec is everything a containered run is made of; Args builds the docker argv from it.
type ContainerSpec struct {
	Dist    string
	Image   string
	Exe     string // this binary on the host, mounted read-only
	Root    string // the workspace, read-write
	Cwd     string
	Home    string
	UID     int
	GID     int
	TTY     bool
	RO      []string // host paths mounted read-only at the same path
	RW      []string // host paths mounted read-write at the same path (the session store)
	EnvKeys []string // passed by name: docker reads the value, it never lands in argv
	Argv    []string // the binary's own arguments
}

// containerEnvVar marks a process already inside; the flag is then ignored.
func containerEnvVar(dist string) string { return prefixOf(dist) + "CONTAINERED" }

func prefixOf(dist string) string { return strings.ToUpper(strings.ReplaceAll(dist, "-", "_")) + "_" }

// DefaultRuntimeImage is the image built from the embedded Dockerfile; its tag follows the
// Dockerfile, so a changed runtime is rebuilt, never silently reused.
func DefaultRuntimeImage(dist string) string {
	sum := sha256.Sum256(runtimeDockerfile)
	return dist + "-runtime:" + hex.EncodeToString(sum[:])[:12]
}

// Args is the docker argv for the spec.
func (s ContainerSpec) Args() []string {
	a := []string{"run", "--rm", "--init", "-i"}
	if s.TTY {
		a = append(a, "-t")
	}
	a = append(a, "--network", "host",
		"--user", fmt.Sprintf("%d:%d", s.UID, s.GID),
		"--tmpfs", fmt.Sprintf("%s:rw,exec,uid=%d,gid=%d,mode=0755", s.Home, s.UID, s.GID),
		"-e", "HOME="+s.Home,
		"-e", containerEnvVar(s.Dist)+"=1",
		"-e", prefixOf(s.Dist)+"CONTAINER_IMAGE="+s.Image,
		"-v", s.Exe+":/usr/local/bin/"+s.Dist+":ro",
		"-v", s.Root+":"+s.Root)
	for _, p := range s.RW {
		a = append(a, "-v", p+":"+p)
	}
	for _, p := range s.RO {
		a = append(a, "-v", p+":"+p+":ro")
	}
	for _, k := range s.EnvKeys {
		a = append(a, "-e", k)
	}
	a = append(a, "-w", s.Cwd, s.Image, "/usr/local/bin/"+s.Dist)
	return append(a, s.Argv...)
}

// hostReadOnly lists the host paths a session reads besides the workspace: the dist's global config,
// the machine-shared memory, the instruction/skill/command/agent dirs discovery walks, git's
// identity. Only the ones that exist are mounted.
func hostReadOnly(dist, home string) []string {
	rel := []string{
		".config/" + dist, "." + dist,
		".claude/CLAUDE.md", ".claude/skills", ".claude/commands", ".claude/agents",
		".config/opencode", ".agents/skills",
		".gitconfig", ".config/git",
	}
	var out []string
	for _, r := range rel {
		p := filepath.Join(home, r)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// passEnv picks the variables a session needs, by name: the terminal's, the dist's own, and every
// provider key the environment holds. Values stay in the environment.
func passEnv(dist string, environ []string) []string {
	keep := map[string]bool{}
	for _, k := range []string{"TERM", "COLORTERM", "COLORFGBG", "LANG", "LC_ALL", "TZ", "NO_COLOR"} {
		keep[k] = true
	}
	pre := prefixOf(dist)
	var out []string
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if k == containerEnvVar(dist) || k == pre+"CONTAINER_IMAGE" || k == "HOME" {
			continue
		}
		if keep[k] || strings.HasPrefix(k, pre) || strings.HasSuffix(k, "_API_KEY") || strings.HasSuffix(k, "_BASE_URL") {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// stripContainerFlags drops --containered and --image <x> from the argv the inner binary gets.
func stripContainerFlags(args []string) (rest []string, on bool, image string) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--containered":
			on = true
		case a == "--image" && i+1 < len(args):
			image = args[i+1]
			i++
		case strings.HasPrefix(a, "--image="):
			image = strings.TrimPrefix(a, "--image=")
		default:
			rest = append(rest, a)
		}
	}
	return rest, on, image
}

// Containered runs the binary inside the container and returns its exit code; handled false
// means the flag was absent or the process is already inside, and Main goes on.
func Containered(dist string, args []string, io IO) (code int, handled bool) {
	rest, on, image := stripContainerFlags(args)
	if !on {
		return 0, false
	}
	if io.Env(containerEnvVar(dist)) != "" {
		return 0, false // already inside: the flag is a no-op
	}
	fail := func(format string, a ...any) (int, bool) {
		fmt.Fprintf(io.Err, "@S FAIL\n@? --containered: "+format+"\n", a...)
		return 2, true
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		return fail("docker is not installed (the container needs a Docker engine)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	out, err := exec.CommandContext(ctx, docker, "version", "--format", "{{.Server.Version}}").CombinedOutput()
	cancel()
	if err != nil {
		return fail("the Docker engine does not answer: %s", firstLineOf(string(out)))
	}
	if image == "" {
		image = DefaultRuntimeImage(dist)
		if err := ensureImage(docker, image, io.Err); err != nil {
			return fail("%v", err)
		}
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return fail("cannot find this binary: %v", err)
	}
	cwd, _ := os.Getwd()
	root := cwd
	if f, _ := splitFlags(rest); f.root != "" {
		root, _ = filepath.Abs(f.root)
	}
	if r, _, err := workspace.Discover(root); err == nil {
		root = r
	}
	if !within(cwd, root) {
		cwd = root
	}
	home := io.Env("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	store := filepath.Join(home, ".local", "share", dist)
	if err := os.MkdirAll(store, 0o755); err != nil {
		return fail("the session store %s: %v", store, err)
	}
	spec := ContainerSpec{Dist: dist, Image: image, Exe: exe, Root: root, Cwd: cwd, Home: home, UID: os.Getuid(), GID: os.Getgid(),
		TTY: isTerminal(os.Stdin) && isTerminal(os.Stdout), RO: hostReadOnly(dist, home), RW: []string{store},
		EnvKeys: passEnv(dist, os.Environ()), Argv: rest}
	cmd := exec.Command(docker, spec.Args()...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), true
		}
		return fail("%v", err)
	}
	return 0, true
}

// ensureImage builds the runtime image from the embedded Dockerfile when it is not there yet.
func ensureImage(docker, image string, w io.Writer) error {
	if exec.Command(docker, "image", "inspect", image).Run() == nil {
		return nil
	}
	fmt.Fprintf(w, "building the runtime image %s (first containered run; pulls its base image)…\n", image)
	cmd := exec.Command(docker, "build", "-t", image, "-")
	cmd.Stdin = bytes.NewReader(runtimeDockerfile)
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building %s failed: %v", image, err)
	}
	return nil
}

func within(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
