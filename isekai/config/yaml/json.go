package yaml

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseJSON reads JSON with `//` and `/* */` comments and trailing commas into the same
// node tree Parse builds, so both formats carry file:line origins.
func ParseJSON(src []byte, file string) (*Node, error) {
	j := &jparser{file: file, s: string(src), line: 1}
	if strings.HasPrefix(j.s, "\uFEFF") {
		j.s = j.s[3:]
	}
	if err := j.ws(); err != nil {
		return nil, err
	}
	if j.pos >= len(j.s) {
		return &Node{Kind: Null, File: file, Line: 1}, nil
	}
	n, err := j.value()
	if err != nil {
		return nil, err
	}
	if err := j.ws(); err != nil {
		return nil, err
	}
	if j.pos < len(j.s) {
		return nil, j.err("unexpected %q after the document", j.peekWord())
	}
	return n, nil
}

type jparser struct {
	file string
	s    string
	pos  int
	line int
}

func (j *jparser) err(format string, a ...any) error { return errAt(j.file, j.line, format, a...) }

func (j *jparser) peekWord() string {
	end := j.pos
	for end < len(j.s) && end-j.pos < 12 && j.s[end] != '\n' {
		end++
	}
	return j.s[j.pos:end]
}

func (j *jparser) ws() error {
	for j.pos < len(j.s) {
		c := j.s[j.pos]
		switch {
		case c == '\n':
			j.line++
			j.pos++
		case c == ' ' || c == '\t' || c == '\r':
			j.pos++
		case strings.HasPrefix(j.s[j.pos:], "//"):
			for j.pos < len(j.s) && j.s[j.pos] != '\n' {
				j.pos++
			}
		case strings.HasPrefix(j.s[j.pos:], "/*"):
			end := strings.Index(j.s[j.pos+2:], "*/")
			if end < 0 {
				return j.err("unterminated /* comment")
			}
			j.line += strings.Count(j.s[j.pos:j.pos+2+end], "\n")
			j.pos += end + 4
		default:
			return nil
		}
	}
	return nil
}

func (j *jparser) value() (*Node, error) {
	if err := j.ws(); err != nil {
		return nil, err
	}
	if j.pos >= len(j.s) {
		return nil, j.err("unexpected end of document")
	}
	line := j.line
	switch c := j.s[j.pos]; {
	case c == '{':
		return j.object()
	case c == '[':
		return j.array()
	case c == '"':
		s, err := j.str()
		if err != nil {
			return nil, err
		}
		return &Node{Kind: String, Str: s, File: j.file, Line: line}, nil
	case c == 't' && strings.HasPrefix(j.s[j.pos:], "true"):
		j.pos += 4
		return &Node{Kind: Bool, Bool: true, File: j.file, Line: line}, nil
	case c == 'f' && strings.HasPrefix(j.s[j.pos:], "false"):
		j.pos += 5
		return &Node{Kind: Bool, Bool: false, File: j.file, Line: line}, nil
	case c == 'n' && strings.HasPrefix(j.s[j.pos:], "null"):
		j.pos += 4
		return &Node{Kind: Null, File: j.file, Line: line}, nil
	case c == '-' || c >= '0' && c <= '9':
		return j.number(line)
	}
	return nil, j.err("unexpected %q", j.peekWord())
}

func (j *jparser) number(line int) (*Node, error) {
	start := j.pos
	for j.pos < len(j.s) && strings.IndexByte("+-0123456789.eE", j.s[j.pos]) >= 0 {
		j.pos++
	}
	text := j.s[start:j.pos]
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		return &Node{Kind: Int, Int: i, Str: text, File: j.file, Line: line}, nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, j.err("bad number %q", text)
	}
	return &Node{Kind: Float, Float: f, Str: text, File: j.file, Line: line}, nil
}

func (j *jparser) object() (*Node, error) {
	m := &Node{Kind: Map, File: j.file, Line: j.line}
	j.pos++
	for {
		if err := j.ws(); err != nil {
			return nil, err
		}
		if j.pos >= len(j.s) {
			return nil, j.err("unterminated object")
		}
		if j.s[j.pos] == '}' {
			j.pos++
			return m, nil
		}
		if j.s[j.pos] != '"' {
			return nil, j.err("expected a quoted key, got %q", j.peekWord())
		}
		key, err := j.str()
		if err != nil {
			return nil, err
		}
		if err := j.ws(); err != nil {
			return nil, err
		}
		if j.pos >= len(j.s) || j.s[j.pos] != ':' {
			return nil, j.err("expected `:` after key %q", key)
		}
		j.pos++
		v, err := j.value()
		if err != nil {
			return nil, err
		}
		if m.Get(key) != nil {
			return nil, j.err("duplicate key %q", key)
		}
		m.Keys = append(m.Keys, key)
		m.Vals = append(m.Vals, v)
		if err := j.ws(); err != nil {
			return nil, err
		}
		if j.pos < len(j.s) && j.s[j.pos] == ',' {
			j.pos++
			continue
		}
		if j.pos < len(j.s) && j.s[j.pos] == '}' {
			continue
		}
		if j.pos >= len(j.s) {
			return nil, j.err("unterminated object")
		}
		return nil, j.err("expected `,` or `}` in object")
	}
}

func (j *jparser) array() (*Node, error) {
	l := &Node{Kind: List, File: j.file, Line: j.line}
	j.pos++
	for {
		if err := j.ws(); err != nil {
			return nil, err
		}
		if j.pos >= len(j.s) {
			return nil, j.err("unterminated array")
		}
		if j.s[j.pos] == ']' {
			j.pos++
			return l, nil
		}
		v, err := j.value()
		if err != nil {
			return nil, err
		}
		l.Items = append(l.Items, v)
		if err := j.ws(); err != nil {
			return nil, err
		}
		if j.pos < len(j.s) && j.s[j.pos] == ',' {
			j.pos++
			continue
		}
		if j.pos < len(j.s) && j.s[j.pos] == ']' {
			continue
		}
		if j.pos >= len(j.s) {
			return nil, j.err("unterminated array")
		}
		return nil, j.err("expected `,` or `]` in array")
	}
}

func (j *jparser) str() (string, error) {
	j.pos++ // "
	var b strings.Builder
	for j.pos < len(j.s) {
		c := j.s[j.pos]
		switch c {
		case '"':
			j.pos++
			return b.String(), nil
		case '\n':
			return "", j.err("newline inside a string")
		case '\\':
			j.pos++
			if j.pos >= len(j.s) {
				return "", j.err("unterminated escape")
			}
			switch e := j.s[j.pos]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case '\\', '"', '/':
				b.WriteByte(e)
			case 'u':
				if j.pos+4 >= len(j.s) {
					return "", j.err("short \\u escape")
				}
				v, err := strconv.ParseUint(j.s[j.pos+1:j.pos+5], 16, 32)
				if err != nil {
					return "", j.err("bad \\u escape")
				}
				var buf [4]byte
				n := utf8.EncodeRune(buf[:], rune(v))
				b.Write(buf[:n])
				j.pos += 4
			default:
				return "", j.err("unknown escape \\%c", e)
			}
			j.pos++
		default:
			b.WriteByte(c)
			j.pos++
		}
	}
	return "", j.err("unterminated string")
}
