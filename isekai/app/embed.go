package app

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kaginari/agentic-playground/isekai/onto"
)

// The laws, embedded so `init` can found a world anywhere (a bench container, a fresh
// checkout). A test keeps each equal to the world's own copy (Vitality).
//
//go:embed law/isekai.md
var lawIsekai string

//go:embed law/AGENT-ONE.md
var lawAgentOne string

// LawText is the embedded law of a distribution.
func LawText(dist string) string {
	if dist == "agent-one" {
		return lawAgentOne
	}
	return lawIsekai
}

// Init seeds a minimal world under root: <dist-dir>/ with the law, an empty log, the
// instrument dirs and the memory tiers' folders. Idempotent: an existing file is never
// touched; nothing outside the world dir is written. It returns what it created.
func Init(dist, root string) ([]string, error) {
	d, err := parseDist(dist)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(root, d.WorldDir)
	var made []string
	dirs := []string{"", "instruments", "instruments/loop", "instruments/usage", "instruments/desk", "instruments/toolbox", "memory", "memory/short", "memory/long", "memory/shared", "toolbox", "ontology", "ontology/graph", "tmp"}
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
		d.LawFile:                     LawText(d.Name),
		"log.md":                      "# Chronicle — change log\n\nAppend-only. Newest entries at the bottom. One entry per change.\n\n---\n",
		"name":                        filepath.Base(root) + "\n",
		"memory/shared/notes.jsonl":   "",
		"ontology/schema.ttl":         onto.DefaultSchema,
		".gitignore":                  "instruments/loop/\ninstruments/usage/\ninstruments/desk/\ninstruments/toolbox/\nmemory/short/\nmemory/long/\ntoolbox/registry.json\ntmp/\nconfig.local.*\n",
	}
	for name, body := range files {
		p := filepath.Join(base, name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return made, fmt.Errorf("%s: %w", relOrAbs(root, p), err)
		}
		made = append(made, relOrAbs(root, p))
	}
	return made, nil
}
