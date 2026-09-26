package board

import (
	"bufio"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/memory"
	"github.com/Kaginari/agentic-playground/isekai/onto"
	"github.com/Kaginari/agentic-playground/isekai/toolbox"
)

// Reading says where a figure came from and whether the instrument behind it is lit
// (Nature 9). A silent instrument is shown as silent, never as zero.
type Reading struct {
	Src  string    // the file or package answering
	At   time.Time // when the instrument last spoke (zero = never)
	Lit  bool      // true when data was read this request
	Why  string    // for a silent instrument: why
	Read time.Time // when this reading was taken
}

func lit(src string, at, now time.Time) Reading {
	return Reading{Src: src, At: at, Lit: true, Read: now}
}

func silent(src, why string, since, now time.Time) Reading {
	return Reading{Src: src, At: since, Lit: false, Why: why, Read: now}
}

// String renders the reading for a page footer or a badge.
func (r Reading) String() string {
	switch {
	case r.Lit && !r.At.IsZero():
		return "lit · " + r.Src + " · last " + r.At.Format("2006-01-02 15:04")
	case r.Lit:
		return "lit · " + r.Src
	case !r.At.IsZero():
		return "silent since " + r.At.Format("2006-01-02 15:04") + " · " + r.Src + " — " + r.Why
	default:
		return "never lit · " + r.Src + " — " + r.Why
	}
}

// Body is one live body in the court, as the integrator sees it.
type Body struct {
	Name          string    `json:"name"`
	Rank          string    `json:"rank"`
	Office        string    `json:"office"`
	Model         string    `json:"model"`
	Provider      string    `json:"provider"`
	State         string    `json:"state"` // thinking · tool · waiting on gate · done
	Started       time.Time `json:"started"`
	ContextTokens int       `json:"contextTokens"`
	ContextLimit  int       `json:"contextLimit"`
	Input         int       `json:"input"`
	Output        int       `json:"output"`
	CacheRead     int       `json:"cacheRead"`
	CacheWrite    int       `json:"cacheWrite"`
	USD           *float64  `json:"usd"` // nil = unpriced
}

// Tokens is the body's spend so far.
func (b Body) Tokens() int { return b.Input + b.Output + b.CacheRead + b.CacheWrite }

// ContextPct is the window occupancy, 0 when the limit is unknown.
func (b Body) ContextPct() int {
	if b.ContextLimit <= 0 {
		return 0
	}
	return int(math.Round(100 * float64(b.ContextTokens) / float64(b.ContextLimit)))
}

// UsageRecord is one line of .isekai/instruments/usage/<session>.jsonl (README.md).
type UsageRecord struct {
	TS         time.Time `json:"ts"`
	Session    string    `json:"session"`
	Body       string    `json:"body"`
	Rank       string    `json:"rank"`
	Office     string    `json:"office"`
	Model      string    `json:"model"`
	Provider   string    `json:"provider"`
	Input      int       `json:"input"`
	Output     int       `json:"output"`
	CacheRead  int       `json:"cacheRead"`
	CacheWrite int       `json:"cacheWrite"`
	USD        *float64  `json:"usd"`
}

// Tokens is the record's total.
func (u UsageRecord) Tokens() int { return u.Input + u.Output + u.CacheRead + u.CacheWrite }

// UsageSum is one rollup row.
type UsageSum struct {
	Key        string  `json:"key"`
	Calls      int     `json:"calls"`
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	CacheRead  int     `json:"cacheRead"`
	CacheWrite int     `json:"cacheWrite"`
	USD        float64 `json:"usd"`
	Unpriced   int     `json:"unpriced"` // calls with usd null: the USD column is a floor
}

// Tokens is the row's total.
func (s UsageSum) Tokens() int { return s.Input + s.Output + s.CacheRead + s.CacheWrite }

func (s *UsageSum) add(u UsageRecord) {
	s.Calls++
	s.Input += u.Input
	s.Output += u.Output
	s.CacheRead += u.CacheRead
	s.CacheWrite += u.CacheWrite
	if u.USD != nil {
		s.USD += *u.USD
	} else {
		s.Unpriced++
	}
}

// UsageReport is the usage journal rolled up for a range.
type UsageReport struct {
	Reading  Reading
	Range    string
	Since    time.Time
	Records  int
	Total    UsageSum
	ByBody   []UsageSum
	ByOffice []UsageSum
	ByRank   []UsageSum
	ByModel  []UsageSum
	ByDay    []UsageSum
	Sessions []string
}

// Ranges the usage page offers, in order.
var Ranges = []string{"24h", "7d", "30d", "all"}

// RangeSince turns a range name into its start; "all" is the zero time.
func RangeSince(name string, now time.Time) (time.Time, string) {
	switch name {
	case "24h":
		return now.Add(-24 * time.Hour), name
	case "30d":
		return now.AddDate(0, 0, -30), name
	case "all":
		return time.Time{}, name
	default:
		return now.AddDate(0, 0, -7), "7d"
	}
}

// Rollup folds records dated at or after since into the report's rows.
func Rollup(recs []UsageRecord, since time.Time, rng string) UsageReport {
	r := UsageReport{Range: rng, Since: since, Total: UsageSum{Key: "total"}}
	groups := map[string]map[string]*UsageSum{"body": {}, "office": {}, "rank": {}, "model": {}, "day": {}}
	sessions := map[string]bool{}
	get := func(g, k string) *UsageSum {
		if k == "" {
			k = "—"
		}
		s, ok := groups[g][k]
		if !ok {
			s = &UsageSum{Key: k}
			groups[g][k] = s
		}
		return s
	}
	for _, u := range recs {
		if !since.IsZero() && u.TS.Before(since) {
			continue
		}
		r.Records++
		r.Total.add(u)
		sessions[u.Session] = true
		get("body", u.Body).add(u)
		get("office", u.Office).add(u)
		get("rank", u.Rank).add(u)
		get("model", u.Model).add(u)
		get("day", u.TS.UTC().Format("2006-01-02")).add(u)
	}
	byTokens := func(m map[string]*UsageSum) []UsageSum {
		var out []UsageSum
		for _, s := range m {
			out = append(out, *s)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Tokens() != out[j].Tokens() {
				return out[i].Tokens() > out[j].Tokens()
			}
			return out[i].Key < out[j].Key
		})
		return out
	}
	r.ByBody, r.ByOffice, r.ByRank, r.ByModel = byTokens(groups["body"]), byTokens(groups["office"]), byTokens(groups["rank"]), byTokens(groups["model"])
	r.ByDay = byTokens(groups["day"])
	sort.Slice(r.ByDay, func(i, j int) bool { return r.ByDay[i].Key < r.ByDay[j].Key })
	for s := range sessions {
		r.Sessions = append(r.Sessions, s)
	}
	sort.Strings(r.Sessions)
	return r
}

// Run is one loop journal, as the board shows it.
type Run struct {
	ID     string
	As     string
	Ask    string
	Status string
	At     string
	Turns  int
	Steps  int
	Failed int
}

// LogEntry is one dated `### [...]` entry of log.md.
type LogEntry struct {
	At    string
	Who   string
	Title string
	Body  string // the lines under the heading, verbatim
}

// Off is one law feature the config disabled, with where that was written.
type Off struct {
	Feature string
	Origin  string
}

// Session is the running session, as the integrator reports it.
type Session struct {
	ID       string
	Model    string
	Provider string
	Started  time.Time
	State    string
}

// Node is a creature or a mind in the colony graph.
type Node struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Kind   string   `json:"kind"` // creature | mind
	Rank   string   `json:"rank"` // canonical rank for a creature; "" for a mind
	Lane   string   `json:"lane"` // zone | verdict | global | shared for a mind; rank for a creature
	Doc    string   `json:"doc"`
	Owns   []string `json:"owns"`
	Shared bool     `json:"shared"`
}

// Edge is one typed bond.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Bond string `json:"bond"` // truth | verdict | reports | above | wears
}

// Colony is the creature graph, projected for drawing.
type Colony struct {
	Reading  Reading
	Nodes    []Node   `json:"nodes"`
	Edges    []Edge   `json:"edges"`
	Lanes    []string `json:"lanes"` // drawing order, left to right
	Findings []string `json:"findings"`
	Notes    []string `json:"notes"`
}

// MemoryView is the memory tiers as the board shows them.
type MemoryView struct {
	Reading Reading
	Status  *memory.StatusReport
	Desks   []memory.Desk
	Notes   []memory.Note
	ByKind  map[string]int
}

// ToolboxView is the registry and its instrument.
type ToolboxView struct {
	Reading Reading
	Status  *toolbox.StatusReport
	Entries []*toolbox.Entry
	Offered map[string]bool
	Loaded  map[string]bool
}

// Sources is every feed the board reads. Nil fields are filled from FileSources by New.
// Court, Config, Off and Session come from the integrator: files do not know what is live.
type Sources struct {
	Court   func() []Body
	Config  func() any
	Off     func() []Off
	Session func() Session
	Usage   func() ([]UsageRecord, Reading)
	Loop    func() ([]Run, Reading)
	Log     func() ([]LogEntry, Reading)
	Colony  func() *Colony
	Memory  func() *MemoryView
	Toolbox func() *ToolboxView
	Doc     func(path string) (string, error)
}

// FileSources reads the world's files. Court, Config, Off and Session answer honestly empty.
func FileSources(root, worldDir string, layout onto.Layout, now func() time.Time) Sources {
	if worldDir == "" {
		worldDir = ".isekai"
	}
	if now == nil {
		now = time.Now
	}
	if layout.WorldDir == "" {
		layout.WorldDir = worldDir
	}
	f := &fileSources{root: root, dir: filepath.Join(root, worldDir), worldDir: worldDir, layout: layout, now: now}
	return Sources{
		Court:   func() []Body { return nil },
		Config:  func() any { return nil },
		Off:     func() []Off { return nil },
		Session: func() Session { return Session{} },
		Usage:   f.usage,
		Loop:    f.loop,
		Log:     f.log,
		Colony:  f.colony,
		Memory:  f.memory,
		Toolbox: f.toolbox,
		Doc:     f.doc,
	}
}

func (s Sources) withDefaults(d Sources) Sources {
	if s.Court == nil {
		s.Court = d.Court
	}
	if s.Config == nil {
		s.Config = d.Config
	}
	if s.Off == nil {
		s.Off = d.Off
	}
	if s.Session == nil {
		s.Session = d.Session
	}
	if s.Usage == nil {
		s.Usage = d.Usage
	}
	if s.Loop == nil {
		s.Loop = d.Loop
	}
	if s.Log == nil {
		s.Log = d.Log
	}
	if s.Colony == nil {
		s.Colony = d.Colony
	}
	if s.Memory == nil {
		s.Memory = d.Memory
	}
	if s.Toolbox == nil {
		s.Toolbox = d.Toolbox
	}
	if s.Doc == nil {
		s.Doc = d.Doc
	}
	return s
}

type fileSources struct {
	root, dir, worldDir string
	layout              onto.Layout
	now                 func() time.Time
}

func (f *fileSources) rel(p string) string {
	if r, err := filepath.Rel(f.root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}

// UsageDir is where sessions journal their provider calls.
func (f *fileSources) usageDir() string { return filepath.Join(f.dir, "instruments", "usage") }
func (f *fileSources) loopDir() string  { return filepath.Join(f.dir, "instruments", "loop") }
func (f *fileSources) logPath() string  { return filepath.Join(f.dir, "log.md") }

func jsonlFiles(dir string) ([]string, time.Time) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, time.Time{}
	}
	var out []string
	var latest time.Time
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
		if info, err := e.Info(); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	sort.Strings(out)
	return out, latest
}

// ReadUsage parses every usage journal in dir; unreadable lines are skipped, never guessed.
func ReadUsage(dir string) ([]UsageRecord, int) {
	files, _ := jsonlFiles(dir)
	var out []UsageRecord
	skipped := 0
	for _, p := range files {
		fh, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var u UsageRecord
			if json.Unmarshal([]byte(line), &u) != nil || u.TS.IsZero() {
				skipped++
				continue
			}
			if u.Session == "" {
				u.Session = strings.TrimSuffix(filepath.Base(p), ".jsonl")
			}
			out = append(out, u)
		}
		fh.Close()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	return out, skipped
}

func (f *fileSources) usage() ([]UsageRecord, Reading) {
	src := f.rel(f.usageDir()) + "/*.jsonl"
	recs, skipped := ReadUsage(f.usageDir())
	now := f.now()
	if len(recs) == 0 {
		_, mod := jsonlFiles(f.usageDir())
		return nil, silent(src, "no usage journal yet — no provider call has been recorded", mod, now)
	}
	r := lit(src, recs[len(recs)-1].TS, now)
	if skipped > 0 {
		r.Why = "unreadable lines skipped: " + itoa(skipped)
	}
	return recs, r
}

func (f *fileSources) loop() ([]Run, Reading) {
	src := f.rel(f.loopDir()) + "/*.jsonl"
	now := f.now()
	states, err := loop.Runs(f.loopDir())
	if err != nil || len(states) == 0 {
		return nil, silent(src, "no loop journal yet — no run has been journaled", time.Time{}, now)
	}
	var out []Run
	var latest time.Time
	for _, s := range states {
		r := Run{ID: s.ID, As: s.As, Ask: s.Ask, Status: s.Status, At: s.At, Turns: s.Turns, Steps: len(s.Steps)}
		for _, st := range s.Steps {
			if st.Status != "done" && st.Status != "pending" {
				r.Failed++
			}
		}
		if t, err := time.Parse(time.RFC3339Nano, s.At); err == nil && t.After(latest) {
			latest = t
		}
		out = append(out, r)
	}
	return out, lit(src, latest, now)
}

var reLogHead = regexp.MustCompile(`^### \[([^\]]+)\]\s*(.*?)\s+—\s+(.*)$`)

// ParseLog splits log.md into its dated entries, newest first.
func ParseLog(text string) []LogEntry {
	var out []LogEntry
	var cur *LogEntry
	var body []string
	flush := func() {
		if cur != nil {
			cur.Body = strings.TrimRight(strings.Join(body, "\n"), "\n")
			out = append(out, *cur)
		}
		cur, body = nil, nil
	}
	for _, line := range strings.Split(text, "\n") {
		if m := reLogHead.FindStringSubmatch(line); m != nil {
			flush()
			cur = &LogEntry{At: m[1], Who: m[2], Title: m[3]}
			continue
		}
		if strings.HasPrefix(line, "### ") {
			flush()
			cur = &LogEntry{Title: strings.TrimPrefix(line, "### ")}
			continue
		}
		if cur != nil {
			body = append(body, line)
		}
	}
	flush()
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (f *fileSources) log() ([]LogEntry, Reading) {
	src := f.rel(f.logPath())
	now := f.now()
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		return nil, silent(src, "no log.md — the world has not written its first entry", time.Time{}, now)
	}
	entries := ParseLog(string(b))
	var at time.Time
	if info, err := os.Stat(f.logPath()); err == nil {
		at = info.ModTime()
	}
	if len(entries) == 0 {
		return nil, silent(src, "log.md holds no dated entry", at, now)
	}
	return entries, lit(src, at, now)
}

// laneOf places a mind by whom it serves (tempest's rule): any elf/kijin wearer → global;
// uniform slime wearers → zone; uniform orc wearers → verdict; mixed → shared; nobody → the
// race prefix of its name, else shared.
func laneOf(name string, wearerRanks []string, shared bool) string {
	if shared {
		return "shared"
	}
	set := map[string]bool{}
	for _, r := range wearerRanks {
		set[r] = true
	}
	if set["elf"] || set["kijin"] || set["rimuru"] {
		return "global"
	}
	base := 0
	for _, r := range []string{"slime", "orc"} {
		if set[r] {
			base++
		}
	}
	switch {
	case base == 1 && set["slime"]:
		return "zone"
	case base == 1 && set["orc"]:
		return "verdict"
	case base > 1:
		return "shared"
	}
	switch strings.SplitN(name, "-", 2)[0] {
	case "slime":
		return "zone"
	case "orc":
		return "verdict"
	case "elf", "kijin":
		return "global"
	}
	return "shared"
}

// BuildColony projects a loaded ontology into nodes, edges and lanes.
func BuildColony(w *onto.World) *Colony {
	g := w.Graph
	c := &Colony{Notes: append([]string(nil), w.Notes...)}
	for _, f := range w.Validate() {
		c.Findings = append(c.Findings, f.String())
	}
	docOf := func(t onto.Term) string {
		for _, d := range g.Objects(t, onto.Is("doc")) {
			for _, p := range g.Objects(d, onto.Is("path")) {
				return p.Value
			}
		}
		return ""
	}
	rankOf := func(t onto.Term) string {
		best := ""
		for _, ty := range g.Objects(t, onto.I(onto.RDFType)) {
			l := strings.ToLower(ty.Local())
			if l == "creature" || l == "class" || l == "fact" || l == "doc" || l == "mind" {
				continue
			}
			if best == "" || l == "slime" || l == "orc" || l == "elf" || l == "kijin" || l == "rimuru" {
				best = l
			}
		}
		return best
	}
	// creatures
	seen := map[string]bool{}
	var creatures []onto.Term
	for _, t := range g.Instances(onto.Is("Creature")) {
		if !t.IsIRI() || seen[t.Value] {
			continue
		}
		seen[t.Value] = true
		creatures = append(creatures, t)
	}
	wearers := map[string][]string{}
	for _, t := range creatures {
		n := Node{ID: t.Local(), Name: t.Local(), Kind: "creature", Rank: rankOf(t), Doc: docOf(t)}
		n.Lane = n.Rank
		for _, o := range g.Objects(t, onto.Is("owns")) {
			n.Owns = append(n.Owns, o.Value)
		}
		c.Nodes = append(c.Nodes, n)
		for _, bond := range []string{"truth", "verdict", "reports"} {
			for _, o := range g.Objects(t, onto.Is(bond)) {
				c.Edges = append(c.Edges, Edge{From: t.Local(), To: o.Local(), Bond: bond})
			}
		}
		for _, o := range g.Objects(t, onto.Is("above")) {
			direct := false
			for _, bond := range []string{"truth", "verdict", "reports"} {
				if g.Has(onto.Triple{S: t, P: onto.Is(bond), O: o}) {
					direct = true
				}
			}
			if !direct && isAssertedAbove(g, t, o) {
				c.Edges = append(c.Edges, Edge{From: t.Local(), To: o.Local(), Bond: "above"})
			}
		}
		for _, m := range g.Objects(t, onto.Is("wears")) {
			c.Edges = append(c.Edges, Edge{From: t.Local(), To: m.Local(), Bond: "wears"})
			wearers[m.Local()] = append(wearers[m.Local()], n.Rank)
		}
	}
	for _, m := range g.Instances(onto.Is("Mind")) {
		if !m.IsIRI() {
			continue
		}
		name := strings.TrimPrefix(m.Local(), "mind-")
		shared := false
		for _, v := range g.Objects(m, onto.Is("shared")) {
			shared = v.Value == "true"
		}
		c.Nodes = append(c.Nodes, Node{ID: m.Local(), Name: name, Kind: "mind", Lane: laneOf(name, wearers[m.Local()], shared), Doc: docOf(m), Shared: shared})
	}
	sort.Slice(c.Nodes, func(i, j int) bool { return c.Nodes[i].ID < c.Nodes[j].ID })
	sort.Slice(c.Edges, func(i, j int) bool {
		a, b := c.Edges[i], c.Edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Bond != b.Bond {
			return a.Bond < b.Bond
		}
		return a.To < b.To
	})
	c.Lanes = []string{"mind:zone", "slime", "mind:verdict", "orc", "mind:global", "elf", "rimuru"}
	extra := map[string]bool{}
	for _, n := range c.Nodes {
		if n.Kind == "creature" && n.Rank != "" && !containsStr(c.Lanes, n.Rank) && !extra[n.Rank] {
			extra[n.Rank] = true
			c.Lanes = append(c.Lanes, n.Rank)
		}
	}
	c.Lanes = append(c.Lanes, "mind:shared")
	return c
}

// isAssertedAbove keeps only the `above` bonds that are one hop (not the transitive closure).
func isAssertedAbove(g *onto.Graph, s, o onto.Term) bool {
	for _, mid := range g.Objects(s, onto.Is("above")) {
		if mid != o && g.Has(onto.Triple{S: mid, P: onto.Is("above"), O: o}) {
			return false
		}
	}
	return true
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func (f *fileSources) colony() *Colony {
	now := f.now()
	src := f.rel(onto.OntologyDirIn(f.root, f.layout)) + " + " + f.worldDir + "/{ranks}/*/"
	w, err := onto.LoadLayout(f.root, f.layout)
	if err != nil {
		return &Colony{Reading: silent(src, "ontology failed to load: "+err.Error(), time.Time{}, now)}
	}
	c := BuildColony(w)
	creatures := 0
	for _, n := range c.Nodes {
		if n.Kind == "creature" && n.ID != "rimuru" {
			creatures++
		}
	}
	if creatures == 0 {
		c.Reading = silent(src, "no creature on disk — the world has no ranks yet", time.Time{}, now)
		return c
	}
	c.Reading = lit(src, time.Time{}, now)
	return c
}

func (f *fileSources) memory() *MemoryView {
	now := f.now()
	src := f.worldDir + "/memory/"
	w, err := memory.OpenIn(f.root, f.worldDir)
	if err != nil {
		return &MemoryView{Reading: silent(src, err.Error(), time.Time{}, now)}
	}
	w.Now = f.now
	st := w.Status("")
	v := &MemoryView{Status: st, Desks: st.Desks, ByKind: map[string]int{}}
	notesPath := filepath.Join(f.dir, "memory", "shared", "notes.jsonl")
	var latest time.Time
	for _, raw := range memory.ReadJSONL(notesPath) {
		var n memory.Note
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		v.Notes = append(v.Notes, n)
		k := "untyped"
		if n.Kind != nil && *n.Kind != "" {
			k = *n.Kind
		}
		v.ByKind[k]++
		if t, err := time.Parse(time.RFC3339Nano, n.At); err == nil && t.After(latest) {
			latest = t
		}
	}
	for i, j := 0, len(v.Notes)-1; i < j; i, j = i+1, j-1 {
		v.Notes[i], v.Notes[j] = v.Notes[j], v.Notes[i]
	}
	if len(v.Notes) == 0 && len(st.Desks) == 0 && !st.Long.Indexed {
		v.Reading = silent(src, "no index, no desk, no shared note yet", time.Time{}, now)
	} else {
		v.Reading = lit(src, latest, now)
	}
	return v
}

func (f *fileSources) toolbox() *ToolboxView {
	now := f.now()
	src := f.worldDir + "/toolbox/registry.json + " + f.worldDir + "/instruments/toolbox/loads.jsonl"
	w, err := toolbox.OpenIn(f.root, f.worldDir)
	if err != nil {
		return &ToolboxView{Reading: silent(src, err.Error(), time.Time{}, now)}
	}
	w.Now = f.now
	st := w.Status(math.NaN())
	v := &ToolboxView{Status: st, Offered: map[string]bool{}, Loaded: map[string]bool{}}
	if st.Loaded.Reg != nil {
		v.Entries = st.Loaded.Reg.Entries
	}
	for _, raw := range memory.ReadJSONL(w.LoadsPath()) {
		var x struct {
			Ev    string `json:"ev"`
			Names []any  `json:"names"`
			Name  any    `json:"name"`
		}
		if json.Unmarshal(raw, &x) != nil {
			continue
		}
		switch x.Ev {
		case "offer":
			for _, n := range x.Names {
				if s, ok := n.(string); ok {
					v.Offered[s] = true
				}
			}
		case "load":
			if s, ok := x.Name.(string); ok {
				v.Loaded[s] = true
			}
		}
	}
	var at time.Time
	if info, err := os.Stat(w.LoadsPath()); err == nil {
		at = info.ModTime()
	}
	if len(v.Entries) == 0 {
		v.Reading = silent(src, "registry empty — no mind, command, tool or body found", at, now)
	} else {
		v.Reading = lit(src, at, now)
	}
	return v
}

var errOutside = errors.New("doc outside the world")

// doc reads a markdown file inside the world: the world dir or a mind dir, nothing else.
func (f *fileSources) doc(p string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean("/" + p))[1:]
	if !strings.HasSuffix(clean, ".md") {
		return "", errOutside
	}
	ok := false
	for _, base := range []string{f.worldDir + "/", ".opencode/skills/", ".opencode/skill/", ".claude/skills/", ".claude/agents/", ".opencode/agents/", ".opencode/agent/"} {
		if strings.HasPrefix(clean, base) {
			ok = true
		}
	}
	if !ok {
		return "", errOutside
	}
	b, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(clean)))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
