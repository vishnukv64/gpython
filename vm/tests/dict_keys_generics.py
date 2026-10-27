# dict keys keep their first-inserted subclass object; GenericAlias substitutes TypeVars
class T(tuple):
    def split(self): return "ok"
t = T(("a",))
d = {t: 1, True: 2}
d[1] = 5
d[("a",)] = 9
print([type(k).__name__ for k in d], d)
c = dict(d); print([type(k).__name__ for k in c.keys()])
del d[t]; d[("a",)] = 1; print([type(k).__name__ for k in d])
d.clear(); d[t]=1; print(type(next(iter(d))).__name__)

from typing import TypeVar, Dict
T=TypeVar("T"); U=TypeVar("U")
a = dict[T, list[U]]
print(a.__parameters__, a[int, str], a[int, str].__parameters__)
try: a[int]
except TypeError as e: print("TE")
try: dict[int, str][int]
except TypeError as e: print("TE2")

from typing import Callable, TypeVar, Union
import os
T=TypeVar("T")
A = Union[str, "os.PathLike[str]"]
X = Callable[[T], T]
print(X.__parameters__)
import types
g = types.GenericAlias(dict, (T, int))
print(g[str])
print(dict[T, int][str], list[T][int], tuple[T, ...][str])
