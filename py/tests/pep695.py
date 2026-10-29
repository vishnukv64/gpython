"""PEP 695 / 696: type statement, generic functions and classes."""
import typing


def show(label, f):
    try:
        print(label, "->", f())
    except BaseException as e:
        print(label, "->", type(e).__name__, e)


type X = int
print(type(X).__name__, X, X.__name__, X.__value__, X.__type_params__)

calls = []


def late():
    calls.append("evaluated")
    return str


type Lazy = late()
print("not evaluated yet:", calls)
print(Lazy.__value__, calls)
print(Lazy.__value__, calls)

type Forward = Undefined
show("forward", lambda: Forward.__value__)
Undefined = bytes
print(Forward.__value__)

type Pair[A, B] = tuple[A, B]
print(Pair.__type_params__, Pair.__value__)
print(Pair[int, str].__args__ if hasattr(Pair[int, str], "__args__") else "?")
show("nongeneric subscript", lambda: X[int])


def f[T](x: T) -> T:
    return x


print(f.__type_params__, f.__name__, f.__qualname__, f(3))
T = f.__type_params__[0]
print(type(T).__name__, T, T.__name__, T.__bound__, T.__constraints__,
      T.__covariant__, T.__contravariant__, T.__infer_variance__)


def g[T: int, U: (int, str), *Ts, **P](a: T, b: U) -> None:
    pass


T, U, Ts, P = g.__type_params__
print(T, U, Ts, P)
print(T.__bound__, T.__constraints__)
print(U.__bound__, U.__constraints__)
print(type(Ts).__name__, type(P).__name__, P.args, P.kwargs)

seen = []


def lazy_bound():
    seen.append("bound")
    return int


def h[T: lazy_bound()](x): pass


print("bound lazy:", seen)
print(h.__type_params__[0].__bound__, seen)
print(h.__type_params__[0].__bound__, seen)


class C[T]:
    def m(self, x: T) -> T:
        return x

    def tp(self):
        return T

    attr = [T]


print(C.__type_params__, C.__name__, C.__qualname__, C.__mro__[1:])
print(C().tp(), C.attr, C().m(5))
print(C.m.__qualname__)
print(C[int])


class D[K: str, V = int](dict):
    pass


print(D.__type_params__, [p.has_default() for p in D.__type_params__])
K, V = D.__type_params__
print(V.__default__, K.__default__ is typing.NoDefault)


def dflt[A = int, *Bs = *tuple[int, str], **Q = [int]](): pass


A, Bs, Q = dflt.__type_params__
print(A.__default__, A.has_default(), Q.has_default(), Bs.has_default())


# shadowing: a class attribute and a type parameter with the same name
class E[T]:
    T = 5
    def m(self) -> T: ...


print(E.T)


# type parameters are not visible outside
show("outside", lambda: T2)
def uses[T2](): pass
show("outside again", lambda: T2)


# decorators apply to the result, outside the type params
def deco(fn):
    fn.decorated = True
    return fn


@deco
def dd[T](): pass


print(dd.decorated, dd.__type_params__)


def outer():
    def inner[T](x: T): return x
    class Inner[T]: pass
    return inner.__qualname__, Inner.__qualname__


print(outer())

# nested generics: inner sees the outer parameter
def make[T]():
    def inner[U](x: T, y: U): return (T, U)
    return inner


print(make().__type_params__, make()(1, 2))

for src in (
    "def f[T, T](): pass",
    "class C[T, T]: pass",
    "type X[T, T] = int",
    "def f[T = int, U](): pass",
    "def f[T: (int,)](): pass",
    "def f[](): pass",
    "type = 5\ntype\n",
    "type X = int; type Y = str",
    "type X[T] = list[T]",
    "type X" + chr(10),
    "type X[T: (a, b)] = T",
    "def f[*T: int](): pass",
    "class C[**P = int]: pass",
):
    try:
        compile(src, "<s>", "exec")
        print("ok", repr(src))
    except SyntaxError as e:
        print("SyntaxError", repr(src), e.msg)

type = 7
print(type)
match 3:
    case type:
        print("matched", type)
