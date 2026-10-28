# profile.py (pure Python) and cProfile (_lsprof) over the same work.
# Timers are deterministic: each timer read advances a counter.
import cProfile, profile, pstats, io, sys

def fib(n):
    return n if n < 2 else fib(n - 1) + fib(n - 2)

def helper(x):
    return x * 2

def work():
    t = 0
    for i in range(3):
        t += fib(5) + helper(i)
    return t

def mk_timer():
    c = [0]
    def t():
        c[0] += 1
        return c[0]
    return t

def rows(stats):
    # (name, ncalls, primitive calls, callers) for the functions defined here
    out = []
    for (fn, line, name), (cc, nc, tt, ct, callers) in stats.items():
        if name in ("fib", "helper", "work"):
            out.append((name, nc, cc, sorted(k[2] for k in callers if not k[2].startswith('<'))))
    return sorted(out)

for label, mod in (("cProfile", cProfile), ("profile", profile)):
    pr = mod.Profile(mk_timer()) if mod is profile else mod.Profile()
    pr.runcall(work)
    pr.create_stats()
    print(label, rows(pr.stats))

# top-level helpers and context-manager use
pr = cProfile.Profile()
with pr:
    work()
pr.create_stats()
print("with", rows(pr.stats))

out = io.StringIO()
pr = cProfile.Profile()
pr.runcall(work)
st = pstats.Stats(pr, stream=out)
st.strip_dirs().sort_stats("calls")
st.print_stats("fib")
text = out.getvalue()
print("pstats rows:", [l.split()[-1] for l in text.splitlines() if "fib" in l])
print("sortkey:", pstats.SortKey.CUMULATIVE, pstats.SortKey.TIME.value)

# exceptions and generators keep calls balanced
def boom():
    raise ValueError("x")
def gen():
    yield 1
    yield 2
def mix():
    try:
        boom()
    except ValueError:
        pass
    return sum(gen())
pr = cProfile.Profile()
pr.runcall(mix)
pr.create_stats()
print("mix", rows({k: v for k, v in pr.stats.items()}), sorted(k[2] for k in pr.stats if k[2] in ("boom", "gen", "mix")))
print("enabled?", sys.getprofile())
