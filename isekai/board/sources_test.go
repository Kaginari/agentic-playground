package board

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/onto"
)

var fixedNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func fixtureRoot(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fixtureSources(t *testing.T, name string) Sources {
	return FileSources(fixtureRoot(t, name), ".isekai", onto.Layout{}, func() time.Time { return fixedNow })
}

func TestUsageReadAndRollup(t *testing.T) {
	s := fixtureSources(t, "world")
	recs, rd := s.Usage()
	if !rd.Lit {
		t.Fatalf("usage should be lit: %s", rd)
	}
	if len(recs) != 5 {
		t.Fatalf("want 5 records (one bad line skipped), got %d", len(recs))
	}
	if !strings.Contains(rd.Why, "skipped: 1") {
		t.Errorf("the skipped line must be named: %q", rd.Why)
	}
	since, rng := RangeSince("7d", fixedNow)
	r := Rollup(recs, since, rng)
	if r.Records != 5 || r.Total.Calls != 5 {
		t.Errorf("total calls %d", r.Total.Calls)
	}
	if r.Total.Tokens() != 1200+300+20000+5000+800+900+500+100+2000+400+30000+1000+700+1500 {
		t.Errorf("total tokens %d", r.Total.Tokens())
	}
	if r.Total.Unpriced != 1 {
		t.Errorf("one call is unpriced (usd null), got %d", r.Total.Unpriced)
	}
	if len(r.ByDay) != 2 || r.ByDay[0].Key != "2026-09-25" {
		t.Errorf("by day: %+v", r.ByDay)
	}
	if r.ByBody[0].Key != "rimuru" {
		t.Errorf("biggest body should be rimuru: %+v", r.ByBody)
	}
	if len(r.Sessions) != 2 {
		t.Errorf("sessions: %v", r.Sessions)
	}
	day, _ := RangeSince("24h", fixedNow)
	if today := Rollup(recs, day, "24h"); today.Records != 2 {
		t.Errorf("24h should hold today's 2 calls, got %d", today.Records)
	}
	if all, _ := RangeSince("all", fixedNow); !all.IsZero() {
		t.Error("all = zero time")
	}
}

func TestUsageSilentOnEmptyWorld(t *testing.T) {
	s := fixtureSources(t, "empty")
	recs, rd := s.Usage()
	if len(recs) != 0 || rd.Lit {
		t.Fatalf("empty world must be silent, not zero: %+v", rd)
	}
	if !strings.HasPrefix(rd.String(), "never lit") {
		t.Errorf("reading: %s", rd)
	}
	if _, rd := s.Loop(); rd.Lit {
		t.Error("loop should be silent")
	}
	if _, rd := s.Log(); rd.Lit {
		t.Error("log should be silent")
	}
	if c := s.Colony(); c.Reading.Lit {
		t.Error("colony should be silent with no creatures")
	}
	if m := s.Memory(); m.Reading.Lit {
		t.Error("memory should be silent")
	}
	if tb := s.Toolbox(); tb.Reading.Lit {
		t.Error("toolbox should be silent")
	}
}

func TestLogParse(t *testing.T) {
	s := fixtureSources(t, "world")
	entries, rd := s.Log()
	if !rd.Lit || len(entries) != 2 {
		t.Fatalf("log: %d entries, %s", len(entries), rd)
	}
	if entries[0].At != "2026-09-25T18:30:00+02:00" || !strings.HasPrefix(entries[0].Who, "court-body") {
		t.Errorf("newest first: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Body, "token TTL is 15m") {
		t.Errorf("body kept: %q", entries[0].Body)
	}
}

func TestLoopRuns(t *testing.T) {
	s := fixtureSources(t, "world")
	runs, rd := s.Loop()
	if !rd.Lit || len(runs) != 1 {
		t.Fatalf("runs: %+v %s", runs, rd)
	}
	if runs[0].Status != "PASS" || runs[0].As != "slime-auth" || runs[0].Steps != 1 {
		t.Errorf("run: %+v", runs[0])
	}
}

func TestColonyGraph(t *testing.T) {
	s := fixtureSources(t, "world")
	c := s.Colony()
	if !c.Reading.Lit {
		t.Fatalf("colony: %s", c.Reading)
	}
	byID := map[string]Node{}
	for _, n := range c.Nodes {
		byID[n.ID] = n
	}
	if byID["slime-auth"].Rank != "slime" || byID["orc-core"].Rank != "orc" || byID["elf-voice"].Rank != "elf" {
		t.Errorf("ranks: %+v", byID)
	}
	if byID["mind-slime-auth-mind"].Lane != "zone" {
		t.Errorf("slime-auth-mind lane: %q", byID["mind-slime-auth-mind"].Lane)
	}
	if byID["mind-tdd"].Lane != "shared" || !byID["mind-tdd"].Shared {
		t.Errorf("tdd (host skill, no race prefix) is shared: %+v", byID["mind-tdd"])
	}
	if byID["slime-auth"].Doc != ".isekai/slime/auth/README.md" {
		t.Errorf("doc: %q", byID["slime-auth"].Doc)
	}
	want := map[string]bool{"slime-auth truth orc-core": false, "orc-core verdict elf-voice": false, "elf-voice reports rimuru": false, "slime-auth wears mind-tdd": false}
	for _, e := range c.Edges {
		k := e.From + " " + e.Bond + " " + e.To
		if _, ok := want[k]; ok {
			want[k] = true
		}
		if e.Bond == "above" {
			t.Errorf("transitive closure leaked into the drawing: %+v", e)
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("missing edge %s", k)
		}
	}
	if c.Lanes[0] != "mind:zone" || c.Lanes[len(c.Lanes)-1] != "mind:shared" {
		t.Errorf("lanes: %v", c.Lanes)
	}
}

func TestMemoryAndToolboxViews(t *testing.T) {
	s := fixtureSources(t, "world")
	m := s.Memory()
	if !m.Reading.Lit {
		t.Fatalf("memory: %s", m.Reading)
	}
	if len(m.Notes) != 3 || m.ByKind["colony"] != 1 || m.ByKind["territory"] != 1 || m.ByKind["untyped"] != 1 {
		t.Errorf("notes by kind: %v (%d notes)", m.ByKind, len(m.Notes))
	}
	if m.Notes[0].Text != "an untyped note" {
		t.Errorf("newest first: %+v", m.Notes[0])
	}
	if len(m.Desks) == 0 {
		t.Error("the fixture mind has a desk")
	}
	tb := s.Toolbox()
	if !tb.Reading.Lit {
		t.Fatalf("toolbox: %s", tb.Reading)
	}
	if !tb.Offered["tdd"] || !tb.Loaded["tdd"] || tb.Loaded["slime-auth-mind"] {
		t.Errorf("offered %v loaded %v", tb.Offered, tb.Loaded)
	}
	if len(tb.Entries) < 2 {
		t.Errorf("registry entries: %d", len(tb.Entries))
	}
}

func TestDocIsFenced(t *testing.T) {
	s := fixtureSources(t, "world")
	if _, err := s.Doc(".isekai/slime/auth/README.md"); err != nil {
		t.Errorf("world doc readable: %v", err)
	}
	if _, err := s.Doc(".claude/skills/tdd/SKILL.md"); err != nil {
		t.Errorf("mind doc readable: %v", err)
	}
	for _, bad := range []string{"../../go.mod", ".isekai/../.isekai/../sources.go", "/etc/passwd", ".isekai/name"} {
		if _, err := s.Doc(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}
