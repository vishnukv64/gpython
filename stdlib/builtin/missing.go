// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builtin

import (
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const callable_doc = `Return True if the object argument appears callable, False if not.

If this returns True, it is still possible that a call fails, but if it is
False, calling object will never succeed.`

func builtin_callable(self, obj py.Object) (py.Object, error) {
	// A type is callable (it constructs instances), as is anything with a
	// __call__ method or one supplied by its type.
	if _, ok := obj.(*py.Type); ok {
		return py.True, nil
	}
	if _, ok := obj.(py.I__call__); ok {
		return py.True, nil
	}
	if obj.Type().Lookup("__call__") != nil {
		return py.True, nil
	}
	return py.False, nil
}

const id_doc = `Return the identity of an object.

This is guaranteed to be unique among simultaneously existing objects.  It is
derived from the object's address where it has one, and from a counter for the
value types (int, str, float) that have no address of their own.`

// lastID backs `id` for the object shapes that are not pointers.
var lastID int64

func builtin_id(self, obj py.Object) (py.Object, error) {
	if obj == nil {
		return py.Int(0), nil
	}
	v := reflect.ValueOf(obj)
	switch v.Kind() {
	case reflect.Ptr, reflect.UnsafePointer, reflect.Chan, reflect.Map, reflect.Func, reflect.Slice:
		if !v.IsNil() {
			return py.Int(int64(v.Pointer())), nil
		}
	}
	lastID++
	return py.Int(lastID), nil
}

const hash_doc = `Return the hash value of the object (if it has one).

Hash values are integers.  Two objects that compare equal must have the same
hash value.`

func builtin_hash(self, obj py.Object) (py.Object, error) {
	// An explicit __hash__ wins, and a __hash__ of None makes the object
	// unhashable.
	if h, ok := obj.(py.I__hash__); ok {
		return h.M__hash__()
	}

	// The concrete types that have a natural hash, before consulting the
	// type's namespace: the value types do not register __hash__ itself.
	switch v := obj.(type) {
	case py.NoneType:
		return py.Int(0), nil
	case py.Bool:
		if v {
			return py.Int(1), nil
		}
		return py.Int(0), nil
	case py.Int:
		return py.Int(int64(v)), nil
	case py.Float:
		return py.Int(int64(float64(v))), nil
	case py.String:
		// FNV-1a: deterministic, and equal strings hash equal.
		var h uint64 = 14695981039346656037
		for i := 0; i < len(v); i++ {
			h ^= uint64(v[i])
			h *= 1099511628211
		}
		return py.Int(int64(h & (1<<63 - 1))), nil
	}

	if obj.Type().Lookup("__hash__") == nil {
		return nil, py.ExceptionNewf(py.TypeError, "unhashable type: '%s'", obj.Type().Name)
	}
	// A __hash__ supplied by the object's own type is invoked through the
	// normal attribute protocol.
	h, err := py.GetAttrString(obj, "__hash__")
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "unhashable type: '%s'", obj.Type().Name)
	}
	return py.Call(h, py.Tuple{}, py.StringDict{})
}

const issubclass_doc = `Return whether class is a derived class of another class or of any of
the classes in the classinfo tuple.`

func builtin_issubclass(self py.Object, args py.Tuple) (py.Object, error) {
	var cls, classinfo py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "issubclass", 2, 2, &cls, &classinfo); err != nil {
		return nil, err
	}

	class, ok := cls.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "issubclass() arg 1 must be a class")
	}

	if infos, ok := classinfo.(py.Tuple); ok {
		for _, info := range infos {
			parent, ok := info.(*py.Type)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "issubclass() arg 2 must be a class or tuple of classes")
			}
			if class.IsSubtype(parent) {
				return py.True, nil
			}
		}
		return py.False, nil
	}

	parent, ok := classinfo.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "issubclass() arg 2 must be a class or tuple of classes")
	}
	if class.IsSubtype(parent) {
		return py.True, nil
	}
	return py.False, nil
}

const dir_doc = `dir([object]) -> list of strings

If called without an argument, return the names in the current scope.  Else,
return an alphabetized list of names comprising (some of) the attributes of
the given object, and of attributes reachable from it.`

// builtinDir implements dir().  It needs the interpreter frame for the
// no-argument form, so it is reached through InternalMethodDir.
func builtinDir(f *py.Frame, args py.Tuple) (py.Object, error) {
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "dir expected at most 1 argument, got %d", len(args))
	}

	if len(args) == 0 {
		f.FastToLocals()
		names := make([]string, 0, f.Locals.Len())
		for _, name := range f.Locals.Keys() {
			names = append(names, name)
		}
		sort.Strings(names)
		return namesToList(names), nil
	}

	obj := args[0]
	seen := map[string]bool{}

	// Attributes supplied by the type and everything it inherits.
	for t := obj.Type(); t != nil; t = t.Base {
		for _, name := range t.Dict.Keys() {
			seen[name] = true
		}
	}
	// Attributes carried by the object itself.
	if d, ok := obj.(py.IGetDict); ok {
		for _, name := range d.GetDict().Keys() {
			seen[name] = true
		}
	}
	if m, ok := obj.(*py.Module); ok {
		for _, name := range m.Globals.Keys() {
			seen[name] = true
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return namesToList(names), nil
}

func namesToList(names []string) *py.List {
	items := make([]py.Object, len(names))
	for i, name := range names {
		items[i] = py.String(name)
	}
	return py.NewListFromItems(items)
}

const format_doc = `format(value[, format_spec]) -> string

Return value.__format__(format_spec).`

func builtin_format(self py.Object, args py.Tuple) (py.Object, error) {
	var value py.Object
	spec := py.Object(py.String(""))
	if err := py.UnpackTuple(args, py.StringDict{}, "format", 1, 2, &value, &spec); err != nil {
		return nil, err
	}

	if f, ok := value.(py.I__format__); ok {
		return f.M__format__(spec)
	}

	specStr, ok := spec.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "format() argument 2 must be str, not %s", spec.Type().Name)
	}

	switch v := value.(type) {
	case py.String:
		return formatString(string(v), string(specStr))
	case py.Int, py.Bool:
		return formatInt(v, string(specStr))
	case py.Float:
		return formatFloat(v, string(specStr))
	case py.NoneType:
		if specStr == "" {
			return py.String("None"), nil
		}
	}

	if specStr == "" {
		return py.Str(value)
	}
	return nil, py.ExceptionNewf(py.TypeError, "unsupported format string passed to %s.__format__", value.Type().Name)
}

// formatSpec is the parsed "[[fill]align][sign][#][0][width][,][.prec][type]"
// mini-language, keeping only the parts this module implements.
type formatSpec struct {
	fill      rune
	align     byte
	width     int
	comma     bool
	precision int // -1 when absent
	verb      byte
}

func parseFormatSpec(spec string) (formatSpec, error) {
	out := formatSpec{fill: ' ', precision: -1}
	rest := spec

	if len(rest) >= 2 {
		switch rest[1] {
		case '<', '>', '^':
			out.fill = rune(rest[0])
			out.align = rest[1]
			rest = rest[2:]
		}
	}
	if out.align == 0 && len(rest) >= 1 {
		switch rest[0] {
		case '<', '>', '^':
			out.align = rest[0]
			rest = rest[1:]
		}
	}
	if strings.HasPrefix(rest, ",") {
		out.comma = true
		rest = rest[1:]
	}
	if dot := strings.IndexByte(rest, '.'); dot >= 0 {
		prec := rest[dot+1:]
		rest = rest[:dot]
		i := 0
		for i < len(prec) && prec[i] >= '0' && prec[i] <= '9' {
			i++
		}
		if i == 0 {
			return out, py.ExceptionNewf(py.ValueError, "Format specifier missing precision")
		}
		out.precision = 0
		for _, r := range prec[:i] {
			out.precision = out.precision*10 + int(r-'0')
		}
		rest += prec[i:]
	}
	// A leading '0' before the width requests zero padding.
	if out.align == 0 && len(rest) >= 2 && rest[0] == '0' && rest[1] >= '0' && rest[1] <= '9' {
		out.fill = '0'
		rest = rest[1:]
	}
	// width: leading digits
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		out.width = out.width*10 + int(rest[i]-'0')
		i++
	}
	rest = rest[i:]
	if strings.Contains(rest, ",") {
		out.comma = true
		rest = strings.ReplaceAll(rest, ",", "")
	}
	if rest != "" {
		out.verb = rest[len(rest)-1]
		if len(rest) != 1 {
			return out, py.ExceptionNewf(py.ValueError, "Invalid format specifier %q", spec)
		}
	}
	return out, nil
}

func formatString(value, spec string) (py.Object, error) {
	if spec == "" {
		return py.String(value), nil
	}
	f, err := parseFormatSpec(spec)
	if err != nil {
		return nil, err
	}
	if f.verb != 0 && f.verb != 's' {
		return nil, py.ExceptionNewf(py.ValueError, "Unknown format code %q for object of type 'str'", string(f.verb))
	}
	if f.align == 0 {
		f.align = '<' // strings are left aligned by default
	}
	return py.String(pad(value, f)), nil
}

func formatInt(value py.Object, spec string) (py.Object, error) {
	i, err := py.Index(value)
	if err != nil {
		return nil, err
	}

	if spec != "" {
		f, err := parseFormatSpec(spec)
		if err != nil {
			return nil, err
		}
		switch f.verb {
		case 'f', 'F', 'e', 'E', 'g', 'G', '%':
			// CPython lets an int be formatted with the float types.
			return formatFloat(py.Float(i), spec)
		}
		body, err := intBody(py.Int(i), f)
		if err != nil {
			return nil, err
		}
		if f.align == 0 {
			f.align = '>'
		}
		return py.String(pad(body, f)), nil
	}

	return py.Str(value)
}

// intBody renders an integer in the requested base, with separators applied.
func intBody(value py.Object, f formatSpec) (string, error) {
	verb := f.verb
	if verb == 0 {
		verb = 'd'
	}

	var text string
	switch verb {
	case 'd':
		text = fmt.Sprintf("%d", int64(value.(py.Int)))
	case 'b', 'o', 'x', 'X':
		base := 2
		switch verb {
		case 'o':
			base = 8
		case 'x', 'X':
			base = 16
		}
		n, ok := toBigInt(value)
		if !ok {
			return "", py.ExceptionNewf(py.ValueError, "Unknown format code %q for object of type 'int'", string(verb))
		}
		text = n.Text(base)
		if verb == 'X' {
			text = strings.ToUpper(text)
		}
	default:
		return "", py.ExceptionNewf(py.ValueError, "Unknown format code %q for object of type 'int'", string(verb))
	}

	if f.comma && verb == 'd' {
		text = groupThousands(text)
	}
	return text, nil
}

func toBigInt(value py.Object) (*big.Int, bool) {
	switch v := value.(type) {
	case py.Int:
		return big.NewInt(int64(v)), true
	case *py.BigInt:
		return (*big.Int)(v), true
	}
	return nil, false
}

// groupThousands inserts the "," separator every three digits of the integer
// part.
func groupThousands(text string) string {
	neg := strings.HasPrefix(text, "-")
	if neg {
		text = text[1:]
	}
	var out strings.Builder
	for i, r := range text {
		if i > 0 && (len(text)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(r)
	}
	if neg {
		return "-" + out.String()
	}
	return out.String()
}

func formatFloat(value py.Float, spec string) (py.Object, error) {
	if spec == "" {
		return py.Str(value)
	}
	f, err := parseFormatSpec(spec)
	if err != nil {
		return nil, err
	}

	verb := f.verb
	if verb == 0 {
		verb = 'g'
	}

	format := "%"
	if f.precision >= 0 {
		format += "." + itoa(f.precision)
	}
	switch verb {
	case 'f', 'F', 'e', 'E', 'g', 'G':
		format += string(verb)
	case '%':
		format += "%"
	default:
		return nil, py.ExceptionNewf(py.ValueError, "Unknown format code %q for object of type 'float'", string(verb))
	}

	body := fmt.Sprintf(format, float64(value))
	if f.comma {
		if dot := strings.IndexByte(body, '.'); dot >= 0 {
			body = groupThousands(body[:dot]) + body[dot:]
		} else {
			body = groupThousands(body)
		}
	}
	if f.align == 0 {
		f.align = '>'
	}
	return py.String(pad(body, f)), nil
}

// pad applies filling, alignment and width to an already rendered value.
func pad(body string, f formatSpec) string {
	width := len([]rune(body))
	if f.width <= width {
		return body
	}
	gap := f.width - width
	fill := string(f.fill)
	switch f.align {
	case '<':
		return body + strings.Repeat(fill, gap)
	case '^':
		left := gap / 2
		return strings.Repeat(fill, left) + body + strings.Repeat(fill, gap-left)
	default:
		return strings.Repeat(fill, gap) + body
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
