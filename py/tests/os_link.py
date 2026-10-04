"""os.symlink/readlink/link, os.path.lexists, and os.access."""

import os
import os.path

# The harness runs this from the repository root and from the package directory
# depending on how it is invoked, so the link names are resolved against THIS
# FILE's directory rather than the current one.
here = os.path.dirname(os.path.abspath(__file__))

# Clean up any leftovers from an interrupted earlier run, so the test is
# idempotent - a symlink that already exists would otherwise be an error.
for leftover in (os.path.join(here, "_link_target.txt"), os.path.join(here, "_link_sym"), os.path.join(here, "_link_hard")):
    if os.path.lexists(leftover):
        os.unlink(leftover)

target = os.path.join(here, "_link_target.txt")
with open(target, "w") as f:
    f.write("contents" + chr(10))

link = os.path.join(here, "_link_sym")
os.symlink(os.path.basename(target), link)
print("lexists link:", os.path.lexists(link))
print("exists link:", os.path.exists(link))
print("readlink:", os.readlink(link))
print("read through link:", open(link).read().strip())

hard = os.path.join(here, "_link_hard")
os.link(target, hard)
print("hard link contents:", open(hard).read().strip())

os.unlink(link)
os.unlink(hard)
os.unlink(target)
print("after unlink:", os.path.lexists(link), os.path.lexists(hard), os.path.lexists(target))

print("flags:", os.F_OK, os.X_OK, os.W_OK, os.R_OK)
print("access dir:", os.access(".", os.R_OK))
print("access missing:", os.access("no-such-file-xyz", os.F_OK))

doc = "finished"
