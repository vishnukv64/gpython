# golden: posix-only (CPython on Windows: ntpath, drives, WinError text, no chown)
"""Every os / os.path function that takes a path accepts str, bytes and
os.PathLike, and rejects anything else with CPython's wording."""

import os
import os.path
import pathlib
import shutil
import stat
import tempfile

root = os.path.realpath(tempfile.mkdtemp())
home = os.getcwd()


def reset():
    os.chdir(home)
    shutil.rmtree(root, ignore_errors=True)
    os.makedirs(root + "/d/sub")
    with open(root + "/f", "w") as fh:
        fh.write("hello")
    os.symlink(root + "/f", root + "/l")


class BadReturn:
    def __fspath__(self):
        return 1


class BadRaise:
    def __fspath__(self):
        raise RuntimeError("boom")


class Good:
    def __init__(self, p):
        self.p = p

    def __fspath__(self):
        return self.p


def mask(s):
    if os.name == "nt":
        # Windows paths print with backslashes, both as text and in reprs
        # (doubled); the golden is POSIX, so fold them back to "/".
        for p, tag in ((root, "<T>"), (home, "<H>")):
            s = s.replace(repr(p)[1:-1], tag).replace(p, tag)
        s = s.replace("\\\\", "/").replace("\\", "/")
    return s.replace(root, "<T>").replace(home, "<H>")


def norm(r):
    if isinstance(r, os.stat_result):
        return "stat_result size=%d" % r.st_size
    if type(r).__name__ == "statvfs_result":
        return "statvfs_result"
    if isinstance(r, list) and r and isinstance(r[0], (str, bytes)):
        return sorted(r)
    return r


def attempt(label, fn):
    reset()
    try:
        r = fn()
    except BaseException as e:
        out = "%s: %s" % (type(e).__name__, e)
    else:
        out = "ok " + repr(norm(r))
    print(mask("%-34s %s" % (label, out)))


def R(*parts):
    return root + "/" + "/".join(parts)


BAD = [("int", 12345), ("float", 1.5), ("None", None), ("list", []),
       ("badret", BadReturn()), ("badraise", BadRaise())]


DIRS = {"os.listdir": "d", "os.scandir": "d", "os.chdir": "d", "os.walk": "d",
        "os.rmdir": "d/sub", "os.removedirs": "d/sub", "os.mkdir": "new",
        "os.makedirs": "new/x", "os.mkfifo": "new"}


def sweep(name, call):
    """call(arg) -> the operation, given the path under test."""
    t = R(DIRS.get(name, "f"))
    attempt(name + " str", lambda: call(t))
    attempt(name + " Path", lambda: call(pathlib.Path(t)))
    attempt(name + " bytes", lambda: call(os.fsencode(t)))
    attempt(name + " Good", lambda: call(Good(t)))
    for label, bad in BAD:
        if bad is None and name in ("os.listdir", "os.scandir"):
            continue  # None means the current directory; tested below
        if bad == [] and name == "os.path.commonprefix":
            continue  # a list of lists is a different call
        attempt(name + " " + label, lambda: call(bad))


# one path argument (a file)
one = {
    "os.listdir": lambda a: os.listdir(a),
    "os.scandir": lambda a: sorted(e.name for e in os.scandir(a)),
    "os.chdir": lambda a: os.chdir(a),
    "os.chmod": lambda a: os.chmod(a, 0o600),
    "os.chown": lambda a: os.chown(a, -1, -1),
    "os.utime": lambda a: os.utime(a, (1, 2)),
    "os.access": lambda a: os.access(a, os.F_OK),
    "os.readlink": lambda a: os.readlink(a),
    "os.truncate": lambda a: os.truncate(a, 2),
    "os.stat": lambda a: os.stat(a),
    "os.lstat": lambda a: os.lstat(a),
    "os.walk": lambda a: list(os.walk(a)),
    "os.statvfs": lambda a: os.statvfs(a),
    "os.remove": lambda a: os.remove(a),
    "os.unlink": lambda a: os.unlink(a),
    "os.rmdir": lambda a: os.rmdir(a),
    "os.removedirs": lambda a: os.removedirs(a),
    "os.mkdir": lambda a: os.mkdir(a),
    "os.makedirs": lambda a: os.makedirs(a),
    "os.mkfifo": lambda a: os.mkfifo(a),
    "os.fspath": lambda a: os.fspath(a),
    "os.fsencode": lambda a: os.fsencode(a),
    "os.fsdecode": lambda a: os.fsdecode(a),
    "os.open": lambda a: os.close(os.open(a, os.O_RDONLY)),
    "os.path.exists": lambda a: os.path.exists(a),
    "os.path.lexists": lambda a: os.path.lexists(a),
    "os.path.isfile": lambda a: os.path.isfile(a),
    "os.path.isdir": lambda a: os.path.isdir(a),
    "os.path.islink": lambda a: os.path.islink(a),
    "os.path.isabs": lambda a: os.path.isabs(a),
    "os.path.ismount": lambda a: os.path.ismount(a),
    "os.path.abspath": lambda a: os.path.abspath(a),
    "os.path.basename": lambda a: os.path.basename(a),
    "os.path.dirname": lambda a: os.path.dirname(a),
    "os.path.expanduser": lambda a: os.path.expanduser(a),
    "os.path.expandvars": lambda a: os.path.expandvars(a),
    "os.path.normpath": lambda a: os.path.normpath(a),
    "os.path.normcase": lambda a: os.path.normcase(a),
    "os.path.realpath": lambda a: os.path.realpath(a),
    "os.path.relpath": lambda a: os.path.relpath(a, root),
    "os.path.split": lambda a: os.path.split(a),
    "os.path.splitext": lambda a: os.path.splitext(a),
    "os.path.splitdrive": lambda a: os.path.splitdrive(a),
    "os.path.getsize": lambda a: os.path.getsize(a),
    "os.path.getmtime": lambda a: type(os.path.getmtime(a)).__name__,
    "os.path.getatime": lambda a: type(os.path.getatime(a)).__name__,
    "os.path.getctime": lambda a: type(os.path.getctime(a)).__name__,
    "os.path.join": lambda a: os.path.join(a, "x"),
    "os.path.commonpath": lambda a: os.path.commonpath([a, a]),
    "os.path.commonprefix": lambda a: os.path.commonprefix([a, a]),
    "os.path.samefile": lambda a: os.path.samefile(a, R("f")),
}
for name, call in one.items():
    sweep(name, call)

# two path arguments: the bad value goes in each slot in turn
two = {
    "os.rename": lambda s, d: os.rename(s, d),
    "os.replace": lambda s, d: os.replace(s, d),
    "os.link": lambda s, d: os.link(s, d),
    "os.symlink": lambda s, d: os.symlink(s, d),
    "os.renames": lambda s, d: os.renames(s, d),
}
for name, call in two.items():
    attempt(name + " Path,Path", lambda: call(pathlib.Path(R("f")), pathlib.Path(R("new"))))
    attempt(name + " bytes,bytes", lambda: call(os.fsencode(R("f")), os.fsencode(R("new"))))
    attempt(name + " str,Path", lambda: call(R("f"), pathlib.Path(R("new"))))
    for label, bad in BAD:
        attempt(name + " src " + label, lambda: call(bad, R("new")))
        attempt(name + " dst " + label, lambda: call(R("f"), bad))

# errors that name the path
attempt("os.listdir missing Path", lambda: os.listdir(pathlib.Path(R("nope"))))
attempt("os.listdir missing bytes", lambda: os.listdir(os.fsencode(R("nope"))))
attempt("os.listdir bytes result", lambda: os.listdir(os.fsencode(R("d"))))
attempt("os.listdir Path result", lambda: os.listdir(pathlib.Path(R("d"))))
attempt("os.listdir default", lambda: len(os.listdir()) > 0)
attempt("os.listdir None", lambda: len(os.listdir(None)) > 0)
attempt("os.scandir None", lambda: len(list(os.scandir(None))) > 0)
attempt("os.chdir missing", lambda: os.chdir(pathlib.Path(R("nope"))))
attempt("os.chdir Path", lambda: (os.chdir(pathlib.Path(R("d"))), os.getcwd())[1])
attempt("os.rename missing", lambda: os.rename(pathlib.Path(R("nope")), R("x")))
attempt("os.rename Path moved", lambda: (os.rename(pathlib.Path(R("f")), pathlib.Path(R("g"))), os.path.exists(R("g")))[1])
attempt("os.replace Path moved", lambda: (os.replace(pathlib.Path(R("f")), pathlib.Path(R("g"))), os.path.exists(R("g")))[1])
attempt("os.link Path", lambda: (os.link(pathlib.Path(R("f")), pathlib.Path(R("h"))), os.stat(R("h")).st_nlink)[1])
attempt("os.symlink Path", lambda: (os.symlink(pathlib.Path(R("f")), pathlib.Path(R("s"))), os.readlink(R("s")) == R("f"))[1])
attempt("os.readlink bytes", lambda: os.readlink(os.fsencode(R("l"))) == os.fsencode(R("f")))
attempt("os.readlink Path", lambda: os.readlink(pathlib.Path(R("l"))) == R("f"))
attempt("os.readlink not link", lambda: os.readlink(pathlib.Path(R("f"))))
attempt("os.chmod Path", lambda: (os.chmod(pathlib.Path(R("f")), 0o600), oct(stat.S_IMODE(os.stat(R("f")).st_mode)))[1])
attempt("os.chmod missing", lambda: os.chmod(pathlib.Path(R("nope")), 0o600))
attempt("os.utime Path", lambda: (os.utime(pathlib.Path(R("f")), (5, 6)), os.stat(R("f")).st_mtime)[1])
attempt("os.truncate Path", lambda: (os.truncate(pathlib.Path(R("f")), 2), os.path.getsize(R("f")))[1])
attempt("os.truncate missing", lambda: os.truncate(pathlib.Path(R("nope")), 2))
attempt("os.mkfifo Path", lambda: (os.mkfifo(pathlib.Path(R("ff"))), stat.S_ISFIFO(os.stat(R("ff")).st_mode))[1])
attempt("os.open Path write", lambda: (os.close(os.open(pathlib.Path(R("w")), os.O_WRONLY | os.O_CREAT)), os.path.exists(R("w")))[1])
attempt("os.open missing", lambda: os.open(pathlib.Path(R("nope")), os.O_RDONLY))
attempt("os.access Path", lambda: [os.access(pathlib.Path(R("f")), m) for m in (os.F_OK, os.R_OK)])
attempt("os.access missing", lambda: os.access(pathlib.Path(R("nope")), os.F_OK))
attempt("os.stat fd bad", lambda: os.stat(999))
attempt("os.statvfs fd bad", lambda: os.statvfs(999))
attempt("os.stat missing Path", lambda: os.stat(pathlib.Path(R("nope"))))
attempt("os.lstat missing bytes", lambda: os.lstat(os.fsencode(R("nope"))))
attempt("os.walk Path", lambda: [(a, sorted(b), sorted(c)) for a, b, c in os.walk(pathlib.Path(R("d")))])
attempt("os.walk bytes", lambda: [(a, sorted(b), sorted(c)) for a, b, c in os.walk(os.fsencode(R("d")))])
attempt("os.removedirs Path", lambda: (os.removedirs(pathlib.Path(R("d", "sub"))), os.path.exists(R("d")))[1])
attempt("os.removedirs missing", lambda: os.removedirs(pathlib.Path(R("nope"))))
attempt("os.rmdir missing", lambda: os.rmdir(pathlib.Path(R("nope"))))
attempt("os.unlink missing", lambda: os.unlink(pathlib.Path(R("nope"))))
attempt("os.remove missing", lambda: os.remove(pathlib.Path(R("nope"))))
attempt("os.mkdir exists", lambda: os.mkdir(pathlib.Path(R("d"))))
attempt("os.makedirs Path", lambda: (os.makedirs(pathlib.Path(R("a", "b"))), os.path.isdir(R("a", "b")))[1])
attempt("os.path.join Path", lambda: os.path.join(pathlib.Path("a"), "b"))
attempt("os.path.join mixed", lambda: os.path.join("a", b"b"))
attempt("os.path.split Path", lambda: os.path.split(pathlib.Path("/a/b")))
attempt("os.path.basename bytes", lambda: os.path.basename(b"/a/b"))
attempt("os.path.exists int fd", lambda: os.path.exists(999))
attempt("os.path.isdir None", lambda: os.path.isdir(None))
attempt("os.path.getsize missing", lambda: os.path.getsize(pathlib.Path(R("nope"))))
attempt("os.path.samefile bad", lambda: os.path.samefile(R("f"), None))
attempt("os.fspath Path", lambda: os.fspath(pathlib.Path("/a/b")))
attempt("os.fspath Good bytes", lambda: os.fspath(Good(b"x")))
attempt("os.fsdecode bytes", lambda: os.fsdecode(b"x"))
attempt("os.fsencode str", lambda: os.fsencode("x"))
attempt("os.fspath noarg", lambda: os.fspath())

shutil.rmtree(root, ignore_errors=True)
