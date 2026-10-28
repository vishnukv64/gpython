class C:
    __slots__ = ('a', 'b')
    def __init__(self, a):
        self.a = a

c = C(1)
print(c.a)
c.b = 2
print(c.b)
for attr in ('z',):
    try:
        setattr(c, attr, 1)
    except AttributeError as e:
        print('AttributeError', e)
for expr in ('c.__dict__', 'vars(c)'):
    try:
        eval(expr)
    except (AttributeError, TypeError) as e:
        print(type(e).__name__, e)
del c.a
try:
    del c.a
except AttributeError as e:
    print('AttributeError', e)
print(hasattr(c, 'a'), hasattr(c, '__dict__'))
print(type(C.a).__name__, C.a)
print(C.__slots__)

class D(C):
    pass
d = D(1)
d.new = 5
print(d.new, sorted(d.__dict__))

class E(C):
    __slots__ = ('c',)
e = E(1)
e.c = 3
e.b = 4
print(e.a, e.b, e.c)
try:
    e.d = 1
except AttributeError as ex:
    print('AttributeError', ex)

class F:
    __slots__ = ('x', '__dict__')
f = F()
f.x = 1
f.y = 2
print(f.x, f.y, f.__dict__)

class G:
    __slots__ = ('x', '__weakref__')
import weakref
g = G()
r = weakref.ref(g)
print(r() is g)

class H:
    __slots__ = 'only'
h = H()
h.only = 1
print(h.only)
try:
    h.other = 1
except AttributeError as ex:
    print('AttributeError', ex)

class P:
    __slots__ = ('__priv',)
    def __init__(self):
        self.__priv = 7
    def get(self):
        return self.__priv
print(P().get(), '_P__priv' in P.__dict__)

try:
    class Bad:
        __slots__ = ('a',)
        a = 1
except ValueError as ex:
    print('ValueError', ex)
try:
    class Bad2:
        __slots__ = (1,)
except TypeError as ex:
    print('TypeError', ex)
try:
    class Bad3(F):
        __slots__ = ('__dict__',)
except TypeError as ex:
    print('TypeError', ex)

class Empty:
    __slots__ = ()
try:
    Empty().x = 1
except AttributeError as ex:
    print('AttributeError', ex)

class Prop:
    __slots__ = ('_v',)
    @property
    def v(self):
        return self._v
    @v.setter
    def v(self, x):
        self._v = x
p = Prop()
p.v = 9
print(p.v)
