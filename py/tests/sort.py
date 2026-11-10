calls = []
def key(x):
    calls.append(x)
    return -x
a = [5, 3, 9, 1, 3, 7, 2, 8] * 20
sorted(a, key=key)
print("key calls", len(calls), len(a))
calls.clear(); b = list(a); b.sort(key=key, reverse=True); print(len(calls), b[:5])

# stability with reverse
p = [(1, 'a'), (0, 'b'), (1, 'c'), (0, 'd'), (1, 'e')]
print(sorted(p, key=lambda t: t[0]))
print(sorted(p, key=lambda t: t[0], reverse=True))
print(sorted(iter([3, 1, 2]), reverse=True), sorted("hello"), sorted({3: 1, 1: 2}))

print(sorted([3, 1.5, 2, 2.0, True, -0.0, 0, 1e100, 10**30]))
nan = float("nan")
for l in ([3.0, nan, 1.0, 2.0], [1.0, 2.0, nan, 0.0, nan, -1.0]):
    print(repr(sorted(l)), repr(sorted(l, reverse=True)))
print(sorted(["b", "é", "a", "z", "\u4e2d", "\U0001f600", "\uffff", "B", ""]))
print(sorted([(2, "a"), (1, "z"), (1, "b"), (2, "A")]))

def bad(x):
    if x == 4: raise KeyError(x)
    return x
l = [5, 4, 3]
try: l.sort(key=bad)
except KeyError as e: print("key err", e, sorted(l))
class Bad:
    def __init__(s, v): s.v = v
    def __lt__(s, o):
        if s.v == 3 or o.v == 3: raise RuntimeError("lt")
        return s.v < o.v
l = [Bad(i) for i in (5, 3, 1, 4)]
try: l.sort()
except RuntimeError as e: print("lt err", e, sorted(x.v for x in l))
try: sorted([1, "a"])
except TypeError as e: print(e)

l = [3, 1, 2]
def mut(x):
    print("len during", len(l))
    l.append(9)
    return x
try: l.sort(key=mut)
except ValueError as e: print(e, l)
l = [3, 1, 2]
print(l.sort(key=lambda x: x), l)

class C:
    def __init__(s, v): s.v = v
    def __lt__(s, o): return s.v > o.v
    def __repr__(s): return "C%d" % s.v
print(sorted(map(C, [1, 3, 2])), sorted([]), sorted([1]), sorted([2], key=lambda x: calls.append(x) or x))
