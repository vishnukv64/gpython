// Copyright 2025 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

import (
	"os"
	"sync"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	lib "modernc.org/sqlite/lib"

	"github.com/vishnukv64/gpython/py"
)

// SQLite hands a C function pointer and one uintptr of user data to every
// callback, and the transpiled library cannot call Go closures.  So each
// registration stores its Python objects here under an id and passes the id as
// the user data; the package-level trampolines below look it up.
var cbs = struct {
	sync.Mutex
	m map[uintptr]interface{}
	n uintptr
}{m: map[uintptr]interface{}{}}

func cbAdd(v interface{}) uintptr {
	cbs.Lock()
	defer cbs.Unlock()
	cbs.n++
	cbs.m[cbs.n] = v
	return cbs.n
}

func cbGet(id uintptr) interface{} {
	cbs.Lock()
	defer cbs.Unlock()
	return cbs.m[id]
}

func cbDrop(id uintptr) {
	cbs.Lock()
	defer cbs.Unlock()
	delete(cbs.m, id)
}

// cFuncPointer turns a top-level Go func into the pointer the transpiled
// library calls (same technique as modernc.org/sqlite itself).
func cFuncPointer[T any](f T) uintptr {
	return *(*uintptr)(unsafe.Pointer(&struct{ f T }{f}))
}

type fnCB struct {
	fn  py.Object
	cls py.Object // aggregates: the class; instances are cbAdd'ed too
}

// report prints a callback's exception when enable_callback_tracebacks is on;
// otherwise it is dropped, as in CPython.
func report(err error) {
	if !callbackTrace || err == nil {
		return
	}
	e := py.MakeException(err)
	(&py.ExceptionInfo{Type: e.Type(), Value: e}).TracebackDump(os.Stderr)
}

func resultError(tls *libc.TLS, ctx uintptr, msg string) {
	z, err := libc.CString(msg)
	if err != nil {
		return
	}
	lib.Xsqlite3_result_error(tls, ctx, z, -1)
	libc.Xfree(tls, z)
}

func valueArgs(tls *libc.TLS, argc int32, argv uintptr) py.Tuple {
	out := make(py.Tuple, argc)
	for i := range out {
		v := *(*uintptr)(unsafe.Pointer(argv + uintptr(i)*ptrSize))
		switch lib.Xsqlite3_value_type(tls, v) {
		case lib.SQLITE_INTEGER:
			out[i] = py.Int(lib.Xsqlite3_value_int64(tls, v))
		case lib.SQLITE_FLOAT:
			out[i] = py.Float(lib.Xsqlite3_value_double(tls, v))
		case lib.SQLITE_TEXT:
			n := int(lib.Xsqlite3_value_bytes(tls, v))
			out[i] = py.String(libc.GoBytes(lib.Xsqlite3_value_text(tls, v), n))
		case lib.SQLITE_BLOB:
			n := int(lib.Xsqlite3_value_bytes(tls, v))
			b := make(py.Bytes, n)
			if n > 0 {
				copy(b, libc.GoBytes(lib.Xsqlite3_value_blob(tls, v), n))
			}
			out[i] = b
		default:
			out[i] = py.None
		}
	}
	return out
}

// setResult stores a Python value as the function result; false means the
// type is unsupported or an int overflowed.
func setResult(tls *libc.TLS, ctx uintptr, r py.Object) bool {
	switch x := r.(type) {
	case py.NoneType:
		lib.Xsqlite3_result_null(tls, ctx)
	case py.Bool:
		var n int64
		if x {
			n = 1
		}
		lib.Xsqlite3_result_int64(tls, ctx, n)
	case py.Int:
		lib.Xsqlite3_result_int64(tls, ctx, int64(x))
	case *py.BigInt:
		n, err := x.GoInt64()
		if err != nil {
			lib.Xsqlite3_result_error_toobig(tls, ctx)
			return true
		}
		lib.Xsqlite3_result_int64(tls, ctx, n)
	case py.Float:
		lib.Xsqlite3_result_double(tls, ctx, float64(x))
	case py.String:
		blobOrText(tls, ctx, []byte(x), true)
	case py.Bytes:
		blobOrText(tls, ctx, x, false)
	case *py.ByteArray:
		b, err := py.BytesFromObject(x)
		if err != nil {
			return false
		}
		blobOrText(tls, ctx, b, false)
	default:
		return false
	}
	return true
}

func blobOrText(tls *libc.TLS, ctx uintptr, b []byte, text bool) {
	if len(b) == 0 && !text {
		lib.Xsqlite3_result_zeroblob(tls, ctx, 0)
		return
	}
	p := libc.Xmalloc(tls, types.Size_t(len(b)+1))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(b)), b)
	if text {
		lib.Xsqlite3_result_text(tls, ctx, p, int32(len(b)), sqliteTransient)
	} else {
		lib.Xsqlite3_result_blob(tls, ctx, p, int32(len(b)), sqliteTransient)
	}
	libc.Xfree(tls, p)
}

func scalarTramp(tls *libc.TLS, ctx uintptr, argc int32, argv uintptr) {
	cb := cbGet(lib.Xsqlite3_user_data(tls, ctx)).(*fnCB)
	r, err := call(cb.fn, valueArgs(tls, argc, argv))
	if err != nil {
		report(err)
		resultError(tls, ctx, errNoCallbackMsg)
		return
	}
	if !setResult(tls, ctx, r) {
		resultError(tls, ctx, errNoCallbackMsg)
	}
}

// aggInstance returns the Python aggregate object for this query, creating it
// on first use.  Its cbs id lives in SQLite's per-aggregate context slot.
func aggInstance(tls *libc.TLS, ctx uintptr, create bool) (py.Object, uintptr) {
	slot := lib.Xsqlite3_aggregate_context(tls, ctx, int32(ptrSize))
	if slot == 0 {
		return nil, 0
	}
	p := (*uintptr)(unsafe.Pointer(slot))
	if *p != 0 {
		return cbGet(*p).(py.Object), *p
	}
	if !create {
		return nil, 0
	}
	cb := cbGet(lib.Xsqlite3_user_data(tls, ctx)).(*fnCB)
	inst, err := call(cb.cls, nil)
	if err != nil {
		report(err)
		resultError(tls, ctx, "user-defined aggregate's '__init__' method raised error")
		return nil, 0
	}
	*p = cbAdd(inst)
	return inst, *p
}

func callMethod(inst py.Object, name string, args py.Tuple) (py.Object, error) {
	m, err := py.GetAttrString(inst, name)
	if err != nil {
		return nil, err
	}
	return call(m, args)
}

func stepTramp(tls *libc.TLS, ctx uintptr, argc int32, argv uintptr) {
	inst, _ := aggInstance(tls, ctx, true)
	if inst == nil {
		return
	}
	if _, err := callMethod(inst, "step", valueArgs(tls, argc, argv)); err != nil {
		report(err)
		resultError(tls, ctx, "user-defined aggregate's 'step' method raised error")
	}
}

func inverseTramp(tls *libc.TLS, ctx uintptr, argc int32, argv uintptr) {
	inst, _ := aggInstance(tls, ctx, false)
	if inst == nil {
		return
	}
	if _, err := callMethod(inst, "inverse", valueArgs(tls, argc, argv)); err != nil {
		report(err)
		resultError(tls, ctx, "user-defined window function's 'inverse' method raised error")
	}
}

func valueTramp(tls *libc.TLS, ctx uintptr) {
	inst, _ := aggInstance(tls, ctx, false)
	if inst == nil {
		return
	}
	r, err := callMethod(inst, "value", nil)
	if err != nil {
		report(err)
		resultError(tls, ctx, "user-defined window function's 'value' method raised error")
	} else if !setResult(tls, ctx, r) {
		resultError(tls, ctx, "user-defined window function's 'value' method raised error")
	}
}

func finalTramp(tls *libc.TLS, ctx uintptr) {
	inst, id := aggInstance(tls, ctx, false)
	if inst == nil {
		return // step never ran (no rows) or __init__ failed
	}
	defer cbDrop(id)
	r, err := callMethod(inst, "finalize", nil)
	if err != nil {
		report(err)
		resultError(tls, ctx, "user-defined aggregate's 'finalize' method raised error")
	} else if !setResult(tls, ctx, r) {
		resultError(tls, ctx, "user-defined aggregate's 'finalize' method raised error")
	}
}

func collationTramp(tls *libc.TLS, pApp uintptr, nl int32, l uintptr, nr int32, r uintptr) int32 {
	cb := cbGet(pApp).(*fnCB)
	res, err := call(cb.fn, py.Tuple{py.String(libc.GoBytes(l, int(nl))), py.String(libc.GoBytes(r, int(nr)))})
	if err != nil {
		report(err)
		return 0
	}
	n, err := goInt(res)
	if err != nil {
		report(err)
		return 0
	}
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func authTramp(tls *libc.TLS, pApp uintptr, op int32, a, b, c, d uintptr) int32 {
	cb := cbGet(pApp).(*fnCB)
	str := func(p uintptr) py.Object {
		if p == 0 {
			return py.None
		}
		return py.String(libc.GoString(p))
	}
	res, err := call(cb.fn, py.Tuple{py.Int(op), str(a), str(b), str(c), str(d)})
	if err != nil {
		report(err)
		return lib.SQLITE_DENY
	}
	n, err := goInt(res)
	if err != nil || (n != lib.SQLITE_OK && n != lib.SQLITE_DENY && n != lib.SQLITE_IGNORE) {
		return lib.SQLITE_DENY
	}
	return int32(n)
}

func progressTramp(tls *libc.TLS, pApp uintptr) int32 {
	cb := cbGet(pApp).(*fnCB)
	res, err := call(cb.fn, nil)
	if err != nil {
		report(err)
		return 1
	}
	if toBool(res) {
		return 1
	}
	return 0
}

func traceTramp(tls *libc.TLS, mask uint32, pApp, p, x uintptr) int32 {
	cb := cbGet(pApp).(*fnCB)
	sql := lib.Xsqlite3_expanded_sql(tls, p)
	if sql == 0 {
		sql = x
	} else {
		defer lib.Xsqlite3_free(tls, sql)
	}
	if _, err := call(cb.fn, py.Tuple{py.String(libc.GoString(sql))}); err != nil {
		report(err)
	}
	return 0
}

// keep registers a new callback id on d so close drops it.
func (d *Db) keep(v interface{}) uintptr {
	id := cbAdd(v)
	d.ids = append(d.ids, id)
	return id
}

func registerCallbacks(def func(*py.Type, string, interface{})) {
	type A = py.Tuple
	type O = py.Object
	isNone := func(o O) bool { return o == py.None }
	check := func(self O) (*Db, error) {
		d := self.(*Db)
		return d, d.open()
	}
	// create_function(name, nargs, fn_or_None, deterministic)
	def(dbType, "create_function", func(self O, a A) (O, error) {
		d, err := check(self)
		if err != nil {
			return nil, err
		}
		name, narg := d.cstr(string(a[0].(py.String))), int32(a[1].(py.Int))
		defer d.free(name)
		flags := int32(lib.SQLITE_UTF8)
		if toBool(a[3]) {
			flags |= lib.SQLITE_DETERMINISTIC
		}
		var rc int32
		if isNone(a[2]) {
			rc = lib.Xsqlite3_create_function(d.tls, d.db, name, narg, flags, 0, 0, 0, 0)
		} else {
			id := d.keep(&fnCB{fn: a[2]})
			rc = lib.Xsqlite3_create_function(d.tls, d.db, name, narg, flags, id, cFuncPointer(scalarTramp), 0, 0)
		}
		if rc != lib.SQLITE_OK {
			return nil, d.sqliteError(rc)
		}
		return py.None, nil
	})
	// create_aggregate(name, nargs, cls_or_None, deterministic, window)
	def(dbType, "create_aggregate", func(self O, a A) (O, error) {
		d, err := check(self)
		if err != nil {
			return nil, err
		}
		name, narg := d.cstr(string(a[0].(py.String))), int32(a[1].(py.Int))
		defer d.free(name)
		flags := int32(lib.SQLITE_UTF8)
		if toBool(a[3]) {
			flags |= lib.SQLITE_DETERMINISTIC
		}
		var rc int32
		if isNone(a[2]) {
			rc = lib.Xsqlite3_create_window_function(d.tls, d.db, name, narg, flags, 0, 0, 0, 0, 0, 0)
		} else {
			id := d.keep(&fnCB{cls: a[2]})
			var val, inv uintptr
			if toBool(a[4]) {
				val, inv = cFuncPointer(valueTramp), cFuncPointer(inverseTramp)
			}
			rc = lib.Xsqlite3_create_window_function(d.tls, d.db, name, narg, flags, id,
				cFuncPointer(stepTramp), cFuncPointer(finalTramp), val, inv, 0)
		}
		if rc != lib.SQLITE_OK {
			return nil, d.sqliteError(rc)
		}
		return py.None, nil
	})
	def(dbType, "create_collation", func(self O, a A) (O, error) {
		d, err := check(self)
		if err != nil {
			return nil, err
		}
		name := d.cstr(string(a[0].(py.String)))
		defer d.free(name)
		var rc int32
		if isNone(a[1]) {
			rc = lib.Xsqlite3_create_collation_v2(d.tls, d.db, name, lib.SQLITE_UTF8, 0, 0, 0)
		} else {
			id := d.keep(&fnCB{fn: a[1]})
			rc = lib.Xsqlite3_create_collation_v2(d.tls, d.db, name, lib.SQLITE_UTF8, id, cFuncPointer(collationTramp), 0)
		}
		if rc != lib.SQLITE_OK {
			return nil, d.sqliteError(rc)
		}
		return py.None, nil
	})
	def(dbType, "set_authorizer", func(self O, a A) (O, error) {
		d, err := check(self)
		if err != nil {
			return nil, err
		}
		var rc int32
		if isNone(a[0]) {
			rc = lib.Xsqlite3_set_authorizer(d.tls, d.db, 0, 0)
		} else {
			rc = lib.Xsqlite3_set_authorizer(d.tls, d.db, cFuncPointer(authTramp), d.keep(&fnCB{fn: a[0]}))
		}
		if rc != lib.SQLITE_OK {
			return nil, d.sqliteError(rc)
		}
		return py.None, nil
	})
	def(dbType, "set_progress_handler", func(self O, a A) (O, error) {
		d, err := check(self)
		if err != nil {
			return nil, err
		}
		if isNone(a[0]) {
			lib.Xsqlite3_progress_handler(d.tls, d.db, 0, 0, 0)
		} else {
			n, err := goInt(a[1])
			if err != nil {
				return nil, err
			}
			lib.Xsqlite3_progress_handler(d.tls, d.db, int32(n), cFuncPointer(progressTramp), d.keep(&fnCB{fn: a[0]}))
		}
		return py.None, nil
	})
	def(dbType, "set_trace_callback", func(self O, a A) (O, error) {
		d, err := check(self)
		if err != nil {
			return nil, err
		}
		if isNone(a[0]) {
			lib.Xsqlite3_trace_v2(d.tls, d.db, 0, 0, 0)
		} else {
			lib.Xsqlite3_trace_v2(d.tls, d.db, lib.SQLITE_TRACE_STMT, cFuncPointer(traceTramp), d.keep(&fnCB{fn: a[0]}))
		}
		return py.None, nil
	})
}

func call(fn py.Object, args py.Tuple) (py.Object, error) {
	return py.Call(fn, args, py.NewStringDict())
}
