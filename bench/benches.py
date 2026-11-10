# Each benchmark returns a checksum so both interpreters are checked for the
# same answer, not just timed.
import sys, time

def fib():
    def f(n): return n if n < 2 else f(n - 1) + f(n - 2)
    return f(25)

def loops():
    s = 0
    for i in range(1_000_000):
        s += i * i % 7
    return s

def floats():
    x = 0.0
    for i in range(1, 300_000):
        x += (i ** 0.5) / (i + 1.5)
    return round(x, 6)

def strings():
    parts = []
    for i in range(100_000):
        parts.append(f"{i}:{i * 2:05d}")
    s = ",".join(parts)
    return len(s.split(",")) + len(s.upper()) + s.count("9") + len(s.replace("1", "one"))

def lists():
    a = [i * 7 % 1000 for i in range(200_000)]
    a.sort()
    b = sorted(a, key=lambda v: -v)
    return sum(a[::3]) + b[0] + len([x for x in a if x % 2])

def dicts():
    d = {}
    for i in range(200_000):
        d[f"k{i % 5000}"] = d.get(f"k{i % 5000}", 0) + i
    return sum(d.values()) + len(d)

def classes():
    class P:
        __slots__ = ("x", "y")
        def __init__(self, x, y): self.x, self.y = x, y
        def add(self, o): return P(self.x + o.x, self.y + o.y)
    p = P(0, 0)
    one = P(1, 2)
    for _ in range(200_000):
        p = p.add(one)
    return p.x + p.y

def generators():
    def gen(n):
        for i in range(n):
            if i % 3: yield i
    return sum(x * 2 for x in gen(500_000))

def bigint():
    a, b = 0, 1
    for _ in range(5000):
        a, b = b, a + b
    return len(str(a)) + (3 ** 2000 % 1_000_003)

def regex():
    import re
    pat = re.compile(r"(\w+)@(\w+)\.com")
    text = " ".join(f"user{i}@host{i % 50}.com junk{i}" for i in range(20_000))
    return len(pat.findall(text)) + len(re.sub(r"\d+", "#", text))

def json_():
    import json
    data = [{"id": i, "name": f"n{i}", "tags": ["a", "b"], "v": i / 3} for i in range(20_000)]
    s = json.dumps(data)
    return len(s) + len(json.loads(s))

def exceptions():
    n = 0
    for i in range(100_000):
        try:
            if i % 2: raise ValueError(i)
        except ValueError:
            n += 1
    return n

def closures():
    def make(k):
        return lambda x: x + k
    fs = [make(k) for k in range(100)]
    s = 0
    for i in range(3_000):
        for f in fs:
            s = f(s) % 1_000_007
    return s

BENCHES = [fib, loops, floats, strings, lists, dicts, classes, generators,
           bigint, regex, json_, exceptions, closures]

if __name__ == "__main__":
    want = sys.argv[1:]
    for b in BENCHES:
        if want and b.__name__ not in want:
            continue
        best, res = None, None
        for _ in range(3):
            t = time.perf_counter()
            res = b()
            dt = time.perf_counter() - t
            best = dt if best is None or dt < best else best
        print(f"{b.__name__} {res} {best:.6f}", flush=True)
