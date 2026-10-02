"""tempfile: create temporary files and directories safely.

Run with:  /tmp/gpy examples/stdlib/tempfile_demo.py

mkstemp() makes a uniquely named file and returns (file descriptor, path);
mkdtemp() makes a uniquely named directory. Both place them under the
directory gettempdir() reports, and both are the caller's to clean up.

Interpreter notes:
  * mkstemp() returns (fd, path) where the first element is an int file
    descriptor; the file is already open. os.write() does not exist here, so
    wrap the descriptor with os.fdopen() to write to it. os.close() works.
  * tempfile.NamedTemporaryFile and tempfile.TemporaryDirectory are NOT
    available, so the with-statement helpers are built by hand below.
  * tempfile.tempdir starts as None; gettempdir() still reports a directory.
"""

import os
import tempfile

print("--- where temporary files go ---")
print("gettempdir():  ", tempfile.gettempdir())
print("gettempdirb(): ", tempfile.gettempdirb())
print("tempfile.tempdir is:", tempfile.tempdir)
print("it is a directory:  ", os.path.isdir(tempfile.gettempdir()))

print()
print("--- mkstemp makes a uniquely named file ---")
descriptor, path = tempfile.mkstemp()
print("returned descriptor is an int:", isinstance(descriptor, int))
print("path:                        ", path)
print("the file exists:             ", os.path.exists(path))
# os.write() is absent here, but os.fdopen() wraps the descriptor in a file.
handle = os.fdopen(descriptor, "w")
handle.write("written through the descriptor\n")
handle.close()
print("after writing and closing, size:", os.path.getsize(path), "bytes")
with open(path) as handle:
    print("contents:", repr(handle.read()))
os.remove(path)
print("after remove, exists:", os.path.exists(path))

print()
print("--- the names are unique ---")
paths = [tempfile.mkstemp()[1] for _ in range(3)]
for candidate in paths:
    print("  ", candidate)
print("all distinct:", len(set(paths)) == len(paths))
for candidate in paths:
    os.remove(candidate)
print("cleaned up:", all([not os.path.exists(candidate) for candidate in paths]))

print()
print("--- a prefix and suffix keep names readable ---")
descriptor, path = tempfile.mkstemp(prefix="gpydemo-", suffix=".txt")
os.close(descriptor)
print("path:", os.path.basename(path))
print("starts with the prefix:", os.path.basename(path).startswith("gpydemo-"))
print("ends with the suffix:  ", os.path.basename(path).endswith(".txt"))
os.remove(path)

print()
print("--- mkdtemp makes a unique directory ---")
directory = tempfile.mkdtemp()
print("directory:", directory)
print("it exists and is a directory:", os.path.isdir(directory))
inside = os.path.join(directory, "scratch.txt")
with open(inside, "w") as handle:
    handle.write("inside the temp dir\n")
print("contents of the new dir:", os.listdir(directory))

print()
print("--- a worked example: temporary files that clean themselves up ---")


class TemporaryFile:
    """A context manager, standing in for tempfile.NamedTemporaryFile."""

    def __init__(self, prefix="tmp", suffix=""):
        descriptor, self.path = tempfile.mkstemp(prefix=prefix, suffix=suffix)
        os.close(descriptor)
        self.handle = None

    def __enter__(self):
        self.handle = open(self.path, "w")
        return self

    def __exit__(self, *details):
        if self.handle is not None:
            self.handle.close()
        if os.path.exists(self.path):
            os.remove(self.path)
        return False


with TemporaryFile(prefix="job-", suffix=".log") as scratch:
    scratch.handle.write("line one\nline two\n")
    print("writing to:", os.path.basename(scratch.path))
    print("exists while open:", os.path.exists(scratch.path))

print("exists after the with block:", os.path.exists(scratch.path))
print("(the file removed itself)")


class TemporaryDirectory:
    """A context manager, standing in for tempfile.TemporaryDirectory."""

    def __init__(self):
        self.path = tempfile.mkdtemp()

    def __enter__(self):
        return self

    def __exit__(self, *details):
        # Remove the files this demo put there, then the directory itself.
        for entry in os.listdir(self.path):
            os.remove(os.path.join(self.path, entry))
        os.rmdir(self.path)
        return False

    def write(self, name, text):
        full = os.path.join(self.path, name)
        with open(full, "w") as handle:
            handle.write(text)
        return full


with TemporaryDirectory() as workspace:
    first = workspace.write("a.txt", "alpha\n")
    second = workspace.write("b.txt", "beta\n")
    print("workspace:", workspace.path)
    print("holds:", sorted(os.listdir(workspace.path)))
    print("first file is", os.path.getsize(first), "bytes")
    print("second file is", os.path.getsize(second), "bytes")

print("workspace removed:", not os.path.exists(workspace.path))

print()
print("--- what is available ---")
for name in ["mkstemp", "mkdtemp", "gettempdir", "gettempdirb",
             "NamedTemporaryFile", "TemporaryDirectory", "TemporaryFile"]:
    print("  tempfile.%-20s %s" % (name, hasattr(tempfile, name)))
