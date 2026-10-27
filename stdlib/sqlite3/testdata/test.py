"""sqlite3 over the pure-Go SQLite: every line printed here is compared with
the output of CPython 3.14's sqlite3 (test_sqlite3_golden.txt)."""
import sqlite3
import datetime
import os
import tempfile


def show(label, fn):
    try:
        r = fn()
    except Exception as e:
        code = getattr(e, "sqlite_errorname", None)
        print(label, "->", type(e).__name__ + ":", e, code or "")
    else:
        print(label, "->", repr(r))


# ---- module surface ----
print("api", sqlite3.apilevel, sqlite3.paramstyle, sqlite3.threadsafety)
print("version", sqlite3.sqlite_version_info >= (3, 35), isinstance(sqlite3.sqlite_version, str))
print("consts", sqlite3.PARSE_DECLTYPES, sqlite3.PARSE_COLNAMES, sqlite3.SQLITE_OK,
      sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.LEGACY_TRANSACTION_CONTROL)
for n in ("Warning", "Error", "InterfaceError", "DatabaseError", "DataError", "OperationalError",
          "IntegrityError", "InternalError", "ProgrammingError", "NotSupportedError"):
    c = getattr(sqlite3, n)
    print(n, c.__name__, c.__module__, [b.__name__ for b in c.__mro__[1:]])
print("types", sqlite3.Connection.__module__, sqlite3.Cursor.__module__, sqlite3.Row.__module__)
print("complete", sqlite3.complete_statement("select 1;"), sqlite3.complete_statement("select 1"))

# ---- basic DML, cursor attributes ----
con = sqlite3.connect(":memory:")
cur = con.cursor()
print("fresh", cur.rowcount, cur.lastrowid, cur.description)
cur.execute("create table t(a integer primary key, b text, c real, d blob)")
print("create", cur.rowcount, cur.lastrowid, cur.description)
cur.execute("insert into t(b, c, d) values(?, ?, ?)", ("x", 1.5, b"\x00\x01"))
print("insert", cur.rowcount, cur.lastrowid, con.in_transaction)
cur.executemany("insert into t(b) values(?)", [("a",), ("b",), ("c",)])
print("many", cur.rowcount, cur.lastrowid)
cur.execute("select a, b from t order by a")
print("select", cur.rowcount, cur.lastrowid, cur.description)
print("fetchone", cur.fetchone(), cur.fetchmany(2), cur.fetchall(), cur.fetchone())
cur.arraysize = 2
cur.execute("select a from t")
print("arraysize", cur.fetchmany(), cur.fetchmany(10))
cur.execute("update t set b = 'q' where a > 1")
print("update", cur.rowcount, cur.lastrowid)
cur.execute("delete from t where a = 99")
print("delete none", cur.rowcount)
cur.execute("insert into t(b) values('z') returning a, b")
print("returning", cur.rowcount, cur.fetchall(), cur.rowcount)
print("iter", [r for r in cur.execute("select a from t where a < 3")])
con.commit()
print("commit", con.in_transaction)

# ---- types round trip ----
cur.execute("create table ty(i, f, s, b, n)")
cur.execute("insert into ty values(?,?,?,?,?)", (2**62, -0.25, "h\u00e9llo \u4e16\u754c", b"\xff\x00\xfe", None))
cur.execute("insert into ty values(?,?,?,?,?)", (True, 1e300, "", b"", None))
cur.execute("insert into ty values(?,?,?,?,?)", (bytearray(b"ab"), 3, "a\x00b", memoryview(b"mv"), None))
for row in cur.execute("select * from ty"):
    print("row", row, [type(v).__name__ for v in row])
show("typeof", lambda: cur.execute("select typeof(i), typeof(f), typeof(s), typeof(b), typeof(n) from ty").fetchall())
show("bigint", lambda: cur.execute("select ?", (2**70,)))
show("neg64", lambda: cur.execute("select ?", (-2**63,)).fetchall())
show("unsupported", lambda: cur.execute("select ?", (object(),)))
show("set", lambda: cur.execute("select ?", ({1},)))

# ---- parameter styles and binding errors ----
show("qmark", lambda: cur.execute("select ?, ?", (1, 2)).fetchall())
show("named", lambda: cur.execute("select :x, :y, :x", {"x": 1, "y": "z"}).fetchall())
show("numbered", lambda: cur.execute("select ?2, ?1", ("a", "b")).fetchall())
show("list", lambda: cur.execute("select ?, ?", [1, 2]).fetchall())
show("too many", lambda: cur.execute("select ?", (1, 2)))
show("too few", lambda: cur.execute("select ?, ?", (1,)))
show("missing key", lambda: cur.execute("select :a", {"b": 1}))
show("dict for qmark", lambda: cur.execute("select ?", {"a": 1}))
show("seq for named", lambda: cur.execute("select :a", (1,)))
show("int params", lambda: cur.execute("select ?", 5))
show("none params", lambda: cur.execute("select ?", None))
show("extra dict key", lambda: cur.execute("select :a", {"a": 1, "zz": 2}).fetchall())
show("two statements", lambda: cur.execute("select 1; select 2"))
show("trailing comment", lambda: cur.execute("select 1; -- c").fetchall())
show("empty sql", lambda: (cur.execute("") is cur, cur.description, cur.fetchall()))
show("sql type", lambda: cur.execute(5))
show("executemany select", lambda: cur.executemany("select ?", [(1,)]))
show("executemany dict", lambda: (cur.executemany("insert into ty(i) values(:v)", [{"v": 7}, {"v": 8}]), cur.rowcount)[1])
show("executemany gen", lambda: (cur.executemany("insert into ty(i) values(?)", ((i,) for i in range(5))), cur.rowcount)[1])

# ---- errors ----
show("syntax", lambda: cur.execute("selec 1"))
show("no table", lambda: cur.execute("select * from nope"))
show("no column", lambda: cur.execute("select zz from t"))
show("unique", lambda: cur.execute("insert into t(a) values(1)"))
cur.execute("create table nn(x not null, y unique, z check (z > 0), f references t(a))")
show("notnull", lambda: cur.execute("insert into nn(x) values(NULL)"))
cur.execute("insert into nn values(1, 1, 1, NULL)")
show("unique2", lambda: cur.execute("insert into nn values(1, 1, 1, NULL)"))
show("check", lambda: cur.execute("insert into nn values(1, 2, -1, NULL)"))
cur.execute("pragma foreign_keys = on")
show("fk", lambda: cur.execute("insert into nn values(1, 3, 1, 12345)") is cur)
try:
    cur.execute("insert into nn values(1, 1, 1, NULL)")
except sqlite3.IntegrityError as e:
    print("integrity", e.sqlite_errorcode, e.sqlite_errorname, isinstance(e, sqlite3.DatabaseError), isinstance(e, sqlite3.Error))
show("readonly fn", lambda: con.execute("pragma query_only = 1").fetchall())
show("readonly write", lambda: con.execute("insert into t(b) values('w')"))
con.execute("pragma query_only = 0")

# ---- transactions (legacy isolation_level) ----
print("isolation", repr(con.isolation_level), con.autocommit)
con.commit()
con.execute("create table tx(x)")
print("ddl in tx", con.in_transaction)
con.execute("insert into tx values(1)")
print("dml starts tx", con.in_transaction)
con.rollback()
print("rolled back", con.execute("select count(*) from tx").fetchone(), con.in_transaction)
con.execute("insert into tx values(1)")
con.commit()
print("committed", con.execute("select count(*) from tx").fetchone())
with con:
    con.execute("insert into tx values(2)")
print("ctx commit", con.execute("select count(*) from tx").fetchone(), con.in_transaction)
try:
    with con:
        con.execute("insert into tx values(3)")
        raise ValueError("boom")
except ValueError:
    pass
print("ctx rollback", con.execute("select count(*) from tx").fetchone(), con.in_transaction)
for lvl in (None, "", "DEFERRED", "immediate", "EXCLUSIVE"):
    con.isolation_level = lvl
    print("level", repr(con.isolation_level))
show("bad level", lambda: setattr(con, "isolation_level", "BOGUS"))
con.isolation_level = None
con.execute("begin")
print("manual begin", con.in_transaction)
con.execute("insert into tx values(9)")
con.execute("rollback")
print("manual rollback", con.execute("select count(*) from tx").fetchone(), con.in_transaction)
con.isolation_level = ""
print("total_changes", con.total_changes > 0)

# ---- autocommit (3.12+) ----
ac = sqlite3.connect(":memory:", autocommit=False)
print("ac False", ac.autocommit, ac.in_transaction)
ac.execute("create table z(x)")
ac.execute("insert into z values(1)")
ac.rollback()
print("ac rollback", ac.in_transaction, ac.execute("select count(*) from sqlite_master").fetchone())
ac.execute("create table z(x)")
ac.commit()
print("ac commit", ac.in_transaction, ac.execute("select name from sqlite_master").fetchall())
ac.autocommit = True
print("ac True", ac.autocommit, ac.in_transaction)
ac.execute("insert into z values(5)")
print("ac auto", ac.in_transaction)
ac.rollback()
print("ac rollback noop", ac.execute("select x from z").fetchall())
show("ac bad", lambda: setattr(ac, "autocommit", 2))
show("ac bad ctor", lambda: sqlite3.connect(":memory:", autocommit="x"))
ac.close()

# ---- row_factory / Row / text_factory ----
con.row_factory = sqlite3.Row
r = con.execute("select 1 as one, 'two' as Two, 3.0 as three").fetchone()
print("Row", r["one"], r["TWO"], r[2], r[-1], r[0:2], r.keys(), len(r), tuple(r), list(r))
show("Row bad key", lambda: r["nope"])
show("Row bad idx", lambda: r[9])
show("Row bad type", lambda: r[1.5])
print("Row eq", r == con.execute("select 1 as one, 'two' as Two, 3.0 as three").fetchone(), r != 5, hash(r) == hash(r))
print("Row seq", isinstance(r, __import__("collections.abc").abc.Sequence))
con.row_factory = lambda cursor, row: {d[0]: v for d, v in zip(cursor.description, row)}
print("dict factory", con.execute("select 1 as a, 2 as b").fetchall())
con.row_factory = None
c2 = con.cursor()
c2.row_factory = sqlite3.Row
print("cursor factory", type(c2.execute("select 1").fetchone()).__name__, con.row_factory)
con.text_factory = bytes
print("text bytes", con.execute("select 'abc', 5, x'00'").fetchone())
con.text_factory = lambda b: b.decode("utf-8").upper()
print("text custom", con.execute("select 'abc', 5, x'00'").fetchone())
con.text_factory = str
con.execute("create table bad(x)")
con.execute("insert into bad values (cast(x'ff' as text))")
show("bad utf8", lambda: con.execute("select x from bad").fetchall())
con.text_factory = bytes
print("bad utf8 bytes", con.execute("select x from bad").fetchall())
con.text_factory = str

# ---- adapters and converters ----
class Point:
    def __init__(self, x, y):
        self.x, self.y = x, y

    def __conform__(self, protocol):
        if protocol is sqlite3.PrepareProtocol:
            return "%f;%f" % (self.x, self.y)

class Pt2:
    def __init__(self, x):
        self.x = x

sqlite3.register_adapter(Pt2, lambda p: "pt2:%d" % p.x)
print("conform", con.execute("select ?", (Point(1.5, -2),)).fetchone())
print("adapter", con.execute("select ?", (Pt2(4),)).fetchone())
print("adapt()", sqlite3.adapt(Pt2(7)), sqlite3.adapt(5, sqlite3.PrepareProtocol, "alt"))
show("adapt none", lambda: sqlite3.adapt(object()))

def conv_point(b):
    x, y = map(float, b.split(b";"))
    return Point(x, y)

sqlite3.register_converter("point", conv_point)
sqlite3.register_converter("Wrapped", lambda b: ("w", b))
cd = sqlite3.connect(":memory:", detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES)
cd.execute("create table p(pt point, w wrapped(10), n integer, t text)")
cd.execute("insert into p values(?, ?, ?, ?)", (Point(4, 5), "raw", 7, "txt"))
cd.execute("insert into p values(NULL, NULL, NULL, NULL)")
row = cd.execute("select pt, w, n, t from p").fetchone()
print("decltypes", type(row[0]).__name__, (row[0].x, row[0].y), row[1:])
print("null conv", cd.execute("select pt, w from p where n is null").fetchone())
cur = cd.execute('select pt as "t [wrapped]", w as "w [nada]", n from p')
print("colnames", cur.description, cur.fetchone())
cur = cd.execute('select t as "x[wrapped]" from p')
print("colname nospace", cur.description[0][0], cur.fetchone())
cur = cd.execute("select 1 as 'a [wrapped] b'")
print("colname mid", cur.description[0][0], cur.fetchone())
cp = sqlite3.connect(":memory:", detect_types=sqlite3.PARSE_DECLTYPES)
cp.execute("create table p(pt point)")
cp.execute("insert into p values('1.0;2.0')")
print("decl only", type(cp.execute("select pt from p").fetchone()[0]).__name__,
      cp.execute('select pt as "x [point]" from p').description[0][0])
cn = sqlite3.connect(":memory:")
cn.execute("create table p(pt point)")
cn.execute("insert into p values('1.0;2.0')")
print("no detect", cn.execute("select pt from p").fetchone())
show("register_converter type", lambda: sqlite3.register_converter(5, None))
ts = sqlite3.connect(":memory:", detect_types=sqlite3.PARSE_DECLTYPES)
import warnings
with warnings.catch_warnings():
    warnings.simplefilter("ignore")
    ts.execute("create table dt(d date, t timestamp)")
    ts.execute("insert into dt values(?, ?)", (datetime.date(2024, 2, 29), datetime.datetime(2024, 2, 29, 13, 5, 9, 250)))
    print("date/timestamp", ts.execute("select d, t from dt").fetchone())
    print("stored", ts.execute("select typeof(d), d, t from dt", ).description is not None)
    cn = sqlite3.connect(":memory:")
    cn.execute("create table dt(d, t)")
    cn.execute("insert into dt values(?, ?)", (datetime.date(2024, 2, 29), datetime.datetime(2024, 2, 29, 13, 5, 9)))
    print("stored text", cn.execute("select d, t from dt").fetchone())

# ---- user functions ----
con.create_function("twice", 1, lambda x: x * 2)
con.create_function("conc", -1, lambda *a: "-".join(map(str, a)))
con.create_function("nada", 0, lambda: None)
con.create_function("blob", 0, lambda: b"\x01\x02")
con.create_function("boom", 0, lambda: 1 / 0)
con.create_function("badret", 0, lambda: object())
con.create_function("big", 0, lambda: 2**70)
con.create_function("det", 1, lambda x: x + 1, deterministic=True)
print("udf", con.execute("select twice(21), twice('ab'), twice(1.5), conc(1, 'a', NULL), nada(), blob()").fetchone())
show("udf exc", lambda: con.execute("select boom()"))
show("udf badret", lambda: con.execute("select badret()"))
show("udf big", lambda: con.execute("select big()"))
show("udf unknown", lambda: con.execute("select nosuch(1)"))
show("udf argc", lambda: con.execute("select twice(1, 2)"))
print("udf det", con.execute("select det(1)").fetchone())
show("udf not callable", lambda: con.create_function("q", 1, 5))
con.create_function("upd", 1, lambda x: x)
print("udf in where", con.execute("select a from t where upd(a) = 2").fetchall())

class Summer:
    def __init__(self):
        self.t = 0

    def step(self, v):
        self.t += v

    def finalize(self):
        return self.t

class BadInit:
    def __init__(self):
        raise RuntimeError("no")

class BadStep:
    def step(self, v):
        raise RuntimeError("no")

    def finalize(self):
        return 0

class BadFinal:
    def step(self, v):
        pass

    def finalize(self):
        raise RuntimeError("no")

con.create_aggregate("mysum", 1, Summer)
con.create_aggregate("badinit", 1, BadInit)
con.create_aggregate("badstep", 1, BadStep)
con.create_aggregate("badfinal", 1, BadFinal)
print("aggregate", con.execute("select mysum(a), mysum(a) from t").fetchone())
print("aggregate empty", con.execute("select mysum(a) from t where a < 0").fetchone())
print("aggregate group", con.execute("select b, mysum(a) from t group by b order by b").fetchall())
show("agg badinit", lambda: con.execute("select badinit(a) from t"))
show("agg badstep", lambda: con.execute("select badstep(a) from t"))
show("agg badfinal", lambda: con.execute("select badfinal(a) from t"))

class WinSum:
    def __init__(self):
        self.t = 0

    def step(self, v):
        self.t += v

    def inverse(self, v):
        self.t -= v

    def value(self):
        return self.t

    def finalize(self):
        return self.t

con.create_window_function("wsum", 1, WinSum)
print("window", con.execute("select a, wsum(a) over (order by a rows between 1 preceding and current row) from t order by a").fetchall())

# ---- collation, authorizer, progress, trace ----
con.create_collation("rev", lambda a, b: (a < b) - (a > b))
print("collation", con.execute("select b from t order by b collate rev").fetchall())
show("bad collation name", lambda: con.create_collation("bad name", lambda a, b: 0))
con.create_collation("rev", None)
show("collation removed", lambda: con.execute("select b from t order by b collate rev"))

seen = []
def auth(action, a, b, db, source):
    seen.append(action)
    if action == sqlite3.SQLITE_DELETE:
        return sqlite3.SQLITE_DENY
    return sqlite3.SQLITE_OK
con.set_authorizer(auth)
con.execute("select a from t").fetchall()
print("authorizer saw read", sqlite3.SQLITE_READ in seen, sqlite3.SQLITE_SELECT in seen)
show("authorizer deny", lambda: con.execute("delete from t"))
con.set_authorizer(None)
print("authorizer off", con.execute("select count(*) from t").fetchone())

calls = []
con.set_progress_handler(lambda: calls.append(1) or 0, 1)
con.execute("select count(*) from t, t, t").fetchone()
print("progress called", len(calls) > 0)
con.set_progress_handler(lambda: 1, 1)
show("progress abort", lambda: con.execute("select count(*) from t, t, t").fetchone())
con.set_progress_handler(None, 1)

traced = []
con.set_trace_callback(traced.append)
con.execute("select ?", (42,)).fetchone()
con.set_trace_callback(None)
print("trace", [s for s in traced if "42" in s])

# ---- executescript ----
cur = con.cursor()
cur.executescript("""
    create table sc(a, b);
    insert into sc values (1, 'x');
    insert into sc values (2, 'y');
""")
print("script", con.execute("select * from sc").fetchall(), con.in_transaction)
show("script error", lambda: cur.executescript("insert into sc values (3,'z'); bogus;"))
print("script partial", con.execute("select count(*) from sc").fetchone())
show("script type", lambda: cur.executescript(b"select 1"))
con.execute("insert into sc values (9, 'q')")
cur.executescript("select 1")
print("script commits", con.in_transaction, con.execute("select count(*) from sc").fetchone())

# ---- closing and misuse ----
c3 = sqlite3.connect(":memory:")
cu = c3.cursor()
cu.close()
show("closed cursor", lambda: cu.execute("select 1"))
show("closed cursor fetch", lambda: cu.fetchone())
show("closed cursor close", lambda: cu.close())
c3.close()
c3.close()
show("closed con execute", lambda: c3.execute("select 1"))
show("closed con cursor", lambda: c3.cursor())
show("closed con commit", lambda: c3.commit())
show("closed con total", lambda: c3.total_changes)
show("closed in_tx", lambda: c3.in_transaction)
c4 = sqlite3.connect(":memory:")
cu = c4.execute("select 1 union select 2")
c4.close()
show("cursor after con close", lambda: cu.fetchone())

# ---- factories, subclassing ----
class MyCon(sqlite3.Connection):
    def cursor(self, *a, **k):
        return super().cursor(MyCur)

class MyCur(sqlite3.Cursor):
    pass

m = sqlite3.connect(":memory:", factory=MyCon)
print("factory", type(m).__name__, type(m.execute("select 1")).__name__, isinstance(m, sqlite3.Connection))
show("bad cursor factory", lambda: m.cursor.__func__ and sqlite3.Connection.cursor(m, factory=lambda c: 5))
show("connect type", lambda: sqlite3.connect(5))
show("cursor type", lambda: sqlite3.Cursor(5))

# ---- file database, uri, timeout ----
d = tempfile.mkdtemp()
path = os.path.join(d, "f.db")
f1 = sqlite3.connect(path)
f1.execute("create table k(v)")
f1.execute("insert into k values('persist')")
f1.commit()
f1.close()
f2 = sqlite3.connect(path)
print("file db", f2.execute("select v from k").fetchall())
f2.close()
fu = sqlite3.connect("file:" + path + "?mode=ro", uri=True)
print("uri ro", fu.execute("select v from k").fetchall())
show("uri ro write", lambda: fu.execute("insert into k values(1)"))
fu.close()
show("uri bad", lambda: sqlite3.connect("file:" + os.path.join(d, "missing.db") + "?mode=ro", uri=True))
show("cannot open", lambda: sqlite3.connect(os.path.join(d, "no", "such", "dir.db")))
import pathlib
fp = sqlite3.connect(pathlib.Path(path))
print("pathlike", fp.execute("select count(*) from k").fetchone())
fp.close()
fb = sqlite3.connect(os.fsencode(path))
print("bytes path", fb.execute("select count(*) from k").fetchone())
fb.close()
os.remove(path)
os.rmdir(d)

# ---- misc API ----
print("limits", con.getlimit(sqlite3.SQLITE_LIMIT_VARIABLE_NUMBER) > 0)
old = con.setlimit(sqlite3.SQLITE_LIMIT_SQL_LENGTH, 100)
show("limit hit", lambda: con.execute("select " + "1+" * 60 + "1"))
con.setlimit(sqlite3.SQLITE_LIMIT_SQL_LENGTH, old)
print("getconfig", con.getconfig(sqlite3.SQLITE_DBCONFIG_ENABLE_FKEY))
con.setconfig(sqlite3.SQLITE_DBCONFIG_ENABLE_FKEY, False)
print("setconfig", con.getconfig(sqlite3.SQLITE_DBCONFIG_ENABLE_FKEY))
src = sqlite3.connect(":memory:")
src.execute("create table b(x)")
src.executemany("insert into b values(?)", [(i,) for i in range(100)])
src.commit()
dst = sqlite3.connect(":memory:")
prog = []
src.backup(dst, pages=1, progress=lambda status, remaining, total: prog.append(status))
print("backup", dst.execute("select count(*), sum(x) from b").fetchone(), len(prog) >= 1, prog[-1])
show("backup self", lambda: src.backup(src))
show("backup type", lambda: src.backup(5))
data = src.serialize()
print("serialize", isinstance(data, bytes), len(data) > 0)
d2 = sqlite3.connect(":memory:")
d2.deserialize(data)
print("deserialize", d2.execute("select count(*) from b").fetchone())
print("iterdump", list(src.iterdump()))
src.execute("create table bl(b blob)")
src.execute("insert into bl values(zeroblob(8))")
src.commit()
with src.blobopen("bl", "b", 1) as bl:
    print("blob", len(bl), bl.read(), bl.tell())
    bl.seek(2)
    bl.write(b"abc")
    bl.seek(0)
    print("blob rw", bl.read(), bl[1], bl[2:5], bl[-1])
    show("blob long write", lambda: bl.write(b"x" * 20))
    show("blob seek", lambda: bl.seek(100))
    bl[0] = 7
    print("blob setitem", bl[0:3])
show("blob closed", lambda: bl.read())
show("blob missing", lambda: src.blobopen("bl", "b", 99))
con.interrupt()
print("done")
