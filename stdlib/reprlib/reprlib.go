// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reprlib provides the implementation of python's 'reprlib' module:
// repr() with limits on the size of the result.
//
// A REPL prints the repr of whatever is at the prompt, and repr() of a large
// container can be arbitrarily long or even infinite for a cycle, so reprlib
// caps each collection at a fixed number of elements and each string or
// integer at a fixed number of characters, replacing the rest with a fill
// value.
//
// The module is a port of CPython's Lib/reprlib.py.  One part is deliberately
// not portable: CPython dispatches on the class's __module__ to decide whether
// repr_list and friends apply (so that a user subclass of list gets
// repr_instance instead).  This interpreter's types do not carry __module__,
// so a value is dispatched on its built-in type name alone; the effect is that
// a subclass of list also goes through repr_list.
package reprlib

import (
	"fmt"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Redo the builtin repr() (representation) but with limits on most sizes.

The function repr() is a bound method of a module-level Repr instance, so
reprlib.repr(x) caps x at the default limits.  Build a Repr with different
limits to change them.`

func init() {
	reprType.Dict.Set("maxlevel", intProp(func(self py.Object) *int { return &self.(*Repr).MaxLevel }))
	reprType.Dict.Set("maxtuple", intProp(func(self py.Object) *int { return &self.(*Repr).MaxTuple }))
	reprType.Dict.Set("maxlist", intProp(func(self py.Object) *int { return &self.(*Repr).MaxList }))
	reprType.Dict.Set("maxarray", intProp(func(self py.Object) *int { return &self.(*Repr).MaxArray }))
	reprType.Dict.Set("maxdict", intProp(func(self py.Object) *int { return &self.(*Repr).MaxDict }))
	reprType.Dict.Set("maxset", intProp(func(self py.Object) *int { return &self.(*Repr).MaxSet }))
	reprType.Dict.Set("maxfrozenset", intProp(func(self py.Object) *int { return &self.(*Repr).MaxFrozenSet }))
	reprType.Dict.Set("maxdeque", intProp(func(self py.Object) *int { return &self.(*Repr).MaxDeque }))
	reprType.Dict.Set("maxstring", intProp(func(self py.Object) *int { return &self.(*Repr).MaxString }))
	reprType.Dict.Set("maxlong", intProp(func(self py.Object) *int { return &self.(*Repr).MaxLong }))
	reprType.Dict.Set("maxother", intProp(func(self py.Object) *int { return &self.(*Repr).MaxOther }))
	reprType.Dict.Set("fillvalue", strProp(func(self py.Object) *string { return &self.(*Repr).FillValue }))
	reprType.Dict.Set("indent", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return self.(*Repr).Indent, nil
		},
		Fset: func(self py.Object, value py.Object) error {
			self.(*Repr).Indent = value
			return nil
		},
	})

	setMethod("repr", func(self py.Object, args py.Tuple) (py.Object, error) {
		var x py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "repr", 1, 1, &x); err != nil {
			return nil, err
		}
		return self.(*Repr).repr1(x, self.(*Repr).MaxLevel)
	}, "repr(x) -- cap the repr of x at this Repr's limits.")
	setMethod("repr1", func(self py.Object, args py.Tuple) (py.Object, error) {
		var x py.Object
		var levelObj py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "repr1", 2, 2, &x, &levelObj); err != nil {
			return nil, err
		}
		level, err := toInt(levelObj)
		if err != nil {
			return nil, err
		}
		return self.(*Repr).repr1(x, level)
	}, "repr1(x, level) -- like repr(x) but with an explicit recursion level.")
	setMethod("repr_instance", func(self py.Object, args py.Tuple) (py.Object, error) {
		var x py.Object
		var levelObj py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "repr_instance", 2, 2, &x, &levelObj); err != nil {
			return nil, err
		}
		return self.(*Repr).reprInstance(x)
	}, "repr_instance(x, level) -- repr of an object with no special handling.")

	globals := py.NewStringDict()
	globals.Set("Repr", reprType)
	aRepr := &Repr{
		MaxLevel: 6, MaxTuple: 6, MaxList: 6, MaxArray: 5, MaxDict: 4,
		MaxSet: 6, MaxFrozenSet: 6, MaxDeque: 6, MaxString: 30, MaxLong: 40,
		MaxOther: 30, FillValue: "...", Indent: py.None,
	}
	globals.Set("aRepr", aRepr)
	globals.Set("repr", py.MustNewMethod("repr", func(self py.Object, args py.Tuple) (py.Object, error) {
		var x py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "repr", 1, 1, &x); err != nil {
			return nil, err
		}
		return aRepr.repr1(x, aRepr.MaxLevel)
	}, 0, "repr(x) -- a Repr with the default limits."))
	globals.Set("recursive_repr", py.MustNewMethod("recursive_repr", recursiveRepr, 0, recursiveRepr_doc))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "reprlib",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

func setMethod(name string, fn interface{}, doc string) {
	reprType.Dict.Set(name, py.MustNewMethod(name, fn, 0, doc))
}

const reprClass_doc = `Repr() -- a repr() with configurable limits.

Instances carry maxlevel, maxtuple, maxlist, maxarray, maxdict, maxset,
maxfrozenset, maxdeque, maxstring, maxlong, maxother, fillvalue and indent;
repr(x) renders x using them.`

// Repr is one configured repr().
type Repr struct {
	MaxLevel, MaxTuple, MaxList, MaxArray, MaxDict int
	MaxSet, MaxFrozenSet, MaxDeque                 int
	MaxString, MaxLong, MaxOther                   int
	FillValue                                      string
	// Indent is None, an int number of spaces, or a string.  It is stored as
	// the Python object so that all three forms survive.
	Indent py.Object
}

var reprType = py.NewTypeX("reprlib.Repr", reprClass_doc, reprNew, nil)

func (r *Repr) Type() *py.Type { return reprType }

func reprNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	r := &Repr{
		MaxLevel: 6, MaxTuple: 6, MaxList: 6, MaxArray: 5, MaxDict: 4,
		MaxSet: 6, MaxFrozenSet: 6, MaxDeque: 6, MaxString: 30, MaxLong: 40,
		MaxOther: 30, FillValue: "...", Indent: py.None,
	}
	if len(args) != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "Repr() takes no positional arguments")
	}
	setInt := func(key string, dst *int) error {
		if v, ok := kwargs.Get(key); ok {
			n, err := toInt(v)
			if err != nil {
				return err
			}
			*dst = n
		}
		return nil
	}
	for _, kv := range []struct {
		key string
		dst *int
	}{
		{"maxlevel", &r.MaxLevel}, {"maxtuple", &r.MaxTuple},
		{"maxlist", &r.MaxList}, {"maxarray", &r.MaxArray},
		{"maxdict", &r.MaxDict}, {"maxset", &r.MaxSet},
		{"maxfrozenset", &r.MaxFrozenSet}, {"maxdeque", &r.MaxDeque},
		{"maxstring", &r.MaxString}, {"maxlong", &r.MaxLong},
		{"maxother", &r.MaxOther},
	} {
		if err := setInt(kv.key, kv.dst); err != nil {
			return nil, err
		}
	}
	if v, ok := kwargs.Get("fillvalue"); ok {
		s, err := py.StrAsString(v)
		if err != nil {
			return nil, err
		}
		r.FillValue = s
	}
	if v, ok := kwargs.Get("indent"); ok {
		r.Indent = v
	}
	return r, nil
}

// repr1 dispatches on the value's type, mirroring Repr.repr1.
func (r *Repr) repr1(x py.Object, level int) (py.Object, error) {
	switch x.Type().Name {
	case "tuple":
		return r.reprIterable(x, level, "(", ")", r.MaxTuple, ",")
	case "list":
		return r.reprIterable(x, level, "[", "]", r.MaxList, "")
	case "set":
		return r.reprSetLike(x, level, "set()", "{", "}", r.MaxSet)
	case "frozenset":
		return r.reprSetLike(x, level, "frozenset()", "frozenset({", "})", r.MaxFrozenSet)
	case "dict":
		return r.reprDict(x, level)
	case "str":
		return r.reprString(x)
	case "int", "bigint":
		return r.reprInt(x)
	case "bool", "NoneType":
		// CPython routes bool and None through repr_instance; they have no
		// repr_bool or repr_None, so the result is the same either way.
		return r.reprInstance(x)
	case "array":
		return r.reprArray(x, level)
	}
	// deque is not a distinct Go type here; collections.deque is an instance
	// whose repr_instance output is already bounded by maxother.
	return r.reprInstance(x)
}

func (r *Repr) reprArray(x py.Object, level int) (py.Object, error) {
	typecode, err := py.GetAttrString(x, "typecode")
	if err != nil {
		return r.reprInstance(x)
	}
	tc, err := py.StrAsString(typecode)
	if err != nil {
		return r.reprInstance(x)
	}
	n, err := lengthOf(x)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return py.String(fmt.Sprintf("array('%s')", tc)), nil
	}
	header := fmt.Sprintf("array('%s', [", tc)
	return r.reprIterable(x, level, header, "])", r.MaxArray, "")
}

func (r *Repr) reprSetLike(x py.Object, level int, empty, left, right string, maxIter int) (py.Object, error) {
	n, err := lengthOf(x)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return py.String(empty), nil
	}
	items, err := possiblySorted(x)
	if err != nil {
		return nil, err
	}
	return r.reprIterableItems(items, level, left, right, maxIter, "")
}

func (r *Repr) reprDict(x py.Object, level int) (py.Object, error) {
	n, err := lengthOf(x)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return py.String("{}"), nil
	}
	if level <= 0 {
		return py.String("{" + r.FillValue + "}"), nil
	}
	keys, err := possiblySortedKeys(x)
	if err != nil {
		return nil, err
	}
	newLevel := level - 1
	pieces := []string{}
	for i, key := range keys {
		if i >= r.MaxDict {
			break
		}
		val, err := py.GetItem(x, key)
		if err != nil {
			return nil, err
		}
		keyRepr, err := r.reprAsString(key, newLevel)
		if err != nil {
			return nil, err
		}
		valRepr, err := r.reprAsString(val, newLevel)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, keyRepr+": "+valRepr)
	}
	if n > r.MaxDict {
		pieces = append(pieces, r.FillValue)
	}
	joined, err := r.join(pieces, level)
	if err != nil {
		return nil, err
	}
	return py.String("{" + joined + "}"), nil
}

func (r *Repr) reprIterable(x py.Object, level int, left, right string, maxIter int, trail string) (py.Object, error) {
	n, err := lengthOf(x)
	if err != nil {
		return nil, err
	}
	var s string
	if level <= 0 && n > 0 {
		s = r.FillValue
	} else {
		newLevel := level - 1
		pieces := []string{}
		for i := 0; i < n && i < maxIter; i++ {
			elem, err := py.GetItem(x, py.Int(i))
			if err != nil {
				return nil, err
			}
			rep, err := r.reprAsString(elem, newLevel)
			if err != nil {
				return nil, err
			}
			pieces = append(pieces, rep)
		}
		if n > maxIter {
			pieces = append(pieces, r.FillValue)
		}
		var err error
		s, err = r.join(pieces, level)
		if err != nil {
			return nil, err
		}
		if n == 1 && trail != "" && r.Indent == py.None {
			right = trail + right
		}
	}
	return py.String(left + s + right), nil
}

func (r *Repr) reprIterableItems(items []py.Object, level int, left, right string, maxIter int, trail string) (py.Object, error) {
	n := len(items)
	var s string
	if level <= 0 && n > 0 {
		s = r.FillValue
	} else {
		newLevel := level - 1
		pieces := []string{}
		for i := 0; i < n && i < maxIter; i++ {
			rep, err := r.reprAsString(items[i], newLevel)
			if err != nil {
				return nil, err
			}
			pieces = append(pieces, rep)
		}
		if n > maxIter {
			pieces = append(pieces, r.FillValue)
		}
		var err error
		s, err = r.join(pieces, level)
		if err != nil {
			return nil, err
		}
		if n == 1 && trail != "" && r.Indent == py.None {
			right = trail + right
		}
	}
	return py.String(left + s + right), nil
}

// join is Repr._join: the separator is ", " normally, or a newline plus an
// indent derived from the level when indent is set.
func (r *Repr) join(pieces []string, level int) (string, error) {
	if r.Indent == py.None {
		return strings.Join(pieces, ", "), nil
	}
	if len(pieces) == 0 {
		return "", nil
	}
	indentText := ""
	switch v := r.Indent.(type) {
	case py.Int:
		if v < 0 {
			return "", py.ExceptionNewf(py.ValueError, "Repr.indent cannot be negative int (was %d)", int(v))
		}
		indentText = strings.Repeat(" ", int(v))
	case py.String:
		indentText = string(v)
	default:
		return "", py.ExceptionNewf(py.TypeError, "Repr.indent must be a str, int or None, not %s", r.Indent.Type().Name)
	}
	depth := r.MaxLevel - level + 1
	if depth < 0 {
		depth = 0
	}
	sep := ",\n" + strings.Repeat(indentText, depth)
	joined := sep + strings.Join(pieces, sep) + sep
	// CPython's `sep.join(('', *pieces, ''))[1:-len(indent) or None]`: the
	// leading separator character and the trailing indent are dropped.
	if len(indentText) == 0 {
		return joined[1:], nil
	}
	if len(joined) < 1+len(indentText) {
		return joined[1:], nil
	}
	return joined[1 : len(joined)-len(indentText)], nil
}

// reprString is Repr.repr_str: it reprs the head of the string, and if that is
// too long it reprs head and tail with the fill value between them.
func (r *Repr) reprString(x py.Object) (py.Object, error) {
	s, err := py.StrAsString(x)
	if err != nil {
		return nil, err
	}
	head := s
	if len(head) > r.MaxString {
		head = head[:r.MaxString]
	}
	quoted, err := pyReprString(head)
	if err != nil {
		return nil, err
	}
	if len(quoted) > r.MaxString {
		i := maxInt(0, (r.MaxString-3)/2)
		j := maxInt(0, r.MaxString-3-i)
		tailStart := len(s) - j
		if tailStart < i {
			tailStart = i
		}
		inner := s[:i] + s[tailStart:]
		quoted, err = pyReprString(inner)
		if err != nil {
			return nil, err
		}
		// Splice the fill value into the already-quoted string so the quotes
		// stay around the whole thing.
		if i > len(quoted) {
			i = len(quoted)
		}
		if len(quoted)-j < i {
			j = len(quoted) - i
		}
		quoted = quoted[:i] + r.FillValue + quoted[len(quoted)-j:]
	}
	return py.String(quoted), nil
}

// reprInt is Repr.repr_int: a long integer is truncated around the middle.
func (r *Repr) reprInt(x py.Object) (py.Object, error) {
	s, err := py.ReprAsString(x)
	if err != nil {
		return nil, err
	}
	if len(s) > r.MaxLong {
		i := maxInt(0, (r.MaxLong-3)/2)
		j := maxInt(0, r.MaxLong-3-i)
		s = s[:i] + r.FillValue + s[len(s)-j:]
	}
	return py.String(s), nil
}

// reprInstance is Repr.repr_instance: repr(x), truncated around the middle.
func (r *Repr) reprInstance(x py.Object) (py.Object, error) {
	s, err := py.ReprAsString(x)
	if err != nil {
		return py.String(fmt.Sprintf("<%s instance at %p>", x.Type().Name, x)), nil
	}
	if len(s) > r.MaxOther {
		i := maxInt(0, (r.MaxOther-3)/2)
		j := maxInt(0, r.MaxOther-3-i)
		s = s[:i] + r.FillValue + s[len(s)-j:]
	}
	return py.String(s), nil
}

// reprAsString is repr1 with the result converted to a Go string.
func (r *Repr) reprAsString(x py.Object, level int) (string, error) {
	res, err := r.repr1(x, level)
	if err != nil {
		return "", err
	}
	return py.StrAsString(res)
}

// ---------------------------------------------------------------------------
// helpers

func lengthOf(x py.Object) (int, error) {
	res, err := py.Len(x)
	if err != nil {
		return 0, err
	}
	return toInt(res)
}

// possiblySorted is reprlib._possibly_sorted: sorted(x) if it can be sorted,
// otherwise the items in their own order.
func possiblySorted(x py.Object) ([]py.Object, error) {
	items, err := sequenceItems(x)
	if err != nil {
		return nil, err
	}
	l := py.NewListFromItems(items)
	if err := py.SortInPlace(l, py.StringDict{}, "sorted"); err != nil {
		// A set whose elements are not mutually comparable keeps its own
		// order, exactly as CPython's except-Exception fallback does.
		return items, nil
	}
	return l.Items, nil
}

func possiblySortedKeys(x py.Object) ([]py.Object, error) {
	keys, err := py.SequenceList(x)
	if err != nil {
		return nil, err
	}
	l := py.NewListFromItems(keys.Items)
	if err := py.SortInPlace(l, py.StringDict{}, "sorted"); err != nil {
		return keys.Items, nil
	}
	return l.Items, nil
}

func sequenceItems(x py.Object) ([]py.Object, error) {
	l, err := py.SequenceList(x)
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func intProp(ptr func(py.Object) *int) *py.Property {
	return &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.Int(*ptr(self)), nil
		},
		Fset: func(self py.Object, value py.Object) error {
			n, err := toInt(value)
			if err != nil {
				return err
			}
			*ptr(self) = n
			return nil
		},
	}
}

func strProp(ptr func(py.Object) *string) *py.Property {
	return &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.String(*ptr(self)), nil
		},
		Fset: func(self py.Object, value py.Object) error {
			s, err := py.StrAsString(value)
			if err != nil {
				return err
			}
			*ptr(self) = s
			return nil
		},
	}
}

func toInt(o py.Object) (int, error) {
	switch v := o.(type) {
	case py.Int:
		return int(v), nil
	case py.Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case *py.BigInt:
		return v.GoInt()
	}
	return 0, py.ExceptionNewf(py.TypeError, "an integer is required")
}

// pyReprString formats s as CPython repr() does for a str: single quotes
// preferred, doubled or backslashed as needed.
func pyReprString(s string) (string, error) {
	res, err := py.ReprAsString(py.String(s))
	if err != nil {
		return "", err
	}
	return res, nil
}

const recursiveRepr_doc = `recursive_repr(fillvalue='...')

Decorator to make a repr function return fillvalue for a recursive call.`

// recursiveRepr is CPython's recursive_repr.  It is called with the fill value
// and returns the decorating function, which is then applied to the user's
// repr function.
func recursiveRepr(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	fill := py.Object(py.String("..."))
	if len(args) > 0 {
		fill = args[0]
	}
	if v, ok := kwargs.Get("fillvalue"); ok {
		fill = v
	}
	return &recursiveReprDecorator{fill: fill}, nil
}

// recursiveReprDecorator is the object recursive_repr() returns; calling it
// with the user's function produces the guard wrapper.
type recursiveReprDecorator struct {
	fill py.Object
}

var recursiveReprDecoratorType = py.NewType("reprlib.recursive_repr.decorator", "The decorator returned by reprlib.recursive_repr().")

func (d *recursiveReprDecorator) Type() *py.Type { return recursiveReprDecoratorType }

func (d *recursiveReprDecorator) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "recursive_repr() must be applied to exactly one function")
	}
	return &recursiveReprWrapper{fill: d.fill, fn: args[0]}, nil
}

// recursiveReprWrapper is the object the decorator returns.
type recursiveReprWrapper struct {
	fn   py.Object
	fill py.Object
}

var recursiveReprWrapperType = py.NewType("reprlib.recursive_repr", "A repr function that returns a fill value on a recursive call.")

func (w *recursiveReprWrapper) Type() *py.Type { return recursiveReprWrapperType }

// running holds the objects currently inside a repr, keyed by identity.  A
// recursive repr is exactly the re-entry this guards, and it is a decorator on
// a single function's call stack, so process-global is the right scope.
var reprRunning = map[py.Object]bool{}

func (w *recursiveReprWrapper) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "recursive_repr() wrapper expects an argument")
	}
	key := args[0]
	if reprRunning[key] {
		return w.fill, nil
	}
	reprRunning[key] = true
	defer delete(reprRunning, key)
	return py.Call(w.fn, args, kwargs)
}

// M__get__ makes the wrapper work as a method descriptor.
func (w *recursiveReprWrapper) M__get__(instance, owner py.Object) (py.Object, error) {
	if instance == nil || instance == py.None {
		return w, nil
	}
	return &boundRecursiveRepr{wrapper: w, self: instance}, nil
}

type boundRecursiveRepr struct {
	wrapper *recursiveReprWrapper
	self    py.Object
}

func (b *boundRecursiveRepr) Type() *py.Type { return recursiveReprWrapperType }

func (b *boundRecursiveRepr) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	full := append(py.Tuple{b.self}, args...)
	return b.wrapper.M__call__(full, kwargs)
}
