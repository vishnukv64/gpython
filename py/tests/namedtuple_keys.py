# Named tuples key dicts and sets as the plain tuple they equal (and keep
# their own identity as keys); nested named tuples hash; IntEnum members keep
# their identity as keys; itertools.islice takes None bounds (rich does).
from collections import namedtuple
from enum import IntEnum
P = namedtuple("P", "x y")
Q = namedtuple("Q", "a b")
p = P(1,2)
d = {p: "a"}
print(d.get((1,2)), {(1,2): "b"}.get(p), len({p, (1,2)}), (1,2) in {p}, repr(list(d)[0]), repr(next(iter({p}))))
d[(1,2)] = "c"; print(d)
print(len({P(1,2), Q(1,2)}), {P(1,(3,4)): 1}.get((1,(3,4))))
class H(P):
    def __hash__(self): return 7
    def __eq__(self, o): return isinstance(o, H) and tuple(self) == tuple(o)
print({H(1,2): 1}.get((1,2)), {H(1,2): 1}.get(H(1,2)), len({H(1,2), (1,2)}))
class E(IntEnum):
    A = 404
print(repr(list({E.A: 1})[0]), {404: 2}.get(E.A), {E.A: 1}.get(404), repr(next(iter({E.A}))))
print(sorted({P(2,1), P(1,2), (1,2)}))
from typing import NamedTuple
class T(NamedTuple):
    r: int
    g: int
class C(NamedTuple):
    n: str
    t: T
print(hash(C("a", T(1, 2))) == hash(("a", (1, 2))), hash(T(1, 2)) == hash((1, 2)))
from itertools import islice
r=range(10)
for a in [(None,),(3,),(2,None),(2,8,None),(None,None,None),(1,None,3),(None,5,2),(5,2)]:
    print(a, list(islice(r,*a)))
for a in [(-1,),(1,-2),(1,5,0),("x",),(1.5,),(1,2,"s"),(1,2,3,4)]:
    try: islice(r,*a)
    except Exception as e: print(a, type(e).__name__, e)
def g():
    yield 1
    raise KeyError("boom")
try: list(islice(g(), 3, None))
except KeyError as e: print("propagated", e)
