# Regression goldens for interpreter fixes; golden is CPython 3.14 output.
import io, re, struct, types

class G:
    def __getattr__(self, n):
        return object.__getattribute__(self, "_" + n)
    _a = 5
print("getattr", G().a)
print("escape", re.escape("a b\tc\n"))
print("id None", id(None) == id(None))

class E:
    def __init__(self, v): self.v = v
    def __eq__(self, o): return self.v == o.v
print("ne", E(1) != E(1), E(1) != E(2))

def f(xs, w):
    return [f"{x}{w}" for x in xs], (lambda y: f"{y}{w}")(1)
print("fstr", f([1, 2], "z"))
print("nested spec", f"{3.14159:{8}.{2}f}|", "{:{}d}".format(3, 4))

print("atomic", re.match(r"(?>a+)b", "aaab").group(), re.match(r"(?>a+)ab", "aaab"))

class S(io.StringIO): pass
class B(io.BytesIO): pass
print("io", type(S("x")).__name__, type(B(b"x")).__name__, S("hi").read())

class M(types.ModuleType): pass
m = M("mm")
print("mod", type(m).__name__, m.__name__)
class D:
    def __get__(self, obj, owner): return ("got", obj is None)
class H: d = D()
print("desc", H.d, H().d)

st = struct.Struct("<hi")
print("struct", st.size, st.pack(1, 2), st.unpack(st.pack(1, 2)))
print("iter_unpack", list(struct.iter_unpack("<h", b"\x01\x00\x02\x00")))
buf = bytearray(6)
struct.pack_into("<hi", buf, 0, 7, 9)
print("pack_into", bytes(buf))
def g(*, a=1, b): return a, b
print("kwonly", g(b=2))
print("fz", hash(frozenset("ab")) == hash(frozenset("ba")), len({frozenset([1]), frozenset([1])}))
print("range", range(2, 9, 3).start, range(2, 9, 3).stop, range(2, 9, 3).step, hash(range(0)) == hash(range(3, 3)))
def h():
    raise
try:
    try: raise KeyError("k")
    except KeyError: h()
except KeyError as e: print("reraise", repr(e))
