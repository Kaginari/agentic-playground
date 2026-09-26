package app

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kaginari/agent-one/onto"
)

// The policy, embedded so `init` can found a workspace anywhere (a bench container, a fresh
// checkout). A test keeps it equal to the repository's own copy (Docs-as-code).
//
//go:embed policy/AGENT-ONE.md
var policyText string

// PolicyText is the embedded policy.
func PolicyText() string { return policyText }

// Init seeds a minimal workspace under root: <dist-dir>/ with the policy, an empty log, the
// instrument dirs and the memory tiers' folders. Idempotent: an existing file is never
// touched; nothing outside the workspace dir is written. It returns what it created.
func Init(dist, root string) ([]string, error) {
	d, err := parseDist(dist)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(root, d.WorkspaceDir)
	var made []string
	dirs := []string{"", "instruments", "instruments/loop", "instruments/usage", "instruments/working-notes", "instruments/toolbox", "memory", "memory/short", "memory/long", "memory/shared", "toolbox", "ontology", "ontology/graph", "tmp"}
	for _, sub := range dirs {
		p := filepath.Join(base, sub)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return made, err
		}
		made = append(made, relOrAbs(root, p)+"/")
	}
	files := map[string]string{
		d.PolicyFile:                PolicyText(),
		"log.md":                    "# Chronicle — change log\n\nAppend-only. Newest entries at the bottom. One entry per change.\n\n---\n",
		"name":                      filepath.Base(root) + "\n",
		"memory/shared/notes.jsonl": "",
		"ontology/schema.ttl":       onto.DefaultSchema,
		".gitignore":                "instruments/loop/\ninstruments/usage/\ninstruments/working-notes/\ninstruments/toolbox/\nmemory/short/\nmemory/long/\ntoolbox/registry.json\ntmp/\nconfig.local.*\n",
	}
	for name, agent := range files {
		p := filepath.Join(base, name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, []byte(agent), 0o644); err != nil {
			return made, fmt.Errorf("%s: %w", relOrAbs(root, p), err)
		}
		made = append(made, relOrAbs(root, p))
	}
	return made, nil
}
