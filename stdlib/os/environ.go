// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"
	"runtime"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// EnvironType is the type of os.environ.
//
// It is a live VIEW of the process environment rather than the snapshot a
// plain dict would be.  A snapshot looks right until something writes to it:
// "os.environ['X'] = '1'" would land in the dict and never reach the process,
// so a child started by subprocess would not see it and "del os.environ['X']"
// would not unset anything.  Every operation goes through the process
// environment, so reads and writes agree and subprocess inherits what was
// set.
//
// CPython's os.environ is os._Environ, a MutableMapping that behaves the
// same way; the differences here are that this is not a subclass of
// MutableMapping and that the key type is checked the way _Environ checks
// it, raising TypeError for a non-str key.
var EnvironType = py.NewType("os._Environ", "The type of os.environ, a live view of the process environment.")

// Environ is the single os.environ object.
var Environ = &environ{}

type environ struct{}

func (e *environ) Type() *py.Type { return EnvironType }

// lookupEnv reports the value of a variable and whether it is set.  It goes
// to the process environment every time, so a write made through os.environ
// is visible to the next read and to any child process.
func lookupEnv(key string) (string, bool) { return os.LookupEnv(key) }

func setEnv(key, value string) error { return os.Setenv(key, value) }
func unsetEnv(key string) error      { return os.Unsetenv(key) }

// envKey checks a key the way CPython's _Environ does: it must be a string,
// which is what makes "os.environ[15]" a TypeError rather than a lookup that
// silently finds nothing.
func envKey(key py.Object) (string, error) {
	s, ok := key.(py.String)
	if !ok {
		return "", py.ExceptionNewf(py.TypeError, "str expected, not %s", key.Type().Name)
	}
	// Windows variable names are case-insensitive, and os._Environ stores and
	// reports them upper-cased.
	if runtime.GOOS == "windows" {
		return strings.ToUpper(string(s)), nil
	}
	return string(s), nil
}

func (e *environ) M__len__() (py.Object, error) {
	return py.Int(len(os.Environ())), nil
}

func (e *environ) M__contains__(item py.Object) (py.Object, error) {
	key, err := envKey(item)
	if err != nil {
		return nil, err
	}
	_, ok := lookupEnv(key)
	return py.NewBool(ok), nil
}

func (e *environ) M__getitem__(key py.Object) (py.Object, error) {
	k, err := envKey(key)
	if err != nil {
		return nil, err
	}
	v, ok := lookupEnv(k)
	if !ok {
		return nil, py.ExceptionNewf(py.KeyError, "%s", k)
	}
	return py.String(v), nil
}

func (e *environ) M__setitem__(key, value py.Object) (py.Object, error) {
	k, err := envKey(key)
	if err != nil {
		return nil, err
	}
	v, ok := value.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str expected, not %s", value.Type().Name)
	}
	if err := setEnv(k, string(v)); err != nil {
		return nil, err
	}
	return py.None, nil
}

func (e *environ) M__delitem__(key py.Object) (py.Object, error) {
	k, err := envKey(key)
	if err != nil {
		return nil, err
	}
	if _, ok := lookupEnv(k); !ok {
		return nil, py.ExceptionNewf(py.KeyError, "%s", k)
	}
	if err := unsetEnv(k); err != nil {
		return nil, err
	}
	return py.None, nil
}

func (e *environ) M__iter__() (py.Object, error) {
	return py.NewIterator(py.Tuple(envKeys())), nil
}

func (e *environ) M__str__() (py.Object, error) {
	var b strings.Builder
	b.WriteString("environ({")
	keys := envKeys()
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		v, _ := lookupEnv(string(k.(py.String)))
		repK, err := py.ReprAsString(k)
		if err != nil {
			return nil, err
		}
		repV, err := py.ReprAsString(py.String(v))
		if err != nil {
			return nil, err
		}
		b.WriteString(repK)
		b.WriteString(": ")
		b.WriteString(repV)
	}
	b.WriteString("})")
	return py.String(b.String()), nil
}

func (e *environ) M__repr__() (py.Object, error) { return e.M__str__() }

// envKeys lists the names in process order - startup order, with new names
// appended - which is the order of os._Environ's dict.
func envKeys() py.Tuple {
	vs := os.Environ()
	keys := make(py.Tuple, 0, len(vs))
	seen := make(map[string]bool, len(vs))
	for _, evar := range vs {
		// Windows keeps hidden "=C:=..." entries whose name starts with '='.
		i := strings.IndexByte(evar[min(1, len(evar)):], '=')
		if i < 0 {
			continue
		}
		k := evar[:i+1]
		if runtime.GOOS == "windows" {
			k = strings.ToUpper(k)
			if seen[k] {
				continue
			}
			seen[k] = true
		}
		keys = append(keys, py.String(k))
	}
	return keys
}

// environUpdate is MutableMapping.update(other): a mapping (anything with
// keys()) is copied key by key, anything else is an iterable of pairs.
func environUpdate(e *environ, other py.Object) error {
	return mergeInto(other, func(k, v py.Object) error {
		_, err := e.M__setitem__(k, v)
		return err
	})
}

// dictUpdate is dict.update(other) for a plain dict d.
func dictUpdate(d py.StringDict, other py.Object) error {
	return mergeInto(other, func(k, v py.Object) error {
		_, err := d.M__setitem__(k, v)
		return err
	})
}

func mergeInto(other py.Object, set func(k, v py.Object) error) error {
	var ierr error
	if keysFn, err := py.GetAttrString(other, "keys"); err == nil {
		keys, err := py.Call(keysFn, py.Tuple{}, py.NewStringDict())
		if err != nil {
			return err
		}
		err = py.Iterate(keys, func(k py.Object) bool {
			var v py.Object
			if v, ierr = py.GetItem(other, k); ierr == nil {
				ierr = set(k, v)
			}
			return ierr != nil
		})
		if err != nil {
			return err
		}
		return ierr
	}
	err := py.Iterate(other, func(pair py.Object) bool {
		var kv []py.Object
		if ierr = py.Iterate(pair, func(x py.Object) bool { kv = append(kv, x); return false }); ierr != nil {
			return true
		}
		if len(kv) < 2 {
			ierr = py.ExceptionNewf(py.ValueError, "not enough values to unpack (expected 2, got %d)", len(kv))
			return true
		} else if len(kv) > 2 {
			ierr = py.ExceptionNewf(py.ValueError, "too many values to unpack (expected 2, got %d)", len(kv))
			return true
		}
		ierr = set(kv[0], kv[1])
		return ierr != nil
	})
	if err != nil {
		return err
	}
	return ierr
}

func isMapping(o py.Object) bool {
	_, err := py.GetAttrString(o, "keys")
	return err == nil
}

func (e *environ) M__or__(other py.Object) (py.Object, error) {
	if !isMapping(other) {
		return py.NotImplemented, nil
	}
	d := getEnvVariables()
	if err := dictUpdate(d, other); err != nil {
		return nil, err
	}
	return d, nil
}

func (e *environ) M__ror__(other py.Object) (py.Object, error) {
	if !isMapping(other) {
		return py.NotImplemented, nil
	}
	d := py.NewStringDict()
	if err := dictUpdate(d, other); err != nil {
		return nil, err
	}
	if err := dictUpdate(d, e); err != nil {
		return nil, err
	}
	return d, nil
}

func (e *environ) M__ior__(other py.Object) (py.Object, error) {
	if err := environUpdate(e, other); err != nil {
		return nil, err
	}
	return e, nil
}

var environMethods = []*py.Method{
	py.MustNewMethod("get", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var key, def py.Object = nil, py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		k, err := envKey(key)
		if err != nil {
			return nil, err
		}
		if v, ok := lookupEnv(k); ok {
			return py.String(v), nil
		}
		return def, nil
	}, 0, `get(key[, default]) -> value

Return the value for key if key is in the environment, else default.`),
	py.MustNewMethod("keys", func(self py.Object, args py.Tuple) (py.Object, error) {
		return envKeys(), nil
	}, 0, "keys() -> a list of the environment variable names."),
	py.MustNewMethod("values", func(self py.Object, args py.Tuple) (py.Object, error) {
		keys := envKeys()
		vals := make(py.Tuple, 0, len(keys))
		for _, k := range keys {
			v, _ := lookupEnv(string(k.(py.String)))
			vals = append(vals, py.String(v))
		}
		return vals, nil
	}, 0, "values() -> a list of the environment variable values."),
	py.MustNewMethod("items", func(self py.Object, args py.Tuple) (py.Object, error) {
		keys := envKeys()
		items := make(py.Tuple, 0, len(keys))
		for _, k := range keys {
			v, _ := lookupEnv(string(k.(py.String)))
			items = append(items, py.Tuple{k, py.String(v)})
		}
		return items, nil
	}, 0, "items() -> a list of (name, value) pairs."),
	py.MustNewMethod("setdefault", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key, def py.Object = nil, py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "setdefault", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		k, err := envKey(key)
		if err != nil {
			return nil, err
		}
		if v, ok := lookupEnv(k); ok {
			return py.String(v), nil
		}
		s, err := py.Str(def)
		if err != nil {
			return nil, err
		}
		if err := setEnv(k, string(s.(py.String))); err != nil {
			return nil, err
		}
		return def, nil
	}, 0, "setdefault(key[, default]) -> set and return the value if absent."),
	py.MustNewMethod("pop", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var key, def py.Object = nil, nil
		if err := py.UnpackTuple(args, py.StringDict{}, "pop", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		k, err := envKey(key)
		if err != nil {
			return nil, err
		}
		v, ok := lookupEnv(k)
		if !ok {
			if def != nil {
				return def, nil
			}
			return nil, py.ExceptionNewf(py.KeyError, "%s", k)
		}
		if err := unsetEnv(k); err != nil {
			return nil, err
		}
		return py.String(v), nil
	}, 0, "pop(key[, default]) -> remove key and return its value."),
	py.MustNewMethod("popitem", func(self py.Object, args py.Tuple) (py.Object, error) {
		keys := envKeys()
		if len(keys) == 0 {
			return nil, py.ExceptionNewf(py.KeyError, "popitem(): environment is empty")
		}
		k := string(keys[0].(py.String))
		v, _ := lookupEnv(k)
		if err := unsetEnv(k); err != nil {
			return nil, err
		}
		return py.Tuple{py.String(k), py.String(v)}, nil
	}, 0, "popitem() -> remove and return the first (name, value) pair."),
	py.MustNewMethod("clear", func(self py.Object, args py.Tuple) (py.Object, error) {
		for _, k := range envKeys() {
			if err := unsetEnv(string(k.(py.String))); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "clear() -> remove every variable."),
	py.MustNewMethod("update", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		e := self.(*environ)
		if len(args) > 1 {
			return nil, py.ExceptionNewf(py.TypeError, "MutableMapping.update() takes from 1 to 2 positional arguments but %d were given", len(args)+1)
		}
		if len(args) == 1 {
			if err := environUpdate(e, args[0]); err != nil {
				return nil, err
			}
		}
		for _, k := range kwargs.Keys() {
			if _, err := e.M__setitem__(py.String(k), kwargs.GetOrNil(k)); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "update([other, ]**kwds) -> set variables from a mapping or pairs, then keywords."),
	py.MustNewMethod("copy", func(self py.Object, args py.Tuple) (py.Object, error) {
		return getEnvVariables(), nil
	}, 0, "copy() -> a plain dict of the environment."),
}

func init() {
	for _, m := range environMethods {
		EnvironType.Dict.Set(m.Name, m)
	}
}

// Check the interfaces are satisfied.
var _ py.I__or__ = (*environ)(nil)
var _ py.I__ror__ = (*environ)(nil)
var _ py.I__ior__ = (*environ)(nil)
var _ py.I__len__ = (*environ)(nil)
var _ py.I__getitem__ = (*environ)(nil)
var _ py.I__setitem__ = (*environ)(nil)
var _ py.I__delitem__ = (*environ)(nil)
var _ py.I__iter__ = (*environ)(nil)
var _ py.I__contains__ = (*environ)(nil)
