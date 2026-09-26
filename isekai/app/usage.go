package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/config"
	"github.com/Kaginari/agentic-playground/isekai/provider"
)

// UsageRecord is one provider call as the usage journal keeps it — the schema the board reads
// (isekai/board: UsageRecord) and the Harbor adapter sums: input, output, cacheRead,
// cacheWrite as ints; usd null when the model is unpriced (a cost is never guessed).
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

// Tokens is the record's whole spend.
func (u UsageRecord) Tokens() int { return u.Input + u.Output + u.CacheRead + u.CacheWrite }

// Spend is a running total for one body or one session.
type Spend struct {
	Usage    provider.Usage
	USD      float64
	Unpriced int // calls with no price: the USD figure is a floor
	Calls    int
}

func (s *Spend) add(r UsageRecord) {
	s.Usage = s.Usage.Add(provider.Usage{Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, CacheWrite: r.CacheWrite})
	s.Calls++
	if r.USD == nil {
		s.Unpriced++
	} else {
		s.USD += *r.USD
	}
}

// Tokens is the total token spend.
func (s Spend) Tokens() int {
	return s.Usage.Input + s.Usage.Output + s.Usage.CacheRead + s.Usage.CacheWrite
}

// UsageJournal appends one line per provider call to <dir>/<session>.jsonl and keeps the
// session's totals in memory (Nature 9: consumption is an instrument).
type UsageJournal struct {
	Dir     string
	Session string

	mu     sync.Mutex
	total  Spend
	bodies map[string]*Spend
	off    bool
}

// NewUsageJournal opens the journal for a session; dir "" keeps totals without writing.
func NewUsageJournal(dir, session string) *UsageJournal {
	return &UsageJournal{Dir: dir, Session: session, bodies: map[string]*Spend{}, off: dir == ""}
}

// Path is the session's journal file ("" when off).
func (j *UsageJournal) Path() string {
	if j == nil || j.off {
		return ""
	}
	return filepath.Join(j.Dir, j.Session+".jsonl")
}

// Record appends one call and updates the totals.
func (j *UsageJournal) Record(r UsageRecord) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if r.Session == "" {
		r.Session = j.Session
	}
	if r.TS.IsZero() {
		r.TS = time.Now().UTC()
	}
	j.total.add(r)
	b := j.bodies[r.Body]
	if b == nil {
		b = &Spend{}
		j.bodies[r.Body] = b
	}
	b.add(r)
	if j.off {
		return nil
	}
	if err := os.MkdirAll(j.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(j.Path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// Total is the session's spend so far.
func (j *UsageJournal) Total() Spend {
	if j == nil {
		return Spend{}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.total
}

// Body is one body's spend so far.
func (j *UsageJournal) Body(name string) Spend {
	if j == nil {
		return Spend{}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if b := j.bodies[name]; b != nil {
		return *b
	}
	return Spend{}
}

// ReadUsage reads every record under dir (all sessions), oldest file first.
func ReadUsage(dir string) ([]UsageRecord, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []UsageRecord
	for _, n := range names {
		f, err := os.Open(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		for sc.Scan() {
			var r UsageRecord
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				out = append(out, r)
			}
		}
		f.Close()
	}
	return out, nil
}

// Rollup sums records by body, office, rank, model and day, from `since` (zero = all).
type Rollup struct {
	Records  int
	Total    Spend
	ByBody   map[string]*Spend
	ByOffice map[string]*Spend
	ByRank   map[string]*Spend
	ByModel  map[string]*Spend
	ByDay    map[string]*Spend
	Sessions map[string]bool
}

// RollupOf folds records.
func RollupOf(recs []UsageRecord, since time.Time, session string) Rollup {
	r := Rollup{ByBody: map[string]*Spend{}, ByOffice: map[string]*Spend{}, ByRank: map[string]*Spend{}, ByModel: map[string]*Spend{}, ByDay: map[string]*Spend{}, Sessions: map[string]bool{}}
	bump := func(m map[string]*Spend, k string, rec UsageRecord) {
		if k == "" {
			k = "—"
		}
		s := m[k]
		if s == nil {
			s = &Spend{}
			m[k] = s
		}
		s.add(rec)
	}
	for _, rec := range recs {
		if !since.IsZero() && rec.TS.Before(since) {
			continue
		}
		if session != "" && rec.Session != session {
			continue
		}
		r.Records++
		r.Total.add(rec)
		r.Sessions[rec.Session] = true
		bump(r.ByBody, rec.Body, rec)
		bump(r.ByOffice, rec.Office, rec)
		bump(r.ByRank, rec.Rank, rec)
		bump(r.ByModel, rec.Model, rec)
		bump(r.ByDay, rec.TS.UTC().Format("2006-01-02"), rec)
	}
	return r
}

// Lines renders the rollup for a human: a total line and one table per key.
func (r Rollup) Lines() []string {
	cost := func(s *Spend) string {
		if s.Calls == s.Unpriced {
			return "unpriced"
		}
		c := fmt.Sprintf("$%.4f", s.USD)
		if s.Unpriced > 0 {
			c += fmt.Sprintf(" (+%d unpriced)", s.Unpriced)
		}
		return c
	}
	var out []string
	out = append(out, fmt.Sprintf("usage: %d calls · %d sessions · in %d (cache read %d, write %d) · out %d · %s",
		r.Records, len(r.Sessions), r.Total.Usage.Input, r.Total.Usage.CacheRead, r.Total.Usage.CacheWrite, r.Total.Usage.Output, cost(&r.Total)))
	table := func(title string, m map[string]*Spend) {
		if len(m) == 0 {
			return
		}
		out = append(out, "by "+title+":")
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return m[keys[i]].Tokens() > m[keys[j]].Tokens() })
		for _, k := range keys {
			s := m[k]
			out = append(out, fmt.Sprintf("  %-28s %3d calls  in %8d  out %7d  %s", k, s.Calls, s.Usage.Input+s.Usage.CacheRead+s.Usage.CacheWrite, s.Usage.Output, cost(s)))
		}
	}
	table("body", r.ByBody)
	table("office", r.ByOffice)
	table("rank", r.ByRank)
	table("model", r.ByModel)
	table("day", r.ByDay)
	return out
}

// Meter wraps a provider for one body: every call is journaled with the body's rank, office
// and model, priced from config; the session and court budgets are read before the call and
// a crossed line is reported through Over (the loop checkpoints on it).
type Meter struct {
	Provider provider.Provider
	Journal  *UsageJournal
	Body     string
	Rank     string
	Office   string
	Model    string // provider/model-id
	Price    *config.Price
	Session  config.Cap // budgets.session
	Court    config.Cap // budgets.court (applies when Depth > 0)
	Depth    int
	OnState  func(state string) // thinking · done (the live court's state line)
}

func (m *Meter) Name() string { return m.Provider.Name() }

// Over reports why the body may not make another call: a budget line reached.
func (m *Meter) Over() string {
	if m.Journal == nil {
		return ""
	}
	if !m.Session.Off() {
		t := m.Journal.Total()
		if m.Session.Tokens > 0 && t.Tokens() >= m.Session.Tokens {
			return fmt.Sprintf("session token budget reached (%d ≥ %d, budgets.session.tokens)", t.Tokens(), m.Session.Tokens)
		}
		if m.Session.USD > 0 && t.USD >= m.Session.USD {
			return fmt.Sprintf("session cost budget reached ($%.4f ≥ $%.2f, budgets.session.usd)", t.USD, m.Session.USD)
		}
	}
	if m.Depth > 0 && !m.Court.Off() {
		b := m.Journal.Body(m.Body)
		if m.Court.Tokens > 0 && b.Tokens() >= m.Court.Tokens {
			return fmt.Sprintf("court token budget reached for %s (%d ≥ %d, budgets.court.tokens)", m.Body, b.Tokens(), m.Court.Tokens)
		}
		if m.Court.USD > 0 && b.USD >= m.Court.USD {
			return fmt.Sprintf("court cost budget reached for %s ($%.4f ≥ $%.2f, budgets.court.usd)", m.Body, b.USD, m.Court.USD)
		}
	}
	return ""
}

// Complete calls the provider and journals the reading.
func (m *Meter) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if why := m.Over(); why != "" {
		return provider.Response{}, fmt.Errorf("budget: %s", why)
	}
	if m.OnState != nil {
		m.OnState("thinking")
		defer m.OnState("done")
	}
	resp, err := m.Provider.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	rec := UsageRecord{Body: m.Body, Rank: m.Rank, Office: m.Office, Model: m.Model, Provider: m.Provider.Name(),
		Input: resp.Usage.Input, Output: resp.Usage.Output, CacheRead: resp.Usage.CacheRead, CacheWrite: resp.Usage.CacheWrite}
	if m.Price != nil {
		usd := m.Price.Cost(resp.Usage.Input, resp.Usage.Output, resp.Usage.CacheRead, resp.Usage.CacheWrite)
		rec.USD = &usd
	}
	if m.Journal != nil {
		_ = m.Journal.Record(rec)
	}
	return resp, nil
}
