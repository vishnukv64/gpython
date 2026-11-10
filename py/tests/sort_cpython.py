# Sort order under inconsistent comparators must match CPython's listsort
# exactly. Deterministic (own LCG), so CPython's output is the golden.
nan = float('nan')
seed = 12345
def rnd(n):
    global seed
    seed = (seed * 1103515245 + 12345) & 0x7fffffff
    return (seed >> 8) % n

def fmt(l):
    return ' '.join('nan' if x != x else repr(x) for x in l)

class W:
    """compares by .v but NaN-like: v==-1 is unordered with everything"""
    def __init__(s, v, i): s.v = v; s.i = i
    def __lt__(s, o):
        if s.v < 0 or o.v < 0: return False
        return s.v < o.v
    def __repr__(s): return '%d.%d' % (s.v, s.i)

class R:
    """pseudo-random (inconsistent) answers, seeded per call sequence"""
    def __init__(s, i): s.i = i
    def __lt__(s, o): return rnd(3) == 0
    def __repr__(s): return str(s.i)

for n in list(range(0, 40)) + [63, 64, 65, 100, 127, 128, 129, 200, 500, 1000]:
    for rep in range(4):
        l = [nan if rnd(4) == 0 else float(rnd(20)) for _ in range(n)]
        print('f', n, fmt(sorted(l)), '|', fmt(sorted(l, reverse=True)))
        l = [W(rnd(8) - 1, i) for i in range(n)]
        print('w', n, sorted(l), sorted(l, reverse=True))
        l = [R(i) for i in range(n)]
        print('r', n, sorted(l))
        l = [(float(rnd(5)) if rnd(3) else -0.0, rnd(3)) for _ in range(n)]
        print('t', n, sorted(l))
        l = [(rnd(6), rnd(6), rnd(6)) for _ in range(n)]
        print('u', n, sorted(l)[:20], sorted(l, key=lambda t: t[1])[:20])
        l = [(str(rnd(6)), rnd(6)) for _ in range(n)]
        print('s', n, sorted(l)[:20])
        l = [(rnd(3), [rnd(3)]) for _ in range(n)]
        print('m', n, sorted(l)[:20])
