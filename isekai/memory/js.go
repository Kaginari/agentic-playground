package memory

// JS-compatibility helpers: the Go port and memory.js/toolbox.js must produce the same bytes on
// the wire and in the files they share, so the few places where JavaScript's semantics differ
// from Go's (whitespace class, number formatting, toFixed, localeCompare, UTF-16 slicing,
// parseInt) are mirrored here rather than approximated.

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WS is JavaScript's \s class (WhiteSpace + LineTerminator), as a Go regexp class body.
const WS = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

// JSRe compiles a pattern whose `\s` / `\S` mean JavaScript's whitespace class.
func JSRe(pat string) *regexp.Regexp {
	pat = strings.ReplaceAll(pat, `\S`, "[^"+WS+"]")
	pat = strings.ReplaceAll(pat, `\s`, "["+WS+"]")
	return regexp.MustCompile(pat)
}

var reWS = JSRe(`\s+`)

func IsJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func JSTrim(s string) string    { return strings.TrimFunc(s, IsJSSpace) }
func JSTrimEnd(s string) string { return strings.TrimRightFunc(s, IsJSSpace) }
func JSTrimStart(s string) string {
	return strings.TrimLeftFunc(s, IsJSSpace)
}

// CollapseWS is s.replace(/\s+/g, ' ').
func CollapseWS(s string) string { return reWS.ReplaceAllString(s, " ") }

func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// UTF16Slice is s.slice(start, end) in UTF-16 units; a cut inside a surrogate pair drops the rune.
func UTF16Slice(s string, start, end int) string {
	var b strings.Builder
	pos := 0
	for _, r := range s {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if pos >= start && pos+w <= end {
			b.WriteRune(r)
		}
		pos += w
		if pos >= end {
			break
		}
	}
	return b.String()
}

// ReplaceUnits maps every UTF-16 unit not accepted by keep to rep (String.prototype.replace with a
// per-unit class): an astral rune counts as two units.
func ReplaceUnits(s string, keep func(r rune) bool, rep string) string {
	var b strings.Builder
	for _, r := range s {
		if keep(r) {
			b.WriteRune(r)
		} else if r >= 0x10000 {
			b.WriteString(rep + rep)
		} else {
			b.WriteString(rep)
		}
	}
	return b.String()
}

// JSNum is Number.prototype.toString for a double.
func JSNum(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x == 0:
		return "0"
	}
	abs := math.Abs(x)
	if abs < 1e-6 || abs >= 1e21 {
		s := strconv.FormatFloat(x, 'e', -1, 64)
		mant, exp, _ := strings.Cut(s, "e")
		sign, digits := exp[0], strings.TrimLeft(exp[1:], "0")
		if digits == "" {
			digits = "0"
		}
		return mant + "e" + string(sign) + digits
	}
	return strconv.FormatFloat(x, 'f', -1, 64)
}

var half = big.NewRat(1, 2)

// ToFixedStr is Number.prototype.toFixed: exact decimal of the double, ties rounded up.
func ToFixedStr(x float64, d int) string {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) >= 1e21 {
		return JSNum(x)
	}
	neg := x < 0
	r := new(big.Rat).SetFloat64(math.Abs(x))
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	r.Add(r, half)
	n := new(big.Int).Quo(r.Num(), r.Denom()) // floor for a non-negative rational
	s := n.String()
	if d > 0 {
		for len(s) < d+1 {
			s = "0" + s
		}
		s = s[:len(s)-d] + "." + s[len(s)-d:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

// ToFixed is +(x).toFixed(d).
func ToFixed(x float64, d int) float64 {
	v, _ := strconv.ParseFloat(ToFixedStr(x, d), 64)
	if v == 0 {
		return 0
	}
	return v
}

// JSRound is Math.round: nearest, ties toward +Infinity.
func JSRound(x float64) float64 {
	if x != x || math.IsInf(x, 0) || x == math.Trunc(x) {
		return x
	}
	t := math.Floor(x)
	if x-t >= 0.5 {
		return t + 1
	}
	return t
}

// JSParseInt is parseInt(s, 10): a leading decimal prefix, or NaN (ok=false).
func JSParseInt(s string) (float64, bool) {
	s = JSTrimStart(s)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return math.NaN(), false
	}
	v, _ := strconv.ParseFloat(s[:i], 64)
	if neg {
		v = -v
	}
	return v, true
}

var reFloat = regexp.MustCompile(`^[+-]?(Infinity|\d+\.?\d*([eE][+-]?\d+)?|\.\d+([eE][+-]?\d+)?)`)

// JSParseFloat is parseFloat(s): the longest decimal-literal prefix, or NaN (ok=false).
func JSParseFloat(s string) (float64, bool) {
	m := reFloat.FindString(JSTrimStart(s))
	if m == "" {
		return math.NaN(), false
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil && !strings.Contains(err.Error(), "range") {
		return math.NaN(), false
	}
	return v, true
}

// icuPunct is the ICU root order of ASCII punctuation and symbols (variable-weight characters
// come before digits, digits before letters); anything else in that class sorts by code point after.
const icuPunct = "_-,;:!?.'\"()[]{}@*/\\&#%`^+<=>|~$"

// LocaleCompare approximates ICU root collation as Node's String.prototype.localeCompare uses it:
// punctuation < digits < letters on the primary level (case- and accent-insensitive), then
// lowercase before uppercase, then code point.
func LocaleCompare(a, b string) int {
	weight := func(r rune) (int, int) {
		switch {
		case unicode.IsLetter(r):
			return 3, int(unicode.ToLower(r))
		case unicode.IsDigit(r):
			return 2, int(r)
		}
		if i := strings.IndexRune(icuPunct, r); i >= 0 {
			return 1, i
		}
		return 1, len(icuPunct) + int(r)
	}
	ra, rb := []rune(a), []rune(b)
	for i := 0; i < len(ra) && i < len(rb); i++ {
		ca, pa := weight(ra[i])
		cb, pb := weight(rb[i])
		if ca != cb {
			return ca - cb
		}
		if pa != pb {
			return pa - pb
		}
	}
	if len(ra) != len(rb) {
		return len(ra) - len(rb)
	}
	for i := range ra {
		if ra[i] != rb[i] {
			if unicode.IsLower(ra[i]) {
				return -1
			}
			if unicode.IsLower(rb[i]) {
				return 1
			}
			return int(ra[i]) - int(rb[i])
		}
	}
	return 0
}

// JSString is String(v) for a JSON-decoded value; a missing key is passed as (nil, false).
func JSString(v any, present bool) string {
	if !present {
		return "undefined"
	}
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case float64:
		return JSNum(x)
	case bool:
		return strconv.FormatBool(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = JSString(e, true)
		}
		return strings.Join(parts, ",")
	case []string:
		return strings.Join(x, ",")
	case map[string]any:
		return "[object Object]"
	}
	return "[object Object]"
}

func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && x == x
	case string:
		return x != ""
	}
	return true
}

// Locale is Number.prototype.toLocaleString() in the en-US default: grouped, ≤ 3 fraction digits.
func Locale(x float64) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.IsInf(x, 1) {
		return "∞"
	}
	if math.IsInf(x, -1) {
		return "-∞"
	}
	neg := x < 0
	s := strconv.FormatFloat(math.Abs(x), 'f', 3, 64)
	ip, fp, _ := strings.Cut(s, ".")
	fp = strings.TrimRight(fp, "0")
	var b strings.Builder
	for i, c := range ip {
		if i > 0 && (len(ip)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if fp != "" {
		out += "." + fp
	}
	if neg && out != "0" {
		out = "-" + out
	}
	return out
}

// KV / OJ: an ordered JSON object, so an output keeps JSON.stringify's key order and its
// per-kind key presence (a nil value is null; a key left out is undefined).
type KV struct {
	K string
	V any
}
type OJ []KV

// P is one ordered pair.
func P(k string, v any) KV { return KV{k, v} }

func (o OJ) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := MarshalJS(kv.K)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		v, err := MarshalJS(kv.V)
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// MarshalJS is JSON.stringify: compact, no HTML escaping, no trailing newline.
func MarshalJS(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// JSONLine is console.log(JSON.stringify(v)).
func JSONLine(w io.Writer, v any) {
	b, err := MarshalJS(v)
	if err != nil {
		b = []byte(`{"@S":"FAIL","@?":["` + err.Error() + `"]}`)
	}
	w.Write(append(b, '\n'))
}

// Nz turns a nil slice into an empty one so it marshals as [] the way a JS array does.
func Nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ByteLen is Buffer.byteLength.
func ByteLen(s string) int { return len(s) }

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// NameIn is /(^|[^a-z0-9-])NAME(?![a-z0-9-])/i.test(doc): the name as a whole word of the
// [a-z0-9-] alphabet, case-insensitively.
func NameIn(doc, name string) bool {
	d, n := strings.ToLower(doc), strings.ToLower(name)
	if n == "" {
		return false
	}
	from := 0
	for {
		i := strings.Index(d[from:], n)
		if i < 0 {
			return false
		}
		i += from
		okBefore := i == 0 || !isNameChar(d[i-1])
		j := i + len(n)
		okAfter := j >= len(d) || !isNameChar(d[j])
		if okBefore && okAfter {
			return true
		}
		from = i + 1
		if from >= len(d) {
			return false
		}
	}
}

func isNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-'
}

// FirstSentence is /^.*?[.!?](?=\s+[A-Z"“(]|\s*$)/ over the whitespace-collapsed, trimmed text.
func FirstSentence(s string) string {
	s = JSTrim(CollapseWS(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '.' && s[i] != '!' && s[i] != '?' {
			continue
		}
		rest := s[i+1:]
		if rest == "" {
			return s[:i+1]
		}
		if rest[0] == ' ' {
			r, _ := utf8.DecodeRuneInString(strings.TrimLeft(rest, " "))
			if (r >= 'A' && r <= 'Z') || r == '"' || r == '“' || r == '(' {
				return s[:i+1]
			}
		}
	}
	return s
}
