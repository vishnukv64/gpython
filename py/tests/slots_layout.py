import copy, pickle

class A:
    __slots__ = ('a', 'b')
class B(A):
    __slots__ = ('c',)
class C(B):
    __slots__ = ()
class D(C):
    __slots__ = ('d', '__dict__')

for cls in (A, B, C, D):
    o = cls()
    for n in ('a', 'b', 'c', 'd'):
        try:
            setattr(o, n, ord(n))
        except AttributeError as e:
            print(cls.__name__, 'set', n, e)
    for n in ('a', 'b', 'c', 'd'):
        try:
            print(cls.__name__, n, getattr(o, n))
        except AttributeError as e:
            print(cls.__name__, 'get', n, e)

d = D()
d.a = 1
d.zz = 2
print(d.__dict__, d.a)
del d.a
try:
    del d.a
except AttributeError as e:
    print('del', e)
try:
    d.a
except AttributeError as e:
    print('get', e)
d.a = 5
print(d.a)

try:
    vars(A())
except TypeError as e:
    print('vars', e)

class P:
    __slots__ = ('x', 'y')
    def __init__(self, x=None):
        self.x = x
p = P([1])
q = copy.copy(p)
print(q.x is p.x, hasattr(q, 'y'))
for proto in (2, 3, 4):
    s = pickle.loads(pickle.dumps(p, proto))
    print(s.x, hasattr(s, 'y'))

class X:
    __slots__ = ('x',)
class Y:
    __slots__ = ('y',)
try:
    class Z(X, Y):
        pass
except TypeError as e:
    print(e)
class E:
    pass
class W(X, E):
    pass
w = W(); w.x = 1; w.q = 2
print(w.x, w.__dict__)
