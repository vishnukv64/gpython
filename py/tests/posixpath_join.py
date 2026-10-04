"""os.path.join: an absolute component discards what came before."""

import os
import os.path

print("two absolute:", os.path.join("/base", "/opt/x"))
print("mixed:", os.path.join("/base", "sub", "/opt/x"))
print("relative:", os.path.join("a", "b", "c"))
print("trailing slash:", os.path.join("a/", "b"))
print("empty ignored:", os.path.join("a", "", "b"))
print("all empty:", repr(os.path.join("", "")))
print("single:", os.path.join("only"))
print("absolute first:", os.path.join("/a", "b"))
print("leading slashes collapsed:", os.path.join("a", "/b", "c"))
print("dot path:", os.path.join("./a", "b"))

# The property the bug broke: joining an already-absolute entry onto a
# directory must give the entry back, NOT the entry appended to the directory.
entry = "/opt/pkg/x.dist-info"
print("rejoin is identity:", os.path.join("/somewhere/else", entry) == entry)

doc = "finished"
