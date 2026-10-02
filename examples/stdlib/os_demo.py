"""os: the operating system interface.

Run with:  /tmp/gpy examples/stdlib/os_demo.py

Covers the process environment, the working directory, path manipulation and
filesystem mutation.

Interpreter notes:
  * os.stat() and os.lstat() are NOT present, so a file's mode, size and
    timestamps are not reachable through os -- use os.path.getsize() and
    stat.S_ISDIR() on a known mode, as shown below.
  * A failing os.remove()/os.mkdir() raises SystemError, not OSError as in
    CPython.
  * os.path.isfile()/isdir()/exists()/getsize() do work.
"""

import os

print("--- identifying the platform ---")
print("os.name:    ", os.name)
print("os.sep:     ", repr(os.sep))
print("os.altsep:  ", repr(os.altsep))
print("os.extsep:  ", repr(os.extsep))
print("os.curdir:  ", repr(os.curdir))
print("os.pardir:  ", repr(os.pardir))
print("os.linesep: ", repr(os.linesep))
print("os.pathsep: ", repr(os.pathsep))
print("os.devnull: ", repr(os.devnull))

print()
print("--- the working directory ---")
cwd = os.getcwd()
print("getcwd:  ", cwd)
print("it is absolute:", os.path.isabs(cwd))
original = os.getcwd()
os.chdir("/tmp")
print("after chdir('/tmp'):", os.getcwd())
os.chdir(original)
print("restored:           ", os.getcwd() == cwd)

print()
print("--- the environment ---")
print("HOME is set:       ", os.environ.get("HOME") is not None)
print("getenv with a default:", os.getenv("GPY_ABSENT_VARIABLE", "fallback"))
os.putenv("GPY_DEMO", "1")
os.unsetenv("GPY_DEMO")
print("putenv/unsetenv are callable:", callable(os.putenv), callable(os.unsetenv))
print("getpid is an int:", isinstance(os.getpid(), int))

print()
print("--- path manipulation ---")
parts = ("usr", "local", "bin", "tool")
print("join:      ", os.path.join(*parts))
print("basename:  ", os.path.basename("/usr/local/bin/tool"))
print("dirname:   ", os.path.dirname("/usr/local/bin/tool"))
print("splitext:  ", os.path.splitext("archive.tar.gz"))
print("normpath:  ", os.path.normpath("/usr//local/../local/bin"))
print("abspath:   ", os.path.isabs(os.path.abspath("relative/path")))

print()
print("--- a scratch directory to play in ---")
ROOT = "/tmp/gpy_os_demo"
if not os.path.isdir(ROOT):
    os.makedirs(ROOT)
print("created:", ROOT)
print("isdir:  ", os.path.isdir(ROOT))

nested = os.path.join(ROOT, "a", "b", "c")
os.makedirs(nested)
print("makedirs made:", os.path.isdir(nested))
print("exists:       ", os.path.exists(nested))

file_path = os.path.join(nested, "note.txt")
with open(file_path, "w") as handle:
    handle.write("hello gpython\n")
print("wrote a file, size:", os.path.getsize(file_path), "bytes")
print("isfile:            ", os.path.isfile(file_path))
print("listdir of the dir:", os.listdir(nested))
print("listdir of the tree:", sorted(os.listdir(ROOT)))

print()
print("--- removing things ---")
os.remove(file_path)
print("after remove, exists:", os.path.exists(file_path))
os.rmdir(nested)
print("after rmdir, exists: ", os.path.exists(nested))
# nested was ROOT/a/b/c, so the two parent directories are still there.
os.rmdir(os.path.join(ROOT, "a", "b"))
os.rmdir(os.path.join(ROOT, "a"))
print("the a/b/c chain is gone:", not os.path.exists(os.path.join(ROOT, "a")))

print()
print("--- failures raise SystemError, not OSError ---")
print("os.error is OSError:", os.error is OSError)
try:
    os.remove("/no/such/file")
except SystemError as err:
    print("os.remove on a missing file -> SystemError:", err)
try:
    os.mkdir("/tmp")
except SystemError as err:
    print("os.mkdir on an existing dir -> SystemError:", err)
try:
    open("/absolutely/no/such/path")
except FileNotFoundError as err:
    print("open on a missing file -> FileNotFoundError:", err)
    print("  (FileNotFoundError IS an OSError subclass:",
          issubclass(FileNotFoundError, OSError), ")")

print()
print("--- os.stat is absent, so use the path helpers ---")
print("hasattr(os.stat): ", hasattr(os, "stat"))
print("hasattr(os.lstat):", hasattr(os, "lstat"))
print("hasattr(os.system):", hasattr(os, "system"))

print()
print("--- a worked example: a recursive directory walk ---")


def walk_files(root, suffix):
    """Every file below root whose name ends with suffix, sorted."""
    found = []
    pending = [root]
    while pending:
        current = pending.pop()
        for entry in os.listdir(current):
            path = os.path.join(current, entry)
            if os.path.isdir(path):
                pending.append(path)
            elif entry.endswith(suffix):
                found.append(path)
    return sorted(found)


tree = os.path.join(ROOT, "project")
for sub in ["src", "src/util", "docs"]:
    os.makedirs(os.path.join(tree, sub))
for name in ["main.py", "src/app.py", "src/util/helper.py", "docs/readme.md"]:
    with open(os.path.join(tree, name), "w") as handle:
        handle.write("# " + name + "\n")

print("the tree holds:")
for path in walk_files(tree, ""):
    print("  ", path[len(tree) + 1:])
print("only the .py files:")
for path in walk_files(tree, ".py"):
    print("  ", path[len(tree) + 1:], "(%d bytes)" % (os.path.getsize(path),))

print()
print("--- clean up (tracked paths, deepest first) ---")
for name in ["main.py", "src/app.py", "src/util/helper.py", "docs/readme.md"]:
    os.remove(os.path.join(tree, name))
os.rmdir(os.path.join(tree, "src/util"))
os.rmdir(os.path.join(tree, "src"))
os.rmdir(os.path.join(tree, "docs"))
os.rmdir(tree)
os.rmdir(ROOT)
print("scratch tree removed:", not os.path.exists(ROOT))
