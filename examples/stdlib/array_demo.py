"""array: compact homogeneous numeric buffers.

Run with:  /tmp/gpy examples/stdlib/array_demo.py

array.array stores machine-level values in a contiguous buffer, giving a much
smaller footprint than a list of ints for large numeric data.

Interpreter notes:
  * Indexing, iteration, len() and append/extend work.
  * Slicing raises NotImplementedError, and there is no tolist()/tobytes()/pop().
  * Constructing from a bytes object is not supported ('b' typecode must be
    built from a list of ints).
"""

import array

print("--- typecodes this interpreter accepts ---")
print("array.typecodes:", array.typecodes)

print()
print("--- building arrays ---")
ints = array.array("i", [10, 20, 30])
doubles = array.array("d", [1.5, 2.5, 3.5])
chars = array.array("b", [65, 66, 67])
unicode_ = array.array("u", "abc")
print("i from list: ", repr(ints))
print("d from list: ", repr(doubles))
print("b from ints: ", repr(chars))
print("u from str:  ", repr(unicode_))

print()
print("--- the typecode tells you the element type ---")
for name, arr in [("i", ints), ("d", doubles), ("b", chars), ("u", unicode_)]:
    print("  typecode=%s itemsize=%d bytes" % (arr.typecode, arr.itemsize))

print()
print("--- indexing and iteration ---")
print("len(ints):     ", len(ints))
print("ints[0], [-1]: ", ints[0], ints[-1])
print("doubled:       ", [value * 2 for value in ints])
print("sum:           ", sum(doubles))
print("chars as text: ", "".join([chr(value) for value in chars]))

print()
print("--- growing an array ---")
grow = array.array("i", [1, 2, 3])
grow.append(4)
grow.extend([5, 6])
print("after append(4) and extend([5, 6]):", repr(grow))
print("length:", len(grow))

print()
print("--- assignment through the index ---")
editable = array.array("i", [1, 2, 3])
editable[0] = 99
print(repr(editable))

print()
print("--- limitations that differ from CPython ---")
try:
    ints[1:]
except NotImplementedError:
    print("slicing an array raises NotImplementedError (CPython returns a new array)")
print("has tolist: ", hasattr(ints, "tolist"))
print("has tobytes:", hasattr(ints, "tobytes"))
print("has pop:    ", hasattr(ints, "pop"))
