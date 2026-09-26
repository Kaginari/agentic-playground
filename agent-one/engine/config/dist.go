// Package config is the switchboard: it loads the layered configuration, keeps every value's
// origin, validates the policy's refusals, decides permission rules, resolves models and
// providers, and hands the injected rules to the prompt, the ontology and the end-of-turn gate.
package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Dist is the binary's words for its home on disk: directories, env prefix, policy file.
type Dist struct {
	Name         string // the binary name
	WorkspaceDir string // project dir, relative to the workspace root
	GlobalDir    string // ~/.config/<name>, or under XDG_CONFIG_HOME
	DataDir      string // ~/.local/share/<name>, or under XDG_DATA_HOME
	MachineDir   string // ~/.<name> — machine-shared memory
	EnvPrefix    string // AGENT_ONE_
	PolicyFile   string // AGENT-ONE.md
	RankDirs     string // the native member dirs, for [rank-dirs] in a default path
}

// Name is the one distribution this engine serves.
const Name = "agent-one"

var dist = Dist{Name: Name, WorkspaceDir: ".agent-one", PolicyFile: "AGENT-ONE.md", RankDirs: "coord,domain,zone"}

// Names lists the known distributions.
func Names() []string { return []string{Name} }

// ParseDist resolves the distribution by name (or by the binary's basename) with its
// home-relative directories filled in from home and the XDG variables.
func ParseDist(name, home string, env func(string) string) (Dist, error) {
	name = strings.TrimSuffix(filepath.Base(name), ".exe")
	if name != Name {
		return Dist{}, fmt.Errorf("unknown distribution %q (%s)", name, Name)
	}
	d := dist
	d.EnvPrefix = strings.ToUpper(strings.ReplaceAll(d.Name, "-", "_")) + "_"
	if env == nil {
		env = func(string) string { return "" }
	}
	cfg := env("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	data := env("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	d.GlobalDir = filepath.Join(cfg, d.Name)
	d.DataDir = filepath.Join(data, d.Name)
	d.MachineDir = filepath.Join(home, "."+d.Name)
	return d, nil
}
