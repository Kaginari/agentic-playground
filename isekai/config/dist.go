// Package config is the switchboard: it loads the layered configuration of one distribution
// (isekai or agent-zero), keeps every value's origin, validates the law's refusals, decides
// permission rules, resolves models and providers, and hands the injected rules to the
// prompt, the ontology and the end-of-turn gate. The spec is .isekai/canon/config.md.
package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Dist is one distribution's words for the same engine: directories, env prefix, law file.
// Values mirror agent-zero/lexicon.json (dir.root, dir.machine, file.law, tool.binary).
type Dist struct {
	Name       string // "isekai" | "agent-zero"; also the binary name (lexicon tool.binary)
	WorldDir   string // project dir, relative to the world root (lexicon dir.root)
	GlobalDir  string // ~/.config/<name>, or under XDG_CONFIG_HOME
	DataDir    string // ~/.local/share/<name>, or under XDG_DATA_HOME
	MachineDir string // ~/.<name> — machine-shared memory (lexicon dir.machine)
	EnvPrefix  string // ISEKAI_ | AGENT_ZERO_
	LawFile    string // isekai.md | AGENT-ZERO.md (lexicon file.law)
}

var dists = map[string]Dist{
	"isekai":     {Name: "isekai", WorldDir: ".isekai", LawFile: "isekai.md"},
	"agent-zero": {Name: "agent-zero", WorldDir: ".agent-zero", LawFile: "AGENT-ZERO.md"},
}

// Names lists the known distributions.
func Names() []string { return []string{"isekai", "agent-zero"} }

// ParseDist resolves a distribution by name (or by the binary's basename) with its
// home-relative directories filled in from home and the XDG variables.
func ParseDist(name, home string, env func(string) string) (Dist, error) {
	name = strings.TrimSuffix(filepath.Base(name), ".exe")
	d, ok := dists[name]
	if !ok {
		return Dist{}, fmt.Errorf("unknown distribution %q (isekai | agent-zero)", name)
	}
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

// Detect picks the distribution from the world directory found under root (the first that
// exists), else the fallback name.
func Detect(root, fallback string) string {
	for _, n := range Names() {
		if isDir(filepath.Join(root, dists[n].WorldDir)) {
			return n
		}
	}
	return fallback
}
