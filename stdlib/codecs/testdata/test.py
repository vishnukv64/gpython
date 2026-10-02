"""codecs: the module-level helpers and the incremental API.

Every line here is what CPython 3.14 prints for the same program, so a change
to the shape or to the incremental behaviour shows up as a diff.
"""

import codecs

# The module-level helpers UNWRAP the (value, length) pair a codec function
# returns; the codec's own functions keep it.
print("decode:", repr(codecs.decode(b"abc", "utf-8")))
print("encode:", repr(codecs.encode("abc", "utf-8")))
print("lookup.decode keeps the pair:", repr(codecs.lookup("utf-8").decode(b"abc")))
print("lookup.encode keeps the pair:", repr(codecs.lookup("utf-8").encode("abc")))
print("lookup name:", codecs.lookup("utf-8").name)

# The incremental decoder must HOLD BACK a character whose bytes have not all
# arrived.  This is the whole reason the class exists, and it is what
# charset_normalizer relies on when it feeds one byte at a time.
from codecs import IncrementalDecoder
from encodings.utf_8 import IncrementalDecoder as Utf8Decoder

# The BASE class carries no encoding, so it refuses to decode - only the
# per-encoding subclass knows the conversion.  CPython's base behaves the same.
try:
    IncrementalDecoder().decode(b"x")
except NotImplementedError:
    print("base decode: NotImplementedError")

d = Utf8Decoder(errors="ignore")
print("split 1:", repr(d.decode(b"\xc3", final=False)))
print("split 2:", repr(d.decode(b"\xa9", final=False)))
print("then text:", repr(d.decode(b"abc", final=False)))
print("state:", d.getstate())
d.reset()
print("state after reset:", d.getstate())

# A three-byte character arriving in two pieces.
e = Utf8Decoder()
print("three-byte partial:", repr(e.decode(b"\xe2\x82", final=False)))
print("three-byte held:", e.getstate())
print("three-byte done:", repr(e.decode(b"\xac", final=False)))

# The base class says so rather than inventing a result.
base = Utf8Decoder()
try:
    base.decode(b"x")
except NotImplementedError:
    print("base decode: NotImplementedError")

# Per-encoding modules, which code reaches by name.
import importlib

utf8 = importlib.import_module("encodings.utf_8")
print("encodings.utf_8 has IncrementalDecoder:", hasattr(utf8, "IncrementalDecoder"))
u = utf8.IncrementalDecoder(errors="ignore")
print("via encodings.utf_8:", repr(u.decode(b"\xc3", final=False)), repr(u.decode(b"\xa9", final=False)))

from encodings.aliases import aliases

print("alias utf8:", aliases.get("utf8"))
print("alias latin-1:", aliases.get("latin-1"))

# latin-1 decodes every byte, so nothing is ever held back.
latin = importlib.import_module("encodings.latin_1").IncrementalDecoder()
print("latin-1:", repr(latin.decode(b"\xff", final=False)))

doc = "finished"
