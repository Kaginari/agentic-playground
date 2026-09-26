package yaml

import (
	"fmt"
	"strconv"
	"strings"
)

// Emit renders a tree as block YAML inside the subset Parse reads back.
func Emit(n *Node) string {
	var b strings.Builder
	emit(&b, n, 0, false)
	return b.String()
}

func emit(b *strings.Builder, n *Node, indent int, inList bool) {
	pad := strings.Repeat(" ", indent)
	switch {
	case n == nil || n.Scalar():
		b.WriteString(scalarText(n, indent))
		b.WriteString("\n")
	case n.Kind == List:
		if len(n.Items) == 0 {
			b.WriteString("[]\n")
			return
		}
		if !inList {
			b.WriteString("\n")
		}
		for _, it := range n.Items {
			b.WriteString(pad + "- ")
			if it.Scalar() {
				b.WriteString(scalarText(it, indent+2))
				b.WriteString("\n")
			} else {
				emitInline(b, it, indent+2)
			}
		}
	case n.Kind == Map:
		if len(n.Keys) == 0 {
			b.WriteString("{}\n")
			return
		}
		if !inList {
			b.WriteString("\n")
		}
		for i, k := range n.Keys {
			b.WriteString(pad + keyText(k) + ":")
			v := n.Vals[i]
			if v.Scalar() || (v.Kind == Map && len(v.Keys) == 0) || (v.Kind == List && len(v.Items) == 0) {
				b.WriteString(" ")
				emit(b, v, indent+2, false)
			} else {
				emit(b, v, indent+2, false)
			}
		}
	}
}

// emitInline writes a container whose first line shares the `- ` of a list item.
func emitInline(b *strings.Builder, n *Node, indent int) {
	var inner strings.Builder
	emit(&inner, n, indent, true)
	s := inner.String()
	if len(s) > indent {
		s = s[indent:] // the first line's indentation is taken by "- "
	}
	b.WriteString(s)
}

func keyText(k string) string {
	if needsQuote(k) || strings.HasPrefix(k, "-") {
		return strconv.Quote(k)
	}
	return k
}

func scalarText(n *Node, indent int) string {
	if n == nil || n.Kind == Null {
		return "null"
	}
	if n.Kind != String {
		return n.Text()
	}
	s := n.Str
	if strings.Contains(s, "\n") && !strings.ContainsAny(s, "\t\r") && strings.TrimSpace(s) != "" {
		pad := strings.Repeat(" ", indent)
		body := strings.TrimSuffix(s, "\n")
		head := "|"
		if !strings.HasSuffix(s, "\n") {
			head = "|-"
		} else if strings.HasSuffix(s, "\n\n") {
			head = "|+"
			body = s
		}
		var out strings.Builder
		out.WriteString(head)
		for _, l := range strings.Split(body, "\n") {
			out.WriteString("\n")
			if l != "" {
				out.WriteString(pad + l)
			}
		}
		return out.String()
	}
	if needsQuote(s) {
		return strconv.Quote(s)
	}
	return s
}

// needsQuote is true when a plain scalar would read back as something else or not at all.
func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	probe := &Node{}
	typePlain(probe, s)
	if probe.Kind != String {
		return true
	}
	if strings.ContainsAny(s, "\n\t\r\"'\\") {
		return true
	}
	if strings.ContainsAny(string(s[0]), "?:,[]{}#&*!|>%@`") {
		return true
	}
	if s[0] == '-' && (len(s) == 1 || s[1] == ' ') {
		return true
	}
	if strings.Contains(s, ": ") || strings.HasSuffix(s, ":") || strings.Contains(s, " #") {
		return true
	}
	if s[0] == ' ' || s[len(s)-1] == ' ' {
		return true
	}
	return false
}

// Edit rewrites one path of a YAML source in place, keeping every other line and its
// comments: a scalar at an existing key is replaced on its line, a container is replaced as
// a block, a missing path is inserted under its deepest existing ancestor, a one-line flow
// container is re-rendered as a block, and a nil value deletes the key. The result is
// re-parsed before it is returned.
func Edit(src []byte, file string, path []string, value *Node) ([]byte, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("edit: empty path")
	}
	root, err := Parse(src, file)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	if root.Kind == Null {
		root = &Node{Kind: Map, Line: 1, End: 0}
		lines = lines[:0]
	}
	if root.Kind != Map {
		return nil, fmt.Errorf("edit: %s is not a map document", file)
	}
	// chain[i] is the node at path[:i]; it stops at the deepest existing node.
	chain := []*Node{root}
	for i, k := range path {
		cur := chain[len(chain)-1]
		if cur.Kind != Map {
			return nil, fmt.Errorf("edit: %s: %s is a %s, not a map", file, strings.Join(path[:i], "."), cur.Kind)
		}
		next := cur.Get(k)
		if next == nil {
			break
		}
		chain = append(chain, next)
	}
	found := len(chain) - 1 // how many path segments exist
	if found < len(path) && value == nil {
		return nil, fmt.Errorf("edit: %s: %s does not exist", file, strings.Join(path, "."))
	}
	splice := func(from, to int, block []string) ([]byte, error) {
		out := append([]string{}, lines[:from]...)
		out = append(out, block...)
		out = append(out, lines[to:]...)
		return finish(out, file)
	}
	// A one-line container (flow or empty) anywhere on the chain: edit its tree, re-render it.
	for f := 1; f <= found; f++ {
		n := chain[f]
		if n.Scalar() || n.End != n.Line {
			continue
		}
		apply(n, path[f:], value)
		raw := lines[n.Line-1]
		col := keyColumn(raw, path[f-1])
		block := renderEntry(path[f-1], n, col)
		block[0] = raw[:col] + strings.TrimLeft(block[0], " ")
		return splice(n.Line-1, n.End, block)
	}
	if found == len(path) {
		cur := chain[found]
		raw := lines[cur.Line-1]
		col := keyColumn(raw, path[len(path)-1])
		if value == nil {
			return splice(cur.Line-1, cur.End, nil)
		}
		if cur.Scalar() && value.Scalar() && cur.End == cur.Line {
			content := strings.TrimRight(stripComment(raw), " \t")
			comment := raw[len(content):]
			return splice(cur.Line-1, cur.End, []string{raw[:col] + keyText(path[len(path)-1]) + ": " + scalarText(value, col+2) + comment})
		}
		block := renderEntry(path[len(path)-1], value, col)
		block[0] = raw[:col] + strings.TrimLeft(block[0], " ")
		return splice(cur.Line-1, cur.End, block)
	}
	// Insert the missing tail under the deepest existing map.
	parent := chain[found]
	v := value
	for i := len(path) - 1; i > found; i-- {
		v = v.withKey(path[i])
	}
	childIndent, insertAt := 0, len(lines)
	if parent != root {
		childIndent = keyColumn(lines[parent.Line-1], path[found-1]) + 2
		if len(parent.Keys) > 0 {
			childIndent = keyColumn(lines[parent.Vals[0].Line-1], parent.Keys[0])
		}
		insertAt = parent.End
	} else if len(root.Keys) > 0 {
		insertAt = root.End
	}
	return splice(insertAt, insertAt, renderEntry(path[found], v, childIndent))
}

// apply sets (or deletes, when value is nil) a path inside a tree, creating maps on the way.
func apply(n *Node, tail []string, value *Node) {
	if len(tail) == 0 {
		return
	}
	if len(tail) == 1 {
		if value == nil {
			for i, k := range n.Keys {
				if k == tail[0] {
					n.Keys = append(n.Keys[:i], n.Keys[i+1:]...)
					n.Vals = append(n.Vals[:i], n.Vals[i+1:]...)
					return
				}
			}
			return
		}
		n.Set(tail[0], value)
		return
	}
	next := n.Get(tail[0])
	if next == nil || next.Kind != Map {
		if value == nil {
			return
		}
		next = &Node{Kind: Map}
		n.Set(tail[0], next)
	}
	apply(next, tail[1:], value)
}

func (n *Node) withKey(k string) *Node {
	m := &Node{Kind: Map}
	m.Set(k, n)
	return m
}

func finish(lines []string, file string) ([]byte, error) {
	text := strings.Join(lines, "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if _, err := Parse([]byte(text), file); err != nil {
		return nil, fmt.Errorf("edit produced an unreadable document: %w", err)
	}
	return []byte(text), nil
}

// renderEntry renders `key: value` at an indentation as lines (no trailing empty line).
func renderEntry(key string, v *Node, indent int) []string {
	m := &Node{Kind: Map}
	m.Set(key, v)
	var b strings.Builder
	emit(&b, m, indent, true)
	s := strings.TrimSuffix(b.String(), "\n")
	return strings.Split(s, "\n")
}

// keyColumn finds where a key starts on its raw line (after indentation and any `- `).
func keyColumn(raw, key string) int {
	i := 0
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '-') {
		i++
	}
	for _, cand := range []string{key, strconv.Quote(key), "'" + key + "'"} {
		if strings.HasPrefix(raw[i:], cand+":") {
			return i
		}
	}
	return i
}

// Diff renders a unified diff of two texts, line by line.
func Diff(name string, before, after []byte) string {
	a := strings.Split(strings.TrimSuffix(string(before), "\n"), "\n")
	b := strings.Split(strings.TrimSuffix(string(after), "\n"), "\n")
	if len(before) == 0 {
		a = nil
	}
	if len(after) == 0 {
		b = nil
	}
	// LCS table
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	type op struct {
		kind byte
		text string
	}
	var ops []op
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case j < m && (i == n || lcs[i][j+1] >= lcs[i+1][j]):
			ops = append(ops, op{'+', b[j]})
			j++
		default:
			ops = append(ops, op{'-', a[i]})
			i++
		}
	}
	changed := false
	for _, o := range ops {
		if o.kind != ' ' {
			changed = true
			break
		}
	}
	if !changed {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", name, name)
	const ctx = 2
	k := 0
	for k < len(ops) {
		if ops[k].kind == ' ' {
			k++
			continue
		}
		start := k - ctx
		if start < 0 {
			start = 0
		}
		end := k
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := 0
			for end+run < len(ops) && ops[end+run].kind == ' ' {
				run++
			}
			if run > 2*ctx && end+run < len(ops) {
				end += ctx
				break
			}
			if end+run >= len(ops) {
				end += run
				if end-k > 0 {
					end = min(end, len(ops))
				}
				break
			}
			end += run
		}
		if end > len(ops) {
			end = len(ops)
		}
		// trim trailing context past ctx
		trail := 0
		for t := end - 1; t >= start && ops[t].kind == ' '; t-- {
			trail++
		}
		if trail > ctx {
			end -= trail - ctx
		}
		aStart, bStart, aLen, bLen := 0, 0, 0, 0
		ai, bi := 0, 0
		for t := 0; t < start; t++ {
			if ops[t].kind != '+' {
				ai++
			}
			if ops[t].kind != '-' {
				bi++
			}
		}
		aStart, bStart = ai+1, bi+1
		for t := start; t < end; t++ {
			if ops[t].kind != '+' {
				aLen++
			}
			if ops[t].kind != '-' {
				bLen++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", aStart, aLen, bStart, bLen)
		for t := start; t < end; t++ {
			out.WriteByte(ops[t].kind)
			out.WriteString(ops[t].text)
			out.WriteByte('\n')
		}
		k = end
	}
	return out.String()
}
