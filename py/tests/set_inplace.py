def f(a, b):
    a |= b
D = {'k': set()}
f(D['k'], {1})
print(D)
x = set(); y = x; y |= {2}; print(x)
l = []; m = l; m += [1]; print(l)
class T:
    def __init__(s, n): s.n = n
    def __hash__(s): return hash(s.n)
    def __eq__(s, o): return s.n == o.n
D = {'k': set()}
f(D['k'], {T(1)}); print(len(D['k']))
a = {1, 2, 3}; b = a; a &= {2, 3, 9}; print(b); a -= {2}; print(b); a ^= {3, 7}; print(sorted(b))
f = frozenset({1}); g = f; f |= {2}; print(g, f, type(f))
s = {1}; s |= frozenset({5}); print(s)
