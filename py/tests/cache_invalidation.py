import builtins

# reassigning a global inside a loop that reads it
g = 0
def read():
    return g
out = []
for i in range(5):
    g = i * 10
    out.append(read())
print(out)

# del of a global, then NameError
x = 1
def rx():
    return x
print(rx())
del x
try:
    rx()
except NameError as e:
    print("NameError", e)
globals()["x"] = 7
print(rx())

# builtins monkeypatched after first use
def ln(o):
    return len(o)
print(ln("abc"))
real_len = builtins.len
builtins.len = lambda o: -1
print(ln("abc"))
builtins.len = real_len
print(ln("abc"))
builtins.zork = 5
def rz():
    return zork
print(rz())
builtins.zork = 6
print(rz())
zork = 9
print(rz())
del zork
print(rz())
del builtins.zork

# a class method replaced after instances have called it
class A:
    def m(self):
        return "orig"
a = A()
print([a.m() for _ in range(3)])
A.m = lambda self: "new"
print([a.m() for _ in range(3)])

# an instance attribute shadowing a class attribute after the cache warmed
class B:
    v = "class"
    def f(self):
        return "classf"
b = B()
def rv(o):
    return o.v, o.f()
print(rv(b), rv(b))
b.v = "inst"
b.f = lambda: "instf"
print(rv(b))
del b.v
del b.f
print(rv(b))

# a base-class attribute change seen through a subclass
class Base:
    k = 1
    def meth(self):
        return "base"
class Sub(Base):
    pass
s = Sub()
def rk(o):
    return o.k, o.meth()
print(rk(s))
Base.k = 2
Base.meth = lambda self: "base2"
print(rk(s))
Sub.k = 3
print(rk(s))
del Sub.k
print(rk(s))

# __class__ assignment
class P:
    def who(self):
        return "P"
class Q:
    def who(self):
        return "Q"
o = P()
print([o.who() for _ in range(2)])
o.__class__ = Q
print([o.who() for _ in range(2)])

# a property added later
class R:
    pass
r = R()
r.other = "plain"
R.val = "plain"
def rval(o):
    return o.val
print(rval(r))
R.val = property(lambda self: "prop")
print(rval(r))
R.val = property(lambda self: "prop", lambda self, v: None)
r.val = "assigned via setter"
print(rval(r))

# int/float fast paths keep CPython semantics
for a_, b_ in [(2**62, 2**62), (-2**63, -1), (3037000500, 3037000500), (7, -3), (-7, 3), (True, 2), (1.5, 2)]:
    print(a_ + b_, a_ - b_, a_ * b_, a_ % b_, a_ < b_, a_ == b_)
nan = float("nan")
print(nan < nan, nan == nan, nan != nan)
