import os, json, posixpath
class S(str): pass
class I(int): pass
class F(float): pass
class B(bytes): pass
s, i, f, b = S("ab"), I(5), F(2.5), B(b"xy")
print(str(s), repr(s), f"{s}|{i}|{f}", "%s %d %r" % (s, i, s), "{} {}".format(s, i))
print("-".join([s, "c"]), s + "c", "c" + s, s * 2, s[0], s.upper(), len(s), "a" in s, s.split("b"))
print(posixpath.join(s, "x"), posixpath.basename(S("/a/b")), json.dumps(s), json.dumps(i))
print(str(i), int(i), i + 1, i * 2, -i, abs(I(-3)), i == 5, i < 6, hash(i) == hash(5), list(range(I(3))), [1,2,3,4,5,6][i])
print(str(f), float(f), f * 2, round(F(2.567), 1), int(f))
print(bytes(b), b + b"z", b.decode(), len(b), b[0], str(b))
print({s: 1}.get("ab"), {"ab": 1}[s], sorted([S("b"), "a"]), max(I(1), 2), sum([I(1), I(2)]))
print(isinstance(s, str), isinstance(i, int), isinstance(f, float), isinstance(b, bytes), issubclass(S, str))
print(type(s + "c").__name__, type(i + 1).__name__, type(s.upper()).__name__)
print(bool(S("")), bool(I(0)), int("1" + s[:0] + "2"), s.encode(), s.startswith("a"))
d = {}; d[i] = "v"; print(d[5], 5 in d, list(d))
print(chr(I(65)), "abc"[I(1)], [0]*I(2), divmod(I(7), 2), pow(I(2), 3), I(3).bit_length(), I(5).to_bytes(1, "big"))
print(open(os.devnull, S("r")).read(), os.path.exists(S(".")), os.fspath(S("q")))
class P(S):
    def __str__(self): return "P!"
    def __repr__(self): return "Prepr"
print(str(P("z")), repr(P("z")), f"{P('z')}", P("z") + "w")

# ---- extended cases: each line is isolated, errors print their type only ----
import copy, pickle, enum, pathlib, struct, operator, string, re, math, functools

def t(label, fn):
    try:
        r = fn()
        print(label, r if not isinstance(r, (str, int, float, bytes)) or True else r, type(r).__name__)
    except Exception as e:
        print(label, "ERR", type(e).__name__)

class SS(str):
    __slots__ = ()
    def shout(self): return self.upper() + "!"
class II(int):
    def double(self): return II(int(self) * 2)
class N(str):
    def __new__(cls, v): return super().__new__(cls, str(v).strip())
class Nint(int):
    def __new__(cls, v): return super().__new__(cls, int(v) + 1)
class Fs(float): pass
class Bs(bytes): pass
class Cs(complex): pass
class Color(str):
    # what enum.StrEnum members look like: a str that carries a name and value
    def __new__(cls, name, value):
        o = super().__new__(cls, value)
        o.name, o.value = name, value
        return o
    def __str__(self): return str.__str__(self)
    def __repr__(self): return "<Color.%s: %r>" % (self.name, self.value)
class Num(int):
    def __new__(cls, name, value):
        o = super().__new__(cls, value)
        o.name, o.value = name, value
        return o
    def __repr__(self): return "<Num.%s: %r>" % (self.name, self.value)
    def __str__(self): return "Num.%s" % self.name
RED, ONE, TWO = Color("RED", "red"), Num("ONE", 1), Num("TWO", 2)
t("enum-like", lambda: (RED, str(RED), RED == "red", "x" + RED, f"{RED}", f"{ONE}", f"{ONE:03d}", "%s|%d|%r" % (TWO, TWO, TWO), json.dumps({"c": RED, "n": TWO}), posixpath.join(RED, "x"), {RED: 1}["red"], {"red": 1}[RED], sorted([TWO, 1, ONE]), [10, 20, 30][ONE], range(TWO), isinstance(RED, str), RED.upper(), TWO + 1, hash(TWO) == hash(2), RED.name, TWO.value, RED in ("red",), ONE < TWO, bin(TWO), struct.pack("B", TWO), chr(TWO + 64), int(TWO), str(ONE), repr(ONE), format(ONE), format(ONE, "d"), copy.copy(TWO), pickle.loads(pickle.dumps(RED, 4)) == "red"))
class Pth(str):
    def __fspath__(self): return str(self)

ss, ii, fs, bs, cs = SS("hi"), II(7), Fs(1.5), Bs(b"by"), Cs(1 + 2j)
t("slots", lambda: (type(ss).__name__, ss.shout()))
t("double", lambda: ii.double())
t("new", lambda: (N("  x "), type(N(" y")).__name__, len(N("  zz  "))))
t("newint", lambda: (Nint(1), type(Nint(1)).__name__, Nint(1) + 1))
t("cplx", lambda: (cs, type(cs).__name__, cs + 1, abs(Cs(3 + 4j)), cs.real, complex(cs), cs * 2))
t("float", lambda: (fs + 1, 2 * fs, fs // 1, fs % 1, math.floor(fs), math.sqrt(Fs(4)), float(fs), int(fs), fs.is_integer()))
t("floatfmt", lambda: ("%5.1f|%s|%r|{:.2f}".format(fs) % (fs, fs, fs)))
t("bytes", lambda: (bs + b"z", b"z" + bs, bs * 2, bs[0], bs[:1], bs.hex(), bytes(bs), bytearray(bs), b"y" in bs, bs.upper(), bs.decode("ascii")))
t("bytesjoin", lambda: b",".join([bs, b"q"]))
t("struct", lambda: struct.pack(">I", II(258)) + struct.pack("3s", bs))
t("in", lambda: ("h" in ss, ss in "ohio", SS("h") in ["h"], II(3) in [1, 2, 3], II(3) in {3}, SS("a") in {"a": 1}))
t("sorted", lambda: sorted([SS("b"), "a", SS("c")]))
t("sorted-int", lambda: sorted([II(3), 1, II(2)]) + [min(II(5), 4), max(II(5), 9), max(II(5), 4) == 5])
t("minmax-type", lambda: (type(max(II(5), 4)).__name__, type(min(SS("a"), "b")).__name__))
t("sum", lambda: (sum([II(1), II(2)]), sum([Fs(.5), 1]), sum(map(II, range(4)))))
t("dictkeys", lambda: ({SS("a"): 1, II(2): 2, Fs(3.5): 3}))
t("dictkeys2", lambda: ({"a": 1}[SS("a")], {2: "x"}[II(2)], {SS("k"): 1}["k"]))
t("dictkeytype", lambda: [type(k).__name__ for k in {SS("a"): 1, II(2): 2}])
t("setmember", lambda: (SS("a") in {"a"}, len({SS("a"), "a"}), {II(1), True}))
t("percent", lambda: ("%s|%d|%i|%x|%o|%5s|%-5s|%r|%c" % (ss, ii, ii, ii, ii, ss, ss, ss, II(65))))
t("percent-dict", lambda: "%(a)s %(b)d" % {"a": ss, "b": ii})
t("format", lambda: ("{}|{:>5}|{:<4}|{:d}|{:x}|{!r}|{!s}|{:08.3f}".format(ss, ss, ss, ii, ii, ss, ss, fs)))
t("fstr", lambda: f"{ss}|{ss:>5}|{ii:03d}|{ii!r}|{fs:.2f}|{bs!r}|{ss=}")
t("format()", lambda: (format(ss, ">4"), format(ii, "x"), format(fs, ".3"), format(ss)))
t("json", lambda: (json.dumps({ss: ii, "f": fs}), json.dumps([ss, bs.decode()]), json.loads(json.dumps(ss)), ))
t("json-sort", lambda: json.dumps({SS("b"): 1, "a": 2}, sort_keys=True))
# posixpath is POSIX on every host, so this row's golden is host independent.
t("os.path", lambda: (posixpath.join(ss, "x"), posixpath.join("a", ss), posixpath.basename(SS("/a/b.c")), posixpath.dirname(SS("/a/b")), posixpath.splitext(SS("f.txt")), posixpath.abspath(SS("/q/../r")), posixpath.isabs(SS("/x")), posixpath.normpath(SS("a//b")), posixpath.expanduser(SS("x"))))
t("os", lambda: (os.fspath(ss), os.path.exists(SS(".")), os.path.isdir(SS(".")), os.fsencode(SS("a")), os.listdir(SS("."))[:0], os.getenv(SS("NOPE_XYZ"), "d"), os.environ.get(SS("NOPE_XYZ")), 0))
t("os-fspath-obj", lambda: (os.fspath(Pth("p")), posixpath.join(Pth("p"), "q")))
t("open", lambda: open(os.devnull, SS("r")).read() + open(SS(os.devnull)).read())
t("pathlib", lambda: (str(pathlib.PurePosixPath(SS("/a/b"))), pathlib.PurePosixPath(SS("/a")) / SS("b"), pathlib.PurePosixPath("/a").joinpath(SS("c")).name))
t("int()", lambda: (int(ii), int(SS("12")), int(SS(" 7 ")), int(fs), int(ii, 0) if False else int(SS("ff"), 16), int(II(3), 2) if False else 0, int(Bs(b"9")), int(II(9)) is not ii))
t("int-type", lambda: (type(int(ii)).__name__, type(II(II(4))).__name__, type(int(II(4))).__name__))
t("float()", lambda: (float(fs), float(ii), float(SS("2.5")), float(Bs(b"1e2")), type(float(fs)).__name__))
t("str()", lambda: (str(ss), str(ii), str(fs), str(bs), str(bs, "ascii"), str(Bs(b"q"), SS("ascii")), type(str(ss)).__name__, str(Cs(1j))))
t("bytes()", lambda: (bytes(bs), bytes(II(3)), bytes([II(65), 66]), bytes(SS("hi"), "ascii"), bytes(SS("hi"), SS("ascii")), type(bytes(bs)).__name__))
t("complex()", lambda: (complex(cs), complex(ii), complex(fs, ii), complex(SS("1+2j"))))
t("bool", lambda: (bool(SS("")), bool(II(0)), bool(Fs(0.0)), bool(Bs(b"")), bool(Cs(0)), bool(SS("a")), not II(0)))
t("round", lambda: (round(Fs(2.567), 1), round(Fs(2.5)), round(fs)))
t("divmod", lambda: (divmod(II(7), 2), divmod(7, II(2)), divmod(Fs(7.5), 2), pow(II(2), 10), pow(2, II(3), II(5)), II(2) ** II(3), 2 ** II(2), II(7) // 2, II(7) % 4, II(7) / 2))
t("bits", lambda: (II(6) & 3, 3 | II(4), II(6) ^ 2, II(1) << 4, 64 >> II(2), ~II(5), -II(5), +II(5), abs(II(-5)), II(5).bit_length()))
t("cmp", lambda: (II(1) < 2, 2 > II(1), II(1) == 1, 1 == II(1), II(1) != 1, SS("a") < "b", "b" > SS("a"), SS("a") == "a", "a" == SS("a"), SS("a") != "b", Fs(1.0) == 1, bs == b"by", b"by" == bs))
t("cmp-same", lambda: (SS("a") == SS("a"), SS("a") < SS("b"), II(1) <= II(1), II(2) >= II(3)))
t("hash", lambda: (hash(SS("a")) == hash("a"), hash(II(5)) == hash(5), hash(Fs(1.5)) == hash(1.5), hash(bs) == hash(b"by"), hash(Cs(1j)) == hash(1j)))
t("index", lambda: ([0, 1, 2][II(1)], "abc"[II(2)], (1, 2, 3)[II(0):II(2)], list(range(II(1), II(4))), [1] * II(3), "ab" * II(2), b"ab"[II(1)], hex(II(255)), bin(II(5)), oct(II(8)), chr(II(66)), operator.index(II(3))))
t("slice", lambda: ("hello"[II(1):II(3)], "abcdef"[::II(2)]))
t("seq", lambda: (list(SS("ab")), tuple(SS("ab")), set(SS("a")), len(SS("héllo")), SS("abc")[::-1], SS("a,b").split(","), SS("x").join(["1", "2"]), reversed(SS("ab")).__next__(), list(enumerate(SS("ab"))), list(zip(SS("ab"), Bs(b"ab")))))
t("str-methods", lambda: (SS("abc").upper(), SS("abc").find(SS("c")), SS("abc").index("b"), SS("abca").count(SS("a")), SS("abc").replace(SS("b"), SS("X")), SS("a b").split(SS(" ")), SS("  a ").strip(SS(" ")), SS("abc").startswith(SS("a")), SS("abc").startswith((SS("x"), "a")), SS("abc").endswith(SS("c")), SS("a").center(II(5), SS("*")), SS("a").ljust(II(3)), SS("a").zfill(II(3)), SS("a\tb").expandtabs(II(4)), SS("ab").encode(SS("utf-8")), SS("a").partition(SS("a")), SS("%s") % 1))
t("str-types", lambda: [type(x).__name__ for x in (SS("a").upper(), SS("a") + "b", SS("a") * 2, SS("ab")[0], SS("a").strip(), str(SS("a")), SS("a").lower(), SS("a%s") % "b", SS("{}").format(1), SS("a b").split()[0], "".join([SS("a")]), SS("a").encode(), f"{SS('a')}", SS("a").title())])
t("int-types", lambda: [type(x).__name__ for x in (II(1) + 1, 1 + II(1), -II(1), II(1) * 2, abs(II(1)), II(1) // 1, II(2) ** 2, II(1) + II(1), int(II(1)), II(5).real, II(5).conjugate(), hash(II(1)))])
t("float-types", lambda: [type(x).__name__ for x in (Fs(1) + 1, -Fs(1), float(Fs(1)), abs(Fs(-1)), Fs(1.5) // 1, Fs(1.5).real, round(Fs(1.5), 1))])
t("bytes-types", lambda: [type(x).__name__ for x in (Bs(b"a") + b"b", Bs(b"a")[0:1], Bs(b"a") * 2, Bs(b"a").upper(), bytes(Bs(b"a")), Bs(b"a").strip(), Bs(b"a,b").split(b",")[0], b"".join([Bs(b"a")]))])
t("regex", lambda: (re.sub(SS("a"), SS("b"), SS("aaa")), re.match(SS("a+"), SS("aab")).group(), re.findall(SS(r"\d"), SS("a1b2")), re.split(SS(","), SS("a,b")), re.compile(SS("x")).pattern, re.escape(SS("a.b"))))
t("string", lambda: (string.Template(SS("$x")).substitute(x=II(1)), SS("ab").maketrans(SS("a"), SS("c")), SS("abc").translate({ord("a"): SS("z")}), string.capwords(SS("a b"))))
t("copy", lambda: [(type(c).__name__, c) for c in (copy.copy(ss), copy.deepcopy(ii), copy.copy(fs), copy.deepcopy(bs), copy.copy(cs))])
class SA(str): pass
def _attr():
    x = SA("a"); x.extra = 5
    y = copy.copy(x); z = copy.deepcopy(x)
    return (y, z, y.extra, z.extra, x.extra, type(y).__name__)
t("copy-attr", _attr)
t("deepcopy-list", lambda: [type(c).__name__ for c in copy.deepcopy([SS("a"), II(1), Fs(1.0), Bs(b"b")])])
def _pk():
    out = []
    for proto in range(3, 5):
        for o in (SS("p"), II(5), Fs(2.5), Bs(b"q"), Cs(1j),):
            r = pickle.loads(pickle.dumps(o, proto))
            out.append((type(r).__name__, r == o))
    return out
t("pickle", _pk)
def _pk2():
    x = SA("a"); x.extra = {"k": 1}
    r = pickle.loads(pickle.dumps(x))
    return (type(r).__name__, r, r.extra)
t("pickle-attr", _pk2)
t("pickle-plain", lambda: [pickle.loads(pickle.dumps(o)) for o in ("a", 1, 2.5, b"x")])
t("reduce", lambda: (SS("a").__reduce_ex__(2)[0].__name__, II(1).__reduce_ex__(2)[0].__name__, copy.copy(SS("a")) is not None))
class Over(str):
    def __eq__(self, o): return "yes"
    def __hash__(self): return 1
    def __add__(self, o): return "added"
    def __radd__(self, o): return "radded"
    def __len__(self): return 99
    def __format__(self, spec): return "fmt" + spec
    def __getitem__(self, k): return "item"
    def __contains__(self, k): return "cont"
    def __lt__(self, o): return "lt"
    def __bool__(self): return False
    def upper(self): return "UP"
class OverI(int):
    def __int__(self): return 42
    def __index__(self): return 3
    def __float__(self): return 0.5
    def __add__(self, o): return "iadd"
    def __neg__(self): return "neg"
    def __repr__(self): return "OI"
    def __str__(self): return "OIs"
    def __hash__(self): return 77
class OverF(float):
    def __float__(self): return 9.5
    def __repr__(self): return "OF"
class OverB(bytes):
    def __bytes__(self): return b"ob"
    def __repr__(self): return "OB"
o, oi, of, ob = Over("q"), OverI(1), OverF(1.0), OverB(b"z")
t("ov-str", lambda: (o == "q", o + "x", "x" + o, len(o), format(o, "s"), f"{o}", f"{o:>3}", o[0], "a" in o, "contains" if "a" in o else "no", o < "a", bool(o), o.upper(), str(o), repr(o), hash(o), {o: 1}, "%s" % o, "".join([o]), "{}".format(o)))
t("ov-int", lambda: (int(oi), oi + 1, 1 + oi, -oi, [0, 1, 2, 3, 4][oi], float(oi), repr(oi), str(oi), f"{oi}", "%s %r" % (oi, oi), hash(oi), oi == 1, bool(oi), range(oi), hex(oi), "%d" % oi, "%x" % oi, math.floor(OverI(1)) if False else 0, oi * 2, str(int(oi)), abs(oi)))
t("ov-float", lambda: (float(of), repr(of), str(of), of + 1, math.sqrt(of), "%f" % of, "%s" % of))
t("ov-bytes", lambda: (bytes(ob), repr(ob), str(ob), ob + b"x", ob == b"z", b"".join([ob]), len(ob)))
class Pstr(str):
    def __str__(self): return "STR"
    def __repr__(self): return "REPR"
t("pstr", lambda: (str(Pstr("a")), repr(Pstr("a")), f"{Pstr('a')}", "%s %r" % (Pstr("a"), Pstr("a")), "{} {!r} {!s}".format(Pstr("a"), Pstr("a"), Pstr("a")), "".join([Pstr("a")]), Pstr("a") + "b", [Pstr("a")], (Pstr("a"),), {Pstr("a"): Pstr("b")}, print(Pstr("a")), format(Pstr("a")), json.dumps(Pstr("a")), posixpath.join(Pstr("a"), "b"), "a".join([Pstr("b"), Pstr("c")])))
print(Pstr("pp"), Pstr("p2"), sep="|")
print([Pstr("a")], {1: Pstr("b")}, (II(3), SS("x"), Fs(.5), Bs(b"y")))
t("isinst", lambda: (isinstance(ss, SS), isinstance(ss, str), isinstance(ii, (str, int)), issubclass(SS, str), issubclass(type(ii), int), type(ss) is SS, type(ss) == SS, ss.__class__ is SS, ss.__class__.__mro__[1].__name__, SS.__mro__[1] is str, isinstance(ii, float), isinstance(True, II)))
t("attrs", lambda: (setattr(ss, "z", 1) if False else 0, getattr(ii, "real"), getattr(ss, "upper")(), hasattr(ss, "nope"), getattr(ss, "nope", "d"), ii.numerator, ii.imag, fs.real, cs.imag, ii.__add__(1), ii.__index__() if False else 0))
def _dictattr():
    class W(str): pass
    w = W("w"); w.a = 1; w.b = 2
    return (w.__dict__, vars(w), w.a, sorted(dir(w))[:0], w == "w", w)
t("instdict", _dictattr)
t("id-ident", lambda: (ss is ss, ss == SS("hi"), id(ss) == id(ss), [ss].index(ss), [ss].count("hi"), [SS("a")] == ["a"], (II(1),) == (1,), {"k": II(1)} == {"k": 1}))
t("list-ops", lambda: (lambda l: (l.append(ss), l.insert(0, ii), l.extend([fs]), [type(x).__name__ for x in l], l.index(ss), l.remove(ii), l)[3])([]))
t("list-store", lambda: ([type(x).__name__ for x in [SS("a")] + [II(1)]], [type(x).__name__ for x in list((SS("a"), II(1)))], [type(x).__name__ for x in sorted([SS("b"), SS("a")])], [type(x).__name__ for x in {SS("a"): 1}], sorted(type(x).__name__ for x in set([SS("a")])), type(dict(a=SS("x"))["a"]).__name__, type(dict([(SS("k"), II(1))])[ "k"]).__name__, [type(v).__name__ for v in {"a": II(1)}.values()]))
def _stores():
    import collections
    d = {}; d.setdefault(SS("k"), II(1)); d.update({SS("j"): II(2)}); d.update(m=SS("v"))
    s = set(); s.add(SS("s"))
    q = collections.deque(); q.append(SS("q")); q.appendleft(II(1)); q.extend([Fs(1.0)])
    od = collections.OrderedDict(); od[SS("o")] = II(1)
    c = collections.Counter([SS("a"), "a"]); nt = collections.namedtuple("NT", "a b")(SS("x"), II(2))
    dd = collections.defaultdict(list); dd[SS("z")].append(II(1))
    return ([(type(k).__name__, type(v).__name__) for k, v in d.items()], sorted(type(x).__name__ for x in s), [type(x).__name__ for x in q], dict(c), d.get(SS("k")), d.pop(SS("k")), SS("j") in d)
t("stores", _stores)
def _misc():
    import itertools, heapq, bisect, random, time, datetime, decimal, fractions, textwrap, shlex, base64, hashlib, binascii, zlib, codecs, unicodedata, html, urllib.parse
    h = []; heapq.heappush(h, II(3)); heapq.heappush(h, 1); 
    return (list(itertools.chain(SS("a"), [II(1)])), list(itertools.islice(itertools.count(II(1)), II(3))), list(itertools.repeat(SS("r"), II(2))), heapq.heappop(h), bisect.bisect([1, 2, 3], II(2)), textwrap.fill(SS("a b c"), II(3)), textwrap.dedent(SS("  a")), shlex.split(SS("a 'b c'")), base64.b64encode(Bs(b"hi")), base64.b64decode(SS("aGk=")), hashlib.md5(Bs(b"a")).hexdigest(), binascii.hexlify(Bs(b"a")), zlib.crc32(Bs(b"a")), codecs.encode(SS("a"), SS("utf-8")), codecs.decode(Bs(b"a"), SS("ascii")), unicodedata.name(SS("a")), html.escape(SS("<")), urllib.parse.quote(SS("a b")), urllib.parse.urlparse(SS("http://x/y")).path, decimal.Decimal(SS("1.5")) + II(1), decimal.Decimal(II(3)), datetime.timedelta(days=II(1)), datetime.date(II(2020), II(1), II(2)), time.strftime(SS("%Y"), time.gmtime(II(0))), random.Random(II(1)).randint(II(1), II(1)), math.gcd(II(4), 6), math.isqrt(II(16)), math.factorial(II(4)), math.pow(II(2), Fs(2)), math.hypot(II(3), Fs(4)), math.isclose(Fs(1), 1), math.copysign(Fs(1), II(-1)), divmod(II(10), II(3)), str.upper(SS("a")), str.join(SS(","), [SS("a"), "b"]), bytes.decode(Bs(b"a")), int.bit_length(II(8)), dict.fromkeys([SS("a")], II(1)), "".join(map(str, [SS("a"), II(1)])), list(map(int, [SS("1"), II(2)])), list(filter(None, [SS(""), SS("a")])), functools.reduce(operator.add, [II(1), II(2), 3]), operator.add(SS("a"), "b"), operator.mul(II(2), 3), operator.eq(SS("a"), "a"), operator.contains(SS("abc"), SS("b")), operator.getitem(SS("abc"), II(1)), operator.itemgetter(II(0))("x"), operator.attrgetter(SS("real"))(5), getattr(5, SS("real")), isinstance(SS("a"), type(SS("b"))))
t("misc", _misc)
t("struct2", lambda: (struct.unpack(SS(">H"), Bs(b"\x00\x01")), struct.calcsize(SS("I")), struct.pack(SS("<d"), Fs(1.5))))
t("io", lambda: (lambda b: (b.write(SS("ab")), b.write(II(0) and "" or "c"), b.getvalue()))(__import__("io").StringIO())[2])
t("io-bytes", lambda: (lambda b: (b.write(Bs(b"ab")), b.getvalue())[1])(__import__("io").BytesIO(Bs(b"x"))))
t("print-file", lambda: (print(SS("a"), II(1), Fs(.5), Bs(b"b"), sep=SS("-"), end=SS("!\n"), file=__import__("sys").stdout)))
t("exc", lambda: (str(ValueError(SS("m"))), ValueError(SS("m")).args, repr(KeyError(SS("k"))), 0))
def _raise():
    try: raise ValueError(SS("boom"))
    except ValueError as e: return (str(e), type(e.args[0]).__name__)
t("raise", _raise)
t("getitem-dict", lambda: ({SS("a"): 1}.get(SS("a")), {II(1): 2}.get(1), {1.5: 3}.get(Fs(1.5)), {b"k": 1}[Bs(b"k")], {(SS("a"), 1): 2}[("a", II(1))]))
t("tuple-key", lambda: {(SS("a"), II(1)): 1}[("a", 1)])
t("kwargs", lambda: dict(**{SS("a"): 1}) if False else (lambda **k: k)(**{"a": II(1)}))
t("star", lambda: (print(*[SS("a")], sep=""), [*SS("ab")], {**{"a": II(1)}}))
t("match", lambda: (lambda x: {"s": "S"}.get(x, "?"))(SS("s")))
t("sliceidx", lambda: ([1, 2, 3][II(1):], "abc"[:II(2)], [1, 2, 3, 4][::II(2)], "abc"[II(-1)], list(range(II(5)))[II(1):II(3)], [0] * II(0)))
t("int-edge", lambda: (II(2**70) + 1, II(2**70) * 2, str(II(2**70)), int(II(2**70)), hash(II(2**70)) == hash(2**70), II(2**70) == 2**70, II(2**70) > 5, repr(II(2**70)), II(2**70).bit_length(), -II(2**70), II(2**70) // 3, II(2**70) % 7, divmod(II(2**70), 5), [1][II(0)], II(2**70) >> 60, pow(II(2), 80)))
t("neg-err", lambda: [][SS("a")])
t("neg-err2", lambda: "a" + II(1))
t("neg-err3", lambda: II(1) + "a")
t("neg-err4", lambda: int(SS("zz")))
t("neg-err5", lambda: float(SS("zz")))
t("neg-err6", lambda: SS("a") < II(1))
t("neg-err7", lambda: "x".join([II(1)]))
t("neg-err8", lambda: chr(SS("a")))
t("neg-err9", lambda: "abc"[SS("1")])
t("neg-err10", lambda: bytes(SS("a")))
