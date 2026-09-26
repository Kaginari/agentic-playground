package onto

// Vocabulary of the world, one namespace.
var (
	rdfType     = I(RDFType)
	pSubClassOf = Is("subClassOf")
	pSubPropOf  = Is("subPropertyOf")
	pDomain     = Is("domain")
	pRange      = Is("range")
	pInverseOf  = Is("inverseOf")
	cTransitive = Is("Transitive")
	cShape      = Is("Shape")
	pOn         = Is("on")
	pProperty   = Is("property")
	pMinCount   = Is("minCount")
	pMaxCount   = Is("maxCount")
	pDisjoint   = Is("disjoint")
	pValuesHave = Is("valuesHave")
	pName       = Is("name")
	pDoc        = Is("doc")
	pPath       = Is("path")
	pText       = Is("text")
	pSaid       = Is("said")
	pAbove      = Is("above")
	pBelow      = Is("below")
	pTruth      = Is("truth")
	pVerdict    = Is("verdict")
	pReports    = Is("reports")
	pWears      = Is("wears")
	pOwns       = Is("owns")
	pKnows      = Is("knows")
	pAbout      = Is("about")
	pShared     = Is("shared")
	pUnless     = Is("unless")
	cCreature   = Is("Creature")
	cRimuru     = Is("Rimuru")
	cSlime      = Is("Slime")
	cMind       = Is("Mind")
	cDoc        = Is("Doc")
	cFact       = Is("Fact")
	cLaw        = Is("Law")
	tRimuru     = Is("rimuru")
	kindClass   = map[string]Term{"law": Is("Law"), "colony": Is("Colony"), "territory": Is("Territory")}
	bondsByHop  = []Term{pTruth, pVerdict, pReports, pAbove}
	raceClass   = map[string]Term{"elf": Is("Elf"), "orc": Is("Orc"), "slime": Is("Slime"), "kijin": Is("Kijin")}
	mindDirs    = []string{".opencode/skills", ".opencode/skill", ".claude/skills"}
)

// Infer runs forward chaining to a fixpoint: subClassOf and subPropertyOf
// closure, type propagation, subproperty entailment, domain and range typing,
// inverses, and transitive properties. It returns the number of derived
// triples added. Deterministic: every pass walks sorted triples.
func Infer(g *Graph) int {
	added := 0
	for {
		n := 0
		var out []Triple
		// subClassOf / subPropertyOf are transitive
		for _, p := range []Term{pSubClassOf, pSubPropOf} {
			for _, t := range g.Match(nil, &p, nil) {
				for _, u := range g.Match(&t.O, &p, nil) {
					out = append(out, Triple{t.S, p, u.O})
				}
			}
		}
		// (x a C)(C subClassOf D) → x a D
		for _, t := range g.Match(nil, &pSubClassOf, nil) {
			for _, x := range g.Subjects(rdfType, t.S) {
				out = append(out, Triple{x, rdfType, t.O})
			}
		}
		// (x p y)(p subPropertyOf q) → x q y
		for _, t := range g.Match(nil, &pSubPropOf, nil) {
			for _, u := range g.Match(nil, &t.S, nil) {
				out = append(out, Triple{u.S, t.O, u.O})
			}
		}
		// domain / range
		for _, t := range g.Match(nil, &pDomain, nil) {
			for _, u := range g.Match(nil, &t.S, nil) {
				out = append(out, Triple{u.S, rdfType, t.O})
			}
		}
		for _, t := range g.Match(nil, &pRange, nil) {
			for _, u := range g.Match(nil, &t.S, nil) {
				if u.O.Kind != Literal {
					out = append(out, Triple{u.O, rdfType, t.O})
				}
			}
		}
		// inverses, both ways
		for _, t := range g.Match(nil, &pInverseOf, nil) {
			for _, u := range g.Match(nil, &t.S, nil) {
				if u.O.Kind != Literal {
					out = append(out, Triple{u.O, t.O, u.S})
				}
			}
			for _, u := range g.Match(nil, &t.O, nil) {
				if u.O.Kind != Literal {
					out = append(out, Triple{u.O, t.S, u.S})
				}
			}
		}
		// transitive properties
		for _, p := range g.Instances(cTransitive) {
			for _, t := range g.Match(nil, &p, nil) {
				for _, u := range g.Match(&t.O, &p, nil) {
					out = append(out, Triple{t.S, p, u.O})
				}
			}
		}
		for _, t := range out {
			if g.AddDerived(t) {
				n++
			}
		}
		if n == 0 {
			return added
		}
		added += n
	}
}
