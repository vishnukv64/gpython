// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"
	"sort"
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

// envKeys returns the variable names as a sorted tuple of Strings, so that
// iteration is stable from one run to the next.
func envKeys() py.Tuple {
	vs := os.Environ()
	keys := make(py.Tuple, 0, len(vs))
	for _, evar := range vs {
		if i := strings.IndexByte(evar, '='); i >= 0 {
			keys = append(keys, py.String(evar[:i]))
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].(py.String) < keys[j].(py.String) })
	return keys
}

// environMethods implements the mapping API on top of the descriptors above,
// which is what makes environ.get/keys/items/... work.
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
		k := string(keys[len(keys)-1].(py.String))
		v, _ := lookupEnv(k)
		if err := unsetEnv(k); err != nil {
			return nil, err
		}
		return py.Tuple{py.String(k), py.String(v)}, nil
	}, 0, "popitem() -> remove and return the last (name, value) pair."),
	py.MustNewMethod("clear", func(self py.Object, args py.Tuple) (py.Object, error) {
		for _, k := range envKeys() {
			if err := unsetEnv(string(k.(py.String))); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "clear() -> remove every variable."),
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
var _ py.I__len__ = (*environ)(nil)
var _ py.I__getitem__ = (*environ)(nil)
var _ py.I__setitem__ = (*environ)(nil)
var _ py.I__delitem__ = (*environ)(nil)
var _ py.I__iter__ = (*environ)(nil)
var _ py.I__contains__ = (*environ)(nil)
