"""Set iteration follows CPython's SLOT order, not insertion order."""

# A set is a hash table: iteration walks its slots, so the order depends on the
# hashes rather than on how members were added.  These cases are chosen to hold
# under ANY Python implementation's own hashing of small integers, which is the
# identity - so they are stable, unlike a set of strings (CPython randomises
# string hashes per process, and its order genuinely differs run to run).
print(list(set([3, 1, 2])))
print(list(set([5, 4, 3, 2, 1])))
print(list(set([10, 3, 7])))

# hash(-1) is -2, a quirk worth pinning: CPython uses -1 as its "no hash" error
# sentinel, so a real -1 is remapped.
print(hash(-1), hash(-2), hash(0), hash(5))

# The set is still equal regardless of the order it prints in.
print(set([3, 1, 2]) == {1, 2, 3})
print(sorted(set([3, 1, 2])))

doc = "finished"
