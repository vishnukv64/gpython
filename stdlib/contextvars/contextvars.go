// Package contextvars implements the _contextvars module (ContextVar, Token,
// Context, copy_context). The current Context is per goroutine, matching how
// threading runs each Thread in its own goroutine.
package contextvars

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/vishnukv64/gpython/py"
)

// Context is an immutable-by-copy mapping ContextVar -> value.
// ponytail: linear scan over parallel slices; swap for a map if contexts hold hundreds of vars.
type Context struct {
	vars    []*ContextVar
	vals    []py.Object
	entered bool
}

type ContextVar struct {
	name string
	def  py.Object // nil when no default
}

type Token struct {
	ctx  *Context
	v    *ContextVar
	old  py.Object // nil == MISSING
	used bool
}

type missingT struct{}

var Missing py.Object = &missingT{}

var MissingType = py.NewTypeX("Token.MISSING", "", nil, nil)

func (*missingT) Type() *py.Type { return MissingType }
func (*missingT) M__repr__() (py.Object, error) {
	return py.String("<Token.MISSING>"), nil
}

var (
	ContextType = py.NewTypeX("Context", "Context() -> new empty context.", func(*py.Type, py.Tuple, py.StringDict) (py.Object, error) { return &Context{}, nil }, nil)
	VarType     = py.NewTypeX("ContextVar", "Context variable.", newVar, nil)
	TokenType   = py.NewTypeX("Token", "Token returned by ContextVar.set().", nil, nil)
)

func (*Context) Type() *py.Type    { return ContextType }
func (*ContextVar) Type() *py.Type { return VarType }
func (*Token) Type() *py.Type      { return TokenType }

// current context per goroutine id. ponytail: entries of dead goroutines are never freed.
var (
	mu  sync.Mutex
	cur = map[uint64]*Context{}
)

func goroutineID() uint64 { return py.GoroutineID() }

func current() *Context {
	g := goroutineID()
	mu.Lock()
	defer mu.Unlock()
	c := cur[g]
	if c == nil {
		c = &Context{}
		cur[g] = c
	}
	return c
}

func (c *Context) index(v *ContextVar) int {
	for i, x := range c.vars {
		if x == v {
			return i
		}
	}
	return -1
}

func (c *Context) lookup(v *ContextVar) (py.Object, bool) {
	if i := c.index(v); i >= 0 {
		return c.vals[i], true
	}
	return nil, false
}

// with returns a copy with v set to val (or removed when val == nil).
func (c *Context) with(v *ContextVar, val py.Object) *Context {
	n := &Context{vars: append([]*ContextVar(nil), c.vars...), vals: append([]py.Object(nil), c.vals...)}
	i := n.index(v)
	switch {
	case val == nil && i >= 0:
		n.vars = append(n.vars[:i], n.vars[i+1:]...)
		n.vals = append(n.vals[:i], n.vals[i+1:]...)
	case val != nil && i >= 0:
		n.vals[i] = val
	case val != nil:
		n.vars = append(n.vars, v)
		n.vals = append(n.vals, val)
	}
	return n
}

// swap replaces the current context's contents in place (Context identity is
// preserved because Token.ctx and copy_context results are compared by identity).
func (c *Context) set(v *ContextVar, val py.Object) {
	n := c.with(v, val)
	c.vars, c.vals = n.vars, n.vals
}

func newVar(_ *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "ContextVar() takes exactly 1 positional argument (%d given)", len(args))
	}
	name, ok := args[0].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "context variable name must be a str")
	}
	v := &ContextVar{name: string(name)}
	for _, e := range kwargs.Items() {
		if e.Key != "default" {
			return nil, py.ExceptionNewf(py.TypeError, "'%s' is an invalid keyword argument for ContextVar()", e.Key)
		}
		v.def = e.Value
	}
	return v, nil
}

func (v *ContextVar) M__hash__() (py.Object, error) {
	return py.Int(uintptr(unsafe.Pointer(v)) >> 3), nil
}

func (v *ContextVar) M__repr__() (py.Object, error) {
	s := fmt.Sprintf("<ContextVar name='%s'", v.name)
	if v.def != nil {
		r, err := py.Repr(v.def)
		if err != nil {
			return nil, err
		}
		s += fmt.Sprintf(" default=%s", r.(py.String))
	}
	return py.String(s + fmt.Sprintf(" at %p>", v)), nil
}

func (t *Token) M__repr__() (py.Object, error) {
	used := ""
	if t.used {
		used = " used"
	}
	return py.String(fmt.Sprintf("<Token%s var=<ContextVar name='%s' at %p> at %p>", used, t.v.name, t.v, t)), nil
}

func (c *Context) M__len__() (py.Object, error) { return py.Int(len(c.vars)), nil }
func (c *Context) M__iter__() (py.Object, error) {
	items := make([]py.Object, len(c.vars))
	for i, v := range c.vars {
		items[i] = v
	}
	return py.NewIterator(py.NewListFromItems(items)), nil
}

func varArg(o py.Object) (*ContextVar, error) {
	v, ok := o.(*ContextVar)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "a ContextVar key was expected, got %s", repr(o))
	}
	return v, nil
}

func (c *Context) M__getitem__(k py.Object) (py.Object, error) {
	v, err := varArg(k)
	if err != nil {
		return nil, err
	}
	if val, ok := c.lookup(v); ok {
		return val, nil
	}
	return nil, exc(py.KeyError, k)
}

func (c *Context) M__contains__(k py.Object) (py.Object, error) {
	v, err := varArg(k)
	if err != nil {
		return nil, err
	}
	_, ok := c.lookup(v)
	return py.NewBool(ok), nil
}

func init() {
	for _, t := range []*py.Type{ContextType, VarType, TokenType} {
		_ = t
	}
	VarType.Dict.Set("name", &py.Property{Fget: func(s py.Object) (py.Object, error) { return py.String(s.(*ContextVar).name), nil }})
	VarType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		v := self.(*ContextVar)
		if len(args) > 1 {
			return nil, py.ExceptionNewf(py.TypeError, "get expected at most 1 argument, got %d", len(args))
		}
		if val, ok := current().lookup(v); ok {
			return val, nil
		}
		if len(args) == 1 {
			return args[0], nil
		}
		if v.def != nil {
			return v.def, nil
		}
		return nil, exc(py.LookupError, v)
	}, 0, "Return the value for the context variable in the current context."))
	VarType.Dict.Set("set", py.MustNewMethod("set", func(self py.Object, args py.Tuple) (py.Object, error) {
		v := self.(*ContextVar)
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "set expected 1 argument, got %d", len(args))
		}
		c := current()
		old, _ := c.lookup(v)
		c.set(v, args[0])
		return &Token{ctx: c, v: v, old: old}, nil
	}, 0, "Set a new value; returns a Token."))
	VarType.Dict.Set("reset", py.MustNewMethod("reset", func(self py.Object, args py.Tuple) (py.Object, error) {
		v := self.(*ContextVar)
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "reset expected 1 argument, got %d", len(args))
		}
		t, ok := args[0].(*Token)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "expected an instance of Token, got %s", repr(args[0]))
		}
		if t.used {
			return nil, py.ExceptionNewf(py.RuntimeError, "%s has already been used once", repr(t))
		}
		if t.v != v {
			return nil, py.ExceptionNewf(py.ValueError, "%s was created by a different ContextVar", repr(t))
		}
		c := current()
		if t.ctx != c {
			return nil, py.ExceptionNewf(py.ValueError, "%s was created in a different Context", repr(t))
		}
		t.used = true
		c.set(v, t.old)
		return py.None, nil
	}, 0, "Reset to the value before the matching set()."))
	VarType.Dict.Set("__class_getitem__", py.MustNewMethod("__class_getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, py.METH_CLASS, ""))

	TokenType.Dict.Set("var", &py.Property{Fget: func(s py.Object) (py.Object, error) { return s.(*Token).v, nil }})
	TokenType.Dict.Set("old_value", &py.Property{Fget: func(s py.Object) (py.Object, error) {
		if o := s.(*Token).old; o != nil {
			return o, nil
		}
		return Missing, nil
	}})
	TokenType.Dict.Set("MISSING", Missing)
	TokenType.Dict.Set("__class_getitem__", py.MustNewMethod("__class_getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, py.METH_CLASS, ""))

	ContextType.Dict.Set("run", py.MustNewMethod("run", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		c := self.(*Context)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "run() missing 1 required positional argument")
		}
		if c.entered {
			return nil, py.ExceptionNewf(py.RuntimeError, "cannot enter context: %s is already entered", repr(c))
		}
		g := goroutineID()
		mu.Lock()
		prev := cur[g]
		cur[g] = c
		mu.Unlock()
		c.entered = true
		defer func() {
			c.entered = false
			mu.Lock()
			if prev == nil {
				delete(cur, g)
			} else {
				cur[g] = prev
			}
			mu.Unlock()
		}()
		return py.Call(args[0], args[1:], kwargs)
	}, 0, "Run callable in this context."))
	ContextType.Dict.Set("copy", py.MustNewMethod("copy", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*Context).with(nil, nil), nil
	}, 0, "Return a shallow copy."))
	ContextType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, py.ExceptionNewf(py.TypeError, "get expected 1 or 2 arguments, got %d", len(args))
		}
		v, err := varArg(args[0])
		if err != nil {
			return nil, err
		}
		if val, ok := self.(*Context).lookup(v); ok {
			return val, nil
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return py.None, nil
	}, 0, "Return the value for var if set, else default (None)."))
	list := func(name string, f func(c *Context, i int) py.Object) {
		ContextType.Dict.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			c := self.(*Context)
			out := make([]py.Object, len(c.vars))
			for i := range out {
				out[i] = f(c, i)
			}
			return py.NewIterator(py.NewListFromItems(out)), nil
		}, 0, ""))
	}
	list("keys", func(c *Context, i int) py.Object { return c.vars[i] })
	list("values", func(c *Context, i int) py.Object { return c.vals[i] })
	list("items", func(c *Context, i int) py.Object { return py.Tuple{c.vars[i], c.vals[i]} })

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_contextvars", Doc: "Context Variables"},
		Methods: []*py.Method{
			py.MustNewMethod("copy_context", func(py.Object, py.Tuple) (py.Object, error) {
				return current().with(nil, nil), nil
			}, 0, "Return a copy of the current context."),
		},
		Globals: py.NewStringDictFrom(py.DictEntry{Key: "Context", Value: ContextType}, py.DictEntry{Key: "ContextVar", Value: VarType}, py.DictEntry{Key: "Token", Value: TokenType}),
	})
}

func repr(o py.Object) string {
	r, err := py.Repr(o)
	if err != nil {
		return "?"
	}
	return string(r.(py.String))
}

func exc(t *py.Type, arg py.Object) error {
	e, err := py.ExceptionNew(t, py.Tuple{arg}, py.StringDict{})
	if err != nil {
		return err
	}
	return e.(*py.Exception)
}
