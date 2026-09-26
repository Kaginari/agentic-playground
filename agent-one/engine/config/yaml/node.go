// Package yaml reads config documents into one line-numbered node tree. Two readers share
// the tree: a strict YAML subset (Parse) and JSON with comments (ParseJSON). Anything the
// subset does not cover is an error with file:line, never a silent misparse.
package yaml

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Kind is a node's shape.
type Kind int

const (
	Null Kind = iota
	Bool
	Int
	Float
	String
	Map
	List
)

func (k Kind) String() string {
	return [...]string{"null", "bool", "int", "float", "string", "map", "list"}[k]
}

// Node is one value with the place it was read from.
type Node struct {
	Kind  Kind
	File  string
	Line  int
	End   int     // last source line the node spans (Parse only); 0 when unknown
	Str   string  // String: the text; Int/Float: the literal as written
	Bool  bool    // Bool
	Int   int64   // Int
	Float float64 // Float
	Keys  []string
	Vals  []*Node // Map, parallel to Keys, in document order
	Items []*Node // List
}

// Error is a parse or shape error at a place.
type Error struct {
	File string
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.File == "" {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

func errAt(file string, line int, format string, a ...any) error {
	return &Error{File: file, Line: line, Msg: fmt.Sprintf(format, a...)}
}

// Where is "file:line" for messages.
func (n *Node) Where() string {
	if n == nil {
		return ""
	}
	if n.File == "" {
		return fmt.Sprintf("line %d", n.Line)
	}
	return fmt.Sprintf("%s:%d", n.File, n.Line)
}

// Get returns the map entry or nil.
func (n *Node) Get(key string) *Node {
	if n == nil || n.Kind != Map {
		return nil
	}
	for i, k := range n.Keys {
		if k == key {
			return n.Vals[i]
		}
	}
	return nil
}

// Set adds or replaces a map entry, keeping order.
func (n *Node) Set(key string, v *Node) {
	for i, k := range n.Keys {
		if k == key {
			n.Vals[i] = v
			return
		}
	}
	n.Keys = append(n.Keys, key)
	n.Vals = append(n.Vals, v)
}

// Scalar is true for null, bool, int, float and string.
func (n *Node) Scalar() bool { return n != nil && n.Kind <= String }

// Text renders a scalar the way a person wrote it; containers render as JSON.
func (n *Node) Text() string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case Null:
		return "null"
	case Bool:
		return strconv.FormatBool(n.Bool)
	case Int:
		return strconv.FormatInt(n.Int, 10)
	case Float:
		return strconv.FormatFloat(n.Float, 'g', -1, 64)
	case String:
		return n.Str
	}
	return n.JSON()
}

// Value converts the tree to plain Go values (map[string]any, []any, scalars).
func (n *Node) Value() any {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case Null:
		return nil
	case Bool:
		return n.Bool
	case Int:
		return n.Int
	case Float:
		return n.Float
	case String:
		return n.Str
	case List:
		out := make([]any, len(n.Items))
		for i, it := range n.Items {
			out[i] = it.Value()
		}
		return out
	}
	out := map[string]any{}
	for i, k := range n.Keys {
		out[k] = n.Vals[i].Value()
	}
	return out
}

// JSON renders the tree compactly, keys in document order.
func (n *Node) JSON() string {
	var b strings.Builder
	n.writeJSON(&b, "", "")
	return b.String()
}

// Pretty renders the tree indented, keys sorted.
func (n *Node) Pretty() string {
	var b strings.Builder
	n.writeJSON(&b, "", "  ")
	return b.String()
}

func (n *Node) writeJSON(b *strings.Builder, prefix, indent string) {
	if n == nil {
		b.WriteString("null")
		return
	}
	nl := func() {
		if indent != "" {
			b.WriteString("\n" + prefix + indent)
		}
	}
	switch n.Kind {
	case Null:
		b.WriteString("null")
	case Bool, Int, Float:
		b.WriteString(n.Text())
	case String:
		b.WriteString(strconv.Quote(n.Str))
	case List:
		if len(n.Items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[")
		for i, it := range n.Items {
			if i > 0 {
				b.WriteString(",")
			}
			nl()
			it.writeJSON(b, prefix+indent, indent)
		}
		if indent != "" {
			b.WriteString("\n" + prefix)
		}
		b.WriteString("]")
	case Map:
		if len(n.Keys) == 0 {
			b.WriteString("{}")
			return
		}
		idx := make([]int, len(n.Keys))
		for i := range idx {
			idx[i] = i
		}
		if indent != "" {
			sort.SliceStable(idx, func(a, c int) bool { return n.Keys[idx[a]] < n.Keys[idx[c]] })
		}
		b.WriteString("{")
		for j, i := range idx {
			if j > 0 {
				b.WriteString(",")
			}
			nl()
			b.WriteString(strconv.Quote(n.Keys[i]))
			b.WriteString(":")
			if indent != "" {
				b.WriteString(" ")
			}
			n.Vals[i].writeJSON(b, prefix+indent, indent)
		}
		if indent != "" {
			b.WriteString("\n" + prefix)
		}
		b.WriteString("}")
	}
}

// Equal compares two trees by value and key order-insensitively.
func Equal(a, b *Node) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Null:
		return true
	case Bool:
		return a.Bool == b.Bool
	case Int:
		return a.Int == b.Int
	case Float:
		return a.Float == b.Float
	case String:
		return a.Str == b.Str
	case List:
		if len(a.Items) != len(b.Items) {
			return false
		}
		for i := range a.Items {
			if !Equal(a.Items[i], b.Items[i]) {
				return false
			}
		}
		return true
	}
	if len(a.Keys) != len(b.Keys) {
		return false
	}
	for i, k := range a.Keys {
		if !Equal(a.Vals[i], b.Get(k)) {
			return false
		}
	}
	return true
}

// Scalar constructors, for layers built in code (env, flags).

func StringNode(s, file string, line int) *Node {
	return &Node{Kind: String, Str: s, File: file, Line: line}
}
func BoolNode(v bool, file string, line int) *Node {
	return &Node{Kind: Bool, Bool: v, File: file, Line: line}
}
func IntNode(v int64, file string, line int) *Node {
	return &Node{Kind: Int, Int: v, File: file, Line: line}
}
func MapNode(file string, line int) *Node  { return &Node{Kind: Map, File: file, Line: line} }
func ListNode(file string, line int) *Node { return &Node{Kind: List, File: file, Line: line} }

// Coerce reads a plain scalar text the way the YAML reader types it (bool, int, float, null,
// else string). Env and flag values are typed with it.
func Coerce(s, file string, line int) *Node {
	n := &Node{File: file, Line: line}
	typePlain(n, s)
	return n
}

func typePlain(n *Node, s string) {
	switch s {
	case "", "~", "null", "Null", "NULL":
		n.Kind = Null
		return
	case "true", "True", "TRUE":
		n.Kind, n.Bool = Bool, true
		return
	case "false", "False", "FALSE":
		n.Kind, n.Bool = Bool, false
		return
	case ".inf", ".Inf", ".INF", "+.inf":
		n.Kind, n.Float, n.Str = Float, inf(1), s
		return
	case "-.inf", "-.Inf", "-.INF":
		n.Kind, n.Float, n.Str = Float, inf(-1), s
		return
	case ".nan", ".NaN", ".NAN":
		n.Kind, n.Float, n.Str = Float, nan(), s
		return
	}
	if i, ok := parseInt(s); ok {
		n.Kind, n.Int, n.Str = Int, i, s
		return
	}
	if looksFloat(s) {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(s, "_", ""), 64); err == nil {
			n.Kind, n.Float, n.Str = Float, f, s
			return
		}
	}
	n.Kind, n.Str = String, s
}

func parseInt(s string) (int64, bool) {
	t := s
	neg := false
	if strings.HasPrefix(t, "-") || strings.HasPrefix(t, "+") {
		neg = t[0] == '-'
		t = t[1:]
	}
	if t == "" {
		return 0, false
	}
	base := 10
	switch {
	case strings.HasPrefix(t, "0x"):
		base, t = 16, t[2:]
	case strings.HasPrefix(t, "0o"):
		base, t = 8, t[2:]
	case strings.HasPrefix(t, "0b"):
		base, t = 2, t[2:]
	}
	if t == "" {
		return 0, false
	}
	for _, c := range t {
		if !(c >= '0' && c <= '9' || base == 16 && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')) {
			return 0, false
		}
	}
	v, err := strconv.ParseInt(t, base, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

func looksFloat(s string) bool {
	digit, dot, exp := false, false, false
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case c == '.':
			if dot || exp {
				return false
			}
			dot = true
		case c == 'e' || c == 'E':
			if exp || !digit {
				return false
			}
			exp = true
		case c == '+' || c == '-':
			if i != 0 && !(s[i-1] == 'e' || s[i-1] == 'E') {
				return false
			}
		default:
			return false
		}
	}
	return digit && (dot || exp)
}

func inf(sign int) float64 {
	if sign < 0 {
		return -1 / zero
	}
	return 1 / zero
}

func nan() float64 { return zero / zero }

var zero float64
