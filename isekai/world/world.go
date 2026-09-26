package world

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Kaginari/agentic-playground/isekai/loop"
	"github.com/Kaginari/agentic-playground/isekai/memory"
	"github.com/Kaginari/agentic-playground/isekai/onto"
)

// Creature is one ranked body read off its doc — through onto's derivation, never a second
// parser: race, territory, parent bond, worn minds; plus its verify commands (traits as
// readings) and the doc that owns it.
type Creature struct {
	Name      string   // slime-auth (zone-auth under agent-one)
	Rank      string   // the rank's name in the table (slime, orc, elf, kijin, or a custom one)
	Doc       string   // relative path of the owning doc
	Dir       string   // relative path of the creature's directory
	Territory []string // normalized path prefixes it owns
	Parent    string   // one hop up (orc for a slime, elf for an orc, rimuru for an elf)
	Minds     []string // worn minds
	Verify    []string // shell commands whose exit codes are the traits' reading
}

// Options is what Open needs beyond the root and the lexicon.
type Options struct {
	Instructions InstructionOptions
	Ontology     bool  // load the ontology (creatures need it; off = an empty roster)
	Ranks        Ranks // the rank table; nil = DefaultRanks(lex) (config's `rankSet: replace` passes the whole table)
}

// World is one opened world.
type World struct {
	Root         string
	Lex          Lexicon
	Ranks        Ranks
	Law          *Law
	Onto         *onto.World
	Creatures    []Creature
	Instructions []Instruction
	Notes        []string // holes met while opening

	// Foreign are bodies read from another harness's agent files (discovery.agents): they
	// stay on the roster across Reload, beside the creatures derived from the docs.
	Foreign []Creature

	mu       sync.Mutex
	sessions map[*loop.Engine]*loop.Session
	closers  map[*loop.Engine]func()
}

// Discover walks up from dir for the first lexicon whose world dir exists. Both `.isekai` and
// `.agent-one` are tried, in the order given (Lexicons() when none).
func Discover(dir string, lexes ...Lexicon) (string, Lexicon, error) {
	if len(lexes) == 0 {
		lexes = Lexicons()
	}
	var errs []string
	for _, l := range lexes {
		root, err := onto.FindRootIn(dir, l.Layout())
		if err == nil {
			return root, l, nil
		}
		errs = append(errs, err.Error())
	}
	return "", Lexicon{}, errors.New(strings.Join(errs, "; "))
}

// Open loads the world at root under a lexicon: the law, the ontology and its creatures, the
// instructions. A missing law is a note, not a failure — the crest is then empty and the
// system prompt says so.
func Open(root string, lex Lexicon, opt Options) (*World, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(filepath.Join(abs, lex.WorldDir)); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("no %s/ under %s", lex.WorldDir, abs)
	}
	w := &World{Root: abs, Lex: lex, Ranks: opt.Ranks, sessions: map[*loop.Engine]*loop.Session{}, closers: map[*loop.Engine]func(){}}
	if w.Ranks == nil {
		w.Ranks = DefaultRanks(lex)
	}
	if err := w.Ranks.Validate(); err != nil {
		return nil, fmt.Errorf("ranks: %v", err)
	}
	lawPath := filepath.Join(abs, lex.WorldDir, lex.Law)
	if w.Law, err = LoadLaw(lawPath, lex.WorldDir+"/"+lex.Law, lex.Crest); err != nil {
		w.Notes = append(w.Notes, err.Error())
	} else if w.Law.Crest == "" {
		w.Notes = append(w.Notes, w.Law.Path+" has no crest section")
	}
	if opt.Ontology {
		if err := w.Reload(); err != nil {
			return nil, err
		}
	}
	w.Instructions = LoadInstructions(abs, opt.Instructions)
	return w, nil
}

// Dir is <root>/<worldDir>.
func (w *World) Dir() string { return filepath.Join(w.Root, w.Lex.WorldDir) }

// Rel renders an absolute path relative to the root (slash-separated); outside paths stay as is.
func (w *World) Rel(abs string) string {
	if r, err := filepath.Rel(w.Root, abs); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return abs
}

// Reload re-derives the roster from the docs (they change during a session).
func (w *World) Reload() error {
	ow, err := onto.LoadLayout(w.Root, w.Ranks.Layout(w.Lex))
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Onto = ow
	w.Notes = append(w.Notes, ow.Notes...)
	w.Creatures = creaturesOf(w.Root, ow, w.Ranks)
	for _, f := range w.Foreign {
		if w.creatureLocked(f.Name) == nil {
			w.Creatures = append(w.Creatures, f)
		}
	}
	return nil
}

// AddForeign puts a body from another harness's agent file on the roster (a name already on
// it is one body with two sources: the native one wins, Nature 2).
func (w *World) AddForeign(c Creature) {
	w.mu.Lock()
	w.Foreign = append(w.Foreign, c)
	exists := w.creatureLocked(c.Name) != nil
	if !exists {
		w.Creatures = append(w.Creatures, c)
	}
	w.mu.Unlock()
}

func (w *World) creatureLocked(name string) *Creature {
	for i := range w.Creatures {
		if w.Creatures[i].Name == name {
			return &w.Creatures[i]
		}
	}
	return nil
}

var (
	cCreature = onto.Is("Creature")
	pOwns     = onto.Is("owns")
	pWears    = onto.Is("wears")
	pName     = onto.Is("name")
	bondProps = []onto.Term{onto.Is("truth"), onto.Is("verdict"), onto.Is("reports"), onto.Is("above")}
	reRank    = regexp.MustCompile(`(?mi)^\s*[-*]\s*\*\*rank:?\*\*:?\s*(.+?)\s*$`)
)

func creaturesOf(root string, ow *onto.World, ranks Ranks) []Creature {
	g := ow.Graph
	var out []Creature
	for _, t := range g.Instances(cCreature) {
		name := t.Local()
		if name == Rimuru {
			continue
		}
		c := Creature{Name: name}
		d, ok := ow.Docs[t]
		if !ok {
			continue // a bond target with no dir of its own: the BondTarget shape names it
		}
		if strings.HasSuffix(d.Path, "/") {
			c.Dir = strings.TrimSuffix(d.Path, "/")
		} else {
			c.Doc = d.Path
			c.Dir = filepath.ToSlash(filepath.Dir(d.Path))
		}
		// the rank: the folder's base rank, unless the doc's Rank field names an ascended rank on that dir
		parts := strings.Split(c.Dir, "/")
		if len(parts) < 2 {
			continue
		}
		base, ok := ranks.ByDir(parts[1])
		if !ok {
			continue
		}
		c.Rank = base.Name
		text := ""
		if c.Doc != "" {
			text = memory.RdOr(filepath.Join(root, filepath.FromSlash(c.Doc)))
		}
		if m := reRank.FindStringSubmatch(text); m != nil {
			want := strings.ToLower(strings.NewReplacer(" ", "_", "-", "_").Replace(strings.TrimSpace(m[1])))
			for _, r := range ranks {
				if r.Name == want && r.Base == base.Name {
					c.Rank = r.Name
				}
			}
		}
		for _, o := range g.Objects(t, pOwns) {
			c.Territory = append(c.Territory, o.Value)
		}
		for _, p := range bondProps {
			for _, o := range g.Objects(t, p) {
				if g.Derived(onto.Triple{S: t, P: p, O: o}) && !oneHopAbove(g, t, o) {
					continue // the transitive closure, not a hop
				}
				c.Parent = o.Local()
				break
			}
			if c.Parent != "" {
				break
			}
		}
		for _, m := range g.Objects(t, pWears) {
			if n := g.Objects(m, pName); len(n) > 0 {
				c.Minds = append(c.Minds, n[0].Value)
			} else {
				c.Minds = append(c.Minds, strings.TrimPrefix(m.Local(), "mind-"))
			}
		}
		c.Verify = VerifyOf(text)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var (
	reVerifyField = regexp.MustCompile("(?mi)^\\s*[-*]\\s*\\*\\*verify:?\\*\\*:?\\s*`?([^`\\n]+?)`?\\s*$")
	reVerifyHead  = regexp.MustCompile(`(?mi)^##+\s*verify\s*$`)
	reBullet      = regexp.MustCompile("(?m)^\\s*[-*]\\s+`?([^`\\n]+?)`?\\s*$")
)

// VerifyOf reads a creature doc's verify commands: every `- **Verify:** cmd` line, plus the
// bullets of a `## Verify` section. Exit codes, not prose, are the traits' reading.
func VerifyOf(doc string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, m := range reVerifyField.FindAllStringSubmatch(doc, -1) {
		add(m[1])
	}
	if loc := reVerifyHead.FindStringIndex(doc); loc != nil {
		body := doc[loc[1]:]
		if end := regexp.MustCompile(`\n#`).FindStringIndex(body); end != nil {
			body = body[:end[0]]
		}
		for _, m := range reBullet.FindAllStringSubmatch(body, -1) {
			add(m[1])
		}
	}
	return out
}

// Creature finds a body by id (or by its unprefixed name when unambiguous).
func (w *World) Creature(name string) *Creature {
	name = strings.ToLower(strings.TrimSpace(name))
	var hit *Creature
	for i := range w.Creatures {
		c := &w.Creatures[i]
		if c.Name == name {
			return c
		}
		if strings.HasSuffix(c.Name, "-"+name) {
			if hit != nil {
				return nil
			}
			hit = c
		}
	}
	return hit
}

// oneHopAbove is true when no third creature sits between s and o on the above chain.
func oneHopAbove(g *onto.Graph, s, o onto.Term) bool {
	above := onto.Is("above")
	for _, mid := range g.Objects(s, above) {
		if mid != o && g.Has(onto.Triple{S: mid, P: above, O: o}) {
			return false
		}
	}
	return true
}

// RankOf is a creature's rank row (rimuru's for the session or an unknown name).
func (w *World) RankOf(name string) Rank {
	if c := w.Creature(name); c != nil {
		if r, ok := w.Ranks.Get(c.Rank); ok {
			return r
		}
	}
	if rn := w.Ranks.Of(name); rn != "" {
		if r, ok := w.Ranks.Get(rn); ok {
			return r
		}
	}
	r, _ := w.Ranks.Get(Rimuru)
	return r
}

// GateHolders reports whether any creature on the roster holds a gate (an orc, by the law's
// table). Orcs is its old name.
func (w *World) GateHolders() bool {
	for _, c := range w.Creatures {
		if r, ok := w.Ranks.Get(c.Rank); ok && r.HoldsGate {
			return true
		}
	}
	return false
}

// Orcs is GateHolders under the law's name.
func (w *World) Orcs() bool { return w.GateHolders() }

// InTerritory reports whether a root-relative path is inside a creature's territory: one of its
// owned prefixes, or its own directory (a body may always keep its own doc truthful).
func (c *Creature) InTerritory(rel string) bool {
	rel = normRel(rel)
	if c.Dir != "" && under(rel, c.Dir) {
		return true
	}
	for _, t := range c.Territory {
		if under(rel, t) {
			return true
		}
	}
	return false
}

func normRel(p string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(p)), "./"))
}

func under(rel, prefix string) bool {
	prefix = normRel(prefix)
	return prefix != "" && prefix != "." && (rel == prefix || strings.HasPrefix(rel, prefix+"/"))
}

// Owner is the body that owns a root-relative path: the authoring creature with the longest
// matching territory, else the gate holder's, else nil. Its doc is the one Vitality demands.
func (w *World) Owner(rel string) *Creature {
	rel = normRel(rel)
	for _, pick := range []func(Rank) bool{func(r Rank) bool { return r.Authors }, func(r Rank) bool { return r.HoldsGate && !r.Authors }} {
		var best *Creature
		bestLen := -1
		for i := range w.Creatures {
			c := &w.Creatures[i]
			if r, ok := w.Ranks.Get(c.Rank); !ok || !pick(r) {
				continue
			}
			for _, t := range c.Territory {
				if under(rel, t) && len(t) > bestLen {
					best, bestLen = c, len(t)
				}
			}
		}
		if best != nil {
			return best
		}
	}
	return nil
}

func parentDir(d string) string { return filepath.Dir(d) }
