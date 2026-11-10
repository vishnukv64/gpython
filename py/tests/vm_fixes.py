import copy

# 2a: a data descriptor added later beats the instance dict
class A:
    pass
a = A()
a.x = 1
A.x = property(lambda self: "prop")
print(a.x)

# 2b: __getattribute__ applies to method calls
class B:
    def foo(self):
        return "real"
    def __getattribute__(self, n):
        if n == "foo":
            return lambda: "hooked"
        return object.__getattribute__(self, n)
print(B().foo())

# non-data descriptor loses to the instance dict
class C:
    def m(self):
        return "class"
c = C()
c.m = lambda: "inst"
print(c.m())

# 7: identity never panics
print(copy.deepcopy(b"x") == b"x")
print(copy.deepcopy([b"a", (1, b"b")]))
t = b"q" * 3
print(t is t, [] is [], {} is {})
l = [1]; d = {}
print(l is l, d is d, l is d)

# 8: dict subclass keeps instance attrs
class DS(dict):
    pass
d = DS(a=[1])
d.z = 5
print(d.z, d.__dict__, d)
print(copy.deepcopy(d).z)

# python-level data descriptor beats the instance dict; non-data loses
class DD:
    def __get__(self, o, t=None): return "dd"
    def __set__(self, o, v): pass
class ND:
    def __get__(self, o, t=None): return "nd"
class E:
    dd = DD()
    nd = ND()
e = E()
e.__dict__["dd"] = 1
e.__dict__["nd"] = 2
print(e.dd, e.nd)
for _ in range(3):
    print(e.dd, e.nd)

# hook plus __getattr__ fallback
class F:
    def __getattr__(self, n): return "ga:" + n
    def __getattribute__(self, n):
        if n == "zzz":
            raise AttributeError(n)
        return object.__getattribute__(self, n)
f = F()
print(f.zzz, f.yyy)
# bases change visible to cached lookups
class G1: v = 1
class G2(G1): pass
g = G2()
print([g.v for _ in range(3)])
G1.v = 2
print([g.v for _ in range(3)])
G2.v = 3
print(g.v)
del G2.v
print(g.v)
