package onto

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseSubset(t *testing.T) {
	src := `# comment
@prefix ao: <agent-one:> .
@prefix ex: <http://example.org/> .
@base <http://base/> .
ao:x a ao:ZoneWorker, ao:Member ;
    ao:owns "src/auth", 'q' ;      # trailing comment
    ao:n 15 ; ao:d 1.5 ; ao:e 2e3 ; ao:b true ;
    ao:t "hi"@en ; ao:u "raw"^^ex:T ;
    ao:long """two
lines""" ;
    ex:rel <rel> ; ao:esc "a\"b\\c\n\u00e9" .
_:b1 ao:p [ ao:q "in" ] .
[] ao:z ao:x .
`
	g := New()
	pre, err := Parse("t.ttl", src, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pre["ex"] != "http://example.org/" {
		t.Errorf("prefix: %v", pre)
	}
	want := []Triple{
		{Ao("x"), rdfType, Ao("ZoneWorker")},
		{Ao("x"), rdfType, Ao("Member")},
		{Ao("x"), Ao("owns"), L("src/auth")},
		{Ao("x"), Ao("owns"), L("q")},
		{Ao("x"), Ao("n"), Typed("15", XSD+"integer")},
		{Ao("x"), Ao("d"), Typed("1.5", XSD+"decimal")},
		{Ao("x"), Ao("e"), Typed("2e3", XSD+"double")},
		{Ao("x"), Ao("b"), Typed("true", XSD+"boolean")},
		{Ao("x"), Ao("t"), Term{Kind: Literal, Value: "hi", Lang: "en"}},
		{Ao("x"), Ao("u"), Typed("raw", "http://example.org/T")},
		{Ao("x"), Ao("long"), L("two\nlines")},
		{Ao("x"), I("http://example.org/rel"), I("http://base/rel")},
		{Ao("x"), Ao("esc"), L("a\"b\\c\né")},
		{Term{Kind: Blank, Value: "b1"}, Ao("p"), Term{Kind: Blank, Value: "b1"}},
	}
	for _, tr := range want[:len(want)-1] {
		if !g.Has(tr) {
			t.Errorf("missing %v", tr)
		}
	}
	if n := len(g.Match(nil, ptr(Ao("q")), ptr(L("in")))); n != 1 {
		t.Errorf("anonymous node: %d", n)
	}
	if n := len(g.Match(nil, ptr(Ao("z")), ptr(Ao("x")))); n != 1 {
		t.Errorf("empty anonymous subject: %d", n)
	}
	if g.Len() != 16 {
		t.Errorf("len %d", g.Len())
	}
}

func ptr(t Term) *Term { return &t }

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"@prefix ao: <agent-one:> .\nao:x a ao:Y\nao:z a ao:Y .": "t.ttl:3:1: expected '.' to end the statement",
		"@prefix ao: <agent-one:> .\nao:x ao:p ( 1 2 ) .":        "t.ttl:2:11: collections are not supported",
		"foo bar .": "t.ttl:1:1: unexpected word \"foo\"",
		"@prefix ao: <agent-one:> .\nao:x ao:s \"open .": "t.ttl:2:11: unterminated string",
		"@prefix ao: <agent-one:> .\nao:x ao:s <a b> .":  "illegal character",
		"@prefix ao: <agent-one:> .\nao:x ao:s ex:y .":   "t.ttl:2:11: undeclared prefix \"ex:\"",
	}
	for src, want := range cases {
		_, err := Parse("t.ttl", src, New(), nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %s", src, err, want)
		}
	}
}

func TestWriteRoundTrip(t *testing.T) {
	g := New()
	_, err := Parse("s.ttl", DefaultSchema, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.Add(Triple{Ao("x"), Ao("text"), L("say \"hi\"\n")})
	g.Add(Triple{Ao("x"), Ao("t"), Term{Kind: Literal, Value: "hi", Lang: "fr"}})
	g.Add(Triple{Ao("x"), Ao("n"), Int(3)})
	g.Add(Triple{Ao("x"), Ao("far"), I("http://elsewhere/1")})
	var b bytes.Buffer
	if err := Write(&b, g.All(), DefaultPrefixes()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.HasPrefix(out, "@prefix ao: <agent-one:> .\n\n") || strings.Contains(out[24:], "<agent-one:") {
		t.Errorf("prefix block or unprefixed IRIs:\n%s", out[:200])
	}
	if !strings.Contains(out, "ao:x ao:far <http://elsewhere/1> ;\n    ao:n 3 ;\n    ao:t \"hi\"@fr ;\n    ao:text \"say \\\"hi\\\"\\n\" .") {
		t.Errorf("subject block:\n%s", out)
	}
	g2 := New()
	if _, err := Parse("round.ttl", out, g2, nil); err != nil {
		t.Fatal(err)
	}
	a, c := g.All(), g2.All()
	if len(a) != len(c) {
		t.Fatalf("round trip: %d vs %d", len(a), len(c))
	}
	for i := range a {
		if a[i] != c[i] {
			t.Errorf("round trip differs at %d: %v vs %v", i, a[i], c[i])
		}
	}
	var b2 bytes.Buffer
	Write(&b2, g2.All(), DefaultPrefixes())
	if b2.String() != out {
		t.Error("writer is not deterministic")
	}
}

func TestMatchIndexes(t *testing.T) {
	g := New()
	for _, tr := range []Triple{{Ao("a"), Ao("p"), Ao("b")}, {Ao("a"), Ao("q"), Ao("c")}, {Ao("d"), Ao("p"), Ao("b")}} {
		g.Add(tr)
	}
	if n := len(g.Match(ptr(Ao("a")), nil, nil)); n != 2 {
		t.Errorf("by subject: %d", n)
	}
	if got := g.Subjects(Ao("p"), Ao("b")); len(got) != 2 || got[0] != Ao("a") || got[1] != Ao("d") {
		t.Errorf("subjects sorted: %v", got)
	}
	if g.Add(Triple{Ao("a"), Ao("p"), Ao("b")}) {
		t.Error("duplicate add reported as new")
	}
}
