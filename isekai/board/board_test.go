package board

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var pagePaths = []string{"/", "/court", "/usage", "/colony", "/memory", "/toolbox", "/log", "/config"}

func newBoard(t *testing.T, world string, o Options) *Board {
	t.Helper()
	o.WorldRoot = fixtureRoot(t, world)
	if o.Now == nil {
		o.Now = func() time.Time { return fixedNow }
	}
	b := New(o)
	t.Cleanup(b.Close)
	return b
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Body.String()
}

func TestEveryPageRendersOnFixtureAndEmptyWorld(t *testing.T) {
	for _, world := range []string{"world", "empty"} {
		b := newBoard(t, world, Options{})
		for _, p := range pagePaths {
			code, html := get(t, b, p)
			if code != 200 {
				t.Errorf("%s %s: %d\n%s", world, p, code, html)
				continue
			}
			for _, want := range []string{`container-fluid`, `class="row`, `col-12`, `data-bs-theme`, `navbar`} {
				if !strings.Contains(html, want) {
					t.Errorf("%s %s: missing %q", world, p, want)
				}
			}
			if strings.Contains(html, "<no value>") {
				t.Errorf("%s %s: template printed <no value>", world, p)
			}
		}
	}
}

func TestEmptyWorldIsSilentNotZero(t *testing.T) {
	b := newBoard(t, "empty", Options{})
	_, html := get(t, b, "/")
	if !strings.Contains(html, "silent") || !strings.Contains(html, "never lit") {
		t.Error("overview on an empty world must say silent / never lit")
	}
	if strings.Contains(html, "$0.00") {
		t.Error("a silent cost instrument must not render as $0.00")
	}
	for _, p := range []string{"/usage", "/memory", "/toolbox", "/log", "/colony"} {
		_, html := get(t, b, p)
		if !strings.Contains(html, "silent") {
			t.Errorf("%s on an empty world must say silent", p)
		}
	}
}

var (
	reWidthCSS  = regexp.MustCompile(`(?i)(?:^|[^-\w])width\s*:\s*(\d+)px`)
	reWidthAttr = regexp.MustCompile(`(?i)\swidth="(\d+)"`)
)

func TestNoFixedWidthWiderThanAPhone(t *testing.T) {
	b := newBoard(t, "world", Options{})
	check := func(name, text string) {
		for _, re := range []*regexp.Regexp{reWidthCSS, reWidthAttr} {
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				n, _ := strconv.Atoi(m[1])
				if n > 360 {
					t.Errorf("%s: fixed width %dpx > 360", name, n)
				}
			}
		}
	}
	for _, p := range pagePaths {
		_, html := get(t, b, p)
		check(p, html)
	}
	_, css := get(t, b, "/static/board.css")
	check("board.css", css)
}

func TestThemeTokensPresentForBothThemes(t *testing.T) {
	b := newBoard(t, "world", Options{})
	code, css := get(t, b, "/static/board.css")
	if code != 200 {
		t.Fatal("theme css not served")
	}
	dark := css[strings.Index(css, `[data-bs-theme="dark"]`):strings.Index(css, `[data-bs-theme="light"]`)]
	light := css[strings.Index(css, `[data-bs-theme="light"]`):]
	light = light[:strings.Index(light, "}")+1]
	for _, tok := range []string{"--r-slime", "--r-orc", "--r-elf", "--l-zone", "--l-verdict", "--l-global", "--l-shared", "--c-1", "--bs-body-bg"} {
		re := regexp.MustCompile(regexp.QuoteMeta(tok) + `:\s*([^;]+);`)
		d, l := re.FindStringSubmatch(dark), re.FindStringSubmatch(light)
		if d == nil || l == nil {
			t.Errorf("%s missing in a theme (dark %v light %v)", tok, d != nil, l != nil)
			continue
		}
		if strings.TrimSpace(d[1]) == strings.TrimSpace(l[1]) {
			t.Errorf("%s reuses the same value in both themes: %s", tok, d[1])
		}
	}
	if !strings.Contains(css, "prefers-reduced-motion") {
		t.Error("motion must respect prefers-reduced-motion")
	}
	_, html := get(t, b, "/")
	if !strings.Contains(html, `id="themeToggle"`) || !strings.Contains(html, "prefers-color-scheme") {
		t.Error("the page needs a theme toggle that follows the system by default")
	}
}

func TestLexiconSwapChangesLabels(t *testing.T) {
	b := newBoard(t, "world", Options{Names: Names{"page.court": "Subagents", "page.colony": "Team", "rank.slime": "Zone worker", "bond.truth": "ground-truth link"}})
	_, html := get(t, b, "/colony")
	for _, want := range []string{">Subagents<", ">Team<", "Zone worker", "ground-truth link"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing swapped label %q", want)
		}
	}
	for _, gone := range []string{">Court<", ">Colony<", "truth-current"} {
		if strings.Contains(html, gone) {
			t.Errorf("isekai label %q survived the swap", gone)
		}
	}
	d := newBoard(t, "world", Options{})
	_, html = get(t, d, "/")
	if !strings.Contains(html, ">Court<") {
		t.Error("default names are isekai's")
	}
}

func TestCourtFeedAndAPI(t *testing.T) {
	usd := 0.12
	bodies := []Body{{Name: "slime-auth", Rank: "slime", Office: "findings", Model: "claude-sonnet-4-5", Provider: "anthropic", State: "tool", Started: fixedNow.Add(-90 * time.Second), ContextTokens: 50000, ContextLimit: 200000, Input: 1000, Output: 200, USD: &usd}, {Name: "orc-core", Rank: "orc", State: "waiting on gate", Started: fixedNow.Add(-5 * time.Second)}}
	b := newBoard(t, "world", Options{Sources: Sources{Court: func() []Body { return bodies }}})
	code, html := get(t, b, "/court")
	if code != 200 {
		t.Fatal(code)
	}
	for _, want := range []string{"slime-auth", "rank-slime", "25%", "1m30s", "$0.12", "waiting on gate", "unpriced", "window unknown", `data-live="court"`} {
		if !strings.Contains(html, want) {
			t.Errorf("court page missing %q", want)
		}
	}
	_, js := get(t, b, "/api/court")
	var got struct {
		Live   bool   `json:"live"`
		Bodies []Body `json:"bodies"`
	}
	if err := json.Unmarshal([]byte(js), &got); err != nil || !got.Live || len(got.Bodies) != 2 {
		t.Errorf("api/court: %v %+v", err, got)
	}
	_, over := get(t, b, "/")
	if !strings.Contains(over, "max context 25%") {
		t.Error("overview shows the court's max context")
	}
	silentB := newBoard(t, "world", Options{})
	_, html = get(t, silentB, "/court")
	if !strings.Contains(html, "no live") {
		t.Error("without a feed the court is silent, not empty")
	}
}

func TestConfigTreeAndOffList(t *testing.T) {
	cfg := map[string]any{
		"tools":  map[string]any{"webfetch": map[string]any{"enabled": map[string]any{"value": false, "origin": ".isekai/config.yaml:12"}}},
		"models": map[string]any{"default": "anthropic/claude-opus-5-5", "ranks": []any{"a", "b"}},
	}
	b := newBoard(t, "world", Options{Sources: Sources{
		Config: func() any { return cfg },
		Off:    func() []Off { return []Off{{Feature: "tools.webfetch", Origin: ".isekai/config.yaml:12"}} },
	}})
	_, html := get(t, b, "/config")
	for _, want := range []string{"cfg-tree", "cfg-origin", ".isekai/config.yaml:12", "tools.webfetch", "claude-opus-5-5", `class="text-warning">off<`, "<ol"} {
		if !strings.Contains(html, want) {
			t.Errorf("config page missing %q", want)
		}
	}
	_, over := get(t, b, "/")
	if !strings.Contains(over, "tools.webfetch") {
		t.Error("overview shows the off-list")
	}
	empty := newBoard(t, "world", Options{})
	_, html = get(t, empty, "/config")
	if !strings.Contains(html, "silent") {
		t.Error("no config feed → silent")
	}
}

func TestUsagePageRangesAndChartData(t *testing.T) {
	b := newBoard(t, "world", Options{})
	_, html := get(t, b, "/usage?range=all")
	for _, want := range []string{`nav-link active" href="?range=all"`, "rimuru", "slime-auth", "claude-opus-5-5", "unpriced", `id="chartData"`, `data-chart="columns"`} {
		if !strings.Contains(html, want) {
			t.Errorf("usage page missing %q", want)
		}
	}
	re := regexp.MustCompile(`(?s)<script type="application/json" id="chartData">(.*?)</script>`)
	m := re.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("no embedded chart data")
	}
	var chart chartData
	if err := json.Unmarshal([]byte(m[1]), &chart); err != nil {
		t.Fatalf("embedded chart data is not JSON: %v\n%s", err, m[1])
	}
	if len(chart.Days) != 2 || len(chart.Series) != 4 {
		t.Errorf("chart: %+v", chart)
	}
	_, day := get(t, b, "/usage?range=24h")
	if strings.Contains(day, "2026-09-25") {
		t.Error("24h range must not list yesterday's calls")
	}
	_, js := get(t, b, "/api/usage?range=all")
	if !strings.Contains(js, `"byModel"`) {
		t.Error("api/usage")
	}
}

func TestColonyPageEmbedsGraph(t *testing.T) {
	b := newBoard(t, "world", Options{})
	_, html := get(t, b, "/colony")
	re := regexp.MustCompile(`(?s)<script type="application/json" id="colonyData">(.*?)</script>`)
	m := re.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("no colony data")
	}
	var c struct {
		Nodes  []Node      `json:"nodes"`
		Edges  []Edge      `json:"edges"`
		Labels []laneLabel `json:"labels"`
	}
	if err := json.Unmarshal([]byte(m[1]), &c); err != nil {
		t.Fatalf("colony json: %v", err)
	}
	if len(c.Nodes) < 6 || len(c.Edges) < 4 || len(c.Labels) < 4 {
		t.Errorf("colony: %d nodes %d edges %d labels", len(c.Nodes), len(c.Edges), len(c.Labels))
	}
	if !strings.Contains(html, `id="nodePanel"`) || !strings.Contains(html, "offcanvas") {
		t.Error("focus panel is an offcanvas")
	}
	code, doc := get(t, b, "/api/doc?path=.isekai/slime/auth/README.md")
	if code != 200 || !strings.Contains(doc, "slime-auth") {
		t.Errorf("api/doc: %d", code)
	}
	if code, _ := get(t, b, "/api/doc?path=../../go.mod"); code != 404 {
		t.Errorf("doc outside the world must 404, got %d", code)
	}
}

func TestLogSearchAndPartial(t *testing.T) {
	b := newBoard(t, "world", Options{})
	_, html := get(t, b, "/log?q=ttl")
	if !strings.Contains(html, "token TTL") || strings.Contains(html, "the world is born") {
		t.Error("log search filters entries")
	}
	if !strings.Contains(html, "1 of 2") {
		t.Error("log says how many entries matched")
	}
	_, frag := get(t, b, "/log?partial=1")
	if strings.Contains(frag, "<html") || !strings.Contains(frag, "log-entry") {
		t.Error("partial=1 returns only the live fragment")
	}
}

func TestPrefixMount(t *testing.T) {
	b := newBoard(t, "world", Options{Prefix: "/w"})
	if code, _ := get(t, b, "/w/court"); code != 200 {
		t.Errorf("prefixed court: %d", code)
	}
	if code, _ := get(t, b, "/court"); code != 404 {
		t.Errorf("unprefixed path must 404, got %d", code)
	}
	_, html := get(t, b, "/w/")
	if !strings.Contains(html, `href="/w/usage"`) || !strings.Contains(html, `href="/w/static/board.css"`) {
		t.Error("links carry the prefix")
	}
	if code, _ := get(t, b, "/w/assets/bootstrap.min.css"); code != 200 {
		t.Error("bootstrap served under the prefix")
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSEEmitsOnUsageAppend(t *testing.T) {
	root := t.TempDir()
	copyTree(t, fixtureRoot(t, "world"), root)
	b := New(Options{WorldRoot: root, Poll: 15 * time.Millisecond})
	t.Cleanup(b.Close)
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	rd := bufio.NewReader(resp.Body)
	line, _ := rd.ReadString('\n')
	if !strings.HasPrefix(line, "retry:") {
		t.Fatalf("first line %q", line)
	}
	time.Sleep(60 * time.Millisecond) // two polls: the watcher has a baseline
	f, err := os.OpenFile(filepath.Join(root, ".isekai", "instruments", "usage", "s-2026-09-26.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"ts":"2026-09-26T09:00:00Z","session":"s-2026-09-26","body":"rimuru","rank":"rimuru","model":"m","provider":"p","input":1,"output":1,"cacheRead":0,"cacheWrite":0,"usd":null}` + "\n")
	f.Close()
	got := make(chan string, 1)
	go func() {
		for {
			l, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(l, "event: usage") {
				got <- l
				return
			}
		}
	}()
	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("no usage event within 3s of the append")
	}
}

func TestReadmeDocumentsTheUsageSchema(t *testing.T) {
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"`ts`", "`session`", "`body`", "`rank`", "`office`", "`model`", "`provider`", "`input`", "`cacheRead`", "`cacheWrite`", "`usd`"} {
		if !strings.Contains(string(b), f) {
			t.Errorf("README lacks usage field %s", f)
		}
	}
	_ = io.EOF
}
