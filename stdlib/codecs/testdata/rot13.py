# The rot13 codec, and the bytes round trip that ordinary code relies on.
import codecs

# A str goes in and a str comes out, matching CPython's rot_13 codec, and
# letters outside ASCII are left alone rather than encoded.
assert codecs.encode("abc", "rot13") == "nop"
assert codecs.encode("abc", "rot_13") == "nop"
assert codecs.decode("abc", "rot13") == "nop"
assert codecs.encode("nö", "rot13") == "aö"

# The bytes round trip: encode to bytes, decode back to the original.  This is
# the documented use of rot13 as a bytes transform, and it is what this
# interpreter supports in addition to CPython's str-only form.
assert codecs.encode(b"abc", "rot13") == b"nop"
assert codecs.decode(b"abc", "rot13") == b"nop"
assert codecs.decode(codecs.encode(b"abc", "rot13"), "rot13") == b"abc"

# The registry knows the name, and reports CPython's spelling.
assert codecs.lookup("rot13").name == "rot-13"
assert codecs.lookup("rot_13").name == "rot-13"

# rot13 is an involution over ASCII letters.
assert codecs.encode(codecs.encode("Hello, World!", "rot13"), "rot13") == "Hello, World!"

print("OK")

# The test harness requires this last, as the marker that the script ran to the
# end rather than dying part way through.
doc = "finished"
