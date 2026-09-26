// Package onto is the ontology of an isekai world: a Turtle-subset schema, an
// in-memory triple store, a forward-chaining reasoner, shapes, the flow of
// facts along the world's own bonds, and a plain-text projection for prompts.
package onto

import (
	"strconv"
	"strings"
)

// Kind is the sort of a Term.
type Kind uint8

const (
	IRI Kind = iota
	Blank
	Literal
)

// Term is an IRI, a blank node or a literal.
type Term struct {
	Kind     Kind
	Value    string // IRI, blank label, or lexical form
	Lang     string
	Datatype string
}

// Triple is one statement.
type Triple struct{ S, P, O Term }

const (
	// NS is the one namespace of the world's vocabulary.
	NS      = "isekai:"
	RDFType = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"
	XSD     = "http://www.w3.org/2001/XMLSchema#"
)

// I makes an IRI term; L a plain literal; Is a term in the world namespace.
func I(iri string) Term   { return Term{Kind: IRI, Value: iri} }
func L(s string) Term     { return Term{Kind: Literal, Value: s} }
func Is(name string) Term { return Term{Kind: IRI, Value: NS + name} }

// Typed makes a literal with a datatype.
func Typed(s, dt string) Term { return Term{Kind: Literal, Value: s, Datatype: dt} }

// Int makes an xsd:integer literal.
func Int(n int) Term { return Typed(strconv.Itoa(n), XSD+"integer") }

// Bool makes an xsd:boolean literal.
func Bool(b bool) Term { return Typed(strconv.FormatBool(b), XSD+"boolean") }

// Local is the name of an IRI without its namespace; other terms give their value.
func (t Term) Local() string {
	if t.Kind != IRI {
		return t.Value
	}
	if strings.HasPrefix(t.Value, NS) {
		return t.Value[len(NS):]
	}
	if i := strings.LastIndexAny(t.Value, "#/"); i >= 0 && i+1 < len(t.Value) {
		return t.Value[i+1:]
	}
	return t.Value
}

// IsIRI reports whether the term is an IRI.
func (t Term) IsIRI() bool { return t.Kind == IRI }

// AsInt reads an integer literal; ok is false otherwise.
func (t Term) AsInt() (n int, ok bool) {
	if t.Kind != Literal {
		return 0, false
	}
	n, err := strconv.Atoi(t.Value)
	return n, err == nil
}

// String renders the term the way N-Triples would, for messages and sorting.
func (t Term) String() string {
	switch t.Kind {
	case IRI:
		return "<" + t.Value + ">"
	case Blank:
		return "_:" + t.Value
	}
	s := strconv.Quote(t.Value)
	if t.Lang != "" {
		return s + "@" + t.Lang
	}
	if t.Datatype != "" {
		return s + "^^<" + t.Datatype + ">"
	}
	return s
}

func cmpTerm(a, b Term) int {
	if a.Kind != b.Kind {
		return int(a.Kind) - int(b.Kind)
	}
	if c := strings.Compare(a.Value, b.Value); c != 0 {
		return c
	}
	if c := strings.Compare(a.Lang, b.Lang); c != 0 {
		return c
	}
	return strings.Compare(a.Datatype, b.Datatype)
}

func cmpTriple(a, b Triple) int {
	if c := cmpTerm(a.S, b.S); c != 0 {
		return c
	}
	if c := cmpTerm(a.P, b.P); c != 0 {
		return c
	}
	return cmpTerm(a.O, b.O)
}
