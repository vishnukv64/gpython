import functools
from collections.abc import Mapping, Sequence

@functools.singledispatch
def g(x):
    "doc of g"
    return 'o'

@g.register
def _(x: int):
    return 'i'

@g.register
def _(x: str):
    return 's'

@g.register(float)
def _(x):
    return 'f'

@g.register(Mapping)
def _(x):
    return 'map'

class A: pass
class B(A): pass

@g.register(A)
def _(x):
    return 'A'

print(g(1), g('s'), g(1.0), g(None), g([]), g({}), g(B()), g(True))
print([t.__name__ for t in g.registry])
print(g.dispatch(int) is g.registry[int], g.dispatch(bytes) is g.registry[object])
print(g.__name__, g.__doc__)

class Foo:
    @functools.singledispatchmethod
    def m(self, x):
        return 'o'
    @m.register
    def _(self, x: int):
        return 'i'
    @m.register(str)
    def _(self, x):
        return 's'
    @functools.singledispatchmethod
    @classmethod
    def c(cls, x):
        return 'co'
    @c.register(int)
    @classmethod
    def _(cls, x):
        return cls.__name__ + 'ci'

print(Foo().m(1), Foo().m('a'), Foo().m(2.0))
print(Foo.c(1), Foo.c('x'))

try:
    @g.register
    def bad(x): pass
except TypeError as e:
    print('TypeError')
try:
    g()
except TypeError as e:
    print('TypeError no args')
