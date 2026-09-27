package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/anthropic"
	"github.com/Kaginari/agent-one/provider/openai"
)

// Route is what a model is resolved for: the role the ask names, the agent's rank and name,
// or one of the binary's own tasks (drain | gate | log | bench).
type Route struct {
	Role   string
	Rank   string
	Member string
	Task   string
}

// Providers builds and caches one client per resolved model, from config: the key from the
// env var the provider names, the base URL, timeout and TLS, the per-provider switches
// (thinking, fallbacks, toolCalls, guidedDecoding, contextWindow) and the model entry's effort.
type Providers struct {
	Cfg *config.Config
	Env func(string) string

	mu      sync.Mutex
	clients map[string]provider.Provider
	windows map[string]int
	holes   []string
}

// NewProviders is the factory for one config.
func NewProviders(cfg *config.Config, env func(string) string) *Providers {
	if env == nil {
		env = os.Getenv
	}
	return &Providers{Cfg: cfg, Env: env, clients: map[string]provider.Provider{}, windows: map[string]int{}}
}

// Resolve picks the model for a route, most specific first (config.ResolveModel).
func (p *Providers) Resolve(r Route) (config.Model, config.Origin) {
	return p.Cfg.ResolveModel(r.Member, r.Rank, r.Role, r.Task)
}

// Route resolves and builds the provider for a route.
func (p *Providers) Route(r Route) (provider.Provider, config.Model, error) {
	m, _ := p.Resolve(r)
	pr, err := p.For(m)
	return pr, m, err
}

// Holes lists what could not be built (a missing key, a disabled provider), one line each.
func (p *Providers) Holes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.holes...)
}

func (p *Providers) hole(h string) {
	for _, x := range p.holes {
		if x == h {
			return
		}
	}
	p.holes = append(p.holes, h)
}

// For builds (or returns the cached) client for a resolved model, with its fallback wrapped
// around it when the entry names one.
func (p *Providers) For(m config.Model) (provider.Provider, error) {
	if m.Ref.Model == "" {
		return nil, errors.New("no model resolved (models.default is unset)")
	}
	key := m.Ref.Model + "|" + m.Ref.Effort
	p.mu.Lock()
	if c, ok := p.clients[key]; ok {
		p.mu.Unlock()
		return c, nil
	}
	p.mu.Unlock()
	primary, err := p.build(m.Provider, m.ID, m.Ref.Effort)
	if err != nil {
		p.mu.Lock()
		p.hole(fmt.Sprintf("model %s (%s): %v", m.Ref.Model, m.Slot, err))
		p.mu.Unlock()
		return nil, err
	}
	var out provider.Provider = primary
	if m.Ref.Fallback != "" {
		if fp, fid, err := config.SplitModel(m.Ref.Fallback); err == nil {
			if fb, err := p.build(fp, fid, ""); err == nil {
				out = &fallback{primary: primary, secondary: fb}
			} else {
				p.mu.Lock()
				p.hole(fmt.Sprintf("fallback %s for %s: %v", m.Ref.Fallback, m.Ref.Model, err))
				p.mu.Unlock()
			}
		}
	}
	p.mu.Lock()
	p.clients[key] = out
	p.mu.Unlock()
	return out, nil
}

func (p *Providers) httpClient(e *config.Provider) (*http.Client, error) {
	timeout := e.Timeout.D()
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if e.TLS.InsecureSkipVerify || e.TLS.CAFile != "" {
		tc := &tls.Config{InsecureSkipVerify: e.TLS.InsecureSkipVerify} //nolint:gosec — the human's own config word
		if e.TLS.CAFile != "" {
			pem, err := os.ReadFile(e.TLS.CAFile)
			if err != nil {
				return nil, fmt.Errorf("tls.caFile: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("tls.caFile %s holds no certificate", e.TLS.CAFile)
			}
			tc.RootCAs = pool
		}
		tr.TLSClientConfig = tc
	}
	return &http.Client{Timeout: timeout, Transport: tr}, nil
}

// build makes one raw client.
func (p *Providers) build(name, id, effort string) (provider.Provider, error) {
	e := p.Cfg.Providers[name]
	if e == nil {
		return nil, fmt.Errorf("no provider %q under providers", name)
	}
	if !e.Enabled {
		return nil, fmt.Errorf("provider %q is disabled (%s)", name, p.Cfg.Where("providers."+name+".enabled"))
	}
	switch e.Type {
	case "mock":
		return newScriptProvider(name+"/"+id, e.Script, p.Cfg.Root)
	case "anthropic":
		key, err := e.ProviderKey(p.Env)
		if err != nil {
			return nil, err
		}
		hc, err := p.httpClient(e)
		if err != nil {
			return nil, err
		}
		c := &anthropic.Client{APIKey: key, Model: id, BaseURL: e.BaseURL, HTTP: hc, Headers: e.Headers,
			Thinking: e.Thinking != "off", Fallbacks: e.Fallbacks != "off", Cache: true, Stream: p.Cfg.Output.Stream,
			Effort: effort, MaxTokens: e.MaxOutputTokens}
		return c, nil
	case "openai":
		key := ""
		if e.APIKeyEnv != "" {
			k, err := e.ProviderKey(p.Env)
			if err != nil {
				return nil, err
			}
			key = k
		}
		hc, err := p.httpClient(e)
		if err != nil {
			return nil, err
		}
		c := &openai.Client{APIKey: key, Model: id, BaseURL: e.BaseURL, HTTP: hc, Headers: e.Headers,
			ToolCalls: e.ToolCalls, Guided: e.GuidedDecoding, Stream: p.Cfg.Output.Stream, MaxTokens: e.MaxOutputTokens}
		if c.ToolCalls == "" {
			c.ToolCalls = "native"
		}
		return c, nil
	}
	return nil, fmt.Errorf("provider %q: unknown type %q", name, e.Type)
}

// Window is the model's real context window: the model entry's contextWindow, else the
// provider's configured number, else (contextWindow: auto on an openai-compatible endpoint) the
// served maximum from /v1/models; 0 when unknown, with the reason as a hole.
func (p *Providers) Window(ctx context.Context, m config.Model) int {
	if m.Entry == nil {
		return 0
	}
	if me := m.Entry.Models[m.ID]; me != nil && me.ContextWindow > 0 {
		return me.ContextWindow
	}
	if !m.Entry.ContextWindow.Auto {
		return m.Entry.ContextWindow.N
	}
	p.mu.Lock()
	if n, ok := p.windows[m.Ref.Model]; ok {
		p.mu.Unlock()
		return n
	}
	p.mu.Unlock()
	n := 0
	if m.Entry.Type == "openai" {
		if pr, err := p.For(m); err == nil {
			if c, ok := unwrap(pr).(*openai.Client); ok {
				cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				got, err := c.ContextWindow(cctx)
				if err != nil {
					p.mu.Lock()
					p.hole(fmt.Sprintf("contextWindow auto for %s: %v", m.Ref.Model, err))
					p.mu.Unlock()
				}
				n = got
			}
		}
	}
	p.mu.Lock()
	p.windows[m.Ref.Model] = n
	p.mu.Unlock()
	return n
}

func unwrap(pr provider.Provider) provider.Provider {
	for {
		switch x := pr.(type) {
		case *fallback:
			pr = x.primary
		case *Meter:
			pr = x.Provider
		default:
			return pr
		}
	}
}

// fallback tries the secondary model when the primary call fails outright (a transport
// error, 429/5xx/529) — never on a refusal, which the provider handles itself.
type fallback struct {
	primary, secondary provider.Provider
}

func (f *fallback) Name() string { return f.primary.Name() }

func (f *fallback) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	resp, err := f.primary.Complete(ctx, req)
	if err == nil || ctx.Err() != nil || !retryable(err) {
		return resp, err
	}
	resp2, err2 := f.secondary.Complete(ctx, req)
	if err2 != nil {
		return resp, fmt.Errorf("%v; fallback %s: %v", err, f.secondary.Name(), err2)
	}
	return resp2, nil
}

func retryable(err error) bool {
	s := err.Error()
	for _, code := range []string{"HTTP 429", "HTTP 500", "HTTP 502", "HTTP 503", "HTTP 504", "HTTP 529"} {
		if strings.Contains(s, code) {
			return true
		}
	}
	return strings.Contains(s, "finish_reason error") || strings.Contains(s, "connection refused") || strings.Contains(s, "EOF") || strings.Contains(s, "timeout") || strings.Contains(s, "no such host")
}
