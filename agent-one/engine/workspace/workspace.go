package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/onto"
)

// Member is one ranked agent read off its doc — through onto's derivation, never a second
// parser: rank, ownership, parent bond, held skills; plus its verify commands (invariants as
// readings) and the doc that owns it.
type Member struct {
	Name      string   // zone-auth (zone-auth under agent-one)
	Rank      string   // the rank's name in the table (zone, domain, coord, service, or a custom one)
	Doc       string   // relative path of the owning doc
	Dir       string   // relative path of the member's directory
	Ownership []string // normalized path prefixes it owns
	Parent    string   // one hop up (domain for a zone worker, coord for an domain owner, orchestrator for an coordinator)
	Skills    []string // held skills
	Verify    []string // shell commands whose exit codes are the invariants' reading
}

// Options is what Open needs beyond the root and the lexicon.
type Options struct {
	Instructions InstructionOptions
	Ontology     bool  // load the ontology (members need it; off = an empty roster)
	Ranks        Ranks // the rank table; nil = DefaultRanks(lex) (config's `rankSet: replace` passes the whole table)
}

// Workspace is one opened workspace.
type Workspace struct {
	Root         string
	Lex          Lexicon
	Ranks        Ranks
	Policy       *Policy
	Onto         *onto.Workspace
	Members      []Member
	Instructions []Instruction
	Notes        []string // holes met while opening

	// Foreign are agents read from another harness's agent files (discovery.agents): they
	// stay on the roster across Reload, beside the members derived from the docs.
	Foreign []Member

	mu       sync.Mutex
	sessions map[*loop.Engine]*loop.Session
	closers  map[*loop.Engine]func()
	// subagentWrote is what the Subagents of a dispatcher's engine wrote and gated on their own
	// account; the dispatcher's end gate leaves those paths alone (never a second gate).
	subagentWrote map[*loop.Engine]map[string]bool
}

// Discover walks up from dir for the workspace dir.
func Discover(dir string) (string, Lexicon, error) {
	l := Default()
	root, err := onto.FindRootIn(dir, l.Layout())
	if err != nil {
		return "", Lexicon{}, err
	}
	return root, l, nil
}

// Open loads the workspace at root under a lexicon: the policy, the ontology and its members, the
// instructions. A missing policy is a note, not a failure — the principles is then empty and the
// system prompt says so.
func Open(root string, lex Lexicon, opt Options) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(filepath.Join(abs, lex.WorkspaceDir)); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("no %s/ under %s", lex.WorkspaceDir, abs)
	}
	w := &Workspace{Root: abs, Lex: lex, Ranks: opt.Ranks, sessions: map[*loop.Engine]*loop.Session{}, closers: map[*loop.Engine]func(){}, subagentWrote: map[*loop.Engine]map[string]bool{}}
	if w.Ranks == nil {
		w.Ranks = DefaultRanks(lex)
	}
	if err := w.Ranks.Validate(); err != nil {
		return nil, fmt.Errorf("ranks: %v", err)
	}
	policyPath := filepath.Join(abs, lex.WorkspaceDir, lex.Policy)
	if w.Policy, err = LoadPolicy(policyPath, lex.WorkspaceDir+"/"+lex.Policy, lex.Principles); err != nil {
		w.Notes = append(w.Notes, err.Error())
	} else if w.Policy.Principles == "" {
		w.Notes = append(w.Notes, w.Policy.Path+" has no principles section")
	}
	if opt.Ontology {
		if err := w.Reload(); err != nil {
			return nil, err
		}
	}
	w.Instructions = LoadInstructions(abs, opt.Instructions)
	return w, nil
}

// Dir is <root>/<workspaceDir>.
func (w *Workspace) Dir() string { return filepath.Join(w.Root, w.Lex.WorkspaceDir) }

// Rel renders an absolute path relative to the root (slash-separated); outside paths stay as is.
func (w *Workspace) Rel(abs string) string {
	if r, err := filepath.Rel(w.Root, abs); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return abs
}

// Reload re-derives the roster from the docs (they change during a session).
func (w *Workspace) Reload() error {
	ow, err := onto.LoadLayout(w.Root, w.Ranks.Layout(w.Lex))
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Onto = ow
	w.Notes = append(w.Notes, ow.Notes...)
	w.Members = membersOf(w.Root, ow, w.Ranks)
	for _, f := range w.Foreign {
		if w.memberLocked(f.Name) == nil {
			w.Members = append(w.Members, f)
		}
	}
	return nil
}

// AddForeign puts a agent from another harness's agent file on the roster (a name already on
// it is one agent with two sources: the native one wins, Principle 2).
func (w *Workspace) AddForeign(c Member) {
	w.mu.Lock()
	w.Foreign = append(w.Foreign, c)
	exists := w.memberLocked(c.Name) != nil
	if !exists {
		w.Members = append(w.Members, c)
	}
	w.mu.Unlock()
}

func (w *Workspace) memberLocked(name string) *Member {
	for i := range w.Members {
		if w.Members[i].Name == name {
			return &w.Members[i]
		}
	}
	return nil
}

var (
	cMember   = onto.Ao("Member")
	pOwns     = onto.Ao("owns")
	pHolds    = onto.Ao("holds")
	pName     = onto.Ao("name")
	bondProps = []onto.Term{onto.Ao("truth"), onto.Ao("verdict"), onto.Ao("reports"), onto.Ao("above")}
	reRank    = regexp.MustCompile(`(?mi)^\s*[-*]\s*\*\*rank:?\*\*:?\s*(.+?)\s*$`)
)

func membersOf(root string, ow *onto.Workspace, ranks Ranks) []Member {
	g := ow.Graph
	var out []Member
	for _, t := range g.Instances(cMember) {
		name := t.Local()
		if name == Orchestrator {
			continue
		}
		c := Member{Name: name}
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
		// the rank: the folder's base rank, unless the doc's Rank field names an promoted rank on that dir
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
			c.Ownership = append(c.Ownership, o.Value)
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
		for _, m := range g.Objects(t, pHolds) {
			if n := g.Objects(m, pName); len(n) > 0 {
				c.Skills = append(c.Skills, n[0].Value)
			} else {
				c.Skills = append(c.Skills, strings.TrimPrefix(m.Local(), "skill-"))
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

// VerifyOf reads a member doc's verify commands: every `- **Verify:** cmd` line, plus the
// bullets of a `## Verify` section. Exit codes, not prose, are the invariants' reading.
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
		agent := doc[loc[1]:]
		if end := regexp.MustCompile(`\n#`).FindStringIndex(agent); end != nil {
			agent = agent[:end[0]]
		}
		for _, m := range reBullet.FindAllStringSubmatch(agent, -1) {
			add(m[1])
		}
	}
	return out
}

// Member finds a agent by id (or by its unprefixed name when unambiguous).
func (w *Workspace) Member(name string) *Member {
	name = strings.ToLower(strings.TrimSpace(name))
	var hit *Member
	for i := range w.Members {
		c := &w.Members[i]
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

// oneHopAbove is true when no third member sits between s and o on the above chain.
func oneHopAbove(g *onto.Graph, s, o onto.Term) bool {
	above := onto.Ao("above")
	for _, mid := range g.Objects(s, above) {
		if mid != o && g.Has(onto.Triple{S: mid, P: above, O: o}) {
			return false
		}
	}
	return true
}

// RankOf is a member's rank row (orchestrator's for the session or an unknown name).
func (w *Workspace) RankOf(name string) Rank {
	if c := w.Member(name); c != nil {
		if r, ok := w.Ranks.Get(c.Rank); ok {
			return r
		}
	}
	if rn := w.Ranks.Of(name); rn != "" {
		if r, ok := w.Ranks.Get(rn); ok {
			return r
		}
	}
	r, _ := w.Ranks.Get(Orchestrator)
	return r
}

// GateHolders reports whether any member on the roster holds a gate (an domain owner, by the policy's
// table). Domain owners is its old name.
func (w *Workspace) GateHolders() bool {
	for _, c := range w.Members {
		if r, ok := w.Ranks.Get(c.Rank); ok && r.HoldsGate {
			return true
		}
	}
	return false
}

// Domain owners is GateHolders under the policy's name.
func (w *Workspace) DomainOwners() bool { return w.GateHolders() }

// InOwnership reports whether a root-relative path is inside a member's ownership: one of its
// owned prefixes, or its own directory (a agent may always keep its own doc truthful).
func (c *Member) InOwnership(rel string) bool {
	rel = normRel(rel)
	if c.Dir != "" && under(rel, c.Dir) {
		return true
	}
	for _, t := range c.Ownership {
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

// Owner is the agent that owns a root-relative path: the authoring member with the longest
// matching ownership, else the gate holder's, else nil. Its doc is the one Docs-as-code demands.
func (w *Workspace) Owner(rel string) *Member {
	rel = normRel(rel)
	for _, pick := range []func(Rank) bool{func(r Rank) bool { return r.Authors }, func(r Rank) bool { return r.HoldsGate && !r.Authors }} {
		var best *Member
		bestLen := -1
		for i := range w.Members {
			c := &w.Members[i]
			if r, ok := w.Ranks.Get(c.Rank); !ok || !pick(r) {
				continue
			}
			for _, t := range c.Ownership {
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
