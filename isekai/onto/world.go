package onto

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// World is a loaded ontology: the schema, the asserted graph files, the
// triples derived from the creatures on disk, and the reasoner's closure.
type World struct {
	Root     string // the directory that holds .isekai/
	Graph    *Graph
	Prefixes map[string]string
	Notes    []string // holes (@?), e.g. a missing schema file
	Docs     map[Term]*DocRef
	Layout   Layout
}

// DocRef points findings back at the doc a creature was derived from.
type DocRef struct {
	Path  string         // relative to Root
	Lines map[string]int // field → line number (1-based)
}

// RankDir is one rank as the derivation needs it: its canonical name, the folder its
// creatures live in (also their id prefix, `<dir>-<name>`) and the rank it reports to
// ("rimuru" at the top). Ranks are data (binary.md §Ranks and bodies): a name the built-in
// schema knows (elf, orc, slime, kijin) maps to its class; any other becomes a derived class
// ⊂ Creature, and its bond to its parent is the generic `above` unless it is truth- or
// verdict-shaped (slime ⇒ orc, orc ⇒ elf) or reports to rimuru.
type RankDir struct {
	Name   string
	Dir    string
	Parent string
}

// Layout is what a distribution renames on disk: the world directory, the law file and the
// rank table.
type Layout struct {
	WorldDir string    // ".isekai"
	Law      string    // "isekai.md"
	Ranks    []RankDir // nil = DefaultLayout's
}

// DefaultLayout is the isekai distribution's: the law's own ranks.
func DefaultLayout() Layout {
	return Layout{WorldDir: ".isekai", Law: "isekai.md", Ranks: []RankDir{
		{"elf", "elf", "rimuru"}, {"orc", "orc", "elf"}, {"slime", "slime", "orc"}, {"kijin", "kijin", "rimuru"},
	}}
}

func (l Layout) norm() Layout {
	d := DefaultLayout()
	if l.WorldDir != "" {
		d.WorldDir = l.WorldDir
	}
	if l.Law != "" {
		d.Law = l.Law
	}
	if l.Ranks != nil {
		d.Ranks = l.Ranks
	}
	return d
}

// ClassOf is the class term of a rank: the schema's for the law's ranks, a derived one
// (CamelCase of the name, ⊂ Creature) for any other.
func ClassOf(rank string) Term {
	rank = strings.ToLower(strings.TrimSpace(rank))
	if c, ok := raceClass[rank]; ok {
		return c
	}
	var b strings.Builder
	for _, part := range strings.FieldsFunc(rank, func(r rune) bool { return r == '_' || r == '-' || r == ' ' }) {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	if b.Len() == 0 {
		return cCreature
	}
	return Is(b.String())
}

// bondFor is the bond a child rank draws to its parent rank.
func bondFor(child, parent string) Term {
	switch {
	case child == "slime" && parent == "orc":
		return pTruth
	case child == "orc" && parent == "elf":
		return pVerdict
	case parent == "rimuru":
		return pReports
	}
	return pAbove
}

// namePattern is the creature-id pattern for this layout, e.g. (?:zone|domain|coord)-x.
func (l Layout) namePattern() string {
	var ds []string
	seen := map[string]bool{}
	for _, r := range l.Ranks {
		if r.Dir != "" && !seen[r.Dir] {
			seen[r.Dir] = true
			ds = append(ds, regexp.QuoteMeta(r.Dir))
		}
	}
	if len(ds) == 0 {
		ds = []string{"never-matches-anything"}
	}
	return `(?:` + strings.Join(ds, "|") + `)-[a-z0-9][a-z0-9-]*`
}

// FindRoot walks up from dir to the nearest directory holding .isekai/.
func FindRoot(dir string) (string, error) { return FindRootIn(dir, DefaultLayout()) }

// FindRootIn walks up from dir to the nearest directory holding the layout's world dir.
func FindRootIn(dir string, l Layout) (string, error) {
	l = l.norm()
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(d, l.WorldDir)); err == nil && st.IsDir() {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", errors.New("no " + l.WorldDir + "/ found here or above")
		}
		d = parent
	}
}

// OntologyDir is where the world keeps its schema and asserted graph.
func OntologyDir(root string) string { return OntologyDirIn(root, DefaultLayout()) }

// OntologyDirIn is OntologyDir under a layout's world dir.
func OntologyDirIn(root string, l Layout) string {
	return filepath.Join(root, l.norm().WorldDir, "ontology")
}

// Load reads schema + graph/*.ttl, derives the world's creatures, and reasons
// to a fixpoint.
func Load(root string) (*World, error) { return LoadLayout(root, DefaultLayout()) }

// LoadLayout is Load under a distribution's layout.
func LoadLayout(root string, l Layout) (*World, error) {
	l = l.norm()
	w := &World{Root: root, Graph: New(), Prefixes: DefaultPrefixes(), Docs: map[Term]*DocRef{}, Layout: l}
	dir := OntologyDirIn(root, l)
	schema := filepath.Join(dir, "schema.ttl")
	if b, err := os.ReadFile(schema); err == nil {
		if w.Prefixes, err = Parse(rel(root, schema), string(b), w.Graph, w.Prefixes); err != nil {
			return nil, err
		}
	} else {
		w.Notes = append(w.Notes, rel(root, schema)+" missing; using the built-in schema")
		if w.Prefixes, err = Parse("schema.ttl", DefaultSchema, w.Graph, w.Prefixes); err != nil {
			return nil, err
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "graph", "*.ttl"))
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if w.Prefixes, err = Parse(rel(root, f), string(b), w.Graph, w.Prefixes); err != nil {
			return nil, err
		}
	}
	if err := w.derive(); err != nil {
		return nil, err
	}
	Infer(w.Graph)
	return w, nil
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}

var (
	reField   = regexp.MustCompile(`(?i)^\s*[-*]?\s*(?:\*\*)?([A-Za-z][A-Za-z ]*?)(?::\*\*|\*\*:|:)\s*(.+?)\s*$`)
	reTick    = regexp.MustCompile("`([^`\n\\s]*/[^`\n\\s]*)`") // a path never holds a space; a command does
	rePath    = regexp.MustCompile(`[\w.\-*]+(?:/[\w.\-*]*)+`)
	reWord    = regexp.MustCompile(`[a-z0-9][a-z0-9-]*`)
	fieldKeys = map[string]string{"reports to": "parent", "orc": "parent", "elf": "parent", "parent": "parent",
		"territory": "territory", "zone": "territory", "owns": "territory", "minds": "minds", "mind": "minds", "wears": "minds", "hats": "minds"}
)

// fields reads `- **Key:** value` lines leniently; keys are normalized.
func fields(doc string) (map[string]string, map[string]int) {
	vals, lines := map[string]string{}, map[string]int{}
	for i, line := range strings.Split(doc, "\n") {
		m := reField.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(m[1]))
		k, ok := fieldKeys[key]
		if !ok {
			continue
		}
		if _, seen := vals[k]; seen {
			continue
		}
		vals[k] = strings.TrimSpace(m[2])
		lines[k] = i + 1
	}
	return vals, lines
}

func normPath(p string) string {
	p = strings.TrimPrefix(p, "./")
	for _, suf := range []string{"/**/*", "/**", "/*", "**", "*"} {
		p = strings.TrimSuffix(p, suf)
	}
	return strings.ToLower(strings.TrimRight(p, "/"))
}

// derive adds the structural triples the world's files imply. Nothing here is
// ever written back to disk.
func (w *World) derive() error {
	g := w.Graph
	l := w.Layout.norm()
	reName := regexp.MustCompile(`(?i)\b(?:rimuru|` + l.namePattern() + `)`)
	// the first rank on a dir is the class its creatures take (an ascended rank shares its base's dir)
	rankOfDir := map[string]RankDir{}
	var dirs []string
	for _, r := range l.Ranks {
		if r.Dir == "" {
			continue
		}
		if _, dup := rankOfDir[r.Dir]; !dup {
			rankOfDir[r.Dir] = r
			dirs = append(dirs, r.Dir)
		}
	}
	rankByName := map[string]RankDir{}
	for _, r := range l.Ranks {
		rankByName[r.Name] = r
	}
	add := func(s, p, o Term) { g.AddDerived(Triple{s, p, o}) }
	for _, d := range dirs {
		c := ClassOf(rankOfDir[d].Name)
		if _, builtin := raceClass[rankOfDir[d].Name]; !builtin {
			add(c, rdfType, Is("Class"))
			add(c, pSubClassOf, cCreature)
		}
	}
	docNode := func(owner Term, path string) {
		d := Is("doc-" + owner.Local())
		add(owner, pDoc, d)
		add(d, rdfType, cDoc)
		add(d, pPath, L(path))
	}
	add(tRimuru, rdfType, cRimuru)
	add(tRimuru, pName, L("rimuru"))
	docNode(tRimuru, l.WorldDir+"/"+l.Law)

	// minds first, so docs can be matched against their names
	minds := map[string]string{}
	for _, base := range mindDirs {
		ents, err := os.ReadDir(filepath.Join(w.Root, base))
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(w.Root, base, e.Name(), "SKILL.md")
			if _, err := os.Stat(p); err == nil {
				if _, dup := minds[e.Name()]; !dup {
					minds[e.Name()] = rel(w.Root, p)
				}
			}
		}
	}
	mindNames := make([]string, 0, len(minds))
	for n := range minds {
		mindNames = append(mindNames, n)
	}
	sort.Strings(mindNames)
	for _, n := range mindNames {
		m := Is("mind-" + n)
		add(m, rdfType, cMind)
		add(m, pName, L(n))
		docNode(m, minds[n])
		// a skill with no race prefix is a shared host tool (the law's shared-skills row), not a
		// creature's mind: it is nobody's to wear, so MindWorn does not apply to it
		if _, ranked := rankOfDir[strings.SplitN(n, "-", 2)[0]]; !ranked {
			add(m, pShared, Bool(true))
		}
	}

	type creature struct {
		race, dir, id, doc string
		text               string
	}
	var cs []creature
	for _, rd := range dirs {
		race := rankOfDir[rd].Name
		d := filepath.Join(w.Root, l.WorldDir, rd)
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			c := creature{race: race, dir: e.Name(), id: rd + "-" + strings.ToLower(e.Name())}
			files, _ := os.ReadDir(filepath.Join(d, e.Name()))
			var mds []string
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".md") {
					mds = append(mds, f.Name())
				}
			}
			sort.Strings(mds)
			for _, want := range []string{"README.md", e.Name() + ".md", "SKILL.md"} {
				for _, f := range mds {
					if f == want && c.doc == "" {
						c.doc = f
					}
				}
			}
			if c.doc == "" && len(mds) > 0 {
				c.doc = mds[0]
			}
			if c.doc != "" {
				p := filepath.Join(d, e.Name(), c.doc)
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				c.text = string(b)
				c.doc = rel(w.Root, p)
			}
			cs = append(cs, c)
		}
	}
	known := map[string]bool{"rimuru": true}
	for _, c := range cs {
		known[c.id] = true
	}
	for _, c := range cs {
		self := Is(c.id)
		add(self, rdfType, ClassOf(c.race))
		add(self, pName, L(c.id))
		rdir := rankByName[c.race].Dir
		if c.doc == "" {
			w.Notes = append(w.Notes, c.id+" has no doc in "+l.WorldDir+"/"+rdir+"/"+c.dir+"/")
			w.Docs[self] = &DocRef{Path: l.WorldDir + "/" + rdir + "/" + c.dir + "/", Lines: map[string]int{}}
			continue
		}
		docNode(self, c.doc)
		vals, lines := fields(c.text)
		w.Docs[self] = &DocRef{Path: c.doc, Lines: lines}

		// parent: the declared field, else the first creature of the expected rank the doc names
		parent := ""
		if v, ok := vals["parent"]; ok {
			if m := reName.FindString(v); m != "" {
				parent = strings.ToLower(m)
			}
		}
		prank := rankByName[c.race].Parent
		if parent == "" {
			wantRace := ""
			if pr, ok := rankByName[prank]; ok && pr.Dir != "" {
				wantRace = pr.Dir + "-"
			}
			for _, m := range reName.FindAllString(c.text, -1) {
				m = strings.ToLower(m)
				if wantRace != "" && strings.HasPrefix(m, wantRace) && m != c.id {
					parent = m
					break
				}
			}
			if parent == "" && prank == "rimuru" {
				parent = "rimuru"
			}
		}
		if parent != "" && parent != c.id {
			prace := "rimuru"
			if parent != "rimuru" {
				prace = rankOfDir[strings.SplitN(parent, "-", 2)[0]].Name
			}
			add(self, bondFor(c.race, prace), Is(parent))
			if !known[parent] {
				add(Is(parent), pName, L(parent))
			}
		}

		// territory: the field plus any back-ticked path in the doc
		seen := map[string]bool{}
		zone := vals["territory"]
		for _, m := range reTick.FindAllStringSubmatch(c.text, -1) {
			zone += " " + m[1]
		}
		for _, p := range rePath.FindAllString(zone, -1) {
			p = normPath(p)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			add(self, pOwns, L(p))
		}

		// worn minds: named in a Minds field, or any mind's name in the doc as a whole word
		lower := strings.ToLower(c.text)
		for _, n := range mindNames {
			if wordIn(lower, strings.ToLower(n)) {
				add(self, pWears, Is("mind-"+n))
			}
		}
		if v, ok := vals["minds"]; ok {
			for _, n := range reWord.FindAllString(strings.ToLower(v), -1) {
				if _, exists := minds[n]; exists {
					add(self, pWears, Is("mind-"+n))
				}
			}
		}
	}
	return nil
}

func wordIn(text, word string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return false
		}
		s, e := i+j, i+j+len(word)
		before := s == 0 || !isWordByte(text[s-1])
		after := e == len(text) || !isWordByte(text[e])
		if before && after {
			return true
		}
		i = s + 1
	}
}

func isWordByte(b byte) bool {
	return b == '-' || b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b >= 0x80
}

// Creature resolves a name like slime-auth (or auth, when unambiguous) to its term.
func (w *World) Creature(name string) (Term, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "rimuru" {
		return tRimuru, nil
	}
	if w.Graph.Has(Triple{Is(name), rdfType, cCreature}) {
		return Is(name), nil
	}
	var hits []Term
	for _, c := range w.Graph.Instances(cCreature) {
		if strings.HasSuffix(c.Local(), "-"+name) {
			hits = append(hits, c)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) > 1 {
		return Term{}, fmt.Errorf("ambiguous creature %q: %s", name, joinLocal(hits))
	}
	return Term{}, fmt.Errorf("unknown creature %q", name)
}

func joinLocal(ts []Term) string {
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = t.Local()
	}
	return strings.Join(s, ", ")
}
