import os
import sys
import tempfile

out, err = sys.stdout, sys.stderr
print("text one")
out.flush()
out.buffer.write(b"bytes two\n")
out.buffer.flush()
print("text three")
out.flush()
out.buffer.write("héllo\n".encode("utf-8"))
out.buffer.flush()
err.write("err four\n")
err.flush()
err.buffer.write(b"bytes five\n")
err.buffer.flush()
print(hasattr(sys.stdin, "buffer"), sys.stdin.buffer.readable(), out.buffer.writable())
out.flush()
n = out.buffer.write(b"abc\n")
out.buffer.flush()
print(n)

path = os.path.join(tempfile.mkdtemp(), "wl.txt")
with open(path, "w") as f:
    f.writelines(map(str, ["a\n", "b\n"]))
    f.writelines(["c\n"])
with open(path) as f:
    print(repr(f.read()))
with open(path, "wb") as f:
    f.writelines([b"x", b"y"])
    print(hasattr(f, "buffer"))
with open(path, "rb") as f:
    print(f.read())
os.remove(path)
os.rmdir(os.path.dirname(path))
