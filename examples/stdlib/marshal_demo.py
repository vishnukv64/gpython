"""marshal: a compact binary format for plain Python values.

Run with:  /tmp/gpy examples/stdlib/marshal_demo.py

marshal.dumps() writes the bytes; marshal.loads() reads them back. The format
is the one CPython itself uses for compiled module files, so it is fast and
compact -- but it is version-specific and never safe to send untrusted data to.

Interpreter notes:
  * dump()/load() (the file-object pair) raise `SystemError: dump not
    implemented` / `SystemError: load not implemented`. Only the string-based
    dumps()/loads() work.
  * Code objects are NOT marshallable here: `ValueError: unmarshallable
    object`, where CPython would happily round-trip a compiled module.
"""

import marshal

print("--- version and supported values ---")
print("marshal.version:", getattr(marshal, "version", "(absent)"))

print()
print("--- round-tripping scalars ---")
for label, value in [
    ("None", None),
    ("True", True),
    ("False", False),
    ("int", 42),
    ("negative int", -7),
    ("float", 3.5),
    ("str", "hello"),
    ("bytes", b"raw"),
]:
    encoded = marshal.dumps(value)
    decoded = marshal.loads(encoded)
    print("  %-14s %-28r -> %r  equal=%s" % (label, encoded, decoded, decoded == value))

print()
print("--- round-tripping containers ---")
values = [
    [1, 2, 3],
    (1, 2, 3),
    {"a": 1},
    {"nested": [1, {"deep": (2, 3)}]},
]
for value in values:
    encoded = marshal.dumps(value)
    decoded = marshal.loads(encoded)
    print("  %-32r -> %d bytes -> %r" % (value, len(encoded), decoded))
    print("  %-32s    equal to the original: %s" % ("", decoded == value))

print()
print("--- the encoding is compact and self-describing ---")
print("dumps(None)  ->", repr(marshal.dumps(None)), "(a single type-code byte)")
print("dumps(1)     ->", repr(marshal.dumps(1)), "(type code + 4-byte int)")
print("dumps('ab')  ->", repr(marshal.dumps("ab")))
print("type codes: N=None T/F=bool i=int g=float u=str s=bytes [=list (=tuple {=dict")

print()
print("--- reading a hand-built buffer ---")
# b'i' plus a little-endian 32-bit int is the on-disk encoding of an int.
manual = b"i\x2a\x00\x00\x00"
print("bytes:     ", manual)
print("loads gives:", marshal.loads(manual))

print()
print("--- malformed input raises ---")
for label, data in [("unknown type code", b"!"), ("empty input", b"")]:
    try:
        marshal.loads(data)
        print("  %-18s decoded unexpectedly" % (label,))
    except SystemError as err:
        print("  %-18s -> SystemError: %s" % (label, err))
    except EOFError as err:
        print("  %-18s -> EOFError: %s" % (label, err))

print()
print("--- unmarshallable values ---")


def a_function():
    return None


class AClass:
    pass


for label, value in [("function", a_function), ("class", AClass), ("module", marshal)]:
    try:
        marshal.dumps(value)
        print("  %-10s marshalled" % (label,))
    except ValueError as err:
        print("  %-10s -> ValueError: %s" % (label, err))

print()
print("--- file-object variants are not implemented ---")
PATH = "/tmp/gpy_marshal_demo.bin"
try:
    with open(PATH, "wb") as handle:
        marshal.dump({"a": 1}, handle)
except SystemError as err:
    print("marshal.dump(value, file) -> SystemError:", err)
with open(PATH, "wb") as handle:
    handle.write(marshal.dumps({"a": 1}))
try:
    with open(PATH, "rb") as handle:
        marshal.load(handle)
except SystemError as err:
    print("marshal.load(file)        -> SystemError:", err)
with open(PATH, "rb") as handle:
    data = handle.read()
print("wrote and read the file myself:", marshal.loads(data))

print()
print("--- code objects are not marshallable here ---")
code = compile("x = 1", "<s>", "exec")
print("compiled a code object:", type(code).__name__)
try:
    marshal.dumps(code)
except ValueError as err:
    print("marshal.dumps(code) -> ValueError:", err)
    print("(CPython round-trips .pyc files through exactly this call)")

print()
print("--- a worked example: a tiny on-disk key-value store ---")


class BinaryStore:
    def __init__(self, path):
        self.path = path

    def write(self, mapping):
        with open(self.path, "wb") as handle:
            handle.write(marshal.dumps(mapping))
        return self

    def read(self):
        with open(self.path, "rb") as handle:
            return marshal.loads(handle.read())

    def size(self):
        with open(self.path, "rb") as handle:
            return len(handle.read())


store = BinaryStore("/tmp/gpy_marshal_store.bin")
store.write({"users": ["ann", "bob"], "counts": [1, 2, 3], "version": 2})
print("read back:", store.read())
print("on-disk size in bytes:", store.size())
print("round trip equal:",
      store.read() == {"users": ["ann", "bob"], "counts": [1, 2, 3], "version": 2})
