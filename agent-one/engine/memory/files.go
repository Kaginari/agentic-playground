package memory

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Rd reads a file with CRLF/CR normalised to LF; ok=false when it cannot be read.
func Rd(p string) (string, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n"), true
}

func RdOr(p string) string { s, _ := Rd(p); return s }

func Exists(p string) bool { _, err := os.Stat(p); return err == nil }

func IsFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func mkd(p string) error { return os.MkdirAll(p, 0o755) }

// WriteAtomic writes temp + rename: a concurrent reader sees the old file or the new one.
func WriteAtomic(p string, data []byte) error {
	if err := mkd(filepath.Dir(p)); err != nil {
		return err
	}
	rb := make([]byte, 3)
	rand.Read(rb)
	tmp := p + "." + strconv.Itoa(os.Getpid()) + "." + hex.EncodeToString(rb) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// AppendLine is one O_APPEND write of JSON + "\n" (atomic below PIPE_BUF); returns the bytes.
func AppendLine(p string, v any) (int, error) {
	if err := mkd(filepath.Dir(p)); err != nil {
		return 0, err
	}
	b, err := MarshalJS(v)
	if err != nil {
		return 0, err
	}
	line := append(b, '\n')
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	_, err = f.Write(line)
	return len(line), err
}

// ReadJSONL returns every line that parses as a JSON object; a torn or foreign line is skipped.
func ReadJSONL(p string) []json.RawMessage {
	var out []json.RawMessage
	for _, l := range strings.Split(RdOr(p), "\n") {
		if l == "" {
			continue
		}
		t := strings.TrimLeft(l, " \t")
		if !strings.HasPrefix(t, "{") || !json.Valid([]byte(l)) {
			continue
		}
		out = append(out, json.RawMessage(l))
	}
	return out
}

// Section is one heading-delimited block of a markdown file (board.js's sectionMap shape).
type Section struct {
	N     int
	T     string
	Depth int
	Text  string
	B     int
}

type SecMap struct {
	Preamble string
	Sections []Section
}

var (
	reFence   = JSRe("^\\s*(```|~~~)")
	reHeading = JSRe(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
)

func SectionMap(text string) SecMap {
	var m SecMap
	var pre, cur []string
	fence := false
	flush := func() {
		if cur != nil {
			s := &m.Sections[len(m.Sections)-1]
			s.Text = strings.Join(cur, "\n")
			s.B = len(s.Text)
		}
	}
	for _, l := range strings.Split(text, "\n") {
		if reFence.MatchString(l) {
			fence = !fence
		}
		if !fence {
			if h := reHeading.FindStringSubmatch(l); h != nil {
				flush()
				m.Sections = append(m.Sections, Section{N: len(m.Sections) + 1, T: JSTrim(h[2]), Depth: len(h[1])})
				cur = []string{l}
				continue
			}
		}
		if cur != nil {
			cur = append(cur, l)
		} else {
			pre = append(pre, l)
		}
	}
	flush()
	m.Preamble = strings.Join(pre, "\n")
	return m
}

func readDirSorted(d string, locale bool) []fs.DirEntry {
	es, err := os.ReadDir(d)
	if err != nil {
		return nil
	}
	if locale {
		sort.SliceStable(es, func(i, j int) bool { return LocaleCompare(es[i].Name(), es[j].Name()) < 0 })
	}
	return es
}

// ListMd walks dir (entries in locale order) and returns every .md file.
func ListMd(dir string) []string {
	var out []string
	var walk func(d string)
	walk = func(d string) {
		for _, e := range readDirSorted(d, true) {
			p := filepath.Join(d, e.Name())
			if e.IsDir() {
				walk(p)
			} else if strings.HasSuffix(e.Name(), ".md") {
				out = append(out, p)
			}
		}
	}
	if Exists(dir) {
		walk(dir)
	}
	return out
}

// Member is a record read off .agent-one/<rank>/<dir>/: id <rank>-<dir>, doc README.md, <dir>.md,
// SKILL.md or the first .md.
type Member struct {
	Name, Dir, Rank, Doc string
}

var RankNames = []string{"coord", "domain", "zone", "service"}

func Members(base string) []Member {
	var out []Member
	for _, rank := range RankNames {
		d := filepath.Join(base, rank)
		if !Exists(d) {
			continue
		}
		for _, e := range readDirSorted(d, true) {
			if !e.IsDir() {
				continue
			}
			var files []string
			for _, f := range readDirSorted(filepath.Join(d, e.Name()), false) {
				if strings.HasSuffix(f.Name(), ".md") {
					files = append(files, f.Name())
				}
			}
			sort.Strings(files)
			doc := ""
			for _, want := range []string{"README.md", e.Name() + ".md", "SKILL.md"} {
				for _, f := range files {
					if f == want && doc == "" {
						doc = f
					}
				}
				if doc != "" {
					break
				}
			}
			if doc == "" && len(files) > 0 {
				doc = files[0]
			}
			c := Member{Name: rank + "-" + e.Name(), Dir: e.Name(), Rank: rank}
			if doc != "" {
				c.Doc = filepath.Join(d, e.Name(), doc)
			}
			out = append(out, c)
		}
	}
	return out
}

// NamedPath is one discovered Skill or command: its name and the first home it was found in.
type NamedPath struct {
	Name, P string
}

var SkillBases = []string{".opencode/skills", ".opencode/skill", ".claude/skills"}
var CommandBases = []string{".claude/commands", ".opencode/commands", ".opencode/command"}

func Skills(root string) []NamedPath {
	var out []NamedPath
	seen := map[string]bool{}
	for _, base := range SkillBases {
		d := filepath.Join(root, base)
		for _, e := range readDirSorted(d, true) {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(d, e.Name(), "SKILL.md")
			if Exists(p) && !seen[e.Name()] {
				seen[e.Name()] = true
				out = append(out, NamedPath{e.Name(), p})
			}
		}
	}
	return out
}

func Commands(root string) []NamedPath {
	var out []NamedPath
	seen := map[string]bool{}
	for _, base := range CommandBases {
		d := filepath.Join(root, base)
		for _, e := range readDirSorted(d, false) {
			n := e.Name()
			if strings.HasSuffix(n, ".md") && !seen[n[:len(n)-3]] {
				seen[n[:len(n)-3]] = true
				out = append(out, NamedPath{n[:len(n)-3], filepath.Join(d, n)})
			}
		}
	}
	return out
}

// GitHead is `git rev-parse HEAD` in root, or nil.
func GitHead(root string) *string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	s := strings.TrimSpace(string(out))
	return &s
}

// Stamps are source mtimes keyed by relative path, in harvest order — the order Object.keys
// returns them in the JS, which every staleness hole lists them in.
type Stamps struct {
	Keys []string
	Map  map[string]float64
}

func (s *Stamps) Add(k string, v float64) {
	if s.Map == nil {
		s.Map = map[string]float64{}
	}
	if _, ok := s.Map[k]; !ok {
		s.Keys = append(s.Keys, k)
	}
	s.Map[k] = v
}

func (s Stamps) MarshalJSON() ([]byte, error) {
	o := make(OJ, 0, len(s.Keys))
	for _, k := range s.Keys {
		o = append(o, KV{k, s.Map[k]})
	}
	return MarshalJS(o)
}

func (s *Stamps) UnmarshalJSON(b []byte) error {
	*s = Stamps{Map: map[string]float64{}}
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		var v float64
		if err := dec.Decode(&v); err != nil {
			return err
		}
		s.Add(kt.(string), v)
	}
	return nil
}

// StaleParts compares two stamp sets: changed, new and gone sources, as the JS phrases them.
func StaleParts(cur, was Stamps) []string {
	var changed, added, gone []string
	for _, p := range cur.Keys {
		if v, ok := was.Map[p]; ok {
			if v != cur.Map[p] {
				changed = append(changed, p)
			}
		} else {
			added = append(added, p)
		}
	}
	for _, p := range was.Keys {
		if _, ok := cur.Map[p]; !ok {
			gone = append(gone, p)
		}
	}
	var parts []string
	for _, s := range []string{say(changed, "changed"), say(added, "new"), say(gone, "gone")} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}

// MtimeMs is Math.round(fs.statSync(p).mtimeMs), computed the way Node computes mtimeMs.
func MtimeMs(p string) (float64, bool) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, false
	}
	t := st.ModTime()
	return JSRound(float64(t.Unix())*1e3 + float64(t.Nanosecond())/1e6), true
}

// NowISO is new Date().toISOString().
func NowISO(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

var reNamed = regexp.MustCompile(`\b(?:coord|domain|zone|service)-[a-z0-9-]+`)

// Named lists every member id a text names, lower-cased.
func Named(s string) []string { return reNamed.FindAllString(strings.ToLower(s), -1) }

// FieldOf reads a `- **Key:** value` line off a member doc.
func FieldOf(doc, key string) string {
	re := JSRe(`(?mi)^\s*-\s*\*\*` + regexp.QuoteMeta(key) + `:\*\*\s*(.+)$`)
	if m := re.FindStringSubmatch(doc); m != nil {
		return m[1]
	}
	return ""
}

var (
	reBacktickPath = regexp.MustCompile("`[^`\n]*/[^`\n]*`")
	rePath         = regexp.MustCompile(`[\w.\-]+(?:/[\w.\-]*)+`)
)

// Relations are whom a member is bound to, read off the member docs' own declarations.
type Relations struct {
	Self     string
	Rank     string
	Parent   string
	Children []string
	Skills   []string
	Zone     []string
}

func RelationsOf(root, base, name string) Relations {
	R := Relations{Self: name}
	all := Members(base)
	var c *Member
	for i := range all {
		if all[i].Name == name || all[i].Dir == name {
			c = &all[i]
			break
		}
	}
	if c == nil || c.Doc == "" {
		return R
	}
	R.Rank = c.Rank
	doc := RdOr(c.Doc)
	want := ""
	switch c.Rank {
	case "zone":
		want = "domain-"
	case "domain":
		want = "coord-"
	}
	if n := Named(FieldOf(doc, "Reports to")); len(n) > 0 {
		R.Parent = n[0]
	} else if want != "" {
		for _, n := range Named(doc) {
			if strings.HasPrefix(n, want) && n != c.Name {
				R.Parent = n
				break
			}
		}
	}
	for _, o := range all {
		if o.Name == c.Name || o.Doc == "" {
			continue
		}
		for _, n := range Named(FieldOf(RdOr(o.Doc), "Reports to")) {
			if n == c.Name {
				R.Children = append(R.Children, o.Name)
				break
			}
		}
	}
	for _, m := range Skills(root) {
		if NameIn(doc, m.Name) {
			R.Skills = append(R.Skills, m.Name)
		}
	}
	zoneText := FieldOf(doc, "Owns") + " " + strings.Join(reBacktickPath.FindAllString(doc, -1), " ")
	seen := map[string]bool{}
	for _, z := range rePath.FindAllString(zoneText, -1) {
		z = strings.ToLower(strings.TrimRight(z, "/"))
		if !seen[z] {
			seen[z] = true
			R.Zone = append(R.Zone, z)
		}
	}
	return R
}

// Has reports whether every token of s is a term of tf (the relation test).
func Has(tf map[string]int, s string) bool {
	ts := Tokens(s)
	if len(ts) == 0 {
		return false
	}
	for _, t := range ts {
		if _, ok := tf[t]; !ok {
			return false
		}
	}
	return true
}
