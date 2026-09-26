package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/sandbox"
)

// Custom is one tools.custom entry: a plain struct config decodes into.
type Custom struct {
	Name        string
	Description string
	Params      json.RawMessage // JSON Schema for the input (default: object, any properties)
	Run         []string        // argv; each {{param}} fills exactly one element, never a shell
	Shell       string          // or: a bash template; params arrive as $P_<name> env variables
	Class       string          // the class floor (default write)
	Timeout     int             // seconds (default 120)
	Cwd         string          // relative to the world root (default the body's cwd)
	Sandbox     bool            // run under the sandbox when one is configured
}

// CustomOptions is what every custom tool shares.
type CustomOptions struct {
	Sandbox *sandbox.Sandbox // used when Custom.Sandbox is true
	Timeout time.Duration    // default when Custom.Timeout is 0
}

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
var customName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// CustomTool builds a tool from a Custom entry. Exactly one of Run and Shell must be set.
func CustomTool(c Custom, opt CustomOptions) (*Tool, error) {
	if !customName.MatchString(c.Name) {
		return nil, fmt.Errorf("tools.custom: name %q must be [a-z][a-z0-9_-]*", c.Name)
	}
	if (len(c.Run) == 0) == (c.Shell == "") {
		return nil, fmt.Errorf("tools.custom.%s: exactly one of run (argv) or shell (template) is required", c.Name)
	}
	floor := Write
	if c.Class != "" {
		cl, ok := ParseClass(c.Class)
		if !ok {
			return nil, fmt.Errorf("tools.custom.%s: class %q is not read|write|outward|destructive", c.Name, c.Class)
		}
		floor = cl
	}
	schema := c.Params
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	} else if !json.Valid(schema) {
		return nil, fmt.Errorf("tools.custom.%s: params is not valid JSON", c.Name)
	}
	timeout := time.Duration(c.Timeout) * time.Second
	if timeout <= 0 {
		timeout = opt.Timeout
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	desc := c.Description
	if desc == "" {
		desc = "custom tool " + c.Name
	}
	classify := func(env Env, in json.RawMessage) Classification {
		params, _ := paramStrings(in)
		best := Classification{Class: floor, Why: "declared by tools.custom." + c.Name}
		if c.Shell != "" {
			if sc := env.ClassifyCommand(c.Shell); sc.Class > best.Class {
				best = sc
			}
			return best
		}
		// argv elements are data, never a shell: the reading only tightens to outward or
		// destructive (a value that looks like `rm -rf /` still asks — erring toward asking).
		text := strings.ReplaceAll(strings.Join(fillArgv(c.Run, params), " "), "\n", " ")
		if sc := env.ClassifyCommand(text); sc.Class >= Outward && sc.Class > best.Class {
			best = sc
		}
		return best
	}
	return &Tool{
		Name:        c.Name,
		Description: desc,
		Schema:      schema,
		Class:       floor,
		Classify:    classify,
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			params, err := paramStrings(in)
			if err != nil {
				return fail("%s: %v", c.Name, err)
			}
			dir := env.Dir()
			if c.Cwd != "" {
				dir = env.Resolve(c.Cwd)
			}
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			var argv []string
			env0 := os.Environ()
			if c.Shell != "" {
				argv = []string{"bash", "-c", c.Shell}
				for _, k := range sortedKeys(params) {
					env0 = append(env0, "P_"+k+"="+params[k])
				}
			} else {
				argv = fillArgv(c.Run, params)
			}
			network := classify(env, in).Class >= Outward
			var cmd *exec.Cmd
			if c.Sandbox && opt.Sandbox != nil {
				cmd = opt.Sandbox.Command(cctx, sandbox.Call{Cwd: dir, Network: network}, env0, argv...)
			} else {
				cmd = exec.CommandContext(cctx, argv[0], argv[1:]...)
				cmd.Dir = dir
				cmd.Env = sandbox.ScrubEnv(env0, nil, nil)
			}
			cmd.Env = append(cmd.Env, "ISEKAI_ROOT="+env.Root)
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			runErr := cmd.Run()
			code := 0
			if runErr != nil {
				if ee, ok := runErr.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else {
					return fail("%s: %v", c.Name, runErr)
				}
			}
			var b strings.Builder
			b.WriteString(out.String())
			if errb.Len() > 0 {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteString("\n")
				}
				b.WriteString("[stderr]\n" + errb.String())
			}
			if cctx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(&b, "\n[killed after %s]", timeout)
				code = 124
			}
			if code != 0 {
				fmt.Fprintf(&b, "\n[exit %d]", code)
			}
			return Result{Output: clip(b.String()), Err: code != 0}
		},
	}, nil
}

// fillArgv replaces every {{param}} in each element; an element is always one argv entry,
// whatever the value contains.
func fillArgv(argv []string, params map[string]string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = placeholder.ReplaceAllStringFunc(a, func(m string) string {
			name := placeholder.FindStringSubmatch(m)[1]
			return params[name]
		})
	}
	return out
}

// paramStrings renders the input's top-level values as strings: scalars plainly, arrays and
// objects as JSON.
func paramStrings(in json.RawMessage) (map[string]string, error) {
	out := map[string]string{}
	if len(in) == 0 {
		return out, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(in, &m); err != nil {
		return nil, fmt.Errorf("input is not an object: %v", err)
	}
	for k, raw := range m {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			out[k] = s
			continue
		}
		out[k] = strings.TrimSpace(string(raw))
	}
	return out, nil
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
