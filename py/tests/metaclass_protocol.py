# Metaclass protocol: __init__ receives the class statement's arguments,
# __call__ governs instance creation and super().__call__ constructs, mro()
# computes (and can be overridden), and builtin generic classes subscript.
import re, typing

class M(type):
    def __init__(cls, name, bases, ns, **kw):
        super().__init__(name, bases, ns, **kw)
        cls.tag = (name, len(bases), sorted(k for k in ns if not k.startswith("__")), kw)
    def __new__(mcls, name, bases, ns, **kw):
        return super().__new__(mcls, name, bases, ns)
    def __call__(cls, *a, **k):
        print("M.__call__", cls.__name__, a, k)
        return super().__call__(*a, **k)

class H(metaclass=M, flavour="x"):
    def __init__(self, v=0):
        self.v = v
    def meth(self):
        return self.v

print(H.tag)
h = H(5)
print(type(h).__name__, h.v, h.meth())

class Sub(H):
    pass
print(Sub.tag, Sub(7).v)

class Single(type):
    _inst = {}
    def __call__(cls, *a):
        if cls not in Single._inst:
            Single._inst[cls] = super().__call__(*a)
        return Single._inst[cls]

class Cfg(metaclass=Single):
    def __init__(self, n=0):
        self.n = n
print(Cfg(1) is Cfg(2), Cfg().n)

class Plain(type):
    def __init__(cls, name, bases, ns):
        cls.made = name
class P(metaclass=Plain):
    pass
print(P.made, type(P).__name__)

class A: pass
class B(A): pass
print(B.mro(), type.mro(B), int.mro())

class RM(type):
    def mro(cls):
        return [cls, object]
class D(A, metaclass=RM):
    pass
print(D.__mro__, issubclass(D, A))

print(re.Pattern[str], re.Match[bytes])
