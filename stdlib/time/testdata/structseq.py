# struct sequences: time.struct_time and sys.version_info.
#
# Both are TUPLES whose elements also have names, so indexing, iteration,
# slicing, comparison against a plain tuple, hashing and named attribute access
# all have to work.  Both were Go structs that modelled only the named half (or
# only the sequence half), so the checks below each failed somewhere.
#
# Nothing here prints an actual version NUMBER: gpython reports 3.10 and CPython
# (the oracle) reports whatever it is, and the point of the test is the SHAPE of
# the object, not which version the interpreter claims to be.  What is printed
# is the relation between the two ways of reaching each element.

import sys
import time

# --- sys.version_info ---------------------------------------------------------
vi = sys.version_info
print("vi len:", len(vi))
print("vi index is int:", all(isinstance(vi[i], int) for i in range(3)))
# The named attributes and the positions must agree, whichever version this is.
print("vi names:", vi.major == vi[0], vi.minor == vi[1], vi.micro == vi[2],
      vi.releaselevel == vi[3], vi.serial == vi[4])
print("vi slice:", len(vi[:3]), isinstance(vi[:3], tuple))
print("vi iter:", list(vi) == [vi[0], vi[1], vi[2], vi[3], vi[4]])
print("vi n_fields:", vi.n_fields, vi.n_sequence_fields, vi.n_unnamed_fields)
# It IS a tuple, so it compares equal to a plain one.
print("vi eq tuple:", vi == (vi.major, vi.minor, vi.micro, vi.releaselevel, vi.serial))
# The 3.9+ spelling of a version check, which is what code branches on.
print("vi ge:", vi >= (3, 8))
print("vi lt:", vi < (99, 0))
print("vi hashable:", isinstance(hash(vi), int))
print("vi as key:", {vi: "found"}[sys.version_info])

# --- time.struct_time --------------------------------------------------------
st = time.localtime()
print("st len:", len(st))
print("st named:", isinstance(st.tm_year, int), isinstance(st.tm_mon, int))
print("st index matches name:", st[0] == st.tm_year)
print("st slice:", len(st[:3]))
print("st eq tuple:", st == tuple(st))
print("st eq self:", st == time.localtime())
print("st hashable:", isinstance(hash(st), int))
print("st in set:", st in {st})
print("st as key:", isinstance({st: 1}[time.localtime()], int))

print("OK")

doc = "finished"
