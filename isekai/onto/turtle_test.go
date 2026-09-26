package onto

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseSubset(t *testing.T) {
	src := `# comment
@prefix is: <isekai:> .
@prefix ex: <http://example.org/> .
@base <http://base/> .
is:x a is:Slime, is:Creature ;
    is:owns "src/auth", 'q' ;      # trailing comment
    is:n 15 ; is:d 1.5 ; is:e 2e3 ; is:b true ;
    is:t "hi"@en ; is:u "raw"^^ex:T ;
    is:long """two
lines""" ;
    ex:rel <rel> ; is:esc "a\"b\\c\n\u00e9" .
_:b1 is:p [ is:q "in" ] .
[] is:z is:x .
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
		{Is("x"), rdfType, Is("Slime")},
		{Is("x"), rdfType, Is("Creature")},
		{Is("x"), Is("owns"), L("src/auth")},
		{Is("x"), Is("owns"), L("q")},
		{Is("x"), Is("n"), Typed("15", XSD+"integer")},
		{Is("x"), Is("d"), Typed("1.5", XSD+"decimal")},
		{Is("x"), Is("e"), Typed("2e3", XSD+"double")},
		{Is("x"), Is("b"), Typed("true", XSD+"boolean")},
		{Is("x"), Is("t"), Term{Kind: Literal, Value: "hi", Lang: "en"}},
		{Is("x"), Is("u"), Typed("raw", "http://example.org/T")},
		{Is("x"), Is("long"), L("two\nlines")},
		{Is("x"), I("http://example.org/rel"), I("http://base/rel")},
		{Is("x"), Is("esc"), L("a\"b\\c\né")},
		{Term{Kind: Blank, Value: "b1"}, Is("p"), Term{Kind: Blank, Value: "b1"}},
	}
	for _, tr := range want[:len(want)-1] {
		if !g.Has(tr) {
			t.Errorf("missing %v", tr)
		}
	}
	if n := len(g.Match(nil, ptr(Is("q")), ptr(L("in")))); n != 1 {
		t.Errorf("anonymous node: %d", n)
	}
	if n := len(g.Match(nil, ptr(Is("z")), ptr(Is("x")))); n != 1 {
		t.Errorf("empty anonymous subject: %d", n)
	}
	if g.Len() != 16 {
		t.Errorf("len %d", g.Len())
	}
}

func ptr(t Term) *Term { return &t }

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"@prefix is: <isekai:> .\nis:x a is:Y\nis:z a is:Y .": "t.ttl:3:1: expected '.' to end the statement",
		"@prefix is: <isekai:> .\nis:x is:p ( 1 2 ) .":        "t.ttl:2:11: collections are not supported",
		"foo bar .": "t.ttl:1:1: unexpected word \"foo\"",
		"@prefix is: <isekai:> .\nis:x is:s \"open .": "t.ttl:2:11: unterminated string",
		"@prefix is: <isekai:> .\nis:x is:s <a b> .":  "illegal character",
		"@prefix is: <isekai:> .\nis:x is:s ex:y .":   "t.ttl:2:11: undeclared prefix \"ex:\"",
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
	g.Add(Triple{Is("x"), Is("text"), L("say \"hi\"\n")})
	g.Add(Triple{Is("x"), Is("t"), Term{Kind: Literal, Value: "hi", Lang: "fr"}})
	g.Add(Triple{Is("x"), Is("n"), Int(3)})
	g.Add(Triple{Is("x"), Is("far"), I("http://elsewhere/1")})
	var b bytes.Buffer
	if err := Write(&b, g.All(), DefaultPrefixes()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.HasPrefix(out, "@prefix is: <isekai:> .\n\n") || strings.Contains(out[24:], "<isekai:") {
		t.Errorf("prefix block or unprefixed IRIs:\n%s", out[:200])
	}
	if !strings.Contains(out, "is:x is:far <http://elsewhere/1> ;\n    is:n 3 ;\n    is:t \"hi\"@fr ;\n    is:text \"say \\\"hi\\\"\\n\" .") {
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
	for _, tr := range []Triple{{Is("a"), Is("p"), Is("b")}, {Is("a"), Is("q"), Is("c")}, {Is("d"), Is("p"), Is("b")}} {
		g.Add(tr)
	}
	if n := len(g.Match(ptr(Is("a")), nil, nil)); n != 2 {
		t.Errorf("by subject: %d", n)
	}
	if got := g.Subjects(Is("p"), Is("b")); len(got) != 2 || got[0] != Is("a") || got[1] != Is("d") {
		t.Errorf("subjects sorted: %v", got)
	}
	if g.Add(Triple{Is("a"), Is("p"), Is("b")}) {
		t.Error("duplicate add reported as new")
	}
}
