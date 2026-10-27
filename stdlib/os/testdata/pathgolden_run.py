# Runs the CPython-generated table (pathgolden.txt)
# against posixpath and ntpath, which must agree with CPython on every host.
import ntpath
import os
import posixpath

lines = open("testdata/pathgolden.txt").read().splitlines()[1:]
ENV = eval(lines[0])
CASES = [eval(l) for l in lines[1:]]
for k, v in ENV.items():
    os.environ[k] = v
if "HOMEPATH" in os.environ:
    del os.environ["HOMEPATH"]

mods = {"posixpath": posixpath, "ntpath": ntpath}
bad = []
for mod, fn, args, kw, kind, want in CASES:
    try:
        got = ("ok", repr(getattr(mods[mod], fn)(*args, **kw)))
    except Exception as e:
        got = ("err", (type(e).__name__, str(e)))
    if kind == "ok":
        good = got == ("ok", want)
    else:
        good = got[0] == "err" and got[1][0] == want[0] and (want[1] is None or got[1][1] == want[1])
    if not good:
        bad.append("%s.%s%r%r: want %s %r, got %r" % (mod, fn, args, kw, kind, want, got))

assert not bad, "%d of %d golden cases differ; first:\n%s" % (len(bad), len(CASES), "\n".join(bad[:25]))
print("golden ok", len(CASES))
