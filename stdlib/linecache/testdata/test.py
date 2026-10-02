import linecache
import os
import tempfile

d = tempfile.mkdtemp()
p = os.path.join(d, "probe.py")
with open(p, "w") as f:
    f.write("line one\nline two\nline three\n")

# getline returns the line WITH its newline, 1-based; out of range is "".
assert linecache.getline(p, 1) == "line one\n", repr(linecache.getline(p, 1))
assert linecache.getline(p, 2) == "line two\n"
assert linecache.getline(p, 0) == ""
assert linecache.getline(p, 99) == ""
assert linecache.getline("/no/such/file", 1) == ""

# getlines returns every line, and the length matches.
lines = linecache.getlines(p)
assert len(lines) == 3, len(lines)
assert lines == ["line one\n", "line two\n", "line three\n"], lines

# The read populated the cache.
assert p in linecache.cache

# checkcache keeps a fresh entry.
linecache.checkcache(p)
assert linecache.getline(p, 3) == "line three\n"

# clearcache empties it, and getline repopulates on demand.
linecache.clearcache()
assert len(linecache.cache) == 0
assert linecache.getline(p, 1) == "line one\n"

# lazycache on an existing entry does not replace it.
assert linecache.lazycache(p, None) is False
assert linecache.getline(p, 2) == "line two\n"

print("linecache ok")
