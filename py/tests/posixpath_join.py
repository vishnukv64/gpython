"""posixpath.join (os.path on POSIX): an absolute component discards what came before."""

import os
import posixpath

print("two absolute:", posixpath.join("/base", "/opt/x"))
print("mixed:", posixpath.join("/base", "sub", "/opt/x"))
print("relative:", posixpath.join("a", "b", "c"))
print("trailing slash:", posixpath.join("a/", "b"))
print("empty ignored:", posixpath.join("a", "", "b"))
print("all empty:", repr(posixpath.join("", "")))
print("single:", posixpath.join("only"))
print("absolute first:", posixpath.join("/a", "b"))
print("leading slashes collapsed:", posixpath.join("a", "/b", "c"))
print("dot path:", posixpath.join("./a", "b"))

# The property the bug broke: joining an already-absolute entry onto a
# directory must give the entry back, NOT the entry appended to the directory.
entry = "/opt/pkg/x.dist-info"
print("rejoin is identity:", posixpath.join("/somewhere/else", entry) == entry)

doc = "finished"
