// Package board is the world, seen: an http.Handler the binary serves on 127.0.0.1 that reads
// the same instruments the session writes and shows them one question per page (binary.md
// §The board, canon/ui.md). Everything it serves is embedded; a page works offline.
package board

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/onto"
)

//go:embed assets/bootstrap.min.css assets/bootstrap.bundle.min.js static/* templates/*
var content embed.FS

// Names are the lexicon labels every template reads: no isekai term is hard-coded in a page.
// Missing keys fall back to DefaultNames; a distribution passes its own (Court → Subagents).
type Names map[string]string

// DefaultNames is the isekai vocabulary.
func DefaultNames() Names {
	return Names{
		"board":                 "Board",
		"page.overview":         "Overview",
		"page.court":            "Court",
		"page.usage":            "Usage",
		"page.colony":           "Colony",
		"page.memory":           "Memory",
		"page.toolbox":          "Toolbox",
		"page.log":              "Log",
		"page.config":           "Config",
		"world":                 "world",
		"session":               "session",
		"body":                  "body",
		"bodies":                "bodies",
		"court_body":            "Court Body",
		"creature":              "creature",
		"creatures":             "creatures",
		"mind":                  "mind",
		"minds":                 "minds",
		"rank":                  "rank",
		"office":                "office",
		"territory":             "territory",
		"desk":                  "desk",
		"thoughts":              "thoughts",
		"law":                   "law",
		"instrument":            "instrument",
		"stress":                "stress",
		"off_list":              "off-list",
		"unsaid":                "the unsaid",
		"rank.rimuru":           "Rimuru",
		"rank.veldora":          "Veldora",
		"rank.elf":              "Elf",
		"rank.orc":              "Orc",
		"rank.slime":            "Slime",
		"rank.kijin":            "Kijin",
		"lane.zone":             "zone",
		"lane.verdict":          "verdict",
		"lane.global":           "global",
		"lane.shared":           "shared",
		"bond.truth":            "truth-current",
		"bond.verdict":          "verdict-current",
		"bond.wears":            "anima-thread",
		"bond.reports":          "reports to",
		"bond.above":            "above",
		"bond.owns":             "owns",
		"kind.law":              "law",
		"kind.colony":           "colony",
		"kind.territory":        "territory",
		"tier.short":            "short",
		"tier.long":             "long",
		"tier.shared":           "shared",
		"toolbox.offered":       "offered",
		"toolbox.loaded":        "loaded",
		"toolbox.kind.mind":     "minds",
		"toolbox.kind.command":  "commands",
		"toolbox.kind.tool":     "tools",
		"toolbox.kind.body":     "bodies",
		"toolbox.kind.external": "external",
	}
}

// Options configures a board.
type Options struct {
	WorldRoot string      // the directory that holds the world dir
	WorldDir  string      // ".isekai" (a distribution renames it)
	Layout    onto.Layout // the ontology's rank layout; zero = the law's
	Names     Names       // lexicon labels, merged over DefaultNames
	Sources   Sources     // feeds; nil fields read files
	Prefix    string      // URL prefix the board is mounted under ("" or "/name")
	Now       func() time.Time
	Poll      time.Duration // how often the watcher looks for changes (default 1s)
	Logger    *log.Logger
}

// Board is the handler.
type Board struct {
	opt     Options
	src     Sources
	names   Names
	pages   map[string]*template.Template
	static  http.Handler
	mux     *http.ServeMux
	hub     *hub
	stop    chan struct{}
	once    sync.Once
	liveCrt bool // an integrator wired Court()
	liveCfg bool
}

// New builds the board. It starts a watcher goroutine; Close stops it.
func New(o Options) *Board {
	if o.WorldDir == "" {
		o.WorldDir = ".isekai"
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Poll <= 0 {
		o.Poll = time.Second
	}
	if o.Logger == nil {
		o.Logger = log.New(&discard{}, "", 0)
	}
	o.Prefix = strings.TrimSuffix(o.Prefix, "/")
	b := &Board{opt: o, hub: newHub(), stop: make(chan struct{})}
	b.liveCrt = o.Sources.Court != nil
	b.liveCfg = o.Sources.Config != nil
	b.src = o.Sources.withDefaults(FileSources(o.WorldRoot, o.WorldDir, o.Layout, o.Now))
	b.names = DefaultNames()
	for k, v := range o.Names {
		b.names[k] = v
	}
	b.pages = parsePages(b.funcs())
	sub, _ := fs.Sub(content, ".")
	b.static = http.FileServer(http.FS(sub))
	b.mux = http.NewServeMux()
	b.routes()
	go b.watch()
	return b
}

// Close stops the watcher and every open event stream.
func (b *Board) Close() { b.once.Do(func() { close(b.stop); b.hub.closeAll() }) }

// Serve listens on addr until ctx ends.
func Serve(ctx context.Context, addr string, o Options) error {
	b := New(o)
	defer b.Close()
	return b.Serve(ctx, addr)
}

// Serve listens on addr until ctx ends.
func (b *Board) Serve(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: b, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		b.Close()
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// ServeHTTP strips the prefix and routes.
func (b *Board) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if b.opt.Prefix != "" {
		if r.URL.Path == b.opt.Prefix {
			http.Redirect(w, r, b.opt.Prefix+"/", http.StatusFound)
			return
		}
		if !strings.HasPrefix(r.URL.Path, b.opt.Prefix+"/") {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, b.opt.Prefix)
		r = r2
	}
	b.mux.ServeHTTP(w, r)
}

func (b *Board) routes() {
	b.mux.HandleFunc("/", b.page("overview", b.overview))
	b.mux.HandleFunc("/court", b.page("court", b.court))
	b.mux.HandleFunc("/usage", b.page("usage", b.usage))
	b.mux.HandleFunc("/colony", b.page("colony", b.colony))
	b.mux.HandleFunc("/memory", b.page("memory", b.memory))
	b.mux.HandleFunc("/toolbox", b.page("toolbox", b.toolbox))
	b.mux.HandleFunc("/log", b.page("log", b.logPage))
	b.mux.HandleFunc("/config", b.page("config", b.config))
	b.mux.HandleFunc("/events", b.events)
	b.mux.HandleFunc("/api/doc", b.apiDoc)
	b.mux.HandleFunc("/api/court", b.apiCourt)
	b.mux.HandleFunc("/api/usage", b.apiUsage)
	b.mux.HandleFunc("/api/colony", b.apiColony)
	b.mux.Handle("/assets/", b.static)
	b.mux.Handle("/static/", b.static)
}

// ---- pages

type view struct {
	Page    string
	Title   string
	Prefix  string
	World   string
	Root    string
	Dir     string
	Now     time.Time
	Partial bool
	Live    string // SSE event names this page patches on
	Data    any
	Query   string
}

func (b *Board) page(name string, fill func(r *http.Request) (any, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name == "overview" && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, live := fill(r)
		v := view{Page: name, Title: b.names["page."+name], Prefix: b.opt.Prefix, World: b.worldName(), Root: b.opt.WorldRoot, Dir: b.opt.WorldDir, Now: b.opt.Now(), Data: data, Live: live, Query: r.URL.RawQuery}
		v.Partial = r.URL.Query().Get("partial") == "1"
		t := b.pages[name]
		var buf bytes.Buffer
		which := "page"
		if v.Partial {
			which = "live"
		}
		if err := t.ExecuteTemplate(&buf, which, v); err != nil {
			b.opt.Logger.Printf("board: %s: %v", name, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(buf.Bytes())
	}
}

func (b *Board) worldName() string {
	name := readName(path.Join(b.opt.WorldRoot, b.opt.WorldDir, "name"))
	if name == "" {
		name = path.Base(b.opt.WorldRoot)
	}
	return name
}

type overviewData struct {
	Session     Session
	SessionSeen bool
	Off         []Off
	OffLive     bool
	Bodies      []Body
	CourtLive   bool
	MaxCtx      int
	Today       UsageReport
	Usage       Reading
	Memory      *MemoryView
	Colony      *Colony
	Creatures   int
	Minds       int
	Runs        []Run
	Loop        Reading
	Log         []LogEntry
	LogReading  Reading
}

func (b *Board) overview(r *http.Request) (any, string) {
	d := overviewData{Session: b.src.Session(), Off: b.src.Off(), OffLive: b.liveCfg, Bodies: b.src.Court(), CourtLive: b.liveCrt}
	d.SessionSeen = d.Session.ID != "" || !d.Session.Started.IsZero()
	for _, x := range d.Bodies {
		if p := x.ContextPct(); p > d.MaxCtx {
			d.MaxCtx = p
		}
	}
	recs, rd := b.src.Usage()
	d.Usage = rd
	now := b.opt.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	d.Today = Rollup(recs, day, "today")
	d.Today.Reading = rd
	if !d.SessionSeen && len(recs) > 0 {
		last := recs[len(recs)-1]
		d.Session = Session{ID: last.Session, Model: last.Model, Provider: last.Provider, Started: last.TS, State: "last seen in the usage journal"}
		d.SessionSeen = true
	}
	d.Memory = b.src.Memory()
	d.Colony = b.src.Colony()
	for _, n := range d.Colony.Nodes {
		switch n.Kind {
		case "creature":
			if n.ID != "rimuru" {
				d.Creatures++
			}
		case "mind":
			d.Minds++
		}
	}
	d.Runs, d.Loop = b.src.Loop()
	if len(d.Runs) > 5 {
		d.Runs = d.Runs[:5]
	}
	d.Log, d.LogReading = b.src.Log()
	if len(d.Log) > 3 {
		d.Log = d.Log[:3]
	}
	return d, "court,usage,loop,log"
}

type courtData struct {
	Bodies  []Body
	Live    bool
	Reading Reading
}

func (b *Board) court(r *http.Request) (any, string) {
	bodies := b.src.Court()
	sort.SliceStable(bodies, func(i, j int) bool { return bodies[i].Started.Before(bodies[j].Started) })
	rd := Reading{Src: "live court feed", Lit: b.liveCrt, Read: b.opt.Now()}
	if !b.liveCrt {
		rd.Why = "no live court feed wired — the integrator supplies Sources.Court"
	}
	return courtData{Bodies: bodies, Live: b.liveCrt, Reading: rd}, "court"
}

type usageData struct {
	UsageReport
	Ranges  []string
	Session string
	Chart   template.JS
}

func (b *Board) usageReport(r *http.Request) usageData {
	recs, rd := b.src.Usage()
	since, rng := RangeSince(r.URL.Query().Get("range"), b.opt.Now())
	session := r.URL.Query().Get("session")
	if session != "" {
		var keep []UsageRecord
		for _, u := range recs {
			if u.Session == session {
				keep = append(keep, u)
			}
		}
		recs = keep
	}
	rep := Rollup(recs, since, rng)
	rep.Reading = rd
	if session != "" {
		rep.Sessions = sessionsOf(recs)
	}
	return usageData{UsageReport: rep, Ranges: Ranges, Session: session, Chart: jsonJS(chartOf(rep))}
}

func sessionsOf(recs []UsageRecord) []string {
	set := map[string]bool{}
	for _, u := range recs {
		set[u.Session] = true
	}
	var out []string
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

type chartSeries struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Values []int  `json:"values"`
}

type chartData struct {
	Days   []string      `json:"days"`
	Series []chartSeries `json:"series"`
	USD    []float64     `json:"usd"`
	Models []UsageSum    `json:"models"`
}

func chartOf(r UsageReport) chartData {
	c := chartData{Models: r.ByModel}
	c.Series = []chartSeries{{Key: "input", Label: "input"}, {Key: "output", Label: "output"}, {Key: "cacheRead", Label: "cache read"}, {Key: "cacheWrite", Label: "cache write"}}
	for _, d := range r.ByDay {
		c.Days = append(c.Days, d.Key)
		c.Series[0].Values = append(c.Series[0].Values, d.Input)
		c.Series[1].Values = append(c.Series[1].Values, d.Output)
		c.Series[2].Values = append(c.Series[2].Values, d.CacheRead)
		c.Series[3].Values = append(c.Series[3].Values, d.CacheWrite)
		c.USD = append(c.USD, d.USD)
	}
	return c
}

func (b *Board) usage(r *http.Request) (any, string) { return b.usageReport(r), "usage" }

type colonyData struct {
	*Colony
	JSON  template.JS
	Lanes []laneLabel
}

type laneLabel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

func (b *Board) laneLabels(c *Colony) []laneLabel {
	used := map[string]bool{}
	for _, n := range c.Nodes {
		if n.Kind == "mind" {
			used["mind:"+n.Lane] = true
		} else {
			used[n.Lane] = true
		}
	}
	var out []laneLabel
	for _, l := range c.Lanes {
		if !used[l] {
			continue
		}
		if strings.HasPrefix(l, "mind:") {
			out = append(out, laneLabel{ID: l, Label: b.names.get("lane."+strings.TrimPrefix(l, "mind:")) + " " + b.names.get("minds"), Kind: "mind"})
		} else {
			out = append(out, laneLabel{ID: l, Label: b.names.get("rank." + l), Kind: "rank"})
		}
	}
	return out
}

func (b *Board) colony(r *http.Request) (any, string) {
	c := b.src.Colony()
	labels := b.laneLabels(c)
	payload := struct {
		*Colony
		Labels []laneLabel `json:"labels"`
	}{c, labels}
	return colonyData{Colony: c, JSON: jsonJS(payload), Lanes: labels}, ""
}

func (b *Board) memory(r *http.Request) (any, string) { return b.src.Memory(), "" }

type toolboxData struct {
	*ToolboxView
	Kinds []string
}

func (b *Board) toolbox(r *http.Request) (any, string) {
	v := b.src.Toolbox()
	set := map[string]bool{}
	for _, e := range v.Entries {
		set[e.Kind] = true
	}
	var kinds []string
	for k := range set {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return toolboxData{ToolboxView: v, Kinds: kinds}, "loop"
}

type logData struct {
	Entries []LogEntry
	Reading Reading
	Q       string
	Total   int
}

func (b *Board) logPage(r *http.Request) (any, string) {
	entries, rd := b.src.Log()
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	d := logData{Reading: rd, Q: q, Total: len(entries)}
	for _, e := range entries {
		if q == "" || strings.Contains(strings.ToLower(e.At+" "+e.Who+" "+e.Title+" "+e.Body), q) {
			d.Entries = append(d.Entries, e)
		}
	}
	return d, "log"
}

type configData struct {
	Tree    template.HTML
	Live    bool
	Off     []Off
	Reading Reading
}

func (b *Board) config(r *http.Request) (any, string) {
	cfg := b.src.Config()
	rd := Reading{Src: "effective config (integrator)", Lit: b.liveCfg && cfg != nil, Read: b.opt.Now()}
	if !rd.Lit {
		rd.Why = "no config feed wired — the integrator supplies Sources.Config"
	}
	return configData{Tree: renderTree(cfg), Live: rd.Lit, Off: b.src.Off(), Reading: rd}, ""
}

// ---- api

func (b *Board) apiDoc(w http.ResponseWriter, r *http.Request) {
	text, err := b.src.Doc(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "not a readable doc inside the world", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(text))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func (b *Board) apiCourt(w http.ResponseWriter, r *http.Request) {
	bodies := b.src.Court()
	if bodies == nil {
		bodies = []Body{}
	}
	writeJSON(w, map[string]any{"live": b.liveCrt, "bodies": bodies, "at": b.opt.Now()})
}

func (b *Board) apiUsage(w http.ResponseWriter, r *http.Request) {
	d := b.usageReport(r)
	writeJSON(w, map[string]any{"reading": d.Reading.String(), "lit": d.Reading.Lit, "range": d.Range, "total": d.Total, "chart": chartOf(d.UsageReport), "byBody": d.ByBody, "byOffice": d.ByOffice, "byRank": d.ByRank, "byModel": d.ByModel, "byDay": d.ByDay})
}

func (b *Board) apiColony(w http.ResponseWriter, r *http.Request) {
	c := b.src.Colony()
	writeJSON(w, struct {
		*Colony
		Labels []laneLabel `json:"labels"`
	}{c, b.laneLabels(c)})
}

// ---- templates

func parsePages(funcs template.FuncMap) map[string]*template.Template {
	pages := map[string]*template.Template{}
	for _, name := range []string{"overview", "court", "usage", "colony", "memory", "toolbox", "log", "config"} {
		t := template.New("layout").Funcs(funcs)
		t = template.Must(t.ParseFS(content, "templates/layout.html", "templates/"+name+".html"))
		pages[name] = t
	}
	return pages
}

func (n Names) get(k string) string {
	if v, ok := n[k]; ok {
		return v
	}
	return k
}

func (b *Board) funcs() template.FuncMap {
	return template.FuncMap{
		"L":          b.names.get,
		"n":          fmtInt,
		"tok":        fmtCompact,
		"usd":        fmtUSD,
		"pct":        func(a, b int) int { return pctOf(a, b) },
		"since":      fmtSince,
		"when":       func(t time.Time) string { return whenOf(t) },
		"lower":      strings.ToLower,
		"add":        func(a, b int) int { return a + b },
		"rankcls":    func(s string) string { return "rank-" + safeClass(s) },
		"lanecls":    func(s string) string { return "lane-" + safeClass(s) },
		"stateCls":   stateClass,
		"stressCls":  stressClass,
		"hasPrefix":  strings.HasPrefix,
		"trimPrefix": strings.TrimPrefix,
		"json":       func(v any) template.JS { return jsonJS(v) },
		"join":       strings.Join,
		"deref": func(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		},
		"derefS": func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		},
		"derefF": func(p *float64) string {
			if p == nil {
				return "unpriced"
			}
			return fmtUSD(*p)
		},
		"reading": func(r Reading) string { return r.String() },
		"litCls": func(r Reading) string {
			if r.Lit {
				return "text-bg-success"
			}
			return "text-bg-warning"
		},
		"dict": dict,
	}
}

func dict(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	return m
}

func safeClass(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "none"
	}
	return b.String()
}

func stateClass(state string) string {
	switch strings.ToLower(state) {
	case "thinking":
		return "text-bg-info"
	case "tool":
		return "text-bg-primary"
	case "waiting on gate", "gate":
		return "text-bg-warning"
	case "done":
		return "text-bg-secondary"
	}
	return "text-bg-light"
}

func stressClass(s string) string {
	switch strings.ToUpper(s) {
	case "STRESSED", "STRESS":
		return "text-bg-danger"
	case "AT LIMIT", "NEAR":
		return "text-bg-warning"
	}
	return "text-bg-success"
}

func pctOf(a, b int) int {
	if b <= 0 {
		return 0
	}
	return int(math.Round(100 * float64(a) / float64(b)))
}

func fmtInt(n int) string {
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func fmtCompact(n int) string {
	f := float64(n)
	switch {
	case f >= 1e9:
		return fmt.Sprintf("%.1fB", f/1e9)
	case f >= 1e6:
		return fmt.Sprintf("%.1fM", f/1e6)
	case f >= 1e4:
		return fmt.Sprintf("%.1fK", f/1e3)
	}
	return fmtInt(n)
}

func fmtUSD(v float64) string {
	if v == 0 {
		return "$0.00"
	}
	if v < 0.01 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func fmtSince(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func whenOf(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

func jsonJS(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	s := string(b)
	s = strings.ReplaceAll(s, "</", "<\\/")
	return template.JS(s)
}

// renderTree renders any config value generically: maps and structs as nested lists, slices as
// lists, scalars as text. A map holding exactly `value` and `origin` is a leaf with its origin.
func renderTree(v any) template.HTML {
	if v == nil {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return template.HTML("<pre>" + template.HTMLEscapeString(fmt.Sprint(v)) + "</pre>")
	}
	var x any
	if json.Unmarshal(raw, &x) != nil {
		return ""
	}
	var sb strings.Builder
	writeTree(&sb, x, 0)
	return template.HTML(sb.String())
}

func writeTree(sb *strings.Builder, v any, depth int) {
	esc := template.HTMLEscapeString
	switch t := v.(type) {
	case map[string]any:
		if val, origin, ok := originLeaf(t); ok {
			sb.WriteString(`<span class="cfg-val">`)
			writeScalar(sb, val)
			sb.WriteString(`</span> <span class="badge text-bg-secondary cfg-origin">` + esc(origin) + `</span>`)
			return
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString(`<ul class="cfg-tree list-unstyled">`)
		for _, k := range keys {
			sb.WriteString(`<li><span class="cfg-key">` + esc(k) + `</span> `)
			writeTree(sb, t[k], depth+1)
			sb.WriteString(`</li>`)
		}
		sb.WriteString(`</ul>`)
	case []any:
		if len(t) == 0 {
			sb.WriteString(`<span class="cfg-val text-secondary">[]</span>`)
			return
		}
		sb.WriteString(`<ol class="cfg-tree">`)
		for _, e := range t {
			sb.WriteString(`<li>`)
			writeTree(sb, e, depth+1)
			sb.WriteString(`</li>`)
		}
		sb.WriteString(`</ol>`)
	default:
		sb.WriteString(`<span class="cfg-val">`)
		writeScalar(sb, t)
		sb.WriteString(`</span>`)
	}
}

func originLeaf(m map[string]any) (val any, origin string, ok bool) {
	if len(m) != 2 {
		return nil, "", false
	}
	var hasV, hasO bool
	for k, v := range m {
		switch strings.ToLower(k) {
		case "value":
			val, hasV = v, true
		case "origin", "source", "from":
			if s, isS := v.(string); isS {
				origin, hasO = s, true
			}
		}
	}
	return val, origin, hasV && hasO
}

func writeScalar(sb *strings.Builder, v any) {
	switch t := v.(type) {
	case nil:
		sb.WriteString(`<span class="text-secondary">null</span>`)
	case string:
		sb.WriteString(template.HTMLEscapeString(t))
	case bool:
		if t {
			sb.WriteString(`<span class="text-success">on</span>`)
		} else {
			sb.WriteString(`<span class="text-warning">off</span>`)
		}
	case float64:
		if t == math.Trunc(t) {
			sb.WriteString(fmt.Sprintf("%d", int64(t)))
		} else {
			sb.WriteString(fmt.Sprint(t))
		}
	case map[string]any, []any:
		writeTree(sb, t, 0)
	default:
		sb.WriteString(template.HTMLEscapeString(fmt.Sprint(t)))
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
