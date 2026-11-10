import functools
class C:
    def __init__(self):
        self.get = functools.cache(self._get)
        self.calls = 0
    def _get(self, x):
        self.calls += 1
        return x * 2
c = C()
print(c.get(2), c.get(2), c.calls)
@functools.cache
def f(x): return x + 1
@functools.lru_cache
def g(x): return x + 2
@functools.lru_cache(maxsize=None)
def h(x): return x + 3
@functools.lru_cache(2)
def k(x): return x + 4
print(f(1), g(1), h(1), k(1), f.cache_info().hits, h.cache_info().maxsize, k.cache_info().maxsize)
