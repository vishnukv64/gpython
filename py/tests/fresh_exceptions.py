# Each raise must be a fresh exception: a shared object accumulated
# tracebacks and __context__ across unrelated raises.
import io, traceback

def depth(e):
    n, tb = 0, e.__traceback__
    while tb is not None:
        n, tb = n + 1, tb.tb_next
    return n

def twice(label, fn):
    got = []
    for _ in range(2):
        try:
            fn()
        except Exception as e:
            got.append(e)
    a, b = got
    print(label, type(a).__name__, a is b, depth(a), depth(b), b.__context__ is None)

twice("shift", lambda: 1 << -1)
twice("bigshift", lambda: (1 << 100) >> -1)
twice("float", lambda: float(1 << 2000))
twice("chr", lambda: chr(1 << 70))
f = io.StringIO()
f.close()
twice("closed", lambda: f.write("x"))
twice("zerodiv", lambda: 1 / 0)
