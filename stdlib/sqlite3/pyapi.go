// Copyright 2025 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

// pyAPI is the Python half of `_sqlite3`: the public classes and functions,
// written against the _connect/Db/Stmt primitives of sqlite3.go.  It mirrors
// CPython's Modules/_sqlite (connection.c, cursor.c, row.c, blob.c,
// microprotocols.c) so that observable behaviour - rowcount, lastrowid,
// implicit transactions, error messages - matches.
const pyAPI = `
import os as _os
import threading as _threading

_MISSING = object()
_SIMPLE = (int, float, str, bytes, bytearray)
_BEGIN_ISO = ("DEFERRED", "IMMEDIATE", "EXCLUSIVE")


class PrepareProtocol:
    __module__ = "sqlite3"

    def __init__(self, *args, **kwargs):
        pass


adapters = {}
converters = {}


def register_adapter(type, caster, /):
    adapters[(type, PrepareProtocol)] = caster


def register_converter(typename, converter, /):
    if not isinstance(typename, str):
        raise TypeError("register_converter() argument 1 must be str, not %s" % type(typename).__name__)
    converters[typename.upper()] = converter


def adapt(obj, proto=PrepareProtocol, alt=_MISSING, /):
    ad = adapters.get((type(obj), proto))
    if ad is not None:
        return ad(obj)
    meth = getattr(proto, "__adapt__", None)
    if meth is not None:
        try:
            return meth(obj)
        except TypeError:
            pass
    meth = getattr(obj, "__conform__", None)
    if meth is not None:
        try:
            return meth(proto)
        except TypeError:
            pass
    if alt is not _MISSING:
        return alt
    raise ProgrammingError("can't adapt")


def enable_callback_tracebacks(enable, /):
    _set_callback_tracebacks(enable)


def _bindval(v):
    if v is None or type(v) in _SIMPLE:
        return v
    v = adapt(v, PrepareProtocol, v)
    if v is None or type(v) in _SIMPLE or type(v) is bool:
        return v
    if isinstance(v, memoryview):
        return v.tobytes()
    if isinstance(v, int):
        return int(v)
    if isinstance(v, float):
        return float(v)
    if isinstance(v, str):
        return str(v)
    if isinstance(v, (bytes, bytearray)):
        return bytes(v)
    return v


def _is_dml(sql):
    i = 0
    n = len(sql)
    while i < n:
        c = sql[i]
        if c in " \t\n\r\f":
            i += 1
        elif sql.startswith("--", i):
            j = sql.find("\n", i)
            i = n if j < 0 else j + 1
        elif sql.startswith("/*", i):
            j = sql.find("*/", i + 2)
            i = n if j < 0 else j + 2
        else:
            break
    head = sql[i:i + 7].lower()
    return head[:6] in ("insert", "update", "delete") or head == "replace"


class Row:
    __module__ = "sqlite3"

    def __init__(self, cursor, data):
        if not isinstance(cursor, Cursor):
            raise TypeError("argument 1 must be sqlite3.Cursor, not %s" % type(cursor).__name__)
        if type(data) is not tuple:
            raise TypeError("argument 2 must be tuple, not %s" % type(data).__name__)
        self._data = data
        self._desc = cursor.description

    def keys(self):
        if self._desc is None:
            return []
        return [d[0] for d in self._desc]

    def __getitem__(self, idx):
        if isinstance(idx, int):
            return self._data[idx]
        if isinstance(idx, str):
            low = idx.lower()
            for i, d in enumerate(self._desc or ()):
                if d[0].lower() == low:
                    return self._data[i]
            raise IndexError("No item with that key")
        if isinstance(idx, slice):
            return self._data[idx]
        raise IndexError("Index must be int or string")

    def __len__(self):
        return len(self._data)

    def __iter__(self):
        return iter(self._data)

    def __hash__(self):
        return hash(self._desc) ^ hash(self._data)

    def __eq__(self, other):
        if not isinstance(other, Row):
            return NotImplemented
        return self._desc == other._desc and self._data == other._data

    def __ne__(self, other):
        if not isinstance(other, Row):
            return NotImplemented
        return not (self._desc == other._desc and self._data == other._data)


class Cursor:
    __module__ = "sqlite3"
    _ready = False

    def __init__(self, connection):
        if not isinstance(connection, Connection):
            raise TypeError("Cursor() argument 1 must be sqlite3.Connection, not %s" % type(connection).__name__)
        self.connection = connection
        self.arraysize = 1
        self.row_factory = None
        self.rowcount = -1
        self.lastrowid = None
        self.description = None
        self._stmt = None
        self._closed = False
        self._convs = None
        self._dml = False
        self._ready = True

    def _check(self):
        if not self._ready:
            raise ProgrammingError("Base Cursor.__init__ not called.")
        con = self.connection
        con._check_thread()
        if self._closed:
            raise ProgrammingError("Cannot operate on a closed cursor.")
        con._check_open()

    def _release(self):
        st = self._stmt
        if st is not None:
            self._stmt = None
            st.reset()

    def _build_desc(self, st):
        names = st.colnames()
        if not names:
            return
        det = self.connection._detect_types
        decls = st.decltypes() if det else None
        convs = [None] * len(names)
        out = []
        for i, name in enumerate(names):
            if det & PARSE_COLNAMES:
                start = None
                for pos, ch in enumerate(name):
                    if ch == "[":
                        start = pos + 1
                    elif ch == "]" and start is not None:
                        convs[i] = converters.get(name[start:pos].upper())
                        break
            if det & PARSE_DECLTYPES and convs[i] is None and decls[i] is not None:
                decl = decls[i]
                end = 0
                while end < len(decl) and decl[end] not in " (":
                    end += 1
                convs[i] = converters.get(decl[:end].upper())
            if det & PARSE_COLNAMES:
                pos = name.find("[")
                if pos >= 0:
                    if pos > 0 and name[pos - 1] == " ":
                        pos -= 1
                    name = name[:pos]
            out.append((name, None, None, None, None, None, None))
        self.description = tuple(out)
        self._convs = convs if any(c is not None for c in convs) else None

    def _params(self, st, params):
        names = st.paramnames()
        need = len(names)
        if isinstance(params, dict) or (not isinstance(params, (tuple, list)) and hasattr(params, "keys") and hasattr(params, "__getitem__")):
            vals = []
            for i, nm in enumerate(names):
                if nm is None:
                    raise ProgrammingError("Binding %d has no name, but you supplied a dictionary (which has only names)." % (i + 1))
                try:
                    vals.append(params[nm[1:]])
                except KeyError:
                    raise ProgrammingError("You did not supply a value for binding parameter %s." % nm) from None
        elif isinstance(params, (tuple, list)) or (hasattr(params, "__len__") and hasattr(params, "__getitem__")):
            if len(params) != need:
                raise ProgrammingError("Incorrect number of bindings supplied. The current statement uses %d, and there are %d supplied." % (need, len(params)))
            for i, nm in enumerate(names):
                if nm is not None and nm[0] != "?":
                    raise ProgrammingError("Binding %d ('%s') is a named parameter, but you supplied a sequence which requires nameless (qmark) placeholders." % (i + 1, nm))
            vals = [params[i] for i in range(need)]
        else:
            raise ProgrammingError("parameters are of unsupported type")
        st.bind(tuple([_bindval(v) for v in vals]))

    def _execute(self, sql, seq, multiple):
        self._check()
        if not isinstance(sql, str):
            raise TypeError("%s() argument 1 must be str, not %s" % ("executemany" if multiple else "execute", type(sql).__name__))
        con = self.connection
        db = con._db
        self.description = None
        self._convs = None
        self.rowcount = 0
        self._release()
        if multiple:
            it = iter(seq)
        else:
            it = iter((seq,))
        try:
            st = db.prepare(sql, True)
            if st is None:
                self.rowcount = -1
                return self
            dml = self._dml = _is_dml(sql)
            self.rowcount = 0 if dml else -1
            if con._autocommit == LEGACY_TRANSACTION_CONTROL and con._isolation_level is not None and dml and db.autocommit():
                db.exec(con._begin)
            if multiple and st.readonly():
                st.finalize()
                raise ProgrammingError("executemany() can only execute DML statements.")
            more = False
            for params in it:
                st.reset()
                self._params(st, params)
                more = st.step()
                if self.description is None:
                    self._build_desc(st)
                if multiple:
                    if dml and not more:
                        self.rowcount += db.changes()
                elif more:
                    break
                elif dml:
                    self.rowcount += db.changes()
            if multiple or not more:
                st.reset()
                self._stmt = None
            else:
                self._stmt = st
            if not multiple:
                self.lastrowid = db.last_rowid()
        except BaseException:
            self.rowcount = -1
            try:
                st.reset()
            except BaseException:
                pass
            self._stmt = None
            raise
        return self

    def execute(self, sql, parameters=()):
        return self._execute(sql, parameters, False)

    def executemany(self, sql, seq_of_parameters):
        return self._execute(sql, seq_of_parameters, True)

    def executescript(self, sql_script):
        self._check()
        if not isinstance(sql_script, str):
            raise TypeError("executescript() argument must be str, not %s" % type(sql_script).__name__)
        con = self.connection
        self._release()
        if con._autocommit == LEGACY_TRANSACTION_CONTROL and not con._db.autocommit():
            con._db.exec("COMMIT")
        con._db.exec(sql_script)
        return self

    def __iter__(self):
        return self

    def __next__(self):
        self._check()
        st = self._stmt
        if st is None:
            raise StopIteration
        con = self.connection
        tf = con.text_factory
        mode = 0 if tf is str else 1 if tf is bytes else 2
        convs = self._convs
        raw = None if convs is None else tuple([c is not None for c in convs])
        try:
            row = st.row(mode, raw, tf)
            if convs is not None:
                row = tuple([v if c is None or v is None else c(v) for v, c in zip(row, convs)])
            rf = self.row_factory
            if rf is not None:
                row = rf(self, row)
            more = st.step()
        except BaseException:
            self._release()
            raise
        if not more:
            if self._dml:
                self.rowcount += con._db.changes()
            self._release()
        return row

    def fetchone(self):
        try:
            return self.__next__()
        except StopIteration:
            return None

    def fetchmany(self, size=None):
        if size is None:
            size = self.arraysize
        out = []
        while len(out) < size:
            try:
                out.append(self.__next__())
            except StopIteration:
                break
        return out

    def fetchall(self):
        out = []
        while True:
            try:
                out.append(self.__next__())
            except StopIteration:
                return out

    def close(self):
        if not self._ready:
            raise ProgrammingError("Base Cursor.__init__ not called.")
        self.connection._check_thread()
        if self._closed:
            self.connection._check_open()
            return
        self.connection._check_open()
        self._release()
        self._closed = True

    def setinputsizes(self, sizes):
        pass

    def setoutputsize(self, size, column=None):
        pass


def _slice_indices(sl, size):
    step = 1 if sl.step is None else sl.step
    if step == 0:
        raise ValueError("slice step cannot be zero")
    lo, hi = (-1, size - 1) if step < 0 else (0, size)
    def clamp(v, default):
        if v is None:
            return default
        if v < 0:
            v += size
        return min(max(v, lo), hi)
    return clamp(sl.start, hi if step < 0 else lo), clamp(sl.stop, lo if step < 0 else hi), step


class Blob:
    __module__ = "sqlite3"

    def __init__(self, raw, con):
        self._raw = raw
        self._con = con
        self._pos = 0

    def _check(self):
        self._con._check_thread()
        if self._raw.closed():
            raise ProgrammingError("Cannot operate on a closed blob.")
        self._con._check_open()

    def close(self):
        self._con._check_thread()
        self._raw.close()

    def __len__(self):
        self._check()
        return self._raw.size()

    def read(self, length=-1, /):
        self._check()
        size = self._raw.size()
        if length < 0 or length > size - self._pos:
            length = size - self._pos
        data = self._raw.read(length, self._pos)
        self._pos += length
        return data

    def write(self, data, /):
        self._check()
        data = bytes(data)
        if len(data) > self._raw.size() - self._pos:
            raise ValueError("data longer than blob length")
        self._raw.write(data, self._pos)
        self._pos += len(data)

    def tell(self):
        self._check()
        return self._pos

    def seek(self, offset, origin=0, /):
        self._check()
        size = self._raw.size()
        if origin == 0:
            pos = offset
        elif origin == 1:
            pos = self._pos + offset
        elif origin == 2:
            pos = size + offset
        else:
            raise ValueError("'origin' should be os.SEEK_SET, os.SEEK_CUR, or os.SEEK_END")
        if pos < 0 or pos > size:
            raise ValueError("offset out of blob range")
        self._pos = pos

    def __enter__(self):
        self._check()
        return self

    def __exit__(self, exc_type, exc, tb):
        self.close()
        return False

    def __getitem__(self, key):
        self._check()
        size = self._raw.size()
        if isinstance(key, slice):
            start, stop, step = _slice_indices(key, size)
            if step == 1:
                return self._raw.read(max(stop - start, 0), start)
            return bytes([self._raw.read(1, i)[0] for i in range(start, stop, step)])
        if not isinstance(key, int):
            raise TypeError("Blob indices must be integers")
        i = int(key)
        if i < 0:
            i += size
        if i < 0 or i >= size:
            raise IndexError("Blob index out of range")
        return self._raw.read(1, i)[0]

    def __setitem__(self, key, value):
        self._check()
        size = self._raw.size()
        if isinstance(key, slice):
            start, stop, step = _slice_indices(key, size)
            idx = list(range(start, stop, step))
            value = bytes(value)
            if len(value) != len(idx):
                raise IndexError("Blob slice assignment is wrong size")
            for i, b in zip(idx, value):
                self._raw.write(bytes([b]), i)
            return
        if not isinstance(key, int):
            raise TypeError("Blob indices must be integers")
        i = int(key)
        if i < 0:
            i += size
        if i < 0 or i >= size:
            raise IndexError("Blob index out of range")
        if not isinstance(value, int):
            raise TypeError("Blob assignment must be an int")
        if value < 0 or value > 255:
            raise ValueError("byte must be in range(0, 256)")
        self._raw.write(bytes([value]), i)


class Connection:
    __module__ = "sqlite3"
    _db = None
    Warning = Warning
    Error = Error
    InterfaceError = InterfaceError
    DatabaseError = DatabaseError
    DataError = DataError
    OperationalError = OperationalError
    IntegrityError = IntegrityError
    InternalError = InternalError
    ProgrammingError = ProgrammingError
    NotSupportedError = NotSupportedError

    def __init__(self, database, timeout=5.0, detect_types=0, isolation_level="",
                 check_same_thread=True, factory=None, cached_statements=128, uri=False,
                 *, autocommit=LEGACY_TRANSACTION_CONTROL):
        if isinstance(database, (bytes, bytearray)):
            database = bytes(database).decode("utf-8", "surrogateescape")
        else:
            database = _os.fsdecode(_os.fspath(database))
        if autocommit not in (True, False, LEGACY_TRANSACTION_CONTROL) or isinstance(autocommit, int) and autocommit not in (True, False, -1):
            raise ValueError("autocommit must be True, False, or sqlite3.LEGACY_TRANSACTION_CONTROL")
        self._isolation_level = None
        self._begin = "BEGIN "
        self._set_isolation(isolation_level)
        self._autocommit = autocommit
        self._detect_types = detect_types
        self._check_same_thread = check_same_thread
        self._thread = _threading.get_ident()
        self.row_factory = None
        self.text_factory = str
        self._db = _connect(database, timeout, uri)
        if autocommit is False:
            self._db.exec("BEGIN")

    def _check_thread(self):
        if self._db is None:
            raise ProgrammingError("Base Connection.__init__ not called.")
        if self._check_same_thread and self._thread != _threading.get_ident():
            raise ProgrammingError("SQLite objects created in a thread can only be used in that same thread. The object was created in thread id %d and this is thread id %d." % (self._thread, _threading.get_ident()))

    def _check_open(self):
        if self._db.closed():
            raise ProgrammingError("Cannot operate on a closed database.")

    def _ok(self):
        self._check_thread()
        self._check_open()

    def _set_isolation(self, level):
        if level is None:
            self._isolation_level = None
            self._begin = ""
            return
        if not isinstance(level, str):
            raise TypeError("isolation_level must be str or None")
        if level != "" and level.upper() not in _BEGIN_ISO:
            raise ValueError("isolation_level string must be '', 'DEFERRED', 'IMMEDIATE', or 'EXCLUSIVE'")
        level = level.upper()
        self._isolation_level = level
        self._begin = "BEGIN " + level

    @property
    def isolation_level(self):
        self._ok()
        return self._isolation_level

    @isolation_level.setter
    def isolation_level(self, level):
        self._ok()
        self._set_isolation(level)

    @property
    def autocommit(self):
        self._ok()
        return self._autocommit

    @autocommit.setter
    def autocommit(self, value):
        self._ok()
        if value not in (True, False, LEGACY_TRANSACTION_CONTROL) or (isinstance(value, int) and not isinstance(value, bool) and value != -1):
            raise ValueError("autocommit must be True, False, or sqlite3.LEGACY_TRANSACTION_CONTROL")
        if value is True:
            if not self._db.autocommit():
                self._db.exec("COMMIT")
        elif value is False:
            if self._db.autocommit():
                self._db.exec("BEGIN")
        self._autocommit = value

    @property
    def in_transaction(self):
        self._ok()
        return not self._db.autocommit()

    @property
    def total_changes(self):
        self._ok()
        return self._db.total_changes()

    def cursor(self, factory=Cursor):
        self._ok()
        cur = factory(self)
        if not isinstance(cur, Cursor):
            raise TypeError("factory must return a cursor, not %s" % type(cur).__name__)
        if self.row_factory is not None:
            cur.row_factory = self.row_factory
        return cur

    # CPython's execute* build the cursor with the connection's own factory,
    # not through an overridden cursor().
    def execute(self, sql, parameters=()):
        return Connection.cursor(self).execute(sql, parameters)

    def executemany(self, sql, parameters):
        return Connection.cursor(self).executemany(sql, parameters)

    def executescript(self, sql_script):
        return Connection.cursor(self).executescript(sql_script)

    def commit(self):
        self._ok()
        if self._autocommit is True:
            return
        if not self._db.autocommit():
            self._db.exec("COMMIT")
        if self._autocommit is False:
            self._db.exec("BEGIN")

    def rollback(self):
        self._ok()
        if self._autocommit is True:
            return
        if not self._db.autocommit():
            self._db.exec("ROLLBACK")
        if self._autocommit is False:
            self._db.exec("BEGIN")

    def close(self):
        self._check_thread()
        if self._db.closed():
            return
        if self._autocommit is False and not self._db.autocommit():
            self._db.exec("ROLLBACK")
        self._db.close()

    def __enter__(self):
        self._ok()
        return self

    def __exit__(self, exc_type, exc_value, exc_tb):
        self._ok()
        if exc_type is None:
            try:
                self.commit()
            except BaseException:
                self.rollback()
                raise
        else:
            self.rollback()
        return False

    def create_function(self, name, narg, func, *, deterministic=False):
        self._ok()
        self._db.create_function(name, narg, func, deterministic)

    def create_aggregate(self, name, n_arg, aggregate_class):
        self._ok()
        self._db.create_aggregate(name, n_arg, aggregate_class, False, False)

    def create_window_function(self, name, num_params, aggregate_class, /):
        self._ok()
        self._db.create_aggregate(name, num_params, aggregate_class, False, True)

    def create_collation(self, name, callback, /):
        self._ok()
        if not isinstance(name, str):
            raise TypeError("create_collation() argument 1 must be str, not %s" % type(name).__name__)
        if callback is not None and not callable(callback):
            raise TypeError("parameter must be callable")
        self._db.create_collation(name, callback)

    def interrupt(self):
        self._ok()
        self._db.interrupt()

    def set_authorizer(self, authorizer_callback):
        self._ok()
        self._db.set_authorizer(authorizer_callback)

    def set_progress_handler(self, progress_handler, n):
        self._ok()
        self._db.set_progress_handler(progress_handler, n)

    def set_trace_callback(self, trace_callback):
        self._ok()
        self._db.set_trace_callback(trace_callback)

    def enable_load_extension(self, enable, /):
        self._ok()
        raise NotSupportedError("enable_load_extension is not supported by this SQLite build")

    def load_extension(self, name, /, *, entrypoint=None):
        self._ok()
        raise NotSupportedError("load_extension is not supported by this SQLite build")

    def iterdump(self, *, filter=None):
        from sqlite3.dump import _iterdump
        return _iterdump(self, filter=filter)

    def backup(self, target, *, pages=-1, progress=None, name="main", sleep=0.250):
        if not isinstance(target, Connection):
            raise TypeError("backup() argument 'target' must be sqlite3.Connection, not %s" % type(target).__name__)
        if progress is not None and not callable(progress):
            raise TypeError("progress argument must be a callable")
        if pages == 0:
            pages = -1
        self._ok()
        target._ok()
        if target is self:
            raise ValueError("target cannot be the same connection instance")
        if target.in_transaction:
            raise OperationalError("target is in transaction")
        ms = int(sleep * 1000)
        b = self._db.backup_init(target._db, "main", name)
        err = None
        while True:
            rc = b.step(pages)
            if progress is not None:
                try:
                    progress(rc, b.remaining(), b.pagecount())
                except BaseException as e:
                    err = e
                    break
            if rc == SQLITE_BUSY or rc == SQLITE_LOCKED:
                self._db.sleep(ms)
            elif rc != SQLITE_OK:
                break
        if err is not None:
            b.finish(SQLITE_DONE)
            raise err
        b.finish(rc)

    def getlimit(self, category, /):
        self._ok()
        return self._db.limit(category, -1)

    def setlimit(self, category, limit, /):
        self._ok()
        return self._db.limit(category, limit)

    def getconfig(self, op, /):
        self._ok()
        return bool(self._db.config(op, -1))

    def setconfig(self, op, enable=True, /):
        self._ok()
        self._db.config(op, 1 if enable else 0)

    def serialize(self, *, name="main"):
        self._ok()
        return self._db.serialize(name)

    def deserialize(self, data, /, *, name="main"):
        self._ok()
        self._db.deserialize(name, bytes(data))

    def blobopen(self, table, column, row, /, *, readonly=False, name="main"):
        self._ok()
        return Blob(self._db.blob_open(table, column, row, readonly, name), self)


def connect(*args, **kwargs):
    factory = kwargs.get("factory")
    if factory is None and len(args) > 5:
        factory = args[5]
    if factory is None:
        factory = Connection
    return factory(*args, **kwargs)


__all__ = [n for n in list(globals()) if not n.startswith("_")]
`
