package config

import (
	"fmt"
	"sort"
	"strings"
)

// Roles are the subagent triad, canonical keys; the lineage order is the tier order.
var Roles = []string{"analyst", "judge", "drafter"}

// Tasks the binary routes on its own.
var Tasks = []string{"drain", "gate", "log", "bench"}

// SplitModel splits "provider/model-id" at the first slash; model ids may contain slashes.
func SplitModel(ref string) (provider, model string, err error) {
	p, m, ok := strings.Cut(ref, "/")
	if !ok || p == "" || m == "" {
		return "", "", fmt.Errorf("model %q is not <provider>/<model-id>", ref)
	}
	return p, m, nil
}

// Model is a resolved model: its provider entry and where the choice came from.
type Model struct {
	Ref      ModelRef
	Provider string
	ID       string
	Entry    *Provider
	Origin   Origin
	Slot     string // models.members.<n> | models.roles.<o> | ranks.<r>.model | models.ranks.<r> | models.tasks.<t> | models.default
}

// ResolveModel picks the model for a call, most specific first: member → role → rank →
// task → default. Empty arguments skip their slot. The role accepts either lexicon's word.
func (c *Config) ResolveModel(member, rank, role, task string) (Model, Origin) {
	role = canonicalRole(role)
	rank = canonicalRank(rank)
	try := func(slot string, ref ModelRef) (Model, bool) {
		if ref.Model == "" {
			return Model{}, false
		}
		m := Model{Ref: ref, Slot: slot}
		if p, id, err := SplitModel(ref.Model); err == nil {
			m.Provider, m.ID = p, id
			m.Entry = c.Providers[p]
		}
		m.Origin, _ = c.Origins[slot]
		if m.Origin.File == "" {
			m.Origin, _ = c.Origins[slot+".model"]
		}
		return m, true
	}
	if member != "" {
		if m, ok := try("models.members."+member, c.Models.Members[member]); ok {
			return m, m.Origin
		}
	}
	if role != "" {
		if m, ok := try("models.roles."+role, c.Models.Roles[role]); ok {
			return m, m.Origin
		}
	}
	if rank != "" {
		if r, ok := c.Rank(rank); ok && r.Model.Model != "" {
			if m, ok := try("ranks."+rank+".model", r.Model); ok {
				return m, m.Origin
			}
		}
		if m, ok := try("models.ranks."+rank, c.Models.Ranks[rank]); ok {
			return m, m.Origin
		}
	}
	if task != "" {
		if m, ok := try("models.tasks."+task, c.Models.Tasks[task]); ok {
			return m, m.Origin
		}
	}
	m, _ := try("models.default", c.Models.Default)
	return m, m.Origin
}

// Mount is the session's own model — the human's choice, never routed.
func (c *Config) Mount() (Model, Origin) { return c.ResolveModel("", "", "", "") }

func canonicalRole(o string) string {
	o = strings.ToLower(o)
	return o
}

func canonicalRank(r string) string {
	r = strings.ToLower(r)
	return r
}

// ModelTable lists the resolved model per role, rank and task, with origins — for status.
func (c *Config) ModelTable() []string {
	var out []string
	m, o := c.Mount()
	out = append(out, fmt.Sprintf("default    %-40s %s", m.Ref.Model, o))
	for _, off := range Roles {
		m, o := c.ResolveModel("", "", off, "")
		out = append(out, fmt.Sprintf("role %-11s %-32s %s", off, m.Ref.Model, o))
	}
	for _, r := range c.ranks {
		m, o := c.ResolveModel("", r.Name, "", "")
		out = append(out, fmt.Sprintf("rank   %-11s %-32s %s", r.Name, m.Ref.Model, o))
	}
	for _, t := range Tasks {
		m, o := c.ResolveModel("", "", "", t)
		out = append(out, fmt.Sprintf("task   %-11s %-32s %s", t, m.Ref.Model, o))
	}
	names := sortedKeys(c.Models.Members)
	for _, n := range names {
		m, o := c.ResolveModel(n, "", "", "")
		out = append(out, fmt.Sprintf("member %-9s %-32s %s", n, m.Ref.Model, o))
	}
	return out
}

// PriceFor finds a model's price; ok is false for an unpriced model (tokens only, no guess).
func (c *Config) PriceFor(ref string) (*Price, bool) {
	p, id, err := SplitModel(ref)
	if err != nil || c.Providers[p] == nil {
		return nil, false
	}
	if e := c.Providers[p].Models[id]; e != nil && e.Price != nil {
		return e.Price, true
	}
	return nil, false
}

// BudgetLines renders the budgets for status: each line says on or off.
func (c *Config) BudgetLines() []string {
	line := func(name string, cap Cap) string {
		if cap.Off() {
			return "budget " + name + ": off"
		}
		return fmt.Sprintf("budget %s: %d tokens · $%.2f", name, cap.Tokens, cap.USD)
	}
	return []string{line("session", c.Budgets.Session), line("subagent", c.Budgets.Subagent)}
}

// tier looks up a model's declared tier; ok is false when the provider or model has none.
func (c *Config) tier(ref string) (int, bool) {
	p, id, err := SplitModel(ref)
	if err != nil {
		return 0, false
	}
	prov := c.Providers[p]
	if prov == nil {
		return 0, false
	}
	if e := prov.Models[id]; e != nil && e.Tier > 0 {
		return e.Tier, true
	}
	for _, e := range prov.Models {
		if e.ID == id && e.Tier > 0 {
			return e.Tier, true
		}
	}
	return 0, false
}

// checkModels validates every model reference and the triad's lineage.
func (c *Config) checkModels() error {
	check := func(slot string, ref ModelRef) error {
		if ref.Model == "" {
			return nil
		}
		p, _, err := SplitModel(ref.Model)
		if err != nil {
			return fmt.Errorf("%s: %s: %v", c.Where(slot), slot, err)
		}
		prov, ok := c.Providers[p]
		if !ok {
			return fmt.Errorf("%s: %s: no provider %q under providers", c.Where(slot), slot, p)
		}
		if !prov.Enabled {
			return fmt.Errorf("%s: %s: provider %q is disabled (%s)", c.Where(slot), slot, p, c.Where("providers."+p+".enabled"))
		}
		if ref.Fallback != "" {
			if _, _, err := SplitModel(ref.Fallback); err != nil {
				return fmt.Errorf("%s: %s.fallback: %v", c.Where(slot), slot, err)
			}
		}
		return nil
	}
	if c.Models.Default.Model == "" {
		return fmt.Errorf("models.default: the mount must be set (<provider>/<model-id>)")
	}
	if err := check("models.default", c.Models.Default); err != nil {
		return err
	}
	for _, o := range sortedKeys(c.Models.Roles) {
		if canonicalRole(o) != o || !contains(Roles, o) {
			return fmt.Errorf("%s: models.roles.%s: roles are %s (agent-one: analyst, judge, drafter)", c.Where("models.roles."+o), o, strings.Join(Roles, ", "))
		}
		if err := check("models.roles."+o, c.Models.Roles[o]); err != nil {
			return err
		}
	}
	for _, r := range sortedKeys(c.Models.Ranks) {
		if _, ok := c.Rank(r); !ok {
			c.Holes = append(c.Holes, fmt.Sprintf("models.ranks.%s names no rank — %s", r, c.Where("models.ranks."+r)))
		}
		if err := check("models.ranks."+r, c.Models.Ranks[r]); err != nil {
			return err
		}
	}
	for _, r := range c.ranks {
		if err := check("ranks."+r.Name+".model", r.Model); err != nil {
			return err
		}
	}
	for _, t := range sortedKeys(c.Models.Tasks) {
		if !contains(Tasks, t) {
			return fmt.Errorf("%s: models.tasks.%s: tasks are %s", c.Where("models.tasks."+t), t, strings.Join(Tasks, ", "))
		}
		if err := check("models.tasks."+t, c.Models.Tasks[t]); err != nil {
			return err
		}
	}
	for _, n := range sortedKeys(c.Models.Members) {
		if err := check("models.members."+n, c.Models.Members[n]); err != nil {
			return err
		}
	}
	// The lineage: analyst ≤ judge ≤ drafter by declared tier; one model for all three
	// holds it trivially and needs no tier.
	tiers := make([]int, len(Roles))
	known := true
	same := true
	for _, off := range Roles {
		m, _ := c.ResolveModel("", "", off, "")
		if m.Ref.Model != c.Models.Default.Model {
			same = false
		}
	}
	if same {
		return nil
	}
	for i, off := range Roles {
		m, _ := c.ResolveModel("", "", off, "")
		t, ok := c.tier(m.Ref.Model)
		if !ok {
			known = false
			c.Holes = append(c.Holes, fmt.Sprintf("models.roles.%s: %s declares no tier — the lineage cannot be checked", off, m.Ref.Model))
			continue
		}
		tiers[i] = t
	}
	if known && (tiers[0] > tiers[1] || tiers[1] > tiers[2]) {
		return fmt.Errorf("models.roles: the lineage %s ≤ %s ≤ %s is broken (tiers %d, %d, %d)", Roles[0], Roles[1], Roles[2], tiers[0], tiers[1], tiers[2])
	}
	return nil
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// checkProviders validates providers: type, key location, headers, models.
func (c *Config) checkProviders() error {
	for _, name := range sortedKeys(c.Providers) {
		p := c.Providers[name]
		at := func(field string) string { return c.Where("providers." + name + "." + field) }
		if !p.Enabled && p.Type == "" {
			continue // `{enabled: false}` alone switches a provider off without its definition
		}
		if p.Type == "" {
			switch name {
			case "anthropic":
				p.Type = "anthropic"
			case "openai", "openrouter", "ollama", "vllm", "groq", "together", "mistral", "deepseek", "sglang", "lmstudio":
				p.Type = "openai"
			case "mock":
				p.Type = "mock"
			default:
				return fmt.Errorf("%s: providers.%s.type is required (anthropic | openai | mock)", c.Where("providers."+name), name)
			}
		}
		switch p.Type {
		case "anthropic", "openai", "mock":
		default:
			return fmt.Errorf("%s: providers.%s.type: %q is not anthropic, openai or mock", at("type"), name, p.Type)
		}
		if p.APIKeyEnv != "" && !isEnvName(p.APIKeyEnv) {
			return fmt.Errorf("%s: providers.%s.apiKeyEnv must name an environment variable, not hold a key", at("apiKeyEnv"), name)
		}
		for _, h := range sortedKeys(p.Headers) {
			lh := strings.ToLower(h)
			if lh == "authorization" || lh == "x-api-key" || lh == "api-key" || looksSecret(p.Headers[h]) {
				return fmt.Errorf("%s: providers.%s.headers.%s carries a credential — keys come from env only (apiKeyEnv)", at("headers."+h), name, h)
			}
		}
		if p.ToolCalls != "" && p.ToolCalls != "native" && p.ToolCalls != "text" {
			return fmt.Errorf("%s: providers.%s.toolCalls: %q is not native or text", at("toolCalls"), name, p.ToolCalls)
		}
		if p.Thinking != "" && p.Thinking != "adaptive" && p.Thinking != "off" {
			return fmt.Errorf("%s: providers.%s.thinking: %q is not adaptive or off", at("thinking"), name, p.Thinking)
		}
		if p.Fallbacks != "" && p.Fallbacks != "default" && p.Fallbacks != "off" {
			return fmt.Errorf("%s: providers.%s.fallbacks: %q is not default or off", at("fallbacks"), name, p.Fallbacks)
		}
		if p.Type != "mock" && p.Enabled && p.BaseURL == "" {
			return fmt.Errorf("%s: providers.%s.baseURL is required", c.Where("providers."+name), name)
		}
		for id, e := range p.Models {
			if e.ID == "" {
				e.ID = id
			}
		}
	}
	return nil
}

func isEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func looksSecret(v string) bool {
	l := strings.ToLower(v)
	return strings.HasPrefix(l, "sk-") || strings.HasPrefix(l, "bearer ") || strings.HasPrefix(l, "sk_")
}

// secretKey refuses an unknown key that would hold a credential in a file.
func secretKey(u unknownKey) error {
	last := u.path[strings.LastIndex(u.path, ".")+1:]
	switch strings.ToLower(last) {
	case "apikey", "api_key", "key", "token", "secret", "password", "auth":
		if strings.HasPrefix(u.path, "providers.") || strings.HasPrefix(u.path, "mcp.") {
			return fmt.Errorf("%s: %s: a credential never lives in a config file — name the env var in apiKeyEnv (or headersEnv)", u.node.Where(), u.path)
		}
	}
	return nil
}

// ProviderKey returns the key's value read from the env var apiKeyEnv names; "" when the
// provider needs none. The value is for the HTTP client only and never for the wire.
func (p *Provider) ProviderKey(env func(string) string) (string, error) {
	if p.APIKeyEnv == "" {
		return "", nil
	}
	if env == nil {
		env = func(string) string { return "" }
	}
	v := env(p.APIKeyEnv)
	if v == "" {
		return "", fmt.Errorf("%s is not set (location only: the key never crosses the wire)", p.APIKeyEnv)
	}
	return v, nil
}

// ProviderNames lists providers in name order.
func (c *Config) ProviderNames() []string {
	names := sortedKeys(c.Providers)
	sort.Strings(names)
	return names
}
