package yaml

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Parse reads the strict YAML subset: block maps, block lists, one-line flow `{}`/`[]`,
// plain / 'single' / "double" scalars, bools, ints, floats, null, `#` comments, `|` and `>`
// block scalars. Anchors, aliases, tags, directives, multiple documents, complex keys,
// merge keys and tab indentation are errors with file:line.
func Parse(src []byte, file string) (*Node, error) {
	p := &parser{file: file}
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	if strings.HasPrefix(text, "\uFEFF") {
		text = text[3:]
	}
	p.lines = strings.Split(text, "\n")
	if err := p.prescan(); err != nil {
		return nil, err
	}
	p.skipBlank()
	if p.i >= len(p.lines) {
		return &Node{Kind: Null, File: file, Line: 1}, nil
	}
	n, err := p.block(p.indent(p.i))
	if err != nil {
		return nil, err
	}
	p.skipBlank()
	if p.i < len(p.lines) {
		return nil, p.err(p.i, "unexpected content at indentation %d", p.indent(p.i))
	}
	return n, nil
}

type parser struct {
	file  string
	lines []string
	i     int
}

func (p *parser) err(i int, format string, a ...any) error {
	return errAt(p.file, i+1, format, a...)
}

// prescan rejects what the subset refuses before any structure is read, and drops a single
// leading `---`.
func (p *parser) prescan() error {
	seenContent := false
	for i, l := range p.lines {
		t := strings.TrimRight(l, " \t")
		switch {
		case strings.HasPrefix(t, "%"):
			return p.err(i, "directives are outside the subset")
		case t == "---" || strings.HasPrefix(t, "--- ") || strings.HasPrefix(t, "---\t"):
			if seenContent {
				return p.err(i, "multiple documents are outside the subset")
			}
			if t != "---" {
				return p.err(i, "content on the document marker line is outside the subset")
			}
			p.lines[i] = ""
			seenContent = true
		case t == "...":
			return p.err(i, "document end marker is outside the subset")
		}
		if strings.TrimSpace(stripComment(l)) != "" {
			seenContent = true
		}
	}
	return nil
}

func (p *parser) indent(i int) int {
	n := 0
	for _, c := range p.lines[i] {
		if c != ' ' {
			break
		}
		n++
	}
	return n
}

// content is the line without indentation and trailing comment; "" for blank lines.
func (p *parser) content(i int) string {
	return strings.TrimSpace(stripComment(p.lines[i]))
}

func (p *parser) skipBlank() {
	for p.i < len(p.lines) && p.content(p.i) == "" {
		p.i++
	}
}

func (p *parser) checkTabs(i int) error {
	l := p.lines[i]
	for j := 0; j < len(l); j++ {
		if l[j] == '\t' {
			return p.err(i, "tab in indentation; use spaces")
		}
		if l[j] != ' ' {
			break
		}
	}
	return nil
}

func isListItem(c string) bool { return c == "-" || strings.HasPrefix(c, "- ") }

func (p *parser) block(indent int) (*Node, error) {
	p.skipBlank()
	if p.i >= len(p.lines) {
		return &Node{Kind: Null, File: p.file, Line: p.i}, nil
	}
	if err := p.checkTabs(p.i); err != nil {
		return nil, err
	}
	if p.indent(p.i) != indent {
		return nil, p.err(p.i, "unexpected indentation %d (expected %d)", p.indent(p.i), indent)
	}
	c := p.content(p.i)
	if isListItem(c) {
		return p.list(indent, false)
	}
	if _, _, ok := splitKey(c); ok {
		return p.mapping(indent)
	}
	n, err := p.inline(c, p.i)
	if err != nil {
		return nil, err
	}
	n.End = p.i + 1
	p.i++
	return n, nil
}

func (p *parser) mapping(indent int) (*Node, error) {
	m := &Node{Kind: Map, File: p.file, Line: p.i + 1}
	for {
		p.skipBlank()
		if p.i >= len(p.lines) {
			return m, nil
		}
		if err := p.checkTabs(p.i); err != nil {
			return nil, err
		}
		ind := p.indent(p.i)
		if ind < indent {
			return m, nil
		}
		if ind > indent {
			return nil, p.err(p.i, "unexpected indentation %d (expected %d)", ind, indent)
		}
		c := p.content(p.i)
		if isListItem(c) {
			return nil, p.err(p.i, "list item where a map key was expected")
		}
		key, rest, ok := splitKey(c)
		if !ok {
			return nil, p.err(p.i, "expected `key: value`, got %q", c)
		}
		if m.Get(key) != nil {
			return nil, p.err(p.i, "duplicate key %q", key)
		}
		line := p.i
		v, err := p.value(rest, indent, line)
		if err != nil {
			return nil, err
		}
		m.Keys = append(m.Keys, key)
		m.Vals = append(m.Vals, v)
		m.End = v.End
	}
}

// list reads items at one indentation; loose is set for a list that shares its parent key's
// indentation, where a sibling key ends it instead of being an error.
func (p *parser) list(indent int, loose bool) (*Node, error) {
	l := &Node{Kind: List, File: p.file, Line: p.i + 1}
	for {
		p.skipBlank()
		if p.i >= len(p.lines) {
			return l, nil
		}
		if err := p.checkTabs(p.i); err != nil {
			return nil, err
		}
		ind := p.indent(p.i)
		if ind < indent {
			return l, nil
		}
		if ind > indent {
			return nil, p.err(p.i, "unexpected indentation %d (expected %d)", ind, indent)
		}
		c := p.content(p.i)
		if !isListItem(c) {
			if loose {
				return l, nil
			}
			return nil, p.err(p.i, "expected a `- ` list item, got %q", c)
		}
		rest := strings.TrimSpace(c[1:])
		line := p.i
		var v *Node
		var err error
		switch {
		case rest == "":
			p.i++
			v, err = p.nested(indent, line)
		case isListItem(rest), isMapEntry(rest):
			// The item body starts on this line: re-read it as a block at the item's column.
			col := indent + 1 + (len(c) - 1 - len(strings.TrimLeft(c[1:], " ")))
			p.lines[p.i] = strings.Repeat(" ", col) + rest
			v, err = p.block(col)
		default:
			v, err = p.value(rest, indent, line)
		}
		if err != nil {
			return nil, err
		}
		l.Items = append(l.Items, v)
		l.End = v.End
	}
}

// value reads what follows `key:` or `- ` at a parent indentation.
func (p *parser) value(rest string, indent, line int) (*Node, error) {
	if rest == "" {
		p.i++
		return p.nested(indent, line)
	}
	if isBlockHeader(rest) {
		p.i++
		return p.blockScalar(rest, indent, line)
	}
	if isListItem(rest) {
		return nil, p.err(line, "a list must start on its own line")
	}
	n, err := p.inline(rest, line)
	if err != nil {
		return nil, err
	}
	n.End = line + 1
	p.i++
	return n, nil
}

// nested reads the block after a bare `key:` or `-`: a deeper map, a deeper list, a list at
// the same indentation (lawful under a key), or null.
func (p *parser) nested(indent, line int) (*Node, error) {
	p.skipBlank()
	if p.i >= len(p.lines) {
		return &Node{Kind: Null, File: p.file, Line: line + 1, End: line + 1}, nil
	}
	if err := p.checkTabs(p.i); err != nil {
		return nil, err
	}
	ind := p.indent(p.i)
	c := p.content(p.i)
	var n *Node
	var err error
	switch {
	case ind > indent:
		n, err = p.block(ind)
	case ind == indent && isListItem(c) && !isListItem(p.content(line)):
		n, err = p.list(indent, true)
	default:
		return &Node{Kind: Null, File: p.file, Line: line + 1, End: line + 1}, nil
	}
	if err != nil {
		return nil, err
	}
	n.Line = line + 1 // a container is placed at its key, where a person looks for it
	return n, nil
}

func isBlockHeader(s string) bool {
	if s == "" || (s[0] != '|' && s[0] != '>') {
		return false
	}
	for _, c := range s[1:] {
		if c != '-' && c != '+' && !(c >= '1' && c <= '9') {
			return false
		}
	}
	return true
}

func (p *parser) blockScalar(header string, indent, line int) (*Node, error) {
	fold := header[0] == '>'
	chomp := "clip"
	explicit := 0
	for _, c := range header[1:] {
		switch {
		case c == '-':
			chomp = "strip"
		case c == '+':
			chomp = "keep"
		default:
			explicit = int(c - '0')
		}
	}
	blockIndent := -1
	if explicit > 0 {
		blockIndent = indent + explicit
	}
	var body []string
	end := line + 1
	for p.i < len(p.lines) {
		raw := p.lines[p.i]
		if strings.TrimSpace(raw) == "" {
			body = append(body, "")
			p.i++
			continue
		}
		ind := p.indent(p.i)
		if blockIndent < 0 {
			if ind <= indent {
				break
			}
			blockIndent = ind
		}
		if ind < blockIndent {
			if ind > indent {
				return nil, p.err(p.i, "block scalar line indented less than its first line")
			}
			break
		}
		if err := p.checkTabs(p.i); err != nil {
			return nil, err
		}
		body = append(body, raw[blockIndent:])
		end = p.i + 1
		p.i++
	}
	// Trailing blank lines belong to chomping, not the body.
	trail := 0
	for len(body) > 0 && body[len(body)-1] == "" {
		body = body[:len(body)-1]
		trail++
	}
	var text string
	if fold {
		var b strings.Builder
		prevText := false
		for _, l := range body {
			switch {
			case l == "":
				b.WriteString("\n")
				prevText = false
			case strings.HasPrefix(l, " "):
				if prevText {
					b.WriteString("\n")
				}
				b.WriteString(l)
				b.WriteString("\n")
				prevText = false
			default:
				if prevText {
					b.WriteString(" ")
				}
				b.WriteString(l)
				prevText = true
			}
		}
		text = strings.TrimRight(b.String(), "\n")
	} else {
		text = strings.Join(body, "\n")
	}
	switch chomp {
	case "clip":
		if len(body) > 0 {
			text += "\n"
		}
	case "keep":
		if len(body) > 0 {
			text += "\n"
		}
		text += strings.Repeat("\n", trail)
	}
	return &Node{Kind: String, Str: text, File: p.file, Line: line + 1, End: end}, nil
}

// inline reads a one-line value: flow, quoted or plain.
func (p *parser) inline(s string, line int) (*Node, error) {
	f := &flow{p: p, s: s, line: line}
	n, err := f.value(true)
	if err != nil {
		return nil, err
	}
	f.ws()
	if f.pos < len(f.s) {
		return nil, p.err(line, "unexpected %q after value", f.s[f.pos:])
	}
	return n, nil
}

type flow struct {
	p    *parser
	s    string
	pos  int
	line int
}

func (f *flow) ws() {
	for f.pos < len(f.s) && f.s[f.pos] == ' ' {
		f.pos++
	}
}

func (f *flow) err(format string, a ...any) error { return f.p.err(f.line, format, a...) }

// value reads one value; top is true for the value of a block key, where a plain scalar runs
// to the end of the line.
func (f *flow) value(top bool) (*Node, error) {
	f.ws()
	if f.pos >= len(f.s) {
		return &Node{Kind: Null, File: f.p.file, Line: f.line + 1}, nil
	}
	c := f.s[f.pos]
	switch c {
	case '{':
		return f.flowMap()
	case '[':
		return f.flowList()
	case '"':
		s, err := f.doubleQuoted()
		if err != nil {
			return nil, err
		}
		return &Node{Kind: String, Str: s, File: f.p.file, Line: f.line + 1}, nil
	case '\'':
		s, err := f.singleQuoted()
		if err != nil {
			return nil, err
		}
		return &Node{Kind: String, Str: s, File: f.p.file, Line: f.line + 1}, nil
	case '&':
		return nil, f.err("anchors (&) are outside the subset")
	case '*':
		return nil, f.err("aliases (*) are outside the subset")
	case '!':
		return nil, f.err("tags (!) are outside the subset")
	case '?':
		if f.pos+1 == len(f.s) || f.s[f.pos+1] == ' ' {
			return nil, f.err("complex keys (?) are outside the subset")
		}
	case '@', '`':
		return nil, f.err("reserved indicator %q", string(c))
	case '|', '>':
		if !top {
			return nil, f.err("block scalars are not allowed inside flow collections")
		}
	}
	return f.plain(top)
}

func (f *flow) plain(top bool) (*Node, error) {
	start := f.pos
	for f.pos < len(f.s) {
		c := f.s[f.pos]
		if !top && (c == ',' || c == '}' || c == ']') {
			break
		}
		if c == ':' && (f.pos+1 == len(f.s) || f.s[f.pos+1] == ' ') {
			if top {
				return nil, f.err("a nested mapping needs its own line (`: ` inside a plain value)")
			}
			break
		}
		f.pos++
	}
	text := strings.TrimSpace(f.s[start:f.pos])
	if text == "" && !top {
		return nil, f.err("empty value in flow collection")
	}
	n := &Node{File: f.p.file, Line: f.line + 1}
	typePlain(n, text)
	return n, nil
}

func (f *flow) flowMap() (*Node, error) {
	m := &Node{Kind: Map, File: f.p.file, Line: f.line + 1}
	f.pos++ // {
	for {
		f.ws()
		if f.pos >= len(f.s) {
			return nil, f.err("unterminated `{` (flow collections must close on the same line)")
		}
		if f.s[f.pos] == '}' {
			f.pos++
			return m, nil
		}
		var key string
		switch f.s[f.pos] {
		case '"':
			k, err := f.doubleQuoted()
			if err != nil {
				return nil, err
			}
			key = k
		case '\'':
			k, err := f.singleQuoted()
			if err != nil {
				return nil, err
			}
			key = k
		default:
			kn, err := f.value(false)
			if err != nil {
				return nil, err
			}
			if !kn.Scalar() {
				return nil, f.err("complex keys are outside the subset")
			}
			key = kn.Text()
		}
		f.ws()
		if f.pos >= len(f.s) || f.s[f.pos] != ':' {
			return nil, f.err("expected `:` after key %q", key)
		}
		f.pos++
		if f.pos < len(f.s) && f.s[f.pos] != ' ' && f.s[f.pos] != ',' && f.s[f.pos] != '}' {
			return nil, f.err("expected a space after `:` for key %q", key)
		}
		v, err := f.value(false)
		if err != nil {
			return nil, err
		}
		if m.Get(key) != nil {
			return nil, f.err("duplicate key %q", key)
		}
		m.Keys = append(m.Keys, key)
		m.Vals = append(m.Vals, v)
		f.ws()
		if f.pos < len(f.s) && f.s[f.pos] == ',' {
			f.pos++
			continue
		}
		if f.pos < len(f.s) && f.s[f.pos] == '}' {
			continue
		}
		if f.pos >= len(f.s) {
			return nil, f.err("unterminated `{` (flow collections must close on the same line)")
		}
		return nil, f.err("expected `,` or `}` in flow map")
	}
}

func (f *flow) flowList() (*Node, error) {
	l := &Node{Kind: List, File: f.p.file, Line: f.line + 1}
	f.pos++ // [
	for {
		f.ws()
		if f.pos >= len(f.s) {
			return nil, f.err("unterminated `[` (flow collections must close on the same line)")
		}
		if f.s[f.pos] == ']' {
			f.pos++
			return l, nil
		}
		v, err := f.value(false)
		if err != nil {
			return nil, err
		}
		l.Items = append(l.Items, v)
		f.ws()
		if f.pos < len(f.s) && f.s[f.pos] == ',' {
			f.pos++
			continue
		}
		if f.pos < len(f.s) && f.s[f.pos] == ']' {
			continue
		}
		if f.pos >= len(f.s) {
			return nil, f.err("unterminated `[` (flow collections must close on the same line)")
		}
		return nil, f.err("expected `,` or `]` in flow list")
	}
}

func (f *flow) singleQuoted() (string, error) {
	f.pos++ // '
	var b strings.Builder
	for f.pos < len(f.s) {
		c := f.s[f.pos]
		if c == '\'' {
			if f.pos+1 < len(f.s) && f.s[f.pos+1] == '\'' {
				b.WriteByte('\'')
				f.pos += 2
				continue
			}
			f.pos++
			return b.String(), nil
		}
		b.WriteByte(c)
		f.pos++
	}
	return "", f.err("unterminated single-quoted string")
}

func (f *flow) doubleQuoted() (string, error) {
	f.pos++ // "
	var b strings.Builder
	for f.pos < len(f.s) {
		c := f.s[f.pos]
		switch c {
		case '"':
			f.pos++
			return b.String(), nil
		case '\\':
			f.pos++
			if f.pos >= len(f.s) {
				return "", f.err("unterminated escape")
			}
			e := f.s[f.pos]
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '0':
				b.WriteByte(0)
			case '\\', '"', '/', ' ':
				b.WriteByte(e)
			case 'x', 'u', 'U':
				width := map[byte]int{'x': 2, 'u': 4, 'U': 8}[e]
				if f.pos+width >= len(f.s) {
					return "", f.err("short \\%c escape", e)
				}
				v, err := strconv.ParseUint(f.s[f.pos+1:f.pos+1+width], 16, 32)
				if err != nil {
					return "", f.err("bad \\%c escape", e)
				}
				var buf [4]byte
				n := utf8.EncodeRune(buf[:], rune(v))
				b.Write(buf[:n])
				f.pos += width
			default:
				return "", f.err("unknown escape \\%c", e)
			}
			f.pos++
		default:
			b.WriteByte(c)
			f.pos++
		}
	}
	return "", f.err("unterminated double-quoted string")
}

// splitKey splits `key: rest` at the first `: ` (or trailing `:`) outside quotes.
func splitKey(c string) (key, rest string, ok bool) {
	if c == "" {
		return "", "", false
	}
	switch c[0] {
	case '"', '\'':
		q := c[0]
		i := 1
		for i < len(c) {
			if c[i] == q {
				if q == '\'' && i+1 < len(c) && c[i+1] == '\'' {
					i += 2
					continue
				}
				break
			}
			if q == '"' && c[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(c) {
			return "", "", false
		}
		after := c[i+1:]
		if !strings.HasPrefix(after, ":") || (len(after) > 1 && after[1] != ' ') {
			return "", "", false
		}
		f := &flow{p: &parser{}, s: c}
		var k string
		var err error
		if q == '"' {
			k, err = f.doubleQuoted()
		} else {
			k, err = f.singleQuoted()
		}
		if err != nil {
			return "", "", false
		}
		return k, strings.TrimSpace(after[1:]), true
	case '{', '[', '-', '|', '>', '&', '*', '!', '?', '@', '`', '%':
		if c[0] != '?' {
			return "", "", false
		}
	}
	for i := 0; i < len(c); i++ {
		if c[i] == ':' && (i+1 == len(c) || c[i+1] == ' ') {
			key = strings.TrimSpace(c[:i])
			if key == "" || strings.ContainsAny(key, "{}[],\"'") {
				return "", "", false
			}
			return key, strings.TrimSpace(c[i+1:]), true
		}
	}
	return "", "", false
}

func isMapEntry(rest string) bool {
	_, _, ok := splitKey(rest)
	return ok
}

// stripComment removes a ` #` comment outside quotes; a leading `#` is a whole-line comment.
func stripComment(l string) string {
	inS, inD := false, false
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case inD:
			if c == '\\' {
				i++
			} else if c == '"' {
				inD = false
			}
		case inS:
			if c == '\'' {
				inS = false
			}
		case c == '"' || c == '\'':
			// A quote opens a string only at a value start; inside a plain scalar (it's) it is text.
			if i == 0 || l[i-1] == ' ' || l[i-1] == '[' || l[i-1] == '{' || l[i-1] == ',' {
				inD, inS = c == '"', c == '\''
			}
		case c == '#':
			if i == 0 || l[i-1] == ' ' || l[i-1] == '\t' {
				return l[:i]
			}
		}
	}
	return l
}
