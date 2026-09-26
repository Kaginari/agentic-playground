// Package memory is the Go port of .isekai/tools/memory.js — the world's memory instrument
// (isekai.md §Memory tiers). Same files, same formats, same ranking: the port and the JS tool
// run against one .isekai/ interchangeably.
package memory

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	IndexV    = 2
	DeskLimit = 5
	PipeBuf   = 4096
)

var Unsaid = []string{"law", "colony", "territory"}

func IsUnsaid(k string) bool {
	for _, u := range Unsaid {
		if u == k {
			return true
		}
	}
	return false
}

// Fail is a wire FAIL: `@S FAIL` and one `@?` per hole, exit 2.
type Fail struct{ Holes []string }

func (f *Fail) Error() string { return strings.Join(f.Holes, "; ") }
func failf(format string, a ...any) error {
	return &Fail{[]string{fmt.Sprintf(format, a...)}}
}

// DefaultWorldDir is the world directory Open and the CLI look for; a distribution's binary
// sets it (".agent-one") before delegating, or opens explicitly with OpenIn.
var DefaultWorldDir = ".isekai"

// World is one reincarnated directory. Home is the machine home (~); Now the clock; Dir the
// world directory name (".isekai", or ".agent-one" under that distribution — the machine
// tier follows it: ~/<Dir>/shared/notes.jsonl).
type World struct {
	Root string
	Home string
	Dir  string
	Now  func() time.Time
}

// Open opens a world under the default directory name.
func Open(root string) (*World, error) { return OpenIn(root, DefaultWorldDir) }

// OpenIn opens a world whose world directory is named dir.
func OpenIn(root, dir string) (*World, error) {
	if dir == "" {
		dir = DefaultWorldDir
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	if !Exists(filepath.Join(abs, dir)) {
		return nil, failf("no %s/ under %s", dir, abs)
	}
	home, _ := os.UserHomeDir()
	return &World{Root: abs, Home: home, Dir: dir, Now: time.Now}, nil
}

func (w *World) dir() string {
	if w.Dir == "" {
		return DefaultWorldDir
	}
	return w.Dir
}

// Isekai is the world directory (<root>/<dir>); the name is the law's, the dir may not be.
func (w *World) Isekai() string { return filepath.Join(w.Root, w.dir()) }
func (w *World) shortDir() string { return filepath.Join(w.Isekai(), "memory", "short") }
func (w *World) longDir() string  { return filepath.Join(w.Isekai(), "memory", "long") }
func (w *World) sharedNotes() string {
	return filepath.Join(w.Isekai(), "memory", "shared", "notes.jsonl")
}
func (w *World) machineNotes() string {
	return filepath.Join(w.Home, w.dir(), "shared", "notes.jsonl")
}
func (w *World) IndexPath() string { return filepath.Join(w.longDir(), "index.json") }
func (w *World) now() string       { return NowISO(w.Now()) }
func (w *World) Rel(p string) string {
	r, err := filepath.Rel(w.Root, p)
	if err != nil {
		return p
	}
	return r
}
func (w *World) tilde(p string) string { return strings.Replace(p, w.Home, "~", 1) }

func (w *World) WorldName() string {
	if s, ok := Rd(filepath.Join(w.Isekai(), "name")); ok {
		return JSTrim(s)
	}
	return filepath.Base(w.Root)
}

// NormAs is String(flags.as || 'rimuru').toLowerCase().
func NormAs(as string) string {
	if as == "" {
		return "rimuru"
	}
	return strings.ToLower(as)
}

// ------------------------------------------------------------------ memories

// Mem is one ranked unit: a log entry, a doc section, a tool's header comment, a shared note.
// The index keeps term counts and a 200-char snippet, not the text — files are the truth.
type Mem struct {
	Kind     string         `json:"kind"`
	Src      string         `json:"src"`
	Title    string         `json:"title"`
	B        int            `json:"b"`
	TF       map[string]int `json:"tf"`
	DL       int            `json:"dl"`
	Snip     string         `json:"snip"`
	When     string         `json:"when,omitempty"`
	Tool     string         `json:"tool,omitempty"`
	Sec      int            `json:"sec,omitempty"`
	Mind     string         `json:"mind,omitempty"`
	Command  string         `json:"command,omitempty"`
	Creature string         `json:"creature,omitempty"`
	Race     string         `json:"race,omitempty"`
	// shared-note fields: live, never indexed
	Scope  string `json:"-"`
	By     string `json:"-"`
	Unsaid string `json:"-"`
}

func (m *Mem) Terms() (map[string]int, int) { return m.TF, m.DL }

type source struct {
	kind, as, p, name, race string
}

func (w *World) sources() []source {
	var out []source
	add := func(kind, as, p, name, race string) {
		if IsFile(p) {
			out = append(out, source{kind, as, p, name, race})
		}
	}
	add("episodic", "log", filepath.Join(w.Isekai(), "log.md"), "", "")
	for _, m := range Minds(w.Root) {
		add("procedural", "mind", m.P, m.Name, "")
	}
	for _, c := range Commands(w.Root) {
		add("procedural", "command", c.P, c.Name, "")
	}
	for _, e := range readDirSorted(filepath.Join(w.Isekai(), "tools"), false) {
		add("procedural", "tool", filepath.Join(w.Isekai(), "tools", e.Name()), e.Name(), "")
	}
	add("semantic", "law", filepath.Join(w.Isekai(), "isekai.md"), "", "")
	for _, c := range Creatures(w.Isekai()) {
		if c.Doc != "" {
			add("semantic", "creature", c.Doc, c.Name, c.Race)
		}
	}
	for _, p := range ListMd(filepath.Join(w.Isekai(), "canon")) {
		add("semantic", "canon", p, strings.TrimSuffix(filepath.Base(p), ".md"), "")
	}
	add("semantic", "readme", filepath.Join(w.Root, "README.md"), "", "")
	return out
}

func label(as, name string) string {
	switch as {
	case "mind", "creature":
		return name + " › "
	case "command":
		return "/" + name + " › "
	case "law":
		return "law › "
	case "canon":
		return "canon " + name + " › "
	case "readme":
		return "README › "
	}
	return ""
}

var (
	reLogEntry   = regexp.MustCompile(`(?m)^###\s+\[([^\]]+)\]\s*(.*)$`)
	reLogSplitAt = regexp.MustCompile(`^###\s+\[`)
	reComment    = JSRe(`^\s*(//|#)`)
)

// splitLog is text.split(/\n(?=###\s+\[)/).
func splitLog(text string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' && reLogSplitAt.MatchString(text[i+1:]) {
			parts = append(parts, text[start:i])
			start = i + 1
		}
	}
	return append(parts, text[start:])
}

func snip(text string, trim bool) string {
	s := CollapseWS(text)
	if trim {
		s = JSTrim(s)
	}
	return UTF16Slice(s, 0, 200)
}

func (w *World) HarvestLong() []*Mem {
	var mems []*Mem
	push := func(s source, title, text string, extra Mem) {
		tf := TermCounts(title + "\n" + text)
		m := extra
		m.Kind, m.Src, m.Title, m.B, m.TF, m.DL, m.Snip = s.kind, w.Rel(s.p), title, len(text), tf.Map, tf.DL, snip(text, true)
		mems = append(mems, &m)
	}
	for _, s := range w.sources() {
		text := RdOr(s.p)
		switch s.as {
		case "log":
			for _, e := range splitLog(text) {
				if m := reLogEntry.FindStringSubmatch(e); m != nil {
					push(s, JSTrim(m[2]), e, Mem{When: JSTrim(m[1])})
				}
			}
		case "tool":
			var head []string
			for _, l := range strings.Split(text, "\n") {
				if reComment.MatchString(l) {
					head = append(head, l)
				}
			}
			if len(head) > 40 {
				head = head[:40]
			}
			h := strings.Join(head, "\n")
			if JSTrim(h) != "" {
				push(s, "tool "+s.name, h, Mem{Tool: s.name})
			}
		default:
			tag := Mem{}
			switch s.as {
			case "mind":
				tag.Mind = s.name
			case "command":
				tag.Command = s.name
			case "creature":
				tag.Creature, tag.Race = s.name, s.race
			}
			for _, sec := range SectionMap(text).Sections {
				tag.Sec = sec.N
				push(s, label(s.as, s.name)+sec.T, sec.Text, tag)
			}
		}
	}
	return mems
}

// Note is one shared-note line as written by remember.
type Note struct {
	At    string  `json:"at"`
	By    string  `json:"by"`
	World string  `json:"world"`
	Tag   *string `json:"tag"`
	Kind  *string `json:"kind"`
	Text  string  `json:"text"`
}

func (w *World) HarvestShared() []*Mem {
	var out []*Mem
	for _, sp := range [][2]string{{"world", w.sharedNotes()}, {"machine", w.machineNotes()}} {
		scope, p := sp[0], sp[1]
		for _, raw := range ReadJSONL(p) {
			var n map[string]any
			if json.Unmarshal(raw, &n) != nil {
				continue
			}
			text, ok := n["text"].(string)
			if !ok {
				continue
			}
			by := JSString(n["by"], has(n, "by"))
			tag, kind := "", ""
			if Truthy(n["tag"]) {
				tag = JSString(n["tag"], true)
			}
			if Truthy(n["kind"]) {
				kind = JSString(n["kind"], true)
			}
			unsaid := ""
			if k, _ := n["kind"].(string); IsUnsaid(k) {
				unsaid = k
			}
			title := scope + " note by " + by
			if tag != "" {
				title += " #" + tag
			}
			if unsaid != "" {
				title += " [" + unsaid + "]"
			}
			src := w.tilde(p)
			if scope == "world" {
				src = w.Rel(p)
			}
			when := ""
			if s, ok := n["at"].(string); ok {
				when = s
			}
			tf := TermCounts(by + " " + tag + " " + kind + "\n" + text)
			out = append(out, &Mem{Kind: "shared", Scope: scope, Src: src, Title: title, When: when, By: by, Unsaid: unsaid,
				B: len(text), Snip: snip(text, false), TF: tf.Map, DL: tf.DL})
		}
	}
	return out
}

func has(m map[string]any, k string) bool { _, ok := m[k]; return ok }

// ------------------------------------------------------------------ index

type Index struct {
	V       int                `json:"v"`
	World   string             `json:"world"`
	BuiltAt string             `json:"builtAt"`
	Head    *string            `json:"head"`
	Sources Stamps             `json:"sources"`
	N       int                `json:"n"`
	AvgDL   float64            `json:"avgdl"`
	IDF     map[string]float64 `json:"idf"`
	Mems    []*Mem             `json:"mems"`
}

func (i *Index) stats() Stats { return Stats{N: i.N, AvgDL: i.AvgDL, IDF: i.IDF} }

func (w *World) sourceStamps() Stamps {
	var o Stamps
	for _, s := range w.sources() {
		if ms, ok := MtimeMs(s.p); ok {
			o.Add(w.Rel(s.p), ms)
		}
	}
	return o
}

func (w *World) BuildIndex() (*Index, error) {
	mems := w.HarvestLong()
	docs := make([]Doc, len(mems))
	for i, m := range mems {
		docs[i] = m
	}
	st := StatsOf(docs)
	idx := &Index{V: IndexV, World: w.WorldName(), BuiltAt: w.now(), Head: GitHead(w.Root), Sources: w.sourceStamps(),
		N: st.N, AvgDL: st.AvgDL, IDF: st.IDF, Mems: mems}
	if idx.Mems == nil {
		idx.Mems = []*Mem{}
	}
	data, err := MarshalJS(idx)
	if err != nil {
		return nil, err
	}
	return idx, WriteAtomic(w.IndexPath(), data)
}

// LoadIndex returns the index, or nil and the hole that names the silence.
func (w *World) LoadIndex() (*Index, string) {
	if !Exists(w.IndexPath()) {
		return nil, "no long-term index — run `memory.js index`"
	}
	raw := RdOr(w.IndexPath())
	var probe struct {
		V    *float64        `json:"v"`
		Mems json.RawMessage `json:"mems"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, fmt.Sprintf("index unreadable (%s) — rerun `memory.js index`", w.Rel(w.IndexPath()))
	}
	if probe.V == nil || *probe.V != IndexV || !strings.HasPrefix(strings.TrimSpace(string(probe.Mems)), "[") {
		return nil, "index built by an older memory.js — rerun `memory.js index`"
	}
	var idx Index
	if err := json.Unmarshal([]byte(raw), &idx); err != nil {
		return nil, fmt.Sprintf("index unreadable (%s) — rerun `memory.js index`", w.Rel(w.IndexPath()))
	}
	return &idx, ""
}

func say(n []string, what string) string {
	if len(n) == 0 {
		return ""
	}
	s := fmt.Sprintf("%d %s: %s", len(n), what, strings.Join(n[:min(3, len(n))], ", "))
	if len(n) > 3 {
		s += ", …"
	}
	return s
}

func (w *World) IndexHoles(idx *Index, why string) []string {
	if idx == nil {
		return []string{why}
	}
	var h []string
	if head := GitHead(w.Root); idx.Head != nil && head != nil && *idx.Head != *head {
		h = append(h, "index older than HEAD — rerun `memory.js index` before trusting")
	}
	if parts := StaleParts(w.sourceStamps(), idx.Sources); len(parts) > 0 {
		h = append(h, "index older than its sources — "+strings.Join(parts, "; ")+" — rerun `memory.js index`")
	}
	return h
}

// ------------------------------------------------------------------ relations

func (w *World) RelationsOf(name string) Relations { return RelationsOf(w.Root, w.Isekai(), name) }

// RelationBoost is the wire's typed bonds applied to memory: small on purpose.
func RelationBoost(m *Mem, R Relations) float64 {
	bst := 0.0
	if R.Self != "rimuru" && Has(m.TF, R.Self) {
		bst += 0.15
	}
	if R.Parent != "" && Has(m.TF, R.Parent) {
		bst += 0.10
	}
	for _, c := range R.Children {
		if Has(m.TF, c) {
			bst += 0.10
			break
		}
	}
	for _, n := range R.Minds {
		if m.Mind == n || Has(m.TF, n) {
			bst += 0.08
			break
		}
	}
	for _, z := range R.Zone {
		if Has(m.TF, z) {
			bst += 0.08
			break
		}
	}
	if m.Kind == "shared" && m.By == R.Self {
		bst += 0.05
	}
	return bst
}

// ------------------------------------------------------------------ short: the semantic cache

func isCacheChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}

func (w *World) CachePath(as string) string {
	return filepath.Join(w.shortDir(), ReplaceUnits(as, isCacheChar, "-")+".jsonl")
}

type cacheEntry struct {
	H       string             `json:"h"`
	ID      string             `json:"id"`
	At      string             `json:"at"`
	As      string             `json:"as"`
	Q       string             `json:"q"`
	F       string             `json:"f"`
	Sig     string             `json:"sig"`
	QV      map[string]float64 `json:"qv"`
	Results []Result           `json:"results"`
	hits    int
}

func (w *World) cacheRead(as string) []*cacheEntry {
	by := map[string]*cacheEntry{}
	var order []string
	for _, raw := range ReadJSONL(w.CachePath(as)) {
		var e cacheEntry
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		if e.H != "" {
			if hit, ok := by[e.H]; ok {
				hit.hits++
			}
		} else if e.ID != "" {
			prev, seen := by[e.ID]
			if seen {
				e.hits = prev.hits
			} else {
				order = append(order, e.ID)
			}
			ec := e
			by[e.ID] = &ec
		}
	}
	out := make([]*cacheEntry, 0, len(order))
	for _, id := range order {
		out = append(out, by[id])
	}
	return out
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func (w *World) cacheSig(idx *Index) string {
	built := "noindex"
	if idx != nil {
		built = idx.BuiltAt
	}
	return fmt.Sprintf("%s|%d|%d", built, fileSize(w.sharedNotes()), fileSize(w.machineNotes()))
}

type cacheHit struct {
	s float64
	e *cacheEntry
}

func (w *World) cacheLookup(as string, qv *Vec, sig, f string, threshold float64) *cacheHit {
	var best *cacheHit
	for _, e := range w.cacheRead(as) {
		if e.Sig != sig || e.F != f || e.QV == nil {
			continue
		}
		s := Cosine(qv, e.QV)
		if s >= threshold && (best == nil || s > best.s) {
			best = &cacheHit{s, e}
		}
	}
	return best
}

// ------------------------------------------------------------------ short: context window + desks

// ContextReading is the running session's context occupancy, read off its transcript.
type ContextReading struct {
	Available             bool
	Tokens, Limit, Stress int
	Zone, Why             string
}

func (c ContextReading) MarshalJSON() ([]byte, error) {
	if !c.Available {
		return MarshalJS(OJ{{"available", false}, {"why", c.Why}})
	}
	return MarshalJS(OJ{{"available", true}, {"tokens", c.Tokens}, {"limit", c.Limit}, {"stress", c.Stress}, {"zone", c.Zone}})
}

func (c *ContextReading) UnmarshalJSON(b []byte) error {
	var v struct {
		Available bool   `json:"available"`
		Tokens    int    `json:"tokens"`
		Limit     int    `json:"limit"`
		Stress    int    `json:"stress"`
		Zone      string `json:"zone"`
		Why       string `json:"why"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*c = ContextReading{v.Available, v.Tokens, v.Limit, v.Stress, v.Zone, v.Why}
	return nil
}

func (w *World) ContextReading() ContextReading {
	slug := ReplaceUnits(w.Root, isASCIIAlnum, "-")
	dir := filepath.Join(w.Home, ".claude", "projects", slug)
	if !Exists(dir) {
		return ContextReading{Why: "no Claude Code transcripts for this root"}
	}
	type tf struct {
		f string
		t float64
	}
	var files []tf
	for _, e := range readDirSorted(dir, false) {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			ms, _ := MtimeMs(filepath.Join(dir, e.Name()))
			files = append(files, tf{e.Name(), ms})
		}
	}
	if len(files) == 0 {
		return ContextReading{Why: "no transcript"}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].t > files[j].t })
	var last map[string]any
	for _, l := range strings.Split(RdOr(filepath.Join(dir, files[0].f)), "\n") {
		if l == "" {
			continue
		}
		var d struct {
			Type    string `json:"type"`
			Message struct {
				Usage map[string]any `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(l), &d) == nil && d.Type == "assistant" && d.Message.Usage != nil {
			last = d.Message.Usage
		}
	}
	if last == nil {
		return ContextReading{Why: "no assistant usage yet"}
	}
	num := func(k string) int {
		f, _ := last[k].(float64)
		return int(f)
	}
	tokens := num("input_tokens") + num("cache_read_input_tokens") + num("cache_creation_input_tokens")
	const limit, stress = 200000, 180000
	zone := "within budget"
	if tokens >= stress {
		zone = "STRESS"
	} else if float64(tokens) >= stress*0.75 {
		zone = "approaching"
	}
	return ContextReading{Available: true, Tokens: tokens, Limit: limit, Stress: stress, Zone: zone}
}

// Desk is one dated ## Thoughts desk: a Mind's, or a creature doc's hat.
type Desk struct {
	Mind     string  `json:"mind"`
	WornBy   *string `json:"wornBy"`
	Thoughts int     `json:"thoughts"`
	Limit    int     `json:"limit"`
	Stress   string  `json:"stress"`
	Src      string  `json:"src"`
}

var (
	reThoughts   = JSRe(`(?i)##\s*Thoughts`)
	reDeskEnd    = JSRe(`\n##?\s`)
	reDeskBullet = JSRe(`^\s*(-|\*|###)`)
	reDate       = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
)

func countThoughts(doc string) int {
	loc := reThoughts.FindStringIndex(doc)
	if loc == nil {
		return 0
	}
	body := doc[loc[1]:]
	if end := reDeskEnd.FindStringIndex(body); end != nil {
		body = body[:end[0]]
	}
	n := 0
	for _, l := range strings.Split(body, "\n") {
		if reDeskBullet.MatchString(l) && reDate.MatchString(l) {
			n++
		}
	}
	return n
}

func (w *World) Desks() []Desk {
	var out []Desk
	read := func(name, p string, owner *string) {
		n := countThoughts(RdOr(p))
		stress := "ok"
		if n > DeskLimit {
			stress = "STRESSED"
		} else if n == DeskLimit {
			stress = "at limit"
		}
		out = append(out, Desk{name, owner, n, DeskLimit, stress, w.Rel(p)})
	}
	for _, m := range Minds(w.Root) {
		read(m.Name, m.P, nil)
	}
	for _, c := range Creatures(w.Isekai()) {
		if c.Doc != "" {
			owner := c.Name
			read(c.Name+"@hat", c.Doc, &owner)
		}
	}
	return out
}

// ------------------------------------------------------------------ commands

type StatusReport struct {
	World string `json:"world"`
	As    string `json:"as"`
	At    string `json:"at"`
	Short struct {
		ContextWindow ContextReading `json:"contextWindow"`
		WorkingMemory struct {
			Desks    int      `json:"desks"`
			Stressed []string `json:"stressed"`
			AtLimit  []string `json:"atLimit"`
		} `json:"workingMemory"`
		SemanticCache struct {
			Entries int    `json:"entries"`
			Hits    int    `json:"hits"`
			Src     string `json:"src"`
		} `json:"semanticCache"`
	} `json:"short"`
	Long struct {
		Indexed    bool    `json:"indexed"`
		BuiltAt    *string `json:"builtAt"`
		Head       *string `json:"head"`
		Stale      bool    `json:"stale"`
		Episodic   *int    `json:"episodic"`
		Procedural *int    `json:"procedural"`
		Semantic   *int    `json:"semantic"`
		Src        string  `json:"src"`
	} `json:"long"`
	Shared struct {
		World   int `json:"world"`
		Machine int `json:"machine"`
		Unsaid  struct {
			Law       int `json:"law"`
			Colony    int `json:"colony"`
			Territory int `json:"territory"`
		} `json:"unsaid"`
		Src []string `json:"src"`
	} `json:"shared"`
	Holes []string `json:"@?"`
	Desks []Desk   `json:"-"`
	Why   string   `json:"-"`
}

func (w *World) Status(as string) *StatusReport {
	as = NormAs(as)
	idx, why := w.LoadIndex()
	shared := w.HarvestShared()
	ctx := w.ContextReading()
	desks := w.Desks()
	cache := w.cacheRead(as)
	hits := 0
	for _, e := range cache {
		hits += e.hits
	}
	holes := w.IndexHoles(idx, why)
	if !ctx.Available {
		holes = append(holes, "context window unmeasured — "+ctx.Why)
	}
	r := &StatusReport{World: w.WorldName(), As: as, At: w.now(), Holes: Nz(holes), Desks: desks, Why: why}
	r.Short.ContextWindow = ctx
	r.Short.WorkingMemory.Desks = len(desks)
	r.Short.WorkingMemory.Stressed, r.Short.WorkingMemory.AtLimit = []string{}, []string{}
	for _, d := range desks {
		switch d.Stress {
		case "STRESSED":
			r.Short.WorkingMemory.Stressed = append(r.Short.WorkingMemory.Stressed, d.Mind)
		case "at limit":
			r.Short.WorkingMemory.AtLimit = append(r.Short.WorkingMemory.AtLimit, d.Mind)
		}
	}
	r.Short.SemanticCache.Entries, r.Short.SemanticCache.Hits, r.Short.SemanticCache.Src = len(cache), hits, w.Rel(w.CachePath(as))
	r.Long.Src = w.Rel(w.IndexPath())
	if idx != nil {
		r.Long.Indexed = true
		b := idx.BuiltAt
		r.Long.BuiltAt, r.Long.Head = &b, idx.Head
		for _, h := range holes {
			if strings.HasPrefix(h, "index older") {
				r.Long.Stale = true
			}
		}
		count := func(k string) *int {
			n := 0
			for _, m := range idx.Mems {
				if m.Kind == k {
					n++
				}
			}
			return &n
		}
		r.Long.Episodic, r.Long.Procedural, r.Long.Semantic = count("episodic"), count("procedural"), count("semantic")
	}
	for _, s := range shared {
		if s.Scope == "world" {
			r.Shared.World++
		} else {
			r.Shared.Machine++
		}
		switch s.Unsaid {
		case "law":
			r.Shared.Unsaid.Law++
		case "colony":
			r.Shared.Unsaid.Colony++
		case "territory":
			r.Shared.Unsaid.Territory++
		}
	}
	r.Shared.Src = []string{w.Rel(w.sharedNotes()), w.tilde(w.machineNotes())}
	return r
}

type IndexResult struct {
	S          string   `json:"@S"`
	World      string   `json:"world"`
	N          int      `json:"n"`
	Episodic   int      `json:"episodic"`
	Procedural int      `json:"procedural"`
	Semantic   int      `json:"semantic"`
	Head       *string  `json:"head"`
	Src        string   `json:"src"`
	Holes      []string `json:"@?"`
}

func (w *World) Index() (*IndexResult, error) {
	idx, err := w.BuildIndex()
	if err != nil {
		return nil, failf("index crashed: %v", err)
	}
	r := &IndexResult{S: "INDEXED", World: idx.World, N: idx.N, Head: idx.Head, Src: w.Rel(w.IndexPath()), Holes: []string{}}
	for _, m := range idx.Mems {
		switch m.Kind {
		case "episodic":
			r.Episodic++
		case "procedural":
			r.Procedural++
		case "semantic":
			r.Semantic++
		}
	}
	if idx.N == 0 {
		r.Holes = append(r.Holes, fmt.Sprintf("nothing to index under %s — no log entries, Minds, commands, tools, law sections, creature docs, canon or README", w.Root))
	}
	return r, nil
}

// Result is one recalled memory: the rounded scores the JS stores and prints.
type Result struct {
	Score   float64 `json:"score"`
	Sim     float64 `json:"sim"`
	Rel     float64 `json:"rel"`
	Rec     float64 `json:"rec"`
	Kind    string  `json:"kind"`
	Title   string  `json:"title"`
	Src     string  `json:"src"`
	Sec     int     `json:"sec,omitempty"`
	When    string  `json:"when,omitempty"`
	Snippet string  `json:"snippet"`
}

type RecallOpts struct {
	As      string
	Tier    string // long | shared | all (default all)
	Kind    string // episodic | procedural | semantic | law | colony | territory
	K       int    // 0 = 5
	NoCache bool
}

type RecallResult struct {
	S        string   `json:"@S"`
	As       string   `json:"as"`
	Q        string   `json:"q"`
	CacheSim *float64 `json:"cacheSim"`
	Results  []Result `json:"results"`
	Holes    []string `json:"@?"`
	hitSim   float64
}

// Bytes is Buffer.byteLength(JSON.stringify(results)) — the @E figure.
func (r *RecallResult) Bytes() int {
	b, _ := MarshalJS(r.Results)
	return len(b)
}

func (w *World) Recall(q string, o RecallOpts) (*RecallResult, error) {
	q = JSTrim(q)
	if q == "" {
		return nil, failf("recall needs a question")
	}
	K := o.K
	if K == 0 {
		K = 5
	}
	if K < 0 {
		return nil, failf("-k needs a positive integer, got %d", o.K)
	}
	tier := o.Tier
	if tier == "" {
		tier = "all"
	}
	if tier != "long" && tier != "shared" && tier != "all" {
		return nil, failf("--tier must be long|shared|all, got %s", tier)
	}
	kind := o.Kind
	unsaidKind := ""
	if IsUnsaid(kind) {
		unsaidKind = kind
	}
	if kind != "" && unsaidKind == "" && kind != "episodic" && kind != "procedural" && kind != "semantic" {
		return nil, failf("--kind must be episodic|procedural|semantic|%s, got %s", strings.Join(Unsaid, "|"), kind)
	}
	if unsaidKind != "" && tier == "long" {
		return nil, failf("--kind %s filters shared notes; use --tier shared|all", kind)
	}
	as := NormAs(o.As)
	idx, why := w.LoadIndex()
	var pool []*Mem
	if tier != "shared" && idx != nil {
		pool = append(pool, idx.Mems...)
	}
	var shared []*Mem
	if tier != "long" {
		shared = w.HarvestShared()
	}
	pool = append(pool, shared...)
	var S Stats
	if idx != nil {
		S = idx.stats()
	} else {
		docs := make([]Doc, len(pool))
		for i, m := range pool {
			docs[i] = m
		}
		S = StatsOf(docs)
	}
	qtf := TermCounts(q)
	R := w.RelationsOf(as)
	kindF := kind
	if kindF == "" {
		kindF = "*"
	}
	f := fmt.Sprintf("%s/%s/%d", tier, kindF, K)
	sig := w.cacheSig(idx)
	qv := QVec(qtf, S.IDFQ)
	var hit *cacheHit
	if !o.NoCache && len(qtf.Order) > 0 {
		hit = w.cacheLookup(as, qv, sig, f, 0.92)
	}
	now := w.Now()
	var ranked []Result
	switch {
	case hit != nil:
		ranked = hit.e.Results
	case len(qtf.Order) > 0:
		for _, m := range pool {
			if kind != "" {
				if unsaidKind != "" {
					if m.Unsaid != unsaidKind {
						continue
					}
				} else if m.Kind != kind {
					continue
				}
			}
			sim, relb, rec := BM25(qtf, m.TF, m.DL, S.AvgDL, S.IDFQ), RelationBoost(m, R), Recency(m.When, now)
			r := Result{Score: ToFixed(sim+relb+rec, 4), Sim: ToFixed(sim, 4), Rel: ToFixed(relb, 2), Rec: ToFixed(rec, 3),
				Kind: m.Kind, Title: m.Title, Src: m.Src, Sec: m.Sec, When: m.When, Snippet: m.Snip}
			if r.Sim > 0 {
				ranked = append(ranked, r)
			}
		}
		sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
		if len(ranked) > K {
			ranked = ranked[:K]
		}
	}
	if ranked == nil {
		ranked = []Result{}
	}
	if hit != nil {
		AppendLine(w.CachePath(as), OJ{{"h", hit.e.ID}, {"at", w.now()}})
	} else if !o.NoCache && len(ranked) > 0 {
		sum := md5.Sum([]byte(sig + "|" + f + "|" + q))
		AppendLine(w.CachePath(as), OJ{{"id", hex.EncodeToString(sum[:])[:12]}, {"at", w.now()}, {"as", as}, {"q", q}, {"f", f}, {"sig", sig}, {"qv", qv.OJ()}, {"results", ranked}})
	}
	holes := []string{}
	if len(qtf.Order) == 0 {
		holes = append(holes, "question has no content words after stop-word removal — nothing to rank")
	}
	if tier != "shared" {
		holes = append(holes, w.IndexHoles(idx, why)...)
	}
	if tier == "shared" && len(shared) == 0 {
		holes = append(holes, "no shared notes yet — `memory.js remember \"<note>\"` writes the first")
	}
	if unsaidKind != "" && len(shared) > 0 {
		found := false
		for _, m := range shared {
			if m.Unsaid == unsaidKind {
				found = true
			}
		}
		if !found {
			holes = append(holes, fmt.Sprintf("no shared note carries kind %s yet — `memory.js remember \"<note>\" --kind %s` writes the first", unsaidKind, unsaidKind))
		}
	}
	res := &RecallResult{S: "MISS", As: as, Q: q, Results: ranked, Holes: holes}
	if hit != nil {
		cs := ToFixed(hit.s, 3)
		res.S, res.CacheSim, res.hitSim = "HIT", &cs, hit.s
	}
	return res, nil
}

type RememberOpts struct {
	As      string
	Machine bool
	Tag     string
	Kind    string // law | colony | territory
}

type RememberResult struct {
	Scope string
	Src   string
	Bytes int
	Note  Note
	Holes []string
}

func (r *RememberResult) MarshalJSON() ([]byte, error) {
	return MarshalJS(OJ{{"@S", "REMEMBERED"}, {"scope", r.Scope}, {"src", r.Src}, {"bytes", r.Bytes},
		{"at", r.Note.At}, {"by", r.Note.By}, {"world", r.Note.World}, {"tag", r.Note.Tag}, {"kind", r.Note.Kind}, {"text", r.Note.Text}, {"@?", Nz(r.Holes)}})
}

func (w *World) Remember(text string, o RememberOpts) (*RememberResult, error) {
	text = JSTrim(text)
	if text == "" {
		return nil, failf("remember needs a note")
	}
	if o.Kind != "" && !IsUnsaid(o.Kind) {
		return nil, failf("--kind must be %s (isekai.md §The unsaid), got %s", strings.Join(Unsaid, "|"), o.Kind)
	}
	scope, p := "world", w.sharedNotes()
	if o.Machine {
		scope, p = "machine", w.machineNotes()
	}
	n := Note{At: w.now(), By: NormAs(o.As), World: w.WorldName(), Text: text}
	if o.Tag != "" {
		t := o.Tag
		n.Tag = &t
	}
	if o.Kind != "" {
		k := o.Kind
		n.Kind = &k
	}
	bytes, err := AppendLine(p, OJ{{"at", n.At}, {"by", n.By}, {"world", n.World}, {"tag", n.Tag}, {"kind", n.Kind}, {"text", n.Text}})
	if err != nil {
		return nil, failf("remember crashed: %v", err)
	}
	r := &RememberResult{Scope: scope, Bytes: bytes, Note: n, Holes: []string{}}
	if bytes > PipeBuf {
		r.Holes = append(r.Holes, fmt.Sprintf("note line is %d bytes > PIPE_BUF %d — not atomic against a concurrent writer; point, don't carry (long form in a doc, its path in the note)", bytes, PipeBuf))
	}
	if o.Machine {
		r.Src = w.tilde(p)
	} else {
		r.Src = w.Rel(p)
	}
	return r, nil
}

type ForgetResult struct {
	S   string `json:"@S"`
	As  string `json:"as"`
	N   int    `json:"n"`
	Src string `json:"src"`
}

// Forget clears the asker's semantic cache — the only memory ever forgotten (Law 4).
func (w *World) Forget(as string) (*ForgetResult, error) {
	as = NormAs(as)
	p := w.CachePath(as)
	n := len(w.cacheRead(as))
	if Exists(p) {
		if err := os.Remove(p); err != nil {
			return nil, failf("forget crashed: %v", err)
		}
	}
	return &ForgetResult{"FORGOT", as, n, w.Rel(p)}, nil
}
