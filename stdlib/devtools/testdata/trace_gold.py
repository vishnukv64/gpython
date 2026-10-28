import trace, sys, io, os

def f(n):
    total = 0
    for i in range(n):
        if i % 2:
            total += i
        else:
            total -= 1
    return total

t = trace.Trace(count=1, trace=0)
t.runfunc(f, 5)
counts = {(os.path.basename(k[0]), k[1]): v for k, v in t.results().counts.items() if k[0].endswith("trace_gold.py")}
for k in sorted(counts): print(k, counts[k])

out = io.StringIO()
old = sys.stdout
sys.stdout = out
try:
    trace.Trace(count=0, trace=1).runfunc(f, 2)
finally:
    sys.stdout = old
print([l.split(":", 1)[-1].strip() for l in out.getvalue().splitlines() if "trace_gold" in l])
