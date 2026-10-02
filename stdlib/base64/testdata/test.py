"""Tests for the base64 module.

Every expected value here was computed with CPython's own base64 module, so
these are exact-match tests against the reference implementation.
"""

import base64
import binascii

def assertEqual(x, y):
    assert x == y, "got: %s, want: %s" % (repr(x), repr(y))

def assertRaises(exc, fn):
    try:
        fn()
    except exc:
        return
    except Exception as e:
        raise AssertionError("raised %s, wanted %s" % (type(e).__name__, exc.__name__))
    raise AssertionError("did not raise %s" % exc.__name__)

# base64
assertEqual(base64.b64encode(b"hello world"), b"aGVsbG8gd29ybGQ=")
assertEqual(base64.b64encode(b""), b"")
assertEqual(base64.b64encode(b"\xfb\xff", altchars=b"-_"), b"-_8=")
assertEqual(base64.b64decode(b"aGVsbG8gd29ybGQ="), b"hello world")
assertEqual(base64.b64decode(b""), b"")
assertEqual(base64.b64decode(b"aGVs bG8=\n"), b"hello")
assertEqual(base64.b64decode(b"-_8=", altchars=b"-_"), b"\xfb\xff")
assertEqual(base64.standard_b64encode(b"\xfb\xff"), b"+/8=")
assertEqual(base64.standard_b64decode(b"+/8="), b"\xfb\xff")
assertEqual(base64.urlsafe_b64encode(b"\xfb\xff"), b"-_8=")
assertEqual(base64.urlsafe_b64decode(b"-_8="), b"\xfb\xff")

# base32
assertEqual(base64.b32encode(b"hello"), b"NBSWY3DP")
assertEqual(base64.b32encode(b""), b"")
assertEqual(base64.b32encode(b"f"), b"MY======")
assertEqual(base64.b32decode(b"NBSWY3DP"), b"hello")
assertEqual(base64.b32decode(b"nbswy3dp", casefold=True), b"hello")
# map01 remaps '1' to the given character and '0' to 'O'; the last base32 digit
# of NBSWY3DP is P, but the sample below ends in 0, which maps to O, giving n.
assertEqual(base64.b32decode(b"NBSWY3D0", map01=b"0"), b"helln")
assertEqual(base64.b32hexencode(b"hello"), b"D1IMOR3F")
assertEqual(base64.b32hexdecode(b"D1IMOR3F"), b"hello")

# base16
assertEqual(base64.b16encode(b"\x01\x02\xff"), b"0102FF")
assertEqual(base64.b16decode(b"0102FF"), b"\x01\x02\xff")
assertEqual(base64.b16decode(b"0102ff", casefold=True), b"\x01\x02\xff")

# ascii85
assertEqual(base64.a85encode(b"hello world"), b"BOu!rD]j7BEbo7")
assertEqual(base64.a85encode(b"hello world", adobe=True), b"<~BOu!rD]j7BEbo7~>")
assertEqual(base64.a85encode(b"\x00\x00\x00\x00"), b"z")
assertEqual(base64.a85encode(b"x", pad=True), b"GQ7^D")
assertEqual(base64.a85decode(b"BOu!rDZ"), b"hello")
assertEqual(base64.a85decode(b"<~BOu!rDZ~>", adobe=True), b"hello")
assertEqual(base64.a85decode(b"z"), b"\x00\x00\x00\x00")

# base85
assertEqual(base64.b85encode(b"hello world"), b"Xk~0{Zy<MXa%^M")
assertEqual(base64.b85encode(b"hello world", pad=True), b"Xk~0{Zy<MXa%^M(")
assertEqual(base64.b85encode(b"x"), b"cm")
assertEqual(base64.b85decode(b"Xk~0{Zy<MXa%^M"), b"hello world")
assertEqual(base64.b85decode(base64.b85encode(b"\x00\x01\x02\x03")), b"\x00\x01\x02\x03")
assertEqual(base64.b85decode(base64.b85encode(b"")), b"")

# decoding errors raise binascii.Error, as CPython's base64 does
assertRaises(binascii.Error, lambda: base64.b64decode(b"aGVsbG8!!"))
assertRaises(binascii.Error, lambda: base64.b64decode(b"a"))
assertRaises(binascii.Error, lambda: base64.b64decode(b"aGVsbG8"))
assertRaises(binascii.Error, lambda: base64.b64decode(b"aGVs bG8=", validate=True))
assertRaises(binascii.Error, lambda: base64.b16decode(b"ABC"))
assertRaises(binascii.Error, lambda: base64.b16decode(b"0102ff"))
# b85 and a85 raise ValueError in CPython, not binascii.Error
assertRaises(ValueError, lambda: base64.b85decode(b"~~~!!"))
assertRaises(ValueError, lambda: base64.a85decode(b"vvvv"))

# a ValueError for a str that is not ASCII
assertRaises(ValueError, lambda: base64.b64decode("\u00e9"))

# binary input must round-trip at every length
for text in (b"", b"a", b"ab", b"abc", b"abcd", b"abcde", bytes(range(64))):
    assertEqual(base64.b64decode(base64.b64encode(text)), text)
    assertEqual(base64.b32decode(base64.b32encode(text)), text)
    assertEqual(base64.b16decode(base64.b16encode(text)), text)
    assertEqual(base64.b85decode(base64.b85encode(text)), text)
    assertEqual(base64.a85decode(base64.a85encode(text)), text)

print("base64: all tests pass")
