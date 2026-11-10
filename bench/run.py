# Compare gpython with CPython: run under CPython,
#   python3 bench/run.py [path-to-gpython] [bench names...]
# In-process timings come from benches.py (best of 3, each returns a
# checksum that must match); startup, import and pystone are wall times
# (best of 5).
import os, subprocess, sys, time

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
GPY = sys.argv[1] if len(sys.argv) > 1 else os.path.join(REPO, "gpython")
ONLY = sys.argv[2:]
CPY = sys.executable

def inproc(exe):
    p = subprocess.run([exe, os.path.join(HERE, "benches.py"), *ONLY], capture_output=True, text=True, timeout=3600)
    if p.returncode:
        print(exe, "failed:", p.stderr[-2000:])
    out = {}
    for line in p.stdout.splitlines():
        name, rest = line.split(" ", 1)
        res, t = rest.rsplit(" ", 1)
        out[name] = (res, float(t))
    return out

def wall(cmd, n=5):
    best = None
    for _ in range(n):
        t = time.perf_counter()
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=3600)
        dt = time.perf_counter() - t
        if p.returncode:
            return None
        best = dt if best is None else min(best, dt)
    return best

# pystone needs time.clock, which CPython removed; give both the same clock.
PYSTONE = ("import sys, time; time.clock = time.perf_counter; "
           "sys.path.insert(0, %r); import pystone; pystone.main(100000)" % os.path.join(REPO, "examples"))

c, g = inproc(CPY), inproc(GPY)
rows = [(n, c[n][1], g.get(n, (None, None))[1], g.get(n, (None,))[0] == c[n][0]) for n in c]
if not ONLY:
    for label, args in [("startup (pass)", ["-c", "pass"]),
                        ("import json,re,argparse,typing,asyncio", ["-c", "import json, re, argparse, typing, asyncio"]),
                        ("pystone 100k", ["-c", PYSTONE])]:
        rows.append((label, wall([CPY] + args), wall([GPY] + args), None))

print(f"{'benchmark':40} {'CPython s':>10} {'gpython s':>10} {'ratio':>7}  same")
for name, ct, gt, same in rows:
    cs = f"{ct:10.4f}" if ct else "    failed"
    gs = f"{gt:10.4f}" if gt else "    failed"
    r = f"{gt / ct:7.2f}" if ct and gt else "    n/a"
    print(f"{name:40} {cs} {gs} {r}  {'' if same is None else same}")
