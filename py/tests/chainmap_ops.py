from collections import ChainMap
c = ChainMap({'a':1,'b':2}, {'b':3,'c':4})
for expr in ("sorted(c.items())", "sorted(c.keys())", "sorted(c.values())", "c.get('c')", "c.get('z', 9)", "len(c)", "sorted(c)", "dict(c)", "'a' in c", "c.maps", "c.new_child({'q':1})['q']", "c.parents.maps", "c.pop('a')", "c.popitem()", "c.setdefault('k', 5)", "c.update(x=1) or c['x']", "repr(c)", "c.copy().maps", "c == ChainMap({'k':5,'x':1,'b':2})", "c | {'z':1}", "c.clear()", "c.maps"):
    try: print(expr, '->', eval(expr))
    except Exception as e: print(expr, 'ERR', type(e).__name__, e)

class Sub(ChainMap):
    def __missing__(self, key):
        return "missing:" + key
s = Sub({"a": 1})
print(type(s).__name__, s["a"], s["zz"], type(s.new_child()).__name__, type(s.parents).__name__)
print(ChainMap.fromkeys("ab", 0).maps, repr(ChainMap()), bool(ChainMap()), bool(ChainMap({}, {"x": 1})))
