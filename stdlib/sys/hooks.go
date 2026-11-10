// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

import (
	"os"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

func ctxOf(self py.Object) py.Context { return self.(*py.Module).Context }

func modGlobals(ctx py.Context, name string) (py.StringDict, error) {
	m, err := ctx.GetModule(name)
	if err != nil {
		return py.StringDict{}, err
	}
	return m.Globals, nil
}

// importLeaf imports a dotted module and returns the module itself.
func importLeaf(ctx py.Context, name string) (py.Object, error) {
	return py.ImportModuleLevelObject(ctx, name, py.NewStringDict(), py.NewStringDict(), py.Tuple{py.String("__name__")}, 0)
}

func warn(ctx py.Context, msg string) error {
	w, err := importLeaf(ctx, "warnings")
	if err != nil {
		return err
	}
	fn, err := py.GetAttrString(w, "warn")
	if err != nil {
		return err
	}
	_, err = py.Call(fn, py.Tuple{py.String(msg), py.RuntimeWarning}, py.NewStringDict())
	return err
}

const breakpointhook_doc = `breakpointhook(*args, **kws)

This hook function is called by built-in breakpoint().`

// sys_breakpointhook is PEP 553: $PYTHONBREAKPOINT names the callable ("0"
// disables it), defaulting to pdb.set_trace.
func sys_breakpointhook(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	ctx := ctxOf(self)
	env := os.Getenv("PYTHONBREAKPOINT")
	if env == "" {
		env = "pdb.set_trace"
	}
	if env == "0" {
		return py.None, nil
	}
	modname, attr := "builtins", env
	if i := strings.LastIndexByte(env, '.'); i >= 0 {
		modname, attr = env[:i], env[i+1:]
	}
	bad := func() (py.Object, error) {
		return py.None, warn(ctx, "Ignoring unimportable $PYTHONBREAKPOINT: \""+env+"\"")
	}
	if modname == "" || attr == "" {
		return bad()
	}
	mod, err := importLeaf(ctx, modname)
	if err != nil {
		if py.IsException(py.ImportError, err) {
			return bad()
		}
		return nil, err
	}
	hook, err := py.GetAttrString(mod, attr)
	if err != nil {
		if py.IsException(py.AttributeError, err) {
			return bad()
		}
		return nil, err
	}
	return py.Call(hook, args, kwargs)
}

func sys_displayhook(self, o py.Object) (py.Object, error) {
	ctx := ctxOf(self)
	if o == py.None {
		return py.None, nil
	}
	bi, err := modGlobals(ctx, "builtins")
	if err != nil {
		return nil, err
	}
	bi.Set("_", py.None)
	sg, err := modGlobals(ctx, "sys")
	if err != nil {
		return nil, err
	}
	out := sg.GetOrNil("stdout")
	if out == nil || out == py.None {
		return nil, py.ExceptionNewf(py.RuntimeError, "lost sys.stdout")
	}
	r, err := py.ReprAsString(o)
	if err != nil {
		return nil, err
	}
	write, err := py.GetAttrString(out, "write")
	if err != nil {
		return nil, err
	}
	if _, err := py.Call(write, py.Tuple{py.String(r + "\n")}, py.NewStringDict()); err != nil {
		return nil, err
	}
	bi.Set("_", o)
	return py.None, nil
}

func sys_excepthook(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 3 {
		return nil, py.ExceptionNewf(py.TypeError, "excepthook expected 3 arguments, got %d", len(args))
	}
	tb, err := importLeaf(ctxOf(self), "traceback")
	if err != nil {
		return nil, err
	}
	fn, err := py.GetAttrString(tb, "print_exception")
	if err != nil {
		return nil, err
	}
	return py.Call(fn, args, py.NewStringDict())
}
