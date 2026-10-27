import enum

class C(enum.Enum):
    Z = 1
    Y = 2
    def f(self): return self.name
    def g(self, n): return self.value + n
    @property
    def p(self): return self.value * 10
    @classmethod
    def cm(cls): return cls.__name__
    @staticmethod
    def sm(x): return x + 1
    def __str__(self): return "C!" + self.name
    def __repr__(self): return "R" + self.name

print(C.Z.f(), C.Y.g(5), C.Z.p, C.Z.cm(), C.cm(), C.Z.sm(1), C.sm(2))
print(str(C.Z), repr(C.Y), f"{C.Z}")
print(C.Z in C, list(C))

class D(enum.Enum):
    A = 1
    def f(self): return self.name
print(D.A.f(), str(D.A), repr(D.A))
try:
    D.A.nope
except AttributeError as e:
    print("AttributeError")
