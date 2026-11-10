# inspect.signature and friends; the golden is CPython 3.14's output.
import functools
import inspect
from inspect import Parameter, Signature, signature


def show(label, fn):
    try:
        print(label, fn())
    except Exception as e:
        print(label, type(e).__name__, e)


def kinds(q, /, r, s=1, *args, t, u=2, **kw): ...
def plain(a, b=2): ...
def annotated(a: int, b: "str" = "x", *, c: float = 1.5) -> bool: ...
async def coro(x): ...
def gen(x):
    yield x
lam = lambda q, /, r: 0
lam2 = lambda a, b=2, /, c=3, *d, e, **k: 0


class C:
    def __init__(self, x=0, y=1): ...
    def meth(self, a, /, b): ...
    @classmethod
    def cm(cls, a): ...
    @staticmethod
    def sm(a, b): ...
    def __call__(self, z): ...


class D(C): ...
class E: ...


def deco(f):
    @functools.wraps(f)
    def w(*a, **k):
        return f(*a, **k)
    return w


@deco
def decorated(a, b=3): ...


def sigattr(): ...
sigattr.__signature__ = Signature([Parameter("only", Parameter.POSITIONAL_ONLY)])

for name in ["kinds", "plain", "annotated", "coro", "gen", "lam", "lam2"]:
    show(name, lambda: signature(globals()[name]))
show("class C", lambda: signature(C))
show("class D", lambda: signature(D))
show("class E", lambda: signature(E))
show("C.meth", lambda: signature(C.meth))
show("C().meth", lambda: signature(C().meth))
show("C.cm", lambda: signature(C.cm))
show("C.sm", lambda: signature(C.sm))
show("C()", lambda: signature(C(1)))
show("partial", lambda: signature(functools.partial(plain, 1)))
show("partial kw", lambda: signature(functools.partial(plain, b=5)))
show("decorated", lambda: signature(decorated))
show("decorated nofollow", lambda: signature(decorated, follow_wrapped=False))
show("sigattr", lambda: signature(sigattr))
show("not callable", lambda: signature(3))

s = signature(kinds)
for p in s.parameters.values():
    print(p.name, p.kind, p.kind.description, p.default is p.empty, p.annotation is p.empty)
print(s.return_annotation is s.empty)
print(signature(annotated).return_annotation, signature(annotated).parameters["a"].annotation)
print(s.replace(return_annotation=int))
print(s.parameters["s"].replace(default=9))
print(Parameter.POSITIONAL_ONLY < Parameter.KEYWORD_ONLY, int(Parameter.VAR_KEYWORD), str(Parameter.KEYWORD_ONLY))

# bind
sig = signature(kinds)
print(sig.bind(1, 2, 3, 4, t=5, z=6))
print(sig.bind(1, 2, t=5).arguments)
b = sig.bind(1, 2, t=5)
b.apply_defaults()
print(b.args, b.kwargs, dict(b.arguments))
show("bind missing", lambda: sig.bind(1))
show("bind missing t", lambda: sig.bind(1, 2))
show("bind posonly kw", lambda: sig.bind(q=1, r=2, t=3))
show("bind too many", lambda: signature(plain).bind(1, 2, 3))
show("bind extra kw", lambda: signature(plain).bind(1, c=3))
show("bind multiple", lambda: signature(plain).bind(1, a=3))
show("bind_partial", lambda: sig.bind_partial(1))

# invalid construction
show("bad order", lambda: Signature([Parameter("a", Parameter.KEYWORD_ONLY), Parameter("b", Parameter.POSITIONAL_ONLY)]))
show("dup", lambda: Signature([Parameter("a", Parameter.POSITIONAL_ONLY), Parameter("a", Parameter.KEYWORD_ONLY)]))
show("bad name", lambda: Parameter("1x", Parameter.POSITIONAL_ONLY))
print(Signature.from_callable(plain), signature(plain) == signature(plain), hash(signature(plain)) == hash(signature(plain)))

# getfullargspec
print(inspect.getfullargspec(kinds))
print(inspect.getfullargspec(annotated))
print(inspect.getfullargspec(C.meth))
print(inspect.getfullargspec(lam2))

# predicates
print(inspect.isfunction(plain), inspect.isfunction(len), inspect.isfunction(C))
print(inspect.ismethod(C().meth), inspect.ismethod(C.meth), inspect.isclass(C), inspect.isclass(C()))
print(inspect.iscoroutinefunction(coro), inspect.iscoroutinefunction(plain))
print(inspect.isgeneratorfunction(gen), inspect.isgeneratorfunction(plain))
print(inspect.isbuiltin(len), inspect.isbuiltin(plain), inspect.ismodule(inspect), inspect.isroutine(len), inspect.isroutine(C.meth))
c = coro(1)
print(inspect.isgenerator(gen(1)), inspect.iscoroutine(c))
c.close()

# unwrap, getattr_static, getmembers
print(inspect.unwrap(decorated) is not decorated, inspect.unwrap(decorated).__name__)
show("unwrap loop", lambda: inspect.unwrap(decorated, stop=lambda f: True) is decorated)
print(type(inspect.getattr_static(C, "sm")).__name__, inspect.getattr_static(C(), "meth").__name__)
show("static missing", lambda: inspect.getattr_static(C, "nope"))
print(inspect.getattr_static(C, "nope", 42))
print([n for n, _ in inspect.getmembers(C) if not n.startswith("__")])
print(inspect.getmembers(C, inspect.isfunction)[0][0])
print(inspect.getmro(D) == (D, C, object))
print(inspect.getdoc(inspect.cleandoc) is not None, inspect.cleandoc("  a\n    b\n   c"))
