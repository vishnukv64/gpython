// Copyright 2025 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sqlite3 provides gpython's `_sqlite3` module on top of the pure-Go
// (no cgo) SQLite in modernc.org/sqlite/lib.
//
// This file is the low-level half: Db and Stmt wrap sqlite3* and sqlite3_stmt*
// one-to-one (prepare/bind/step/column, user functions, callbacks, backup,
// blobs) and raise CPython's exception classes with CPython's messages.  The
// Python-level API - Connection, Cursor, Row, adapters, converters, implicit
// transactions - is Python source in pyapi.go, written against these
// primitives, so that subclassing and row factories behave as in CPython.
// CPython's own Lib/sqlite3 (embedded in stdlib/pylib) then loads over it.
package sqlite3

import (
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	lib "modernc.org/sqlite/lib"

	"github.com/vishnukv64/gpython/py"
)

var (
	errWarning       = newExc(py.ExceptionType, "Warning")
	errError         = newExc(py.ExceptionType, "Error")
	errInterface     = newExc(errError, "InterfaceError")
	errDatabase      = newExc(errError, "DatabaseError")
	errData          = newExc(errDatabase, "DataError")
	errOperational   = newExc(errDatabase, "OperationalError")
	errIntegrity     = newExc(errDatabase, "IntegrityError")
	errInternal      = newExc(errDatabase, "InternalError")
	errProgramming   = newExc(errDatabase, "ProgrammingError")
	errNotSupported  = newExc(errDatabase, "NotSupportedError")
	callbackTrace    bool // enable_callback_tracebacks
	dbType           = py.NewTypeX("sqlite3.Connection", "", nil, nil)
	stmtType         = py.NewTypeX("sqlite3.Statement", "", nil, nil)
	backupType       = py.NewTypeX("sqlite3.Backup", "", nil, nil)
	blobType         = py.NewTypeX("sqlite3.RawBlob", "", nil, nil)
	extCodeNames     = map[int]string{}
	sqliteTransient  = ^uintptr(0)
	ptrSize          = unsafe.Sizeof(uintptr(0))
	errNoCallbackMsg = "user-defined function raised exception"
)

// newExc makes sqlite3.<name>; the dotted Name builtin types use would leave
// __name__ wrong and __module__ unset.
func newExc(base *py.Type, name string) *py.Type {
	t := base.NewType(name, "", nil, nil)
	t.Dict.Set("__module__", py.String("sqlite3"))
	return t
}

// resultCodes are the leading words of SQLITE_* names that are result codes
// (the others - LIMIT_, DBCONFIG_, READ, UPDATE... - share their values).
var resultCodes = map[string]bool{}

func init() {
	for _, w := range strings.Fields("ABORT AUTH BUSY CANTOPEN CONSTRAINT CORRUPT EMPTY ERROR FORMAT FULL INTERNAL INTERRUPT IOERR LOCKED MISMATCH MISUSE NOLFS NOMEM NOTADB NOTFOUND NOTICE PERM PROTOCOL RANGE READONLY SCHEMA TOOBIG WARNING") {
		resultCodes[w] = true
	}
}

// Db is one sqlite3* with its own libc thread state.  A connection is used by
// one Python thread at a time (the Python layer enforces check_same_thread),
// so no lock guards tls; only the finalizer queue is shared across goroutines.
type Db struct {
	tls  *libc.TLS
	db   uintptr
	ids  []uintptr // callback registry ids to drop on close
	live map[*Stmt]struct{}

	mu      sync.Mutex
	pending []uintptr // sqlite3_stmt* of unreachable Stmts, finalized on the next call
}

func (d *Db) Type() *py.Type { return dbType }

// Stmt is one sqlite3_stmt*.
type Stmt struct {
	db *Db
	p  uintptr
}

func (s *Stmt) Type() *py.Type { return stmtType }

// ---- errors ----

// errcode names, from the SQLITE_* constants CPython exports.
func codeName(code int) string {
	if n, ok := extCodeNames[code]; ok {
		return n
	}
	return ""
}

// sqliteError builds the exception _pysqlite_seterror would raise for rc.
func (d *Db) sqliteError(rc int32) *py.Exception {
	ext := int(lib.Xsqlite3_extended_errcode(d.tls, d.db))
	if ext&0xff != int(rc)&0xff {
		ext = int(rc)
	}
	msg := libc.GoString(lib.Xsqlite3_errmsg(d.tls, d.db))
	return newError(ext, msg)
}

func newError(ext int, msg string) *py.Exception {
	var t *py.Type
	switch ext & 0xff {
	case lib.SQLITE_INTERNAL, lib.SQLITE_NOTFOUND:
		t = errInternal
	case lib.SQLITE_NOMEM:
		t = py.MemoryError
	case lib.SQLITE_ERROR, lib.SQLITE_PERM, lib.SQLITE_ABORT, lib.SQLITE_BUSY, lib.SQLITE_LOCKED,
		lib.SQLITE_READONLY, lib.SQLITE_INTERRUPT, lib.SQLITE_IOERR, lib.SQLITE_FULL, lib.SQLITE_CANTOPEN,
		lib.SQLITE_PROTOCOL, lib.SQLITE_EMPTY, lib.SQLITE_SCHEMA:
		t = errOperational
	case lib.SQLITE_CORRUPT:
		t = errDatabase
	case lib.SQLITE_TOOBIG:
		t = errData
	case lib.SQLITE_CONSTRAINT, lib.SQLITE_MISMATCH:
		t = errIntegrity
	case lib.SQLITE_MISUSE, lib.SQLITE_RANGE:
		t = errInterface
	default:
		t = errDatabase
	}
	e := py.ExceptionNewf(t, "%s", msg)
	d := e.GetDict()
	d.Set("sqlite_errorcode", py.Int(ext))
	if n := codeName(ext); n != "" {
		d.Set("sqlite_errorname", py.String(n))
	} else {
		d.Set("sqlite_errorname", py.None)
	}
	return e
}

func progErr(format string, a ...interface{}) error {
	return py.ExceptionNewf(errProgramming, format, a...)
}

// ---- helpers ----

func (d *Db) cstr(s string) uintptr {
	p, err := libc.CString(s)
	if err != nil {
		panic(err)
	}
	return p
}

func (d *Db) free(p uintptr) {
	if p != 0 {
		libc.Xfree(d.tls, p)
	}
}

func (d *Db) malloc(n int) uintptr {
	p := libc.Xmalloc(d.tls, types.Size_t(n))
	if p == 0 && n != 0 {
		panic(py.ExceptionNewf(py.MemoryError, "sqlite: out of memory"))
	}
	return p
}

// drain finalizes statements whose Go owners were garbage collected.
func (d *Db) drain() {
	d.mu.Lock()
	p := d.pending
	d.pending = nil
	d.mu.Unlock()
	for _, s := range p {
		lib.Xsqlite3_finalize(d.tls, s)
	}
}

// locked retries f while SQLite reports a table lock.  An abandoned cursor
// (dropped mid-iteration) holds its statement - and so a read lock - until the
// Go GC notices; CPython frees it by refcount, so force the GC and retry.
func (d *Db) locked(f func() int32) int32 {
	d.drain()
	rc := f()
	for i := 0; i < 3 && rc&0xff == lib.SQLITE_LOCKED; i++ {
		runtime.GC()
		runtime.Gosched()
		d.drain()
		rc = f()
	}
	return rc
}

func (d *Db) open() error {
	if d.db == 0 {
		return progErr("Cannot operate on a closed database.")
	}
	return nil
}

func argsOf(name, format string, args py.Tuple, kw py.StringDict, names []string, out ...*py.Object) error {
	return py.ParseTupleAndKeywords(args, kw, format+":"+name, names, out...)
}

func toBool(o py.Object) bool {
	if o == nil {
		return false
	}
	t, err := py.MakeBool(o)
	return err == nil && t == py.True
}

func goInt(o py.Object) (int64, error) {
	i, err := py.MakeInt(o)
	if err != nil {
		return 0, err
	}
	switch v := i.(type) {
	case py.Int:
		return int64(v), nil
	case *py.BigInt:
		return v.GoInt64()
	}
	return 0, py.ExceptionNewf(py.TypeError, "an integer is required")
}

// ---- values ----

// pyToSQL binds the already-adapted Python value v to parameter idx.
func (s *Stmt) bindOne(idx int, v py.Object) error {
	d := s.db
	var rc int32
	switch x := v.(type) {
	case py.NoneType:
		rc = lib.Xsqlite3_bind_null(d.tls, s.p, int32(idx))
	case py.Bool:
		var n int64
		if x {
			n = 1
		}
		rc = lib.Xsqlite3_bind_int64(d.tls, s.p, int32(idx), n)
	case py.Int:
		rc = lib.Xsqlite3_bind_int64(d.tls, s.p, int32(idx), int64(x))
	case *py.BigInt:
		n, err := x.GoInt64()
		if err != nil {
			return py.ExceptionNewf(py.OverflowError, "Python int too large to convert to SQLite INTEGER")
		}
		rc = lib.Xsqlite3_bind_int64(d.tls, s.p, int32(idx), n)
	case py.Float:
		rc = lib.Xsqlite3_bind_double(d.tls, s.p, int32(idx), float64(x))
	case py.String:
		p := d.malloc(len(x))
		copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(x)), string(x))
		rc = lib.Xsqlite3_bind_text(d.tls, s.p, int32(idx), p, int32(len(x)), sqliteTransient)
		d.free(p)
	case py.Bytes:
		rc = s.bindBlob(idx, x)
	case *py.ByteArray:
		b, err := py.BytesFromObject(x)
		if err != nil {
			return err
		}
		rc = s.bindBlob(idx, b)
	default:
		return progErr("Error binding parameter %d: type '%s' is not supported", idx, v.Type().Name)
	}
	if rc != lib.SQLITE_OK {
		return d.sqliteError(rc)
	}
	return nil
}

func (s *Stmt) bindBlob(idx int, b []byte) int32 {
	d := s.db
	if len(b) == 0 {
		return lib.Xsqlite3_bind_zeroblob(d.tls, s.p, int32(idx), 0)
	}
	p := d.malloc(len(b))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(b)), b)
	rc := lib.Xsqlite3_bind_blob(d.tls, s.p, int32(idx), p, int32(len(b)), sqliteTransient)
	d.free(p)
	return rc
}

// column reads column i.  raw forces BLOB bytes (for converters); mode is how
// TEXT is returned: 0 str, 1 bytes, 2 tf(bytes) for a custom text_factory.
func (s *Stmt) column(i int, raw bool, mode int, tf py.Object) (py.Object, error) {
	d := s.db
	t := lib.Xsqlite3_column_type(d.tls, s.p, int32(i))
	if t == lib.SQLITE_NULL {
		return py.None, nil
	}
	if raw {
		return s.colBytes(i, true), nil
	}
	switch t {
	case lib.SQLITE_INTEGER:
		return py.Int(lib.Xsqlite3_column_int64(d.tls, s.p, int32(i))), nil
	case lib.SQLITE_FLOAT:
		return py.Float(lib.Xsqlite3_column_double(d.tls, s.p, int32(i))), nil
	case lib.SQLITE_TEXT:
		b := s.colBytes(i, false)
		switch mode {
		case 1:
			return b, nil
		case 2:
			return py.Call(tf, py.Tuple{b}, py.NewStringDict())
		}
		if !utf8.Valid(b) {
			name := libc.GoString(lib.Xsqlite3_column_name(d.tls, s.p, int32(i)))
			return nil, py.ExceptionNewf(errOperational, "Could not decode to UTF-8 column '%s' with text '%s'",
				name, strings.ToValidUTF8(string(b), "\ufffd"))
		}
		return py.String(b), nil
	}
	return s.colBytes(i, true), nil
}

func (s *Stmt) colBytes(i int, blob bool) py.Bytes {
	d := s.db
	var p uintptr
	if blob {
		p = lib.Xsqlite3_column_blob(d.tls, s.p, int32(i))
	} else {
		p = lib.Xsqlite3_column_text(d.tls, s.p, int32(i))
	}
	n := int(lib.Xsqlite3_column_bytes(d.tls, s.p, int32(i)))
	b := make(py.Bytes, n)
	if n > 0 {
		copy(b, unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
	}
	return b
}

// ---- Db methods ----

func connect(_ py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
	var name, timeout, uri py.Object
	if err := argsOf("connect", "OOO", args, kw, []string{"database", "timeout", "uri"}, &name, &timeout, &uri); err != nil {
		return nil, err
	}
	d := &Db{tls: libc.NewTLS(), live: map[*Stmt]struct{}{}}
	flags := int32(lib.SQLITE_OPEN_READWRITE | lib.SQLITE_OPEN_CREATE)
	if toBool(uri) {
		flags |= lib.SQLITE_OPEN_URI
	}
	fn := d.cstr(string(name.(py.String)))
	pp := d.malloc(int(ptrSize))
	rc := lib.Xsqlite3_open_v2(d.tls, fn, pp, flags, 0)
	d.db = *(*uintptr)(unsafe.Pointer(pp))
	d.free(fn)
	d.free(pp)
	if rc != lib.SQLITE_OK {
		var e error = newError(int(rc), "unable to open database file")
		if d.db != 0 {
			e = d.sqliteError(rc)
			lib.Xsqlite3_close_v2(d.tls, d.db)
		}
		d.tls.Close()
		return nil, e
	}
	lib.Xsqlite3_extended_result_codes(d.tls, d.db, 1)
	if t, err := py.MakeFloat(timeout); err == nil {
		lib.Xsqlite3_busy_timeout(d.tls, d.db, int32(float64(t.(py.Float))*1000))
	}
	return d, nil
}

func (d *Db) closeDb() (py.Object, error) {
	if d.db == 0 {
		return py.None, nil
	}
	d.drain()
	for s := range d.live {
		s.finalize()
	}
	for _, id := range d.ids {
		cbDrop(id)
	}
	d.ids = nil
	lib.Xsqlite3_close_v2(d.tls, d.db)
	d.db = 0
	d.tls.Close()
	return py.None, nil
}

// prepare compiles sql.  Empty statements (blank / comment only) give None.
// With single set, a second statement after the first is a ProgrammingError.
func (d *Db) prepare(args py.Tuple) (py.Object, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	sql, ok := args[0].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "sql must be a string")
	}
	single := toBool(args[1])
	if int32(len(sql)) < 0 || int32(len(sql)) > lib.Xsqlite3_limit(d.tls, d.db, lib.SQLITE_LIMIT_SQL_LENGTH, -1) {
		return nil, py.ExceptionNewf(errData, "query string is too large")
	}
	z := d.cstr(string(sql))
	defer d.free(z)
	pp := d.malloc(2 * int(ptrSize))
	defer d.free(pp)
	var st, tail uintptr
	rc := d.locked(func() int32 {
		r := lib.Xsqlite3_prepare_v2(d.tls, d.db, z, -1, pp, pp+ptrSize)
		st, tail = *(*uintptr)(unsafe.Pointer(pp)), *(*uintptr)(unsafe.Pointer(pp + ptrSize))
		return r
	})
	if rc != lib.SQLITE_OK {
		return nil, d.sqliteError(rc)
	}
	if st == 0 {
		return py.None, nil
	}
	if single && tail != 0 {
		// Anything but whitespace/comments after the statement is a second one.
		var st2 uintptr
		r := lib.Xsqlite3_prepare_v2(d.tls, d.db, tail, -1, pp, pp+ptrSize)
		st2 = *(*uintptr)(unsafe.Pointer(pp))
		if st2 != 0 {
			lib.Xsqlite3_finalize(d.tls, st2)
		}
		if st2 != 0 || r != lib.SQLITE_OK {
			lib.Xsqlite3_finalize(d.tls, st)
			return nil, progErr("You can only execute one statement at a time.")
		}
	}
	s := &Stmt{db: d, p: st}
	d.live[s] = struct{}{}
	runtime.SetFinalizer(s, func(s *Stmt) {
		if s.p != 0 {
			s.db.mu.Lock()
			s.db.pending = append(s.db.pending, s.p)
			s.db.mu.Unlock()
		}
	})
	return s, nil
}

func (d *Db) exec(sql string) error {
	if err := d.open(); err != nil {
		return err
	}
	z := d.cstr(sql)
	defer d.free(z)
	rc := d.locked(func() int32 { return lib.Xsqlite3_exec(d.tls, d.db, z, 0, 0, 0) })
	if rc != lib.SQLITE_OK {
		return d.sqliteError(rc)
	}
	return nil
}

func (s *Stmt) finalize() {
	if s.p != 0 {
		lib.Xsqlite3_finalize(s.db.tls, s.p)
		s.p = 0
		delete(s.db.live, s)
	}
}

func (s *Stmt) live() error {
	if s.p == 0 {
		return progErr("Cannot operate on a finalized statement.")
	}
	return nil
}

func (s *Stmt) step() (py.Object, error) {
	if err := s.live(); err != nil {
		return nil, err
	}
	d := s.db
	rc := d.locked(func() int32 { return lib.Xsqlite3_step(d.tls, s.p) })
	switch rc {
	case lib.SQLITE_ROW:
		return py.True, nil
	case lib.SQLITE_DONE:
		return py.False, nil
	}
	return nil, d.sqliteError(rc)
}

func (s *Stmt) row(args py.Tuple) (py.Object, error) {
	if err := s.live(); err != nil {
		return nil, err
	}
	n := int(lib.Xsqlite3_column_count(s.db.tls, s.p))
	mode, _ := goInt(args[0])
	var raw py.Tuple
	if t, ok := args[1].(py.Tuple); ok {
		raw = t
	}
	out := make(py.Tuple, n)
	for i := range out {
		v, err := s.column(i, i < len(raw) && toBool(raw[i]), int(mode), args[2])
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (s *Stmt) bind(args py.Tuple) (py.Object, error) {
	if err := s.live(); err != nil {
		return nil, err
	}
	vals, ok := args[0].(py.Tuple)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "bind() needs a tuple")
	}
	for i, v := range vals {
		if err := s.bindOne(i+1, v); err != nil {
			return nil, err
		}
	}
	return py.None, nil
}

func (s *Stmt) names(decl bool) (py.Object, error) {
	if err := s.live(); err != nil {
		return nil, err
	}
	d := s.db
	n := int(lib.Xsqlite3_column_count(d.tls, s.p))
	out := make(py.Tuple, n)
	for i := range out {
		var p uintptr
		if decl {
			p = lib.Xsqlite3_column_decltype(d.tls, s.p, int32(i))
		} else {
			p = lib.Xsqlite3_column_name(d.tls, s.p, int32(i))
		}
		if p == 0 {
			out[i] = py.None
		} else {
			out[i] = py.String(libc.GoString(p))
		}
	}
	return out, nil
}

// paramNames returns one entry per parameter: its name (":x", "?1", ...) or
// None for a bare "?".
func (s *Stmt) paramNames() (py.Object, error) {
	if err := s.live(); err != nil {
		return nil, err
	}
	d := s.db
	n := int(lib.Xsqlite3_bind_parameter_count(d.tls, s.p))
	out := make(py.Tuple, n)
	for i := range out {
		p := lib.Xsqlite3_bind_parameter_name(d.tls, s.p, int32(i+1))
		if p == 0 {
			out[i] = py.None
		} else {
			out[i] = py.String(libc.GoString(p))
		}
	}
	return out, nil
}

func (d *Db) setLimit(args py.Tuple) (py.Object, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	cat, err := goInt(args[0])
	if err != nil {
		return nil, err
	}
	nv, err := goInt(args[1])
	if err != nil {
		return nil, err
	}
	r := lib.Xsqlite3_limit(d.tls, d.db, int32(cat), int32(nv))
	if r < 0 {
		return nil, py.ExceptionNewf(py.ValueError, "'category' is out of bounds")
	}
	return py.Int(r), nil
}

func (d *Db) dbConfig(args py.Tuple) (py.Object, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	op, err := goInt(args[0])
	if err != nil {
		return nil, err
	}
	v, err := goInt(args[1])
	if err != nil {
		return nil, err
	}
	pp := d.malloc(8)
	defer d.free(pp)
	va := libc.NewVaList(int32(v), pp)
	defer libc.Xfree(d.tls, va)
	rc := lib.Xsqlite3_db_config(d.tls, d.db, int32(op), va)
	if rc != lib.SQLITE_OK {
		return nil, py.ExceptionNewf(errOperational, "unknown config option")
	}
	return py.Int(*(*int32)(unsafe.Pointer(pp))), nil
}

func (d *Db) serialize(name string) (py.Object, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	z := d.cstr(name)
	defer d.free(z)
	pl := d.malloc(8)
	defer d.free(pl)
	p := lib.Xsqlite3_serialize(d.tls, d.db, z, pl, 0)
	if p == 0 {
		// An empty database serialises to NULL with size 0 when not in memory.
		p = lib.Xsqlite3_serialize(d.tls, d.db, z, pl, lib.SQLITE_SERIALIZE_NOCOPY)
	}
	n := int(*(*int64)(unsafe.Pointer(pl)))
	if p == 0 {
		return nil, py.ExceptionNewf(py.MemoryError, "serialize failed")
	}
	b := make(py.Bytes, n)
	copy(b, unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
	return b, nil
}

func (d *Db) deserialize(name string, data []byte) (py.Object, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	z := d.cstr(name)
	defer d.free(z)
	p := lib.Xsqlite3_malloc64(d.tls, uint64(len(data)))
	if p == 0 && len(data) != 0 {
		return nil, py.ExceptionNewf(py.MemoryError, "deserialize failed")
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	rc := lib.Xsqlite3_deserialize(d.tls, d.db, z, p, int64(len(data)), int64(len(data)),
		lib.SQLITE_DESERIALIZE_FREEONCLOSE|lib.SQLITE_DESERIALIZE_RESIZEABLE)
	if rc != lib.SQLITE_OK {
		return nil, d.sqliteError(rc)
	}
	return py.None, nil
}

func init() {
	def := func(t *py.Type, name string, f interface{}) {
		t.Dict.Set(name, py.MustNewMethod(name, f, 0, ""))
	}
	type A = py.Tuple
	type O = py.Object
	// Db
	def(dbType, "close", func(self O, a A) (O, error) { return self.(*Db).closeDb() })
	def(dbType, "prepare", func(self O, a A) (O, error) { return self.(*Db).prepare(a) })
	def(dbType, "exec", func(self O, a A) (O, error) {
		if err := self.(*Db).exec(string(a[0].(py.String))); err != nil {
			return nil, err
		}
		return py.None, nil
	})
	def(dbType, "autocommit", func(self O, a A) (O, error) {
		d := self.(*Db)
		if err := d.open(); err != nil {
			return nil, err
		}
		return py.NewBool(lib.Xsqlite3_get_autocommit(d.tls, d.db) != 0), nil
	})
	def(dbType, "closed", func(self O, a A) (O, error) { return py.NewBool(self.(*Db).db == 0), nil })
	def(dbType, "last_rowid", func(self O, a A) (O, error) {
		d := self.(*Db)
		return py.Int(lib.Xsqlite3_last_insert_rowid(d.tls, d.db)), nil
	})
	def(dbType, "changes", func(self O, a A) (O, error) {
		d := self.(*Db)
		return py.Int(lib.Xsqlite3_changes(d.tls, d.db)), nil
	})
	def(dbType, "total_changes", func(self O, a A) (O, error) {
		d := self.(*Db)
		if err := d.open(); err != nil {
			return nil, err
		}
		return py.Int(lib.Xsqlite3_total_changes(d.tls, d.db)), nil
	})
	def(dbType, "interrupt", func(self O, a A) (O, error) {
		d := self.(*Db)
		if err := d.open(); err != nil {
			return nil, err
		}
		lib.Xsqlite3_interrupt(d.tls, d.db)
		return py.None, nil
	})
	def(dbType, "limit", func(self O, a A) (O, error) { return self.(*Db).setLimit(a) })
	def(dbType, "config", func(self O, a A) (O, error) { return self.(*Db).dbConfig(a) })
	def(dbType, "serialize", func(self O, a A) (O, error) { return self.(*Db).serialize(string(a[0].(py.String))) })
	def(dbType, "deserialize", func(self O, a A) (O, error) {
		b, err := py.BytesFromObject(a[1])
		if err != nil {
			return nil, err
		}
		return self.(*Db).deserialize(string(a[0].(py.String)), b)
	})
	registerCallbacks(def)
	registerBackupBlob(def)
	// Stmt
	def(stmtType, "step", func(self O, a A) (O, error) { return self.(*Stmt).step() })
	def(stmtType, "reset", func(self O, a A) (O, error) {
		if s := self.(*Stmt); s.p != 0 {
			lib.Xsqlite3_reset(s.db.tls, s.p)
		}
		return py.None, nil
	})
	def(stmtType, "finalize", func(self O, a A) (O, error) { self.(*Stmt).finalize(); return py.None, nil })
	def(stmtType, "bind", func(self O, a A) (O, error) { return self.(*Stmt).bind(a) })
	def(stmtType, "row", func(self O, a A) (O, error) { return self.(*Stmt).row(a) })
	def(stmtType, "colnames", func(self O, a A) (O, error) { return self.(*Stmt).names(false) })
	def(stmtType, "decltypes", func(self O, a A) (O, error) { return self.(*Stmt).names(true) })
	def(stmtType, "paramnames", func(self O, a A) (O, error) { return self.(*Stmt).paramNames() })
	def(stmtType, "readonly", func(self O, a A) (O, error) {
		s := self.(*Stmt)
		if err := s.live(); err != nil {
			return nil, err
		}
		return py.NewBool(lib.Xsqlite3_stmt_readonly(s.db.tls, s.p) != 0), nil
	})

	g := py.NewStringDict()
	g.Set("_connect", py.MustNewMethod("_connect", connect, 0, ""))
	g.Set("_set_callback_tracebacks", py.MustNewMethod("_set_callback_tracebacks", func(self O, a A) (O, error) {
		callbackTrace = toBool(a[0])
		return py.None, nil
	}, 0, ""))
	g.Set("complete_statement", py.MustNewMethod("complete_statement", func(self O, a A) (O, error) {
		s, ok := a[0].(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "complete_statement() argument 'statement' must be str, not %s", a[0].Type().Name)
		}
		tls := libc.NewTLS()
		defer tls.Close()
		z, err := libc.CString(string(s))
		if err != nil {
			return nil, err
		}
		defer libc.Xfree(tls, z)
		return py.NewBool(lib.Xsqlite3_complete(tls, z) != 0), nil
	}, 0, "Checks if a string contains a complete SQL statement."))
	tls := libc.NewTLS()
	g.Set("sqlite_version", py.String(libc.GoString(lib.Xsqlite3_libversion(tls))))
	tls.Close()
	g.Set("threadsafety", py.Int(3))
	g.Set("PARSE_DECLTYPES", py.Int(1))
	g.Set("PARSE_COLNAMES", py.Int(2))
	g.Set("LEGACY_TRANSACTION_CONTROL", py.Int(-1))
	for _, c := range constants {
		g.Set(c.name, py.Int(c.val))
		if first, _, _ := strings.Cut(c.name[len("SQLITE_"):], "_"); resultCodes[first] {
			extCodeNames[c.val] = c.name
		}
	}
	// Result codes are named by their primary (non-"_xxx") SQLITE_* constant;
	// the table above also holds non-error constants, which never reach newError.
	for _, e := range []*py.Type{errWarning, errError, errInterface, errDatabase, errData,
		errOperational, errIntegrity, errInternal, errProgramming, errNotSupported} {
		g.Set(e.Name, e)
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_sqlite3", FileDesc: "_sqlite3.py", Doc: "C-level SQLite3 interface, backed by modernc.org/sqlite."},
		Globals: g,
		CodeSrc: pyAPI,
	})
}
