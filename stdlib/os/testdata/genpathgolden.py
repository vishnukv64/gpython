"""Regenerate pathgolden.py from CPython's own posixpath and ntpath.

    python3 genpathgolden.py > pathgolden.txt

Both modules are platform independent, so the table is valid on every host.
"""
import itertools
import ntpath
import os
import posixpath
import sys

ENV = {
    "HOME": "/home/u", "USERPROFILE": "C:\\Users\\u", "USERNAME": "u",
    "FOO": "foo", "BAR": "b\\ar", "EMPTY": "", "A_1": "a1",
}
os.environ.update(ENV)
os.environ.pop("HOMEPATH", None)

NT = [
    "", ".", "..", "a", "a\\b", "a/b", "\\a", "/a", "C:", "C:a", "C:\\", "C:/",
    "C:\\a", "c:/a/b", "C:\\a\\..\\b", "C:a\\..", "\\\\server\\share",
    "\\\\server\\share\\", "\\\\server\\share\\x\\y", "//server/share/x",
    "\\\\server", "\\\\?\\C:\\a", "\\\\?\\UNC\\srv\\shr\\f", "\\\\.\\dev",
    "\\\\?\\unc\\s\\h\\x", "a\\", "a\\\\b", "a/./b/../c", "..\\..\\a",
    "C:..\\x", "C:\\..\\x", "\\..\\x", "\u00e9:\\x", "\u00e9:x", "a.b.c",
    ".hidden", "..a", "a.", "a\\.b", "C:\\dir.d\\file", "dir\\file.tar.gz",
    "\\\\", "\\\\\\", "\\\\a\\b\\c\\d", "D:\\", "d:", "\\", "/", "//", "./a",
    "a\\..", "a\\..\\..", "C:\\a\\", "C:\\a\\.\\b", "x:y:z", "UPPER\\Case",
]
NT_ABS = [
    "C:\\", "C:\\a", "c:/a/b", "C:\\a\\..\\b", "D:\\a\\b", "C:\\a\\b\\c",
    "\\\\server\\share\\x\\y", "\\\\server\\share", "//server/share/x",
    "C:\\A\\B", "c:\\a\\B\\c", "C:\\..\\x",
]
NT_RESERVED = [
    "CON", "con.txt", "a\\nul", "a\\com1.x", "LPT\u00b2", "a.", "a ", "ok",
    "a\\b:c", "a?b", "C:\\x\\aux", "COM10", "a\\..", "a|b", "x\u0001y",
    "CONIN$", "prn . ", "\\\\s\\h\\nul",
]
PX = [
    "", ".", "..", "a", "a/b", "/a", "//a", "///a", "////a/b", "/", "//", "///",
    "a/", "a//b", "a/./b/../c", "../../a", "/..", "/../a", "//..", "//a/../b",
    "a/..", "a/../..", "/a/b/", "/a/./b", "a.b.c", ".hidden", "..a", "a.",
    "a/.b", "/dir.d/file", "dir/file.tar.gz", "a\\b", "C:\\a", "\u00e9/x",
    "...", "a/...", ".a.b", "/a/.b.c", "~", "~/x", "/tmp/", "x/y/z.txt",
]
PX_ABS = ["/", "/a", "/a/b", "/a/../b", "//a/b", "/a/b/c", "/A/B", "///x", "/a/./b/"]

cases = []


def add(mod, fn, args, kw=None):
    m = {"posixpath": posixpath, "ntpath": ntpath}[mod]
    kw = kw or {}
    try:
        r = getattr(m, fn)(*args, **kw)
        cases.append((mod, fn, args, kw, "ok", repr(r)))
    except Exception as e:
        msg = None if isinstance(e, TypeError) else str(e)
        cases.append((mod, fn, args, kw, "err", (type(e).__name__, msg)))


def bytesify(xs):
    return [x.encode() for x in xs if x.isascii()]


UNARY = ["normcase", "isabs", "splitdrive", "splitroot", "split", "splitext",
         "basename", "dirname", "normpath", "expanduser", "expandvars"]
for mod, paths, absp in (("ntpath", NT, NT_ABS), ("posixpath", PX, PX_ABS)):
    for fn in UNARY:
        for p in paths:
            add(mod, fn, (p,))
        for p in bytesify(paths):
            add(mod, fn, (p,))
    for p in absp:
        add(mod, "abspath", (p,))
    for a, b in itertools.product(paths[::2], paths):
        add(mod, "join", (a, b))
    for a, b, c in itertools.product(paths[3:9] + paths[-2:], repeat=3):
        add(mod, "join", (a, b, c))
    add(mod, "join", ("a",))
    add(mod, "join", ("a", "b", "c", "d"))
    add(mod, "join", (b"a", b"b"))
    add(mod, "join", ("a", b"b"))
    add(mod, "join", ())
    for a, b in itertools.product(absp, repeat=2):
        add(mod, "relpath", (a, b))
    add(mod, "relpath", ("",))
    for a in absp:
        add(mod, "relpath", (a,), {"start": "/" if mod == "posixpath" else "C:\\"})
    add(mod, "commonprefix", ([],))
    add(mod, "commonprefix", (["abc", "abd", "ab"],))
    add(mod, "commonprefix", (["/usr/lib", "/usr/local"],))
    add(mod, "commonprefix", (["\u00e9a", "\u00e8a"],))
    add(mod, "commonprefix", ([b"abc", b"abd"],))
    add(mod, "commonprefix", (["same"],))
    add(mod, "commonpath", ([],))
    add(mod, "commonpath", ([b"a/b", "a/b"],))
    for combo in itertools.chain(
            ((p,) for p in paths),
            itertools.combinations(paths, 2)):
        add(mod, "commonpath", (list(combo),))
    for combo in itertools.combinations(absp, 3):
        add(mod, "commonpath", (list(combo),))
    add(mod, "commonpath", ([b"/a/b", b"/a/c"] if mod == "posixpath" else [b"C:\\a\\b", b"c:/A/c"],))
    add(mod, "splitext", (b"a/b.c",))
    add(mod, "isabs", (123,))
    add(mod, "normpath", (None,))

for p in NT_RESERVED:
    add("ntpath", "isreserved", (p,))

for v in ["$FOO", "${FOO}", "$FOO/x", "$NOPE", "${NOPE}", "${", "$", "$$", "${}",
          "$A_1.x", "x$FOO$BAR", "${FOO", "'$FOO'", "$EMPTY!", "$-", "a$ b",
          "%FOO%", "%FOO", "%%", "%NOPE%", "%FOO%/%BAR%", "%", "'%FOO%' %FOO%",
          "$$FOO", "'", "'abc", "${FOO}${BAR}", "%FOO%%FOO%", "%A_1%$A_1",
          "$-x", "%%FOO%%", "%a b%", "$\u00e9"]:
    for mod in ("posixpath", "ntpath"):
        add(mod, "expandvars", (v,))
add("posixpath", "expandvars", (b"$FOO/x",))
add("ntpath", "expandvars", (b"%FOO%/x",))
for v in ["~", "~/x", "~\\x", "~u", "~u/x", "~nosuchuser_zz/x", "x~", "", "~~"]:
    for mod in ("posixpath", "ntpath"):
        add(mod, "expanduser", (v,))
add("ntpath", "expanduser", (b"~/x",))

print("# GENERATED by genpathgolden.py from CPython %s; one eval-able case per line." % sys.version.split()[0])
print(repr(ENV))
for c in cases:
    print(repr(c))
