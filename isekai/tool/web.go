package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// WebFetchOptions is tools.webfetch.
type WebFetchOptions struct {
	Enabled   bool
	MaxBytes  int64         // response cap (default 5 MiB)
	Timeout   time.Duration // default 60 s
	Client    *http.Client  // default: a client with Timeout
	UserAgent string
}

// WebFetchTool fetches a URL (GET) and returns it as text: HTML is reduced to its text, other
// text types pass through. Always outward.
func WebFetchTool(opt WebFetchOptions) *Tool {
	return &Tool{
		Name:        "webfetch",
		Description: "Fetch a URL (GET) and return its text; HTML is reduced to readable text (raw: true keeps it). Outward: passes the human gate.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"},"raw":{"type":"boolean"},"max_chars":{"type":"integer"}},"required":["url"]}`),
		Class:       Outward,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ URL string }
			_ = decode(in, &a)
			host := a.URL
			if u, err := url.Parse(a.URL); err == nil && u.Host != "" {
				host = u.Host
			}
			return Classification{Class: Outward, Why: "fetches " + host}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				URL      string
				Raw      bool
				MaxChars int `json:"max_chars"`
			}
			if err := decode(in, &a); err != nil || a.URL == "" {
				return fail("webfetch: url is required")
			}
			if !opt.Enabled {
				return fail("webfetch is off (tools.webfetch.enabled: false)")
			}
			u, err := url.Parse(a.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return fail("webfetch: %q is not an http(s) URL", a.URL)
			}
			t := opt.Timeout
			if t <= 0 {
				t = 60 * time.Second
			}
			client := opt.Client
			if client == nil {
				client = &http.Client{Timeout: t}
			}
			cctx, cancel := context.WithTimeout(ctx, t)
			defer cancel()
			req, err := http.NewRequestWithContext(cctx, http.MethodGet, a.URL, nil)
			if err != nil {
				return fail("webfetch: %v", err)
			}
			ua := opt.UserAgent
			if ua == "" {
				ua = "isekai/1 (+webfetch)"
			}
			req.Header.Set("User-Agent", ua)
			req.Header.Set("Accept", "text/html, text/plain, application/json, text/*;q=0.8, */*;q=0.1")
			resp, err := client.Do(req)
			if err != nil {
				return fail("webfetch: %v", err)
			}
			defer resp.Body.Close()
			max := opt.MaxBytes
			if max <= 0 {
				max = 5 << 20
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
			if err != nil {
				return fail("webfetch: reading body: %v", err)
			}
			truncated := false
			if int64(len(body)) > max {
				body, truncated = body[:max], true
			}
			ct := resp.Header.Get("Content-Type")
			if !textual(ct, body) {
				return fail("webfetch: %s is %s (%d bytes) — not text", a.URL, ct, len(body))
			}
			text := string(body)
			if !a.Raw && strings.Contains(strings.ToLower(ct), "html") {
				text = HTMLToText(text)
			}
			if a.MaxChars > 0 && len(text) > a.MaxChars {
				text = text[:a.MaxChars] + fmt.Sprintf("\n… [%d more chars]", len(text)-a.MaxChars)
			}
			head := fmt.Sprintf("%s %d %s (%d bytes", a.URL, resp.StatusCode, ct, len(body))
			if truncated {
				head += ", truncated at the cap"
			}
			return Result{Output: clip(head + ")\n\n" + text), Err: resp.StatusCode >= 400}
		},
	}
}

func textual(ct string, body []byte) bool {
	ct = strings.ToLower(ct)
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "xml") || strings.Contains(ct, "javascript") || strings.Contains(ct, "yaml") {
		return true
	}
	if ct == "" || strings.Contains(ct, "octet-stream") {
		n := len(body)
		if n > 512 {
			n = 512
		}
		for _, b := range body[:n] {
			if b == 0 {
				return false
			}
		}
		return true
	}
	return false
}

var (
	htmlDrop   = regexp.MustCompile(`(?is)<(script|style|noscript|svg|head)\b.*?</\s*(script|style|noscript|svg|head)\s*>`)
	htmlCmt    = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTitle  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlBlock  = regexp.MustCompile(`(?i)</?(p|div|br|h[1-6]|li|ul|ol|tr|table|section|article|header|footer|pre|blockquote|hr|dd|dt|dl)\b[^>]*>`)
	htmlAnchor = regexp.MustCompile(`(?is)<a\b[^>]*href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a>`)
	htmlTag    = regexp.MustCompile(`(?s)<[^>]+>`)
	multiBlank = regexp.MustCompile(`\n{3,}`)
	spaces     = regexp.MustCompile(`[ \t\r\f]+`)
)

// HTMLToText reduces HTML to readable text: scripts and styles dropped, block tags become line
// breaks, links become `text (href)`, entities decoded, whitespace collapsed.
func HTMLToText(s string) string {
	title := ""
	if m := htmlTitle.FindStringSubmatch(s); m != nil {
		title = strings.TrimSpace(htmlTag.ReplaceAllString(m[1], ""))
	}
	s = htmlCmt.ReplaceAllString(s, "")
	s = htmlDrop.ReplaceAllString(s, "")
	s = htmlAnchor.ReplaceAllStringFunc(s, func(a string) string {
		m := htmlAnchor.FindStringSubmatch(a)
		text := strings.TrimSpace(htmlTag.ReplaceAllString(m[2], ""))
		href := m[1]
		if text == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			return text
		}
		return text + " (" + href + ")"
	})
	s = htmlBlock.ReplaceAllString(s, "\n")
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(spaces.ReplaceAllString(l, " "))
		lines = append(lines, l)
	}
	out := strings.TrimSpace(multiBlank.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
	if title != "" {
		out = "# " + html.UnescapeString(title) + "\n\n" + out
	}
	return out
}

// ---- websearch

// Hit is one search result.
type Hit struct {
	Title, URL, Snippet string
}

// Searcher is a search backend; none configured means the tool is disabled.
type Searcher interface {
	Search(ctx context.Context, query string, n int) ([]Hit, error)
}

// SearcherFunc adapts a function to Searcher.
type SearcherFunc func(ctx context.Context, query string, n int) ([]Hit, error)

func (f SearcherFunc) Search(ctx context.Context, q string, n int) ([]Hit, error) {
	return f(ctx, q, n)
}

// WebSearchOptions is tools.websearch.
type WebSearchOptions struct {
	Enabled    bool
	Backend    Searcher // nil: disabled (no backend configured)
	MaxResults int      // default 10
}

// Available reports whether the tool can run at all.
func (o WebSearchOptions) Available() bool { return o.Enabled && o.Backend != nil }

// WebSearchTool searches through the configured backend. Always outward.
func WebSearchTool(opt WebSearchOptions) *Tool {
	return &Tool{
		Name:        "websearch",
		Description: "Search the web through the configured backend; returns title, url, snippet per hit. Outward: passes the human gate.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"n":{"type":"integer"}},"required":["query"]}`),
		Class:       Outward,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Query string }
			_ = decode(in, &a)
			return Classification{Class: Outward, Why: "searches the web for " + a.Query}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Query string
				N     int
			}
			if err := decode(in, &a); err != nil || strings.TrimSpace(a.Query) == "" {
				return fail("websearch: query is required")
			}
			if !opt.Enabled {
				return fail("websearch is off (tools.websearch.enabled: false)")
			}
			if opt.Backend == nil {
				return fail("websearch: no backend configured (tools.websearch.backend) — use webfetch on a known URL instead")
			}
			n := a.N
			if n <= 0 || (opt.MaxResults > 0 && n > opt.MaxResults) {
				n = opt.MaxResults
				if n <= 0 {
					n = 10
				}
			}
			hits, err := opt.Backend.Search(ctx, a.Query, n)
			if err != nil {
				return fail("websearch: %v", err)
			}
			if len(hits) == 0 {
				return Result{Output: "no results for " + a.Query}
			}
			var b strings.Builder
			for i, h := range hits {
				fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, h.Title, h.URL)
				if h.Snippet != "" {
					fmt.Fprintf(&b, "   %s\n", strings.TrimSpace(h.Snippet))
				}
			}
			return Result{Output: clip(b.String())}
		},
	}
}
