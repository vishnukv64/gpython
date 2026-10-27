from collections import deque
d = deque([1,2,3,2])
d.remove(2); print(d, d.count(2), d.index(3), d.maxlen)
d.rotate(1); print(d); d.rotate(-1); print(d); d.reverse(); print(d)
d.insert(1, 9); del d[0]; print(d, d.copy(), deque(maxlen=3).maxlen)
import itertools
print(list(itertools.chain([1],[2,3])), list(itertools.chain.from_iterable([[1],[2,3]])))
print(list(itertools.chain.from_iterable(x for x in [[1],[2]])))
class F:
    __x = None
    __slots__ = ()
class G:
    __x = None
    def f(self): return self.__x
    def g(self): self.__x = 3; return self._G__x
print(G().f(), G().g(), [k for k in vars(G) if 'x' in k])
print([k for k in vars(F) if 'x' in k])
from collections import namedtuple
S = namedtuple('S', ['a','b'])
S.a.__doc__ = "x"
s = S(1,2)
print(s.a, s.b, s, S.a.__doc__)
class T(S):
    pass
print(T(3,4).b)
class E(BaseException):
    def __str__(self): return "custom"
class F(E): pass
print(str(E("a")), str(F("a")))
class G(BaseException):
    def __new__(cls, m, x):
        self = BaseException.__new__(cls, m, x)
        return self
    def __str__(self): return "custom"
print(str(G("a", 1)))
async def agen(n):
    for i in range(n):
        yield i
async def main():
    print([x async for x in agen(3)])
    print([x async for x in agen(6) if x % 2])
    print({x: x async for x in agen(2)})
    print(sum([x for x in range(3) if await z(x)]))
    g = (x async for x in agen(3))
    print(type(g).__name__, [y async for y in g])
async def z(x): return x
c = main()
try: c.send(None)
except StopIteration: print("done")
