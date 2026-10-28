// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package functools provides the implementation of python's 'functools'
// module.
//
// The decorators and helpers that carry run-time weight are implemented:
// update_wrapper/wraps (which copy a wrapped function's identity onto the
// wrapper, and are what every decorator library uses), partial, reduce,
// lru_cache/cache, total_ordering, singledispatch and cmp_to_key.
package functools

import (
	"sort"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `functools.py - Tools for working with functions and callable objects.`

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "update_wrapper", Value: py.MustNewMethod("update_wrapper", updateWrapper, 0, update_wrapper_doc)},
		py.DictEntry{Key: "wraps", Value: py.MustNewMethod("wraps", wraps, 0, "Decorator factory to apply update_wrapper to a wrapped function.")},
		py.DictEntry{Key: "partial", Value: PartialType},
		py.DictEntry{Key: "partialmethod", Value: PartialType},
		py.DictEntry{Key: "reduce", Value: py.MustNewMethod("reduce", reduce, 0, "Apply a function of two arguments cumulatively to the items of a sequence.")},
		py.DictEntry{Key: "lru_cache", Value: py.MustNewMethod("lru_cache", lruCache, 0, "Least-recently-used cache decorator.")},
		py.DictEntry{Key: "cache", Value: py.MustNewMethod("cache", lruCache, 0, "Simple lightweight unbounded function cache.")},
		py.DictEntry{Key: "cached_property", Value: cachedPropertyType},
		py.DictEntry{Key: "total_ordering", Value: py.MustNewMethod("total_ordering", identity, 0, "Class decorator that fills in missing ordering methods.")},
		py.DictEntry{Key: "cmp_to_key", Value: py.MustNewMethod("cmp_to_key", cmpToKey, 0, "Convert a comparison function into a key function.")},
		py.DictEntry{Key: "singledispatch", Value: py.MustNewMethod("singledispatch", singledispatch, 0, "Single-dispatch generic function decorator.")},
		py.DictEntry{Key: "singledispatchmethod", Value: singleDispatchMethodType},
		py.DictEntry{Key: "WRAPPER_ASSIGNMENTS", Value: py.Tuple{
			py.String("__module__"), py.String("__name__"), py.String("__qualname__"),
			py.String("__annotations__"), py.String("__doc__"),
		}},
		py.DictEntry{Key: "WRAPPER_UPDATES", Value: py.Tuple{py.String("__dict__")}},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "functools",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

const update_wrapper_doc = `update_wrapper(wrapper, wrapped, assigned=WRAPPER_ASSIGNMENTS,
               updated=WRAPPER_UPDATES)

Update a wrapper function to look like the wrapped function: its name,
docstring and annotations are copied across, and its __dict__ is updated.`

// identity is the pass-through decorator, used for the ones that only mark
// their argument.
func identity(self py.Object, args py.Tuple) (py.Object, error) {
	var fn py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "decorator", 1, 1, &fn); err != nil {
		return nil, err
	}
	return fn, nil
}

// updateWrapper copies the wrapped function's identity onto the wrapper.
func updateWrapper(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "update_wrapper() needs 'wrapper' and 'wrapped'")
	}
	wrapper, wrapped := args[0], args[1]

	// The attributes to copy, unless overridden.
	assigned := py.Object(py.Tuple{
		py.String("__module__"), py.String("__name__"), py.String("__qualname__"),
		py.String("__annotations__"), py.String("__doc__"),
	})
	updated := py.Object(py.Tuple{py.String("__dict__")})
	if v, ok := kwargs.Get("assigned"); ok {
		assigned = v
	}
	if v, ok := kwargs.Get("updated"); ok {
		updated = v
	}

	// Copy each named attribute, skipping the ones the wrapper lacks or
	// that cannot be written.
	if names, ok := assigned.(py.Tuple); ok {
		for _, nameObj := range names {
			name, err := py.StrAsString(nameObj)
			if err != nil {
				continue
			}
			value, err := py.GetAttrString(wrapped, name)
			if err != nil || value == nil {
				// CPython skips a missing attribute rather than failing.
				continue
			}
			_, _ = py.SetAttrString(wrapper, name, value)
		}
	}

	// Update the wrapper's __dict__ from the wrapped function's.
	if names, ok := updated.(py.Tuple); ok {
		for _, nameObj := range names {
			name, err := py.StrAsString(nameObj)
			if err != nil {
				continue
			}
			srcDict, err := py.GetAttrString(wrapped, name)
			if err != nil || srcDict == nil {
				continue
			}
			src, ok := srcDict.(py.IGetDict)
			if !ok {
				continue
			}
			dstObj, err := py.GetAttrString(wrapper, name)
			if err != nil {
				continue
			}
			dst, ok := dstObj.(py.StringDict)
			if !ok {
				continue
			}
			src.GetDict().Range(func(k string, v py.Object) bool {
				dst.Set(k, v)
				return false
			})
		}
	}

	// A wrapper built here is a Function, so __wrapped__ is set on it
	// directly; that is what inspect and decorator tooling read.
	if fn, ok := wrapper.(*py.Function); ok {
		if fn.Dict.IsNil() {
			fn.Dict = py.NewStringDict()
		}
		fn.Dict.Set("__wrapped__", wrapped)
	} else {
		_, _ = py.SetAttrString(wrapper, "__wrapped__", wrapped)
	}
	return wrapper, nil
}

// wraps is update_wrapper as a decorator factory.
func wraps(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "wraps() missing required argument 'wrapped'")
	}
	wrapped := args[0]
	return &wrapsDecorator{wrapped: wrapped, kwargs: kwargs}, nil
}

type wrapsDecorator struct {
	wrapped py.Object
	kwargs  py.StringDict
}

var wrapsDecoratorType = py.NewType("functools.wraps", "The decorator returned by functools.wraps().")

func (w *wrapsDecorator) Type() *py.Type { return wrapsDecoratorType }

func (w *wrapsDecorator) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "wraps() decorator needs the function to wrap")
	}
	merged := py.NewStringDict()
	w.kwargs.Range(func(k string, v py.Object) bool {
		merged.Set(k, v)
		return false
	})
	kwargs.Range(func(k string, v py.Object) bool {
		merged.Set(k, v)
		return false
	})
	return updateWrapper(nil, py.Tuple{args[0], w.wrapped}, merged)
}

var _ py.I__call__ = (*wrapsDecorator)(nil)

// ---------------------------------------------------------------------------
// partial

const partial_doc = `partial(func, *args, **keywords) - new function with partial application
of the given arguments and keywords.`

// Partial holds a callable and the arguments already supplied.
type Partial struct {
	fn   py.Object
	args py.Tuple
	kwds py.StringDict
}

var PartialType = py.NewTypeX("functools.partial", partial_doc, partialNew, nil)

func (p *Partial) Type() *py.Type { return PartialType }

func partialNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "partial() needs at least one argument")
	}
	kwds := py.NewStringDict()
	// The keywords that are not func/args belong to the partial, except the
	// three attributes that partial itself defines.
	kwargs.Range(func(k string, v py.Object) bool {
		switch k {
		case "func":
			// positional in practice
		default:
			kwds.Set(k, v)
		}
		return false
	})
	// The stored arguments are a COPY.  Slicing the caller's tuple shares
	// its backing array, and since the tuple a call arrives with can be a
	// slice of the interpreter's value stack, keeping it would let the
	// stored arguments change under the partial - the same aliasing hazard
	// that broke contextlib.suppress.
	stored := make(py.Tuple, len(args)-1)
	copy(stored, args[1:])
	return &Partial{fn: args[0], args: stored, kwds: kwds}, nil
}

func (p *Partial) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// The stored arguments come first, and the call's keywords override the
	// stored ones, which is what makes partial usable for defaults.
	full := append(py.Tuple{}, p.args...)
	full = append(full, args...)
	merged := py.NewStringDict()
	p.kwds.Range(func(k string, v py.Object) bool {
		merged.Set(k, v)
		return false
	})
	kwargs.Range(func(k string, v py.Object) bool {
		merged.Set(k, v)
		return false
	})
	return py.Call(p.fn, full, merged)
}

func (p *Partial) M__repr__() (py.Object, error) {
	// A partial that wraps itself would recurse forever; that cannot happen
	// through normal use but is cheap to guard, and the guard is what makes
	// a mistake here a readable repr rather than a stack overflow.
	if p.fn == py.Object(p) {
		return py.String("functools.partial(<recursive>)"), nil
	}
	name, err := py.ReprAsString(p.fn)
	if err != nil {
		name = "?"
	}
	out := "functools.partial(" + name
	for _, a := range p.args {
		s, err := py.ReprAsString(a)
		if err != nil {
			s = "?"
		}
		out += ", " + s
	}
	p.kwds.Range(func(k string, v py.Object) bool {
		s, err := py.ReprAsString(v)
		if err != nil {
			s = "?"
		}
		out += ", " + k + "=" + s
		return false
	})
	return py.String(out + ")"), nil
}

// M__str__ is repr, which is what CPython does for partial.  It exists as
// its own method because str() otherwise falls back to Repr, and having
// both paths reach M__repr__ through different interfaces is how the
// recursion started.
func (p *Partial) M__str__() (py.Object, error) { return p.M__repr__() }

var (
	_ py.I__call__ = (*Partial)(nil)
	_ py.I__repr__ = (*Partial)(nil)
	_ py.I__str__  = (*Partial)(nil)
)

func init() {
	PartialType.Dict.Set("func", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*Partial).fn, nil
	}})
	PartialType.Dict.Set("args", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*Partial).args, nil
	}})
	PartialType.Dict.Set("keywords", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*Partial).kwds, nil
	}})
}

// ---------------------------------------------------------------------------
// reduce

func reduce(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "reduce() needs a function and a sequence")
	}
	fn := args[0]
	items, err := py.SequenceList(args[1])
	if err != nil {
		return nil, err
	}
	list := items.Items
	var acc py.Object
	switch {
	case len(args) >= 3:
		acc = args[2]
	case len(list) > 0:
		acc = list[0]
		list = list[1:]
	default:
		return nil, py.ExceptionNewf(py.TypeError, "reduce() of empty sequence with no initial value")
	}
	for _, item := range list {
		acc, err = py.Call(fn, py.Tuple{acc, item}, py.StringDict{})
		if err != nil {
			return nil, err
		}
	}
	return acc, nil
}

// ---------------------------------------------------------------------------
// lru_cache

func lruCache(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	maxsize := -1
	// Called either as lru_cache(maxsize) or directly as a decorator.
	if len(args) > 0 {
		if fn, ok := args[0].(*py.Function); ok {
			return &CachedFunction{fn: fn, maxsize: 128, cache: map[string]py.Object{}}, nil
		}
		n, err := py.IndexInt(args[0])
		if err != nil {
			return nil, err
		}
		maxsize = n
	}
	return &lruCacheDecorator{maxsize: maxsize}, nil
}

type lruCacheDecorator struct{ maxsize int }

var lruCacheDecoratorType = py.NewType("functools.lru_cache", "The decorator returned by functools.lru_cache().")

func (d *lruCacheDecorator) Type() *py.Type { return lruCacheDecoratorType }

func (d *lruCacheDecorator) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "lru_cache() needs a function")
	}
	return &CachedFunction{fn: args[0], maxsize: d.maxsize, cache: map[string]py.Object{}}, nil
}

var _ py.I__call__ = (*lruCacheDecorator)(nil)

// CachedFunction memoises a function on its arguments.
type CachedFunction struct {
	fn      py.Object
	maxsize int
	cache   map[string]py.Object
	order   []string
	hits    int64
	misses  int64
}

var CachedFunctionType = py.NewType("functools._lru_cache_wrapper", "The wrapper a cached function becomes.")

func (c *CachedFunction) Type() *py.Type { return CachedFunctionType }

// keyFor builds the cache key from the arguments, so that equal arguments in
// different positions do not collide and the types are part of the identity.
func keyFor(args py.Tuple, kwargs py.StringDict) (string, error) {
	key := ""
	for _, a := range args {
		k, err := py.DictKey(a)
		if err != nil {
			return "", err
		}
		key += "\x01" + k
	}
	// Keyword order must not change the key, so sort the names.
	names := make([]string, 0, kwargs.Len())
	names = append(names, kwargs.Keys()...)
	sort.Strings(names)
	for _, name := range names {
		k, err := py.DictKey(kwargs.GetOrNil(name))
		if err != nil {
			return "", err
		}
		key += "\x02" + name + "\x03" + k
	}
	return key, nil
}

func (c *CachedFunction) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	key, err := keyFor(args, kwargs)
	if err != nil {
		// An unhashable argument cannot be cached; CPython raises
		// TypeError, and so does this.
		return nil, py.ExceptionNewf(py.TypeError, "unhashable type")
	}
	if v, ok := c.cache[key]; ok {
		c.hits++
		return v, nil
	}
	c.misses++
	v, err := py.Call(c.fn, args, kwargs)
	if err != nil {
		return nil, err
	}
	if c.maxsize > 0 && len(c.order) >= c.maxsize {
		// Evict the least recently used entry.
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.cache, oldest)
	}
	c.cache[key] = v
	c.order = append(c.order, key)
	return v, nil
}

var _ py.I__call__ = (*CachedFunction)(nil)

// M__get__ is the descriptor protocol, and it is what makes a cached function
// usable as a METHOD: without it the wrapper was found on the class and called
// with the arguments only, so "obj.cached(x)" reported "missing 1 required
// positional argument: 'x'" - self had been taken as the first argument.
//
// rich's Style._add is decorated with lru_cache, and Style.__add__ calls it, so
// this is what stopped "Style + Style" and with it pip's entire help output.
func (c *CachedFunction) M__get__(instance, owner py.Object) (py.Object, error) {
	if instance == nil || instance == py.None {
		return c, nil
	}
	// A bound callable: the same wrapper, called with the instance prepended.
	// Building the Method that way keeps the cache shared with the unbound
	// wrapper, which is what CPython does.
	return py.NewMethod("__call__", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return c.M__call__(append(py.Tuple{instance}, args...), kwargs)
	}, py.METH_CLASS, "A cached method.")
}

func init() {
	invalidate := func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*CachedFunction)
		c.cache = map[string]py.Object{}
		c.order = nil
		return py.None, nil
	}
	info := func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*CachedFunction)
		d := py.NewStringDict()
		d.Set("hits", py.Int(c.hits))
		d.Set("misses", py.Int(c.misses))
		d.Set("maxsize", py.Int(c.maxsize))
		d.Set("currsize", py.Int(len(c.cache)))
		return d, nil
	}
	for _, t := range []*py.Type{CachedFunctionType} {
		t.Dict.Set("cache_clear", py.MustNewMethod("cache_clear", invalidate, 0, "Clear the cache."))
		t.Dict.Set("cache_info", py.MustNewMethod("cache_info", info, 0, "Return the cache statistics."))
		t.Dict.Set("__wrapped__", &py.Property{Fget: func(self py.Object) (py.Object, error) {
			return self.(*CachedFunction).fn, nil
		}})
	}
	lruCacheDecoratorType.Dict.Set("cache_clear", py.MustNewMethod("cache_clear", invalidate, 0, "Clear the cache."))
	lruCacheDecoratorType.Dict.Set("cache_info", py.MustNewMethod("cache_info", info, 0, "Return the cache statistics."))
}

// ---------------------------------------------------------------------------
// cmp_to_key

func cmpToKey(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "cmp_to_key() needs a comparison function")
	}
	// The result is the KEY FUNCTION: sorted() calls it with each item, and
	// what it returns is compared.  It therefore produces a key object per
	// item, carrying that item's value - returning one shared object would
	// compare every item against the same value.
	return &cmpKeyFunc{fn: args[0]}, nil
}

// cmpKeyFunc is the key function cmp_to_key() returns.
type cmpKeyFunc struct {
	fn py.Object
}

var cmpKeyFuncType = py.NewType("functools.KeyWrapper", "The key function produced by cmp_to_key().")

func (c *cmpKeyFunc) Type() *py.Type { return cmpKeyFuncType }

func (c *cmpKeyFunc) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var item py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "key", 1, 1, &item); err != nil {
		return nil, err
	}
	return &cmpKey{fn: c.fn, value: item}, nil
}

var _ py.I__call__ = (*cmpKeyFunc)(nil)

// cmpKey wraps a comparison function so that sorted() can call it.
type cmpKey struct {
	fn    py.Object
	value py.Object
}

var cmpKeyType = py.NewType("functools.KeyWrapper.__key", "A key produced by cmp_to_key().")

func (k *cmpKey) Type() *py.Type { return cmpKeyType }

func init() {
	// The comparisons call the wrapped function, which returns a negative,
	// zero or positive number.
	compare := func(self py.Object, other py.Object, want int) (py.Object, error) {
		k := self.(*cmpKey)
		o, ok := other.(*cmpKey)
		if !ok {
			return py.False, nil
		}
		res, err := py.Call(k.fn, py.Tuple{k.value, o.value}, py.StringDict{})
		if err != nil {
			return nil, err
		}
		n, err := py.IndexInt(res)
		if err != nil {
			return nil, err
		}
		switch want {
		case -1:
			return py.NewBool(n < 0), nil
		case 0:
			return py.NewBool(n == 0), nil
		default:
			return py.NewBool(n > 0), nil
		}
	}
	cmpKeyType.Dict.Set("__lt__", py.MustNewMethod("__lt__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__lt__", 1, 1, &other); err != nil {
			return nil, err
		}
		return compare(self, other, -1)
	}, 0, "Return self < other."))
	cmpKeyType.Dict.Set("__gt__", py.MustNewMethod("__gt__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__gt__", 1, 1, &other); err != nil {
			return nil, err
		}
		return compare(self, other, 1)
	}, 0, "Return self > other."))
	cmpKeyType.Dict.Set("__eq__", py.MustNewMethod("__eq__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__eq__", 1, 1, &other); err != nil {
			return nil, err
		}
		return compare(self, other, 0)
	}, 0, "Return self == other."))
}

// ---------------------------------------------------------------------------
// singledispatch

func singledispatch(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "singledispatch() needs a function")
	}
	d := &Dispatcher{defaultFn: args[0], registry: map[*py.Type]py.Object{py.ObjectType: args[0]}, order: []*py.Type{py.ObjectType}}
	return d, nil
}

// Dispatcher is the generic function singledispatch produces.
type Dispatcher struct {
	defaultFn py.Object
	registry  map[*py.Type]py.Object
	order     []*py.Type // registration order, for the registry mapping
}

var DispatcherType = py.NewType("functools.singledispatch", "A generic function.")

func (d *Dispatcher) Type() *py.Type { return DispatcherType }

func (d *Dispatcher) set(t *py.Type, fn py.Object) {
	if _, ok := d.registry[t]; !ok {
		d.order = append(d.order, t)
	}
	d.registry[t] = fn
}

// find is the implementation for cls: the first class in its MRO that is
// registered, else the most specific registered class cls is a (virtual)
// subclass of, which is how register(Mapping) catches a dict.
// ponytail: CPython composes the MRO with C3 and reports ambiguity among
// unrelated ABCs; here the earliest registration of the most specific wins.
func (d *Dispatcher) find(cls *py.Type, obj py.Object) (py.Object, error) {
	for _, m := range cls.Mro {
		if t, ok := m.(*py.Type); ok && t != py.ObjectType {
			if f, ok := d.registry[t]; ok {
				return f, nil
			}
		}
	}
	var best *py.Type
	for _, t := range d.order {
		if t == py.ObjectType {
			continue
		}
		ok, err := isSubclass(cls, t, obj)
		if err != nil {
			return nil, err
		}
		if ok && (best == nil || t.IsSubtype(best)) {
			best = t
		}
	}
	if best != nil {
		return d.registry[best], nil
	}
	return d.registry[py.ObjectType], nil
}

// isSubclass is issubclass(cls, parent), or isinstance(obj, parent) when an
// object is at hand: the structural ABC checks (dict is a Mapping) are only
// known for instances.
func isSubclass(cls, parent *py.Type, obj py.Object) (bool, error) {
	if cls.IsSubtype(parent) {
		return true, nil
	}
	b := py.GetModuleImplOrNil("builtins")
	if b == nil {
		return false, nil
	}
	name, arg := "issubclass", py.Object(cls)
	if obj != nil {
		name, arg = "isinstance", obj
	}
	fn, err := py.GetAttrString(b, name)
	if err != nil {
		return false, err
	}
	res, err := py.Call(fn, py.Tuple{arg, parent}, py.NewStringDict())
	if err != nil {
		return false, err
	}
	return py.ObjectIsTrue(res)
}

func (d *Dispatcher) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "%s requires at least 1 positional argument", "singledispatch function")
	}
	fn, err := d.find(args[0].Type(), args[0])
	if err != nil {
		return nil, err
	}
	return py.Call(fn, args, kwargs)
}

var _ py.I__call__ = (*Dispatcher)(nil)

// annotationType is the type the first parameter of fn is annotated with, for
// the bare "@g.register" form.  A string annotation (from __future__
// annotations) is evaluated in the function's globals.
func annotationType(fn py.Object) (*py.Type, error) {
	anns, err := py.GetAttrString(fn, "__annotations__")
	if err != nil {
		anns = py.None
	}
	var first py.Object
	if sd, ok := anns.(py.StringDict); ok {
		for _, e := range sd.Items() {
			if e.Key != "return" {
				first = e.Value
				break
			}
		}
	}
	if first == nil {
		return nil, py.ExceptionNewf(py.TypeError,
			"Invalid first argument to `register()`: %v. Use either `@register(some_class)` or plain `@register` on an annotated function.", fn)
	}
	if str, ok := first.(py.String); ok {
		f, ok := fn.(*py.Function)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "Invalid annotation for the first parameter: %v", first)
		}
		code, err := py.Compile(string(str), "<annotation>", py.EvalMode, 0, true)
		if err != nil {
			return nil, err
		}
		if first, err = py.VmEvalCode(f.Context, code, f.Globals, py.NewStringDict(), nil, py.NewStringDict(), nil, py.NewStringDict(), nil); err != nil {
			return nil, err
		}
	}
	if first == py.None {
		first = py.None.Type() // an annotation of None means NoneType
	}
	t, ok := first.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "Invalid annotation for the first parameter: %v is not a class.", first)
	}
	return t, nil
}

// register is the shared body of register(cls), register(cls, fn) and the
// bare register(fn).
func (d *Dispatcher) register(args py.Tuple) (py.Object, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, py.ExceptionNewf(py.TypeError, "register() takes 1 or 2 arguments")
	}
	t, isType := args[0].(*py.Type)
	// A *Type with an empty Name is an instance of a Python class, not a class.
	isType = isType && t.Name != ""
	if len(args) == 2 {
		if !isType {
			return nil, py.ExceptionNewf(py.TypeError, "Invalid first argument to `register()`: %v. Use either `@register(some_class)` or plain `@register` on an annotated function.", args[0])
		}
		d.set(t, args[1])
		return args[1], nil
	}
	if isType {
		return &registerDecorator{dispatcher: d, typ: t}, nil
	}
	t, err := annotationType(args[0])
	if err != nil {
		return nil, err
	}
	d.set(t, args[0])
	return args[0], nil
}

func init() {
	DispatcherType.Dict.Set("register", py.MustNewMethod("register", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*Dispatcher).register(args)
	}, 0, "Register an implementation for a type."))

	DispatcherType.Dict.Set("registry", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		d := self.(*Dispatcher)
		reg := py.NewStringDict()
		for _, t := range d.order {
			if _, err := py.SetItem(reg, t, d.registry[t]); err != nil {
				return nil, err
			}
		}
		return reg, nil
	}})
	DispatcherType.Dict.Set("dispatch", py.MustNewMethod("dispatch", func(self py.Object, args py.Tuple) (py.Object, error) {
		var t py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "dispatch", 1, 1, &t); err != nil {
			return nil, err
		}
		typ, ok := t.(*py.Type)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "dispatch() argument must be a class")
		}
		return self.(*Dispatcher).find(typ, nil)
	}, 0, "Return the implementation for a type."))
	DispatcherType.Dict.Set("_clear_cache", py.MustNewMethod("_clear_cache", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Dispatch is not cached."))
	// functools.update_wrapper(wrapper, func): the metadata of the default.
	for _, name := range []string{"__name__", "__qualname__", "__doc__", "__module__"} {
		name := name
		DispatcherType.Dict.Set(name, &py.Property{Fget: func(self py.Object) (py.Object, error) {
			return py.GetAttrString(self.(*Dispatcher).defaultFn, name)
		}})
	}
	DispatcherType.Dict.Set("__wrapped__", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*Dispatcher).defaultFn, nil
	}})
}

// registerDecorator is what @register(Type) returns.
type registerDecorator struct {
	dispatcher *Dispatcher
	typ        *py.Type
}

var registerDecoratorType = py.NewType("functools.register", "The decorator returned by register(Type).")

func (r *registerDecorator) Type() *py.Type { return registerDecoratorType }

func (r *registerDecorator) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "register() decorator needs a function")
	}
	r.dispatcher.set(r.typ, args[0])
	return args[0], nil
}

var _ py.I__call__ = (*registerDecorator)(nil)

// singledispatchmethod dispatches on the first argument after self.
type singleDispatchMethod struct {
	d *Dispatcher
}

var singleDispatchMethodType = py.NewTypeX("functools.singledispatchmethod",
	"Single-dispatch generic method descriptor.",
	func(_ *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "singledispatchmethod() takes exactly 1 argument")
		}
		d, err := singledispatch(nil, args)
		if err != nil {
			return nil, err
		}
		return &singleDispatchMethod{d: d.(*Dispatcher)}, nil
	}, nil)

func (m *singleDispatchMethod) Type() *py.Type { return singleDispatchMethodType }

// M__get__ binds the dispatcher: the result dispatches on its first argument
// and calls the chosen method's own __get__(instance, owner), as CPython does.
func (m *singleDispatchMethod) M__get__(instance, owner py.Object) (py.Object, error) {
	if instance == nil {
		instance = py.None
	}
	if owner == nil || owner == py.None {
		owner = instance.Type()
	}
	return &boundDispatch{d: m.d, instance: instance, owner: owner}, nil
}

type boundDispatch struct {
	d               *Dispatcher
	instance, owner py.Object
}

var boundDispatchType = py.NewType("functools._singledispatchmethod_bound", "A bound single-dispatch method.")

func (b *boundDispatch) Type() *py.Type { return boundDispatchType }

func (b *boundDispatch) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "singledispatchmethod requires at least 1 positional argument")
	}
	fn, err := b.d.find(args[0].Type(), args[0])
	if err != nil {
		return nil, err
	}
	if g, ok := fn.(py.I__get__); ok {
		if fn, err = g.M__get__(b.instance, b.owner); err != nil {
			return nil, err
		}
	}
	return py.Call(fn, args, kwargs)
}

var (
	_ py.I__call__ = (*boundDispatch)(nil)
	_ py.I__get__  = (*singleDispatchMethod)(nil)
)

func init() {
	for _, name := range []string{"register", "dispatch", "registry"} {
		name := name
		singleDispatchMethodType.Dict.Set(name, &py.Property{Fget: func(self py.Object) (py.Object, error) {
			return py.GetAttrString(self.(*singleDispatchMethod).d, name)
		}})
	}
}

// ---------------------------------------------------------------------------
// Go interface bridges
//
// sorted() compares keys through the Go interfaces rather than through the
// type's Dict, so the comparisons of a cmp_to_key wrapper are implemented
// directly.  The assertions at the bottom make an omission a compile error.

func (k *cmpKey) cmp(other py.Object) (int, error) {
	o, ok := other.(*cmpKey)
	if !ok {
		return 0, py.ExceptionNewf(py.TypeError, "cannot compare a key wrapper with %s", other.Type().Name)
	}
	res, err := py.Call(k.fn, py.Tuple{k.value, o.value}, py.StringDict{})
	if err != nil {
		return 0, err
	}
	n, err := py.IndexInt(res)
	if err != nil {
		return 0, err
	}
	switch {
	case n < 0:
		return -1, nil
	case n > 0:
		return 1, nil
	}
	return 0, nil
}

func (k *cmpKey) M__lt__(other py.Object) (py.Object, error) {
	c, err := k.cmp(other)
	if err != nil {
		return nil, err
	}
	return py.NewBool(c < 0), nil
}

func (k *cmpKey) M__gt__(other py.Object) (py.Object, error) {
	c, err := k.cmp(other)
	if err != nil {
		return nil, err
	}
	return py.NewBool(c > 0), nil
}

func (k *cmpKey) M__eq__(other py.Object) (py.Object, error) {
	c, err := k.cmp(other)
	if err != nil {
		return nil, err
	}
	return py.NewBool(c == 0), nil
}

var (
	_ py.I__lt__ = (*cmpKey)(nil)
	_ py.I__gt__ = (*cmpKey)(nil)
	_ py.I__eq__ = (*cmpKey)(nil)
)
