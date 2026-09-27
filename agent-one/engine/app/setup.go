package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"

	"github.com/Kaginari/agent-one/tui"
)

// The first-run setup: after `init` founds a workspace on a terminal, a Huh form asks which model the
// workspace runs on and writes the answer to the workspace's config.yaml. Skipped on a pipe, with --plain,
// when the workspace already has a config, or when <PREFIX>NO_SETUP is set.

type setupChoice struct {
	provider string // "" keeps the global config
	model    string
	fallback string
	baseURL  string
	keyEnv   string
}

var setupPresets = map[string]setupChoice{
	"openrouter": {provider: "openrouter", model: "openrouter/google/gemma-4-31b-it:free", fallback: "openrouter/thinkingmachines/inkling-small:free", keyEnv: "OPENROUTER_API_KEY"},
	"anthropic":  {provider: "anthropic", model: "anthropic/claude-sonnet-5", keyEnv: "ANTHROPIC_API_KEY"},
	"openai":     {provider: "openai", model: "openai/", keyEnv: "OPENAI_API_KEY"},
	"ollama":     {provider: "ollama", model: "ollama/"},
	"custom":     {provider: "vllm", model: "vllm/", baseURL: "http://127.0.0.1:8000/v1", keyEnv: "VLLM_API_KEY"},
}

// wantsSetup says whether init should ask.
func wantsSetup(dist, root string, f cliFlags, io IO) bool {
	if f.plain || io.Env(prefixOf(dist)+"NO_SETUP") != "" {
		return false
	}
	in, ok1 := io.In.(*os.File)
	out, ok2 := io.Out.(*os.File)
	if !ok1 || !ok2 || !isTerminal(in) || !isTerminal(out) {
		return false
	}
	d, err := parseDist(dist)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(root, d.WorkspaceDir, "config.yaml"))
	return errors.Is(err, os.ErrNotExist)
}

// runSetup shows the mascot, asks, and writes the workspace's config.yaml; the path written, or "".
func runSetup(dist, root string, io IO) (string, error) {
	art, tag := tui.Mascot(dist)
	fmt.Fprintln(io.Out)
	for i, a := range art {
		side := ""
		switch i {
		case 1:
			side = tui.Title(dist) + " setup"
		case 2:
			side = tag
		}
		fmt.Fprintf(io.Out, "  %s   %s\n", a, side)
	}
	fmt.Fprintln(io.Out)

	pick := "keep"
	global := "the global config (~/.config/" + dist + "/config.yaml)"
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Which model should this "+worldWord(dist)+" run on?").
			Options(
				huh.NewOption("Keep "+global, "keep"),
				huh.NewOption("OpenRouter free models — gemma-4-31b, inkling-small as fallback", "openrouter"),
				huh.NewOption("Anthropic — Claude", "anthropic"),
				huh.NewOption("OpenAI", "openai"),
				huh.NewOption("Ollama — a local model, no key", "ollama"),
				huh.NewOption("An OpenAI-compatible server (vLLM, LM Studio…)", "custom"),
			).Value(&pick),
	)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin))
	if err := form.Run(); err != nil {
		return "", err
	}
	if pick == "keep" {
		return "", nil
	}
	c := setupPresets[pick]
	fields := []huh.Field{
		huh.NewInput().Title("Model").Description("<provider>/<model-id>").Value(&c.model).Validate(func(s string) error {
			if _, id, ok := strings.Cut(s, "/"); !ok || id == "" {
				return errors.New("name the model after the provider: <provider>/<model-id>")
			}
			return nil
		}),
	}
	if pick == "custom" {
		fields = append(fields, huh.NewInput().Title("Base URL").Value(&c.baseURL))
	}
	if c.keyEnv != "" {
		fields = append(fields, huh.NewInput().Title("The environment variable that holds the key").
			Description("only its name is written — the key itself never lands in a file").Value(&c.keyEnv))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
		return "", err
	}
	d, _ := parseDist(dist)
	p := filepath.Join(root, d.WorkspaceDir, "config.yaml")
	if err := os.WriteFile(p, []byte(setupYAML(c)), 0o644); err != nil {
		return "", err
	}
	if c.keyEnv != "" && io.Env(c.keyEnv) == "" {
		fmt.Fprintf(io.Out, "\n  %s is not set in this shell — export it before the first session.\n", c.keyEnv)
	}
	return relOrAbs(root, p), nil
}

// setupYAML is the workspace's config for a choice.
func setupYAML(c setupChoice) string {
	var b strings.Builder
	b.WriteString("# Written by the setup at init; edit freely (`config explain` shows where each value comes from).\n")
	b.WriteString("providers:\n")
	if c.provider == "vllm" {
		fmt.Fprintf(&b, "  vllm: {enabled: true, type: openai, baseURL: %s, apiKeyEnv: %s, toolCalls: native, contextWindow: auto}\n", c.baseURL, c.keyEnv)
	} else {
		fmt.Fprintf(&b, "  %s: {enabled: true", c.provider)
		if c.keyEnv != "" {
			fmt.Fprintf(&b, ", apiKeyEnv: %s", c.keyEnv)
		}
		b.WriteString("}\n")
	}
	b.WriteString("models:\n")
	if c.fallback != "" {
		fmt.Fprintf(&b, "  default: {model: %s, fallback: %s}\n", c.model, c.fallback)
	} else {
		fmt.Fprintf(&b, "  default: %s\n", c.model)
	}
	return b.String()
}

func worldWord(dist string) string {
	if dist == "agent-one" {
		return "workspace"
	}
	return "workspace"
}
