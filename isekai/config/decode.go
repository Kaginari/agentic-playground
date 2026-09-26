package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/Kaginari/agentic-playground/isekai/config/yaml"
)

// decoder fills the typed schema from the merged tree. A wrong type is an error; an unknown
// key is a hole (@?), so a file written for a newer binary still loads.
type decoder struct {
	dist    Dist
	home    string
	errs    []error
	unknown []unknownKey
	seen    map[string]bool // dotted paths that carried a value
}

type unknownKey struct {
	path string
	node *yaml.Node
}

func (d *decoder) errf(n *yaml.Node, path, format string, a ...any) {
	d.errs = append(d.errs, fmt.Errorf("%s: %s: %s", n.Where(), path, fmt.Sprintf(format, a...)))
}

var (
	durationType = reflect.TypeOf(Duration(0))
	autoIntType  = reflect.TypeOf(AutoInt{})
	modelRefType = reflect.TypeOf(ModelRef{})
)

func (d *decoder) decode(n *yaml.Node, v reflect.Value, path string) {
	if n == nil || n.Kind == yaml.Null {
		return
	}
	d.seen[path] = true
	switch v.Type() {
	case durationType:
		if n.Kind != yaml.String && n.Kind != yaml.Int && n.Kind != yaml.Float {
			d.errf(n, path, "expected a duration or seconds, got %s", n.Kind)
			return
		}
		var dur Duration
		if err := dur.decode(n.Text(), n.Kind != yaml.String); err != nil {
			d.errf(n, path, "%v", err)
			return
		}
		v.Set(reflect.ValueOf(dur))
		return
	case autoIntType:
		if n.Kind != yaml.String && n.Kind != yaml.Int {
			d.errf(n, path, "expected \"auto\" or a number, got %s", n.Kind)
			return
		}
		var a AutoInt
		if err := a.decode(n.Text(), n.Kind == yaml.Int); err != nil {
			d.errf(n, path, "%v", err)
			return
		}
		v.Set(reflect.ValueOf(a))
		return
	case modelRefType:
		d.decodeModelRef(n, v, path)
		return
	}
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		d.decode(n, v.Elem(), path)
	case reflect.Struct:
		if n.Kind != yaml.Map {
			d.errf(n, path, "expected a map, got %s", n.Kind)
			return
		}
		fields := fieldsOf(v.Type())
		for i, k := range n.Keys {
			f, ok := fields[k]
			if !ok {
				d.unknown = append(d.unknown, unknownKey{join(path, k), n.Vals[i]})
				continue
			}
			d.decode(n.Vals[i], v.Field(f), join(path, k))
		}
	case reflect.Map:
		if n.Kind != yaml.Map {
			d.errf(n, path, "expected a map, got %s", n.Kind)
			return
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
		for i, k := range n.Keys {
			ev := reflect.New(v.Type().Elem()).Elem()
			if old := v.MapIndex(reflect.ValueOf(k)); old.IsValid() {
				ev.Set(old)
			}
			d.decode(n.Vals[i], ev, join(path, k))
			v.SetMapIndex(reflect.ValueOf(k), ev)
		}
	case reflect.Slice:
		if n.Kind == yaml.String && v.Type().Elem().Kind() == reflect.String {
			// `command: "npx"` is one argv element
			v.Set(reflect.ValueOf([]string{d.str(n.Str)}))
			return
		}
		if n.Kind != yaml.List {
			d.errf(n, path, "expected a list, got %s", n.Kind)
			return
		}
		s := reflect.MakeSlice(v.Type(), len(n.Items), len(n.Items))
		for i, it := range n.Items {
			d.decode(it, s.Index(i), fmt.Sprintf("%s[%d]", path, i))
		}
		v.Set(s)
	case reflect.String:
		if n.Kind != yaml.String {
			d.errf(n, path, "expected a string, got %s %s", n.Kind, n.Text())
			return
		}
		v.SetString(d.str(n.Str))
	case reflect.Bool:
		if n.Kind != yaml.Bool {
			d.errf(n, path, "expected true or false, got %s %s", n.Kind, n.Text())
			return
		}
		v.SetBool(n.Bool)
	case reflect.Int, reflect.Int64:
		switch {
		case n.Kind == yaml.Int:
			v.SetInt(n.Int)
		case n.Kind == yaml.Float && n.Float == float64(int64(n.Float)):
			v.SetInt(int64(n.Float))
		default:
			d.errf(n, path, "expected an integer, got %s %s", n.Kind, n.Text())
		}
	case reflect.Float64:
		switch n.Kind {
		case yaml.Int:
			v.SetFloat(float64(n.Int))
		case yaml.Float:
			v.SetFloat(n.Float)
		default:
			d.errf(n, path, "expected a number, got %s %s", n.Kind, n.Text())
		}
	default:
		d.errf(n, path, "unsupported field type %s", v.Type())
	}
}

// decodeModelRef accepts "provider/model" or {model, effort, fallback}.
func (d *decoder) decodeModelRef(n *yaml.Node, v reflect.Value, path string) {
	var m ModelRef
	switch n.Kind {
	case yaml.String:
		m.Model = n.Str
	case yaml.Map:
		for i, k := range n.Keys {
			val := n.Vals[i]
			switch k {
			case "model", "effort", "fallback":
				if val.Kind != yaml.String {
					d.errf(val, join(path, k), "expected a string, got %s", val.Kind)
					continue
				}
				switch k {
				case "model":
					m.Model = val.Str
				case "effort":
					m.Effort = val.Str
				case "fallback":
					m.Fallback = val.Str
				}
			default:
				d.unknown = append(d.unknown, unknownKey{join(path, k), val})
			}
		}
	default:
		d.errf(n, path, "expected \"provider/model\" or {model, effort, fallback}, got %s", n.Kind)
		return
	}
	v.Set(reflect.ValueOf(m))
}

// str substitutes the lexicon placeholders and a leading ~/.
func (d *decoder) str(s string) string {
	s = strings.ReplaceAll(s, "[dist-dir]", d.dist.WorldDir)
	s = strings.ReplaceAll(s, "[dist]", d.dist.Name)
	if strings.HasPrefix(s, "~/") && d.home != "" {
		s = filepath.Join(d.home, s[2:])
	}
	return s
}

func join(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

var fieldCache = map[reflect.Type]map[string]int{}

func fieldsOf(t reflect.Type) map[string]int {
	if m, ok := fieldCache[t]; ok {
		return m
	}
	m := map[string]int{}
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		m[strings.Split(tag, ",")[0]] = i
	}
	fieldCache[t] = m
	return m
}
