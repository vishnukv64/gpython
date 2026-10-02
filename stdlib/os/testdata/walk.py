# os.walk: the sequence, in-place pruning, bottom-up order, and symlinks.
# The tree is built by the Go test, which puts its root in GPY_WALK_ROOT so
# that the symlink is created reliably; this script only walks it.
import os

root = os.environ["GPY_WALK_ROOT"]

def rel(p):
    return p[len(root):] or "."

# Top-down: the parent is yielded before its children, and a symlink to a
# directory is a member of dirnames.
seq = [(rel(r), sorted(d), sorted(f)) for r, d, f in os.walk(root)]
want = [(".", ["a", "link_to_a"], ["top.txt"]),
        ("/a", ["b"], ["f1.txt"]),
        ("/a/b", [], [])]
assert seq == want, (seq, want)

# Bottom-up: every child comes before its parent, and the symlinked directory
# is still not descended (followlinks defaults to False).
seq = [(rel(r), sorted(d), sorted(f)) for r, d, f in os.walk(root, topdown=False)]
assert seq == [("/a/b", [], []), ("/a", ["b"], ["f1.txt"]),
               (".", ["a", "link_to_a"], ["top.txt"])], seq

# Pruning dirnames in place: removing "a" and the link means nothing below
# them is visited.
seq = []
for r, d, f in os.walk(root):
    seq.append(rel(r))
    d[:] = []
assert seq == ["."], seq

# followlinks=False must not descend through the symlink; followlinks=True must.
seq = [rel(r) for r, d, f in os.walk(root, followlinks=False)]
assert seq == [".", "/a", "/a/b"], seq
seq = [rel(r) for r, d, f in os.walk(root, followlinks=True)]
assert seq == [".", "/a", "/a/b", "/link_to_a", "/link_to_a/b"], seq

print("OK")
