package onto

// Vocabulary of the workspace, one namespace.
var (
	rdfType       = I(RDFType)
	pSubClassOf   = Ao("subClassOf")
	pSubPropOf    = Ao("subPropertyOf")
	pDomain       = Ao("domain")
	pRange        = Ao("range")
	pInverseOf    = Ao("inverseOf")
	cTransitive   = Ao("Transitive")
	cShape        = Ao("Shape")
	pOn           = Ao("on")
	pProperty     = Ao("property")
	pMinCount     = Ao("minCount")
	pMaxCount     = Ao("maxCount")
	pDisjoint     = Ao("disjoint")
	pValuesHave   = Ao("valuesHave")
	pName         = Ao("name")
	pDoc          = Ao("doc")
	pPath         = Ao("path")
	pText         = Ao("text")
	pSaid         = Ao("said")
	pAbove        = Ao("above")
	pBelow        = Ao("below")
	pTruth        = Ao("truth")
	pVerdict      = Ao("verdict")
	pReports      = Ao("reports")
	pHolds        = Ao("holds")
	pOwns         = Ao("owns")
	pKnows        = Ao("knows")
	pAbout        = Ao("about")
	pShared       = Ao("shared")
	pUnless       = Ao("unless")
	cMember       = Ao("Member")
	cOrchestrator = Ao("Orchestrator")
	cZone         = Ao("ZoneWorker")
	cSkill        = Ao("Skill")
	cDoc          = Ao("Doc")
	cFact         = Ao("Fact")
	cPolicy       = Ao("Policy")
	tOrchestrator = Ao("orchestrator")
	kindClass     = map[string]Term{"policy": Ao("Policy"), "team": Ao("Team"), "domain": Ao("Domain")}
	bondsByHop    = []Term{pTruth, pVerdict, pReports, pAbove}
	rankClass     = map[string]Term{"coord": Ao("Coordinator"), "domain": Ao("DomainOwner"), "zone": Ao("ZoneWorker"), "service": Ao("ServiceOwner")}
	skillDirs     = []string{".opencode/skills", ".opencode/skill", ".claude/skills"}
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
