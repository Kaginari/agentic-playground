package onto

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The Turtle subset: @prefix / @base, IRIs, prefixed names, `a`, `;` and `,`
// lists, `[ … ]` anonymous nodes, string literals with @lang or ^^type,
// numbers, booleans, comments. No collections, no SPARQL-style directives.

type tokKind int

const (
	tEOF tokKind = iota
	tIRI
	tPName // val = "prefix:local"
	tBlank
	tString
	tNumber
	tBool
	tA
	tDot
	tSemi
	tComma
	tLBrack
	tRBrack
	tLParen
	tRParen
	tPrefix
	tBase
	tLang  // val = tag
	tDType // ^^
	tWord  // a bare word that is not a keyword
)

type token struct {
	kind      tokKind
	val       string
	line, col int
}

// SyntaxError carries the file position of a Turtle error.
type SyntaxError struct {
	File      string
	Line, Col int
	Msg       string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
}

type lexer struct {
	file      string
	src       string
	pos       int
	line, col int
}

func (l *lexer) errf(line, col int, f string, a ...any) *SyntaxError {
	return &SyntaxError{File: l.file, Line: line, Col: col, Msg: fmt.Sprintf(f, a...)}
}

func (l *lexer) peek() rune {
	if l.pos >= len(l.src) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	return r
}

func (l *lexer) peekAt(n int) rune {
	p := l.pos
	for i := 0; i < n; i++ {
		if p >= len(l.src) {
			return -1
		}
		_, w := utf8.DecodeRuneInString(l.src[p:])
		p += w
	}
	if p >= len(l.src) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(l.src[p:])
	return r
}

func (l *lexer) next() rune {
	if l.pos >= len(l.src) {
		return -1
	}
	r, w := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += w
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func isNameStart(r rune) bool { return unicode.IsLetter(r) || r == '_' }
func isNameChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '%' || r == ':'
}

func (l *lexer) all() ([]token, error) {
	var toks []token
	for {
		t, err := l.tok()
		if err != nil {
			return nil, err
		}
		toks = append(toks, t)
		if t.kind == tEOF {
			return toks, nil
		}
	}
}

func (l *lexer) tok() (token, error) {
	for {
		r := l.peek()
		switch {
		case r == -1:
			return token{kind: tEOF, line: l.line, col: l.col}, nil
		case r == '#':
			for l.peek() != '\n' && l.peek() != -1 {
				l.next()
			}
		case unicode.IsSpace(r):
			l.next()
		default:
			goto scan
		}
	}
scan:
	line, col := l.line, l.col
	r := l.peek()
	at := func(k tokKind, v string) (token, error) { return token{kind: k, val: v, line: line, col: col}, nil }
	switch {
	case r == '<':
		l.next()
		var b strings.Builder
		for {
			c := l.next()
			if c == -1 || c == '\n' {
				return token{}, l.errf(line, col, "unterminated IRI")
			}
			if c == '>' {
				break
			}
			if c == ' ' || c == '"' || c == '{' || c == '}' || c == '|' || c == '^' || c == '`' {
				return token{}, l.errf(line, col, "illegal character %q in IRI", c)
			}
			b.WriteRune(c)
		}
		return at(tIRI, b.String())
	case r == '"' || r == '\'':
		s, err := l.str(line, col)
		if err != nil {
			return token{}, err
		}
		return at(tString, s)
	case r == '.':
		l.next()
		return at(tDot, ".")
	case r == ';':
		l.next()
		return at(tSemi, ";")
	case r == ',':
		l.next()
		return at(tComma, ",")
	case r == '[':
		l.next()
		return at(tLBrack, "[")
	case r == ']':
		l.next()
		return at(tRBrack, "]")
	case r == '(':
		l.next()
		return at(tLParen, "(")
	case r == ')':
		l.next()
		return at(tRParen, ")")
	case r == '^':
		l.next()
		if l.next() != '^' {
			return token{}, l.errf(line, col, "expected '^^'")
		}
		return at(tDType, "^^")
	case r == '@':
		l.next()
		w := l.word(func(c rune) bool { return unicode.IsLetter(c) || c == '-' })
		switch w {
		case "prefix":
			return at(tPrefix, w)
		case "base":
			return at(tBase, w)
		case "":
			return token{}, l.errf(line, col, "expected language tag or directive after '@'")
		}
		return at(tLang, w)
	case r == '_' && l.peekAt(1) == ':':
		l.next()
		l.next()
		w := l.word(func(c rune) bool { return isNameChar(c) && c != ':' })
		w = l.trimDots(w)
		if w == "" {
			return token{}, l.errf(line, col, "blank node needs a label")
		}
		return at(tBlank, w)
	case unicode.IsDigit(r) || ((r == '+' || r == '-') && unicode.IsDigit(l.peekAt(1))):
		return at(tNumber, l.number())
	case isNameStart(r) || r == ':':
		w := l.word(isNameChar)
		w = l.trimDots(w)
		if !strings.Contains(w, ":") {
			switch w {
			case "a":
				return at(tA, w)
			case "true", "false":
				return at(tBool, w)
			}
			return token{}, l.errf(line, col, "unexpected word %q (prefixed names need a ':')", w)
		}
		return at(tPName, w)
	}
	return token{}, l.errf(line, col, "unexpected character %q", r)
}

// trimDots gives back trailing dots to the stream: they are statement terminators.
func (l *lexer) trimDots(w string) string {
	for strings.HasSuffix(w, ".") {
		w = w[:len(w)-1]
		l.pos--
		l.col--
	}
	return w
}

func (l *lexer) word(ok func(rune) bool) string {
	start := l.pos
	for l.peek() != -1 && ok(l.peek()) {
		l.next()
	}
	return l.src[start:l.pos]
}

func (l *lexer) number() string {
	start := l.pos
	if l.peek() == '+' || l.peek() == '-' {
		l.next()
	}
	for unicode.IsDigit(l.peek()) {
		l.next()
	}
	if l.peek() == '.' && unicode.IsDigit(l.peekAt(1)) {
		l.next()
		for unicode.IsDigit(l.peek()) {
			l.next()
		}
	}
	if (l.peek() == 'e' || l.peek() == 'E') && (unicode.IsDigit(l.peekAt(1)) || ((l.peekAt(1) == '+' || l.peekAt(1) == '-') && unicode.IsDigit(l.peekAt(2)))) {
		l.next()
		if l.peek() == '+' || l.peek() == '-' {
			l.next()
		}
		for unicode.IsDigit(l.peek()) {
			l.next()
		}
	}
	return l.src[start:l.pos]
}

func (l *lexer) str(line, col int) (string, error) {
	q := l.next()
	long := false
	if l.peek() == q && l.peekAt(1) == q {
		l.next()
		l.next()
		long = true
	}
	var b strings.Builder
	for {
		c := l.next()
		if c == -1 {
			return "", l.errf(line, col, "unterminated string")
		}
		if c == q {
			if !long {
				return b.String(), nil
			}
			if l.peek() == q && l.peekAt(1) == q {
				l.next()
				l.next()
				return b.String(), nil
			}
			b.WriteRune(c)
			continue
		}
		if c == '\n' && !long {
			return "", l.errf(line, col, "newline in string (use \"\"\" for long strings)")
		}
		if c != '\\' {
			b.WriteRune(c)
			continue
		}
		e := l.next()
		switch e {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case '"', '\'', '\\':
			b.WriteRune(e)
		case 'u', 'U':
			n := 4
			if e == 'U' {
				n = 8
			}
			var hex strings.Builder
			for i := 0; i < n; i++ {
				hex.WriteRune(l.next())
			}
			v, err := strconv.ParseUint(hex.String(), 16, 32)
			if err != nil {
				return "", l.errf(l.line, l.col, "bad unicode escape \\%c%s", e, hex.String())
			}
			b.WriteRune(rune(v))
		default:
			return "", l.errf(l.line, l.col, "bad escape \\%c", e)
		}
	}
}

// ---------------------------------------------------------------- parser

type parser struct {
	file     string
	toks     []token
	pos      int
	prefixes map[string]string
	base     string
	g        *Graph
	blanks   int
	derived  bool
}

func (p *parser) cur() token { return p.toks[p.pos] }
func (p *parser) advance() token {
	t := p.toks[p.pos]
	if t.kind != tEOF {
		p.pos++
	}
	return t
}

func (p *parser) errf(t token, f string, a ...any) error {
	return &SyntaxError{File: p.file, Line: t.line, Col: t.col, Msg: fmt.Sprintf(f, a...)}
}

func (p *parser) expect(k tokKind, what string) (token, error) {
	t := p.cur()
	if t.kind != k {
		return t, p.errf(t, "expected %s, got %s", what, describe(t))
	}
	return p.advance(), nil
}

func describe(t token) string {
	switch t.kind {
	case tEOF:
		return "end of file"
	case tString:
		return "string " + strconv.Quote(t.val)
	}
	return strconv.Quote(t.val)
}

// Parse reads Turtle from src into g. Triples go in as asserted unless derived
// is set. It returns the prefixes the document declared.
func Parse(file, src string, g *Graph, prefixes map[string]string) (map[string]string, error) {
	lx := &lexer{file: file, src: src, line: 1, col: 1}
	toks, err := lx.all()
	if err != nil {
		return nil, err
	}
	p := &parser{file: file, toks: toks, prefixes: map[string]string{}, g: g}
	for k, v := range prefixes {
		p.prefixes[k] = v
	}
	if err := p.doc(); err != nil {
		return nil, err
	}
	return p.prefixes, nil
}

// ParseFile reads a Turtle file into g.
func ParseFile(path string, g *Graph, prefixes map[string]string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, string(b), g, prefixes)
}

func (p *parser) doc() error {
	for p.cur().kind != tEOF {
		switch p.cur().kind {
		case tPrefix:
			p.advance()
			t, err := p.expect(tPName, "prefix name like 'is:'")
			if err != nil {
				return err
			}
			if !strings.HasSuffix(t.val, ":") || strings.Count(t.val, ":") != 1 {
				return p.errf(t, "prefix name must end with ':'")
			}
			iri, err := p.expect(tIRI, "IRI")
			if err != nil {
				return err
			}
			p.prefixes[strings.TrimSuffix(t.val, ":")] = p.resolve(iri.val)
			if _, err := p.expect(tDot, "'.'"); err != nil {
				return err
			}
		case tBase:
			p.advance()
			iri, err := p.expect(tIRI, "IRI")
			if err != nil {
				return err
			}
			p.base = p.resolve(iri.val)
			if _, err := p.expect(tDot, "'.'"); err != nil {
				return err
			}
		default:
			s, err := p.subject()
			if err != nil {
				return err
			}
			if err := p.predicateObjectList(s, false); err != nil {
				return err
			}
			if _, err := p.expect(tDot, "'.' to end the statement"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) resolve(iri string) string {
	if p.base != "" && !strings.Contains(iri, ":") {
		return p.base + iri
	}
	return iri
}

func (p *parser) pname(t token) (Term, error) {
	i := strings.Index(t.val, ":")
	pre, local := t.val[:i], t.val[i+1:]
	ns, ok := p.prefixes[pre]
	if !ok {
		return Term{}, p.errf(t, "undeclared prefix %q", pre+":")
	}
	return I(ns + local), nil
}

func (p *parser) subject() (Term, error) {
	t := p.cur()
	switch t.kind {
	case tIRI:
		p.advance()
		return I(p.resolve(t.val)), nil
	case tPName:
		p.advance()
		return p.pname(t)
	case tBlank:
		p.advance()
		return Term{Kind: Blank, Value: t.val}, nil
	case tLBrack:
		return p.anon()
	case tLParen:
		return Term{}, p.errf(t, "collections are not supported")
	}
	return Term{}, p.errf(t, "expected a subject, got %s", describe(t))
}

func (p *parser) anon() (Term, error) {
	p.advance()
	p.blanks++
	b := Term{Kind: Blank, Value: fmt.Sprintf("b%d", p.blanks)}
	if p.cur().kind == tRBrack {
		p.advance()
		return b, nil
	}
	if err := p.predicateObjectList(b, true); err != nil {
		return Term{}, err
	}
	if _, err := p.expect(tRBrack, "']'"); err != nil {
		return Term{}, err
	}
	return b, nil
}

func (p *parser) verb() (Term, error) {
	t := p.cur()
	switch t.kind {
	case tA:
		p.advance()
		return I(RDFType), nil
	case tIRI:
		p.advance()
		return I(p.resolve(t.val)), nil
	case tPName:
		p.advance()
		return p.pname(t)
	}
	return Term{}, p.errf(t, "expected a predicate, got %s", describe(t))
}

func (p *parser) predicateObjectList(s Term, inAnon bool) error {
	for {
		v, err := p.verb()
		if err != nil {
			return err
		}
		for {
			o, err := p.object()
			if err != nil {
				return err
			}
			p.emit(Triple{S: s, P: v, O: o})
			if p.cur().kind != tComma {
				break
			}
			p.advance()
		}
		if p.cur().kind != tSemi {
			return nil
		}
		for p.cur().kind == tSemi {
			p.advance()
		}
		if k := p.cur().kind; k == tDot || (inAnon && k == tRBrack) {
			return nil
		}
	}
}

func (p *parser) emit(t Triple) {
	if p.derived {
		p.g.AddDerived(t)
	} else {
		p.g.Add(t)
	}
}

func (p *parser) object() (Term, error) {
	t := p.cur()
	switch t.kind {
	case tIRI, tPName, tBlank, tLBrack:
		return p.subject()
	case tLParen:
		return Term{}, p.errf(t, "collections are not supported")
	case tString:
		p.advance()
		lit := L(t.val)
		switch p.cur().kind {
		case tLang:
			lit.Lang = p.advance().val
		case tDType:
			p.advance()
			dt, err := p.verb()
			if err != nil {
				return Term{}, err
			}
			if dt.Value == RDFType {
				return Term{}, p.errf(t, "'a' is not a datatype")
			}
			lit.Datatype = dt.Value
		}
		return lit, nil
	case tNumber:
		p.advance()
		dt := "integer"
		if strings.ContainsAny(t.val, "eE") {
			dt = "double"
		} else if strings.Contains(t.val, ".") {
			dt = "decimal"
		}
		return Typed(t.val, XSD+dt), nil
	case tBool:
		p.advance()
		return Typed(t.val, XSD+"boolean"), nil
	}
	return Term{}, p.errf(t, "expected an object, got %s", describe(t))
}

// ---------------------------------------------------------------- writer

// Write emits triples as Turtle, grouped by subject, sorted, one prefix block.
func Write(w io.Writer, triples []Triple, prefixes map[string]string) error {
	var b strings.Builder
	names := make([]string, 0, len(prefixes))
	for k := range prefixes {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(&b, "@prefix %s: <%s> .\n", k, prefixes[k])
	}
	if len(names) > 0 && len(triples) > 0 {
		b.WriteString("\n")
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	return WriteTriples(w, triples, prefixes)
}

// WriteTriples emits the statements without a prefix block, for appending to
// a file that already has one.
func WriteTriples(w io.Writer, triples []Triple, prefixes map[string]string) error {
	var b strings.Builder
	ts := append([]Triple(nil), triples...)
	sortTriples(ts)
	// rdf:type first within a subject
	sort.SliceStable(ts, func(i, j int) bool {
		if cmpTerm(ts[i].S, ts[j].S) != 0 {
			return cmpTerm(ts[i].S, ts[j].S) < 0
		}
		ai, aj := ts[i].P.Value == RDFType, ts[j].P.Value == RDFType
		if ai != aj {
			return ai
		}
		return cmpTriple(ts[i], ts[j]) < 0
	})
	f := func(t Term) string { return FormatTerm(t, prefixes) }
	for i := 0; i < len(ts); {
		s := ts[i].S
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(f(s))
		first := true
		for i < len(ts) && ts[i].S == s {
			pv := ts[i].P
			if first {
				b.WriteString(" ")
			} else {
				b.WriteString(" ;\n    ")
			}
			first = false
			if pv.Value == RDFType {
				b.WriteString("a")
			} else {
				b.WriteString(f(pv))
			}
			firstO := true
			for i < len(ts) && ts[i].S == s && ts[i].P == pv {
				if firstO {
					b.WriteString(" ")
				} else {
					b.WriteString(", ")
				}
				firstO = false
				b.WriteString(f(ts[i].O))
				i++
			}
		}
		b.WriteString(" .\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// FormatTerm renders one term in Turtle, using a prefixed name when it can.
func FormatTerm(t Term, prefixes map[string]string) string {
	switch t.Kind {
	case Blank:
		return "_:" + t.Value
	case IRI:
		best, bestNS := "", ""
		for k, ns := range prefixes {
			if strings.HasPrefix(t.Value, ns) && len(ns) > len(bestNS) && validLocal(t.Value[len(ns):]) {
				best, bestNS = k, ns
			}
		}
		if bestNS != "" {
			return best + ":" + t.Value[len(bestNS):]
		}
		return "<" + t.Value + ">"
	}
	switch t.Datatype {
	case XSD + "integer", XSD + "decimal", XSD + "double", XSD + "boolean":
		if validBare(t.Value, t.Datatype) {
			return t.Value
		}
	}
	s := quote(t.Value)
	if t.Lang != "" {
		return s + "@" + t.Lang
	}
	if t.Datatype != "" {
		return s + "^^" + FormatTerm(I(t.Datatype), prefixes)
	}
	return s
}

func validBare(v, dt string) bool {
	lx := &lexer{src: v, line: 1, col: 1}
	toks, err := lx.all()
	if err != nil || len(toks) != 2 {
		return false
	}
	switch dt {
	case XSD + "boolean":
		return toks[0].kind == tBool
	}
	return toks[0].kind == tNumber
}

func validLocal(s string) bool {
	if s == "" || strings.HasSuffix(s, ".") || strings.HasPrefix(s, "-") || strings.HasPrefix(s, ".") {
		return false
	}
	for _, r := range s {
		if !isNameChar(r) || r == '%' {
			return false
		}
	}
	return true
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
