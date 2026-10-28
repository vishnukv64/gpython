# os.stat, os.lstat, os.fstat and os.stat_result.
#
# stat_result is a struct sequence: a 10-item tuple whose fields also have
# names, the times as whole seconds in the sequence and as floats by name.
# Nothing printed here depends on the file's inode, owner or timestamps - only
# on how the fields relate - so the golden is the same on every machine.

import os
import pathlib
import stat
import sys
import tempfile
import time

d = tempfile.mkdtemp()
p = os.path.join(d, "f")
with open(p, "w") as f:
    f.write("hello")
os.symlink(p, p + ".l")

s = os.stat(p)
print("shape:", type(s).__name__, type(s).__module__, len(s), isinstance(s, os.stat_result))
print("size:", s.st_size, s[6] == s.st_size, stat.S_ISREG(s.st_mode))
print("times:", s[7] == int(s.st_atime), isinstance(s.st_mtime, float), s.st_mtime_ns // 10**9 == s[8])
print("tuple:", s == tuple(s), s == os.stat(p), hash(s) == hash(tuple(s)), list(s)[0] == s.st_mode)
print("repr:", repr(s).startswith("os.stat_result(st_mode="), repr(s).count("="))

print("lstat:", stat.S_ISLNK(os.lstat(p + ".l").st_mode), stat.S_ISREG(os.stat(p + ".l").st_mode))
print("follow:", stat.S_ISLNK(os.stat(p + ".l", follow_symlinks=False).st_mode), stat.S_ISDIR(os.stat(d).st_mode))

with open(p) as fh:
    print("fstat:", os.fstat(fh.fileno()) == s, os.stat(fh.fileno()) == s)
print("pathlib:", pathlib.Path(p).stat() == s, os.stat(pathlib.Path(p)) == s)

# The errors are the OSError SUBCLASS CPython picks, with errno and filename.
# Windows reports both as ERROR_PATH_NOT_FOUND (WinError 3 -> ENOENT), as
# CPython does there; the golden keeps the POSIX wording, so Windows checks
# its own and prints the shared lines.
WIN3 = "[WinError 3] The system cannot find the path specified: "
POSIX = {"/nope/x": "error: FileNotFoundError 2 True [Errno 2] No such file or directory: '/nope/x'",
         p + "/sub": "error: NotADirectoryError 20 True [Errno 20] Not a directory: 'D/f/sub'"}
for bad in ("/nope/x", p + "/sub"):
    try:
        os.stat(bad)
    except OSError as e:
        if os.name == "nt":
            assert type(e) is FileNotFoundError and e.errno == 2 and e.winerror == 3, (type(e), e.errno)
            assert str(e) == WIN3 + repr(bad), str(e)
            print(POSIX[bad])
        else:
            print("error:", type(e).__name__, e.errno, e.filename == bad, str(e).replace(d, "D"))
try:
    pathlib.Path("/nope/x").stat()
except OSError as e:
    if os.name == "nt":
        # A WindowsPath spells it with backslashes, and stat() names it so.
        assert str(e) == WIN3 + repr(str(pathlib.Path("/nope/x"))), str(e)
        print("pathlib error: FileNotFoundError 2 [Errno 2] No such file or directory: '/nope/x'")
    else:
        print("pathlib error:", type(e).__name__, e.errno, e)
try:
    os.stat(1.5)
except TypeError as e:
    print("TypeError:", e)

# The other struct sequences carry their module the same way.
for o in (sys.version_info, time.localtime()):
    print("module:", type(o).__name__, type(o).__module__, repr(type(o)))
