# Class instantiation, exceptions, generators and iteration fast paths.

# --- __new__ / __init__ overrides ---
class A:
    def __init__(self, x, y=2, *, z=3):
        self.x, self.y, self.z = x, y, z
print(vars(A(1)), vars(A(1, 5, z=9)))

class N:
    def __new__(cls, v):
        o = object.__new__(cls)
        o.v = v * 2
        return o
    def __init__(self, v): self.w = v
n = N(4); print(n.v, n.w)

class Other:
    def __new__(cls): return 42
    def __init__(self): raise AssertionError("must not run")
print(Other())

class Sub(A): pass
print(vars(Sub(7)))

class BadInit:
    def __init__(self): return 1
try: BadInit()
except TypeError as e: print("TypeError", e)

class NoInit: pass
print(type(NoInit()).__name__)

class Slots:
    __slots__ = ("a", "b")
    def __init__(self, a, b): self.a, self.b = a, b
s = Slots(1, 2); print(s.a, s.b, hasattr(s, "__dict__"))
try: A()
except TypeError: print("TypeError")

# metaclass without hooks (gpython honours neither a custom __call__ nor __init__)
class Meta(type): pass
class M(metaclass=Meta):
    def __init__(self, q): self.q = q
print(M(5).q)

# class mutated after warm-up
class W:
    def __init__(self): self.tag = "old"
for _ in range(5): W()
W.__init__ = lambda self: setattr(self, "tag", "new")
print(W().tag)
W.__new__ = lambda cls: object.__new__(cls)
print(W().tag)
del W.__new__
class WBase:
    def __init__(self): self.tag = "base"
class WD(WBase): pass
for _ in range(5): WD()
WBase.__init__ = lambda self: setattr(self, "tag", "base2")
print(WD().tag)
WD.__init__ = lambda self: setattr(self, "tag", "derived")
print(WD().tag)

# --- exceptions ---
def f():
    try:
        raise ValueError(1)
    except ValueError as e:
        raise KeyError(2) from e
try: f()
except KeyError as e:
    print(repr(e), repr(e.__cause__), repr(e.__context__), e.__suppress_context__)

def g():
    try:
        raise ValueError(1)
    except ValueError:
        raise KeyError(2)
try: g()
except KeyError as e:
    print(repr(e.__context__), e.__suppress_context__, e.__cause__)

def h():
    try:
        raise ValueError(1)
    except ValueError:
        raise KeyError(2) from None
try: h()
except KeyError as e:
    print(e.__context__, e.__suppress_context__, e.__cause__)

cnt = 0
for i in range(50):
    try:
        if i % 2: raise ValueError(i)
    except ValueError as e:
        cnt += e.args[0]
print(cnt)

class E(Exception):
    def __init__(self, a, b): super().__init__(a); self.b = b
try: raise E(1, 2)
except E as e: print(e.args, e.b)
e = ValueError("x"); print(e.args, repr(e), e.__traceback__)
try: raise e
except ValueError as e2:
    print(e2 is e, e.__traceback__ is not None)
    tb = e.__traceback__

import traceback
def deep(n):
    if n == 0: raise RuntimeError("deep")
    deep(n - 1)
try: deep(3)
except RuntimeError as e:
    print([fs.name for fs in traceback.extract_tb(e.__traceback__)])

# --- generators ---
def gen():
    x = yield 1
    print("got", x)
    try:
        y = yield 2
        print("got", y)
    except KeyError as ke:
        print("caught", ke)
        yield 99
    finally:
        print("fin")
    return "done"
gg = gen(); print(next(gg), gg.send("a"))
print(gg.throw(KeyError("k")))
try: next(gg)
except StopIteration as si: print("stop", si.value)

g2 = gen(); next(g2); g2.close()
try: g2.send(1)
except StopIteration: print("closed")

def tf():
    try:
        yield 1
        yield 2
    finally:
        print("cleanup")
tt = tf()
for v in tt:
    print(v); break
tt.close()
t = tf(); next(t); t.close()
try: tf().send(5)
except TypeError as e: print(e)

def inner():
    r = yield 1
    yield r
    return "ret"
def outer():
    res = yield from inner()
    yield res
o = outer(); print(next(o), o.send("S"), next(o))
print(list(outer()))
def fib_gen(n):
    a, b = 0, 1
    for _ in range(n):
        if a % 2: yield a
        a, b = b, a + b
print(sum(x * 2 for x in fib_gen(20)))
def selfg():
    yield next(sg)
sg = selfg()
try: next(sg)
except ValueError as e: print(e)

# --- list iteration while mutated ---
l = [1, 2, 3]; seen = []
for x in l:
    seen.append(x)
    if len(l) < 6: l.append(x + 10)
print(seen)
l = [1, 2, 3, 4, 5]
for x in l:
    l.pop(); print(x)
l = [1, 2, 3]
it = iter(l); next(it); l.clear(); print(list(it))
l = [1, 2]; it = iter(l); print(list(it), list(it)); l.append(3); print(list(it))
print([y for y in [1, 2, 3] if y != 2], type(iter([])).__name__)

# --- range edge cases ---
print(list(range(10, 0, -3)), list(range(0)), list(range(5, 5)), list(range(5, 0)), list(range(0, 5, -1)))
print(list(range(-3, 3, 2)))
print(sum(range(2**62, 2**62 + 4)), list(range(2**63 - 3, 2**63 - 1)))
for i in range(3, -4, -2): print(i, end=" ")
print()
print([i * i for i in range(5)], [(i, j) for i in range(2) for j in range(2)])
try: range(0, 5, 0)
except ValueError as e: print(e)
print(sorted([3, 1, 2], key=lambda v: -v), list(map(lambda v: v + 1, [1, 2])), list(filter(lambda v: v % 2, range(7))))
