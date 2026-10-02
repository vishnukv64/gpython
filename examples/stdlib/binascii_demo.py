"""binascii: low-level conversions between binary and ASCII.

Run with:  /tmp/gpy examples/stdlib/binascii_demo.py

This is the module base64 is built on. Where base64 works at the level of
"encode this blob", binascii works at the level of individual representations:
hex digits, base64 text, and quoted-printable.

Interpreter notes:
  * a2b_base64() wants a *str*, even though its counterpart b2a_base64()
    returns bytes -- passing the bytes straight back raises TypeError.
  * b2a_base64() returns a trailing newline, exactly like CPython.
"""

import binascii
import codecs


def to_text(blob):
    """bytes -> str. This interpreter has no bytes.decode(), so codecs does it."""
    return codecs.decode(blob, "ascii")[0]


data = b"gpython"
print("--- the input ---")
print("data:", data)

print()
print("--- hex conversions ---")
hexed = binascii.hexlify(data)
print("hexlify:  ", hexed)
print("b2a_hex:  ", binascii.b2a_hex(data))
print("unhexlify:", binascii.unhexlify(hexed))
print("a2b_hex:  ", binascii.a2b_hex(hexed))
print("round trip equal:", binascii.unhexlify(binascii.hexlify(data)) == data)

print()
print("--- base64 conversions ---")
b64 = binascii.b2a_base64(data)
print("b2a_base64:", repr(b64), "(note the trailing newline)")
# a2b_base64 needs the text form, not the bytes form.
print("a2b_base64 of the text form:", binascii.a2b_base64(to_text(b64)))
print("a2b_base64 from str:", binascii.a2b_base64("Z3B5dGhvbg=="))
try:
    binascii.a2b_base64(b64)
except TypeError as err:
    print("a2b_base64 on the raw bytes raises TypeError:", err)

print()
print("--- quoted-printable ---")
raw = b"a=b\xc3\xa9"
print("input:    ", raw)
qp = binascii.b2a_qp(raw)
print("b2a_qp:   ", qp)
print("a2b_qp:   ", binascii.a2b_qp(qp))

print()
print("--- crc32: a cheap checksum ---")
for payload in [b"", b"a", b"gpython", b"the quick brown fox"]:
    print("  crc32(%-20r) = %d" % (payload, binascii.crc32(payload)))
print("crc32 is deterministic:",
      binascii.crc32(b"gpython") == binascii.crc32(b"gpython"))


class Checksum:
    """Wrap crc32 in the rolling-update style used by hashlib objects."""

    def __init__(self):
        self.value = 0

    def update(self, chunk):
        self.value = binascii.crc32(chunk, self.value)
        return self

    def hexdigest(self):
        return "%08x" % (self.value & 0xffffffff)


streamer = Checksum()
for chunk in [b"the ", b"quick ", b"brown ", b"fox"]:
    streamer.update(chunk)
print()
print("--- worked example: streaming crc32 over chunks ---")
print("one shot:   ", "%08x" % (binascii.crc32(b"the quick brown fox") & 0xffffffff))
print("incremental:", streamer.hexdigest())
print("they agree: ",
      streamer.hexdigest() == "%08x" % (binascii.crc32(b"the quick brown fox") & 0xffffffff))

print()
print("--- error handling ---")
try:
    binascii.unhexlify(b"zz")
except binascii.Error as err:
    print("unhexlify on bad hex raises binascii.Error:", err)
try:
    binascii.unhexlify(b"abc")
except binascii.Error as err:
    print("odd-length hex raises binascii.Error:", err)
print("binascii.Error subclasses Exception:", issubclass(binascii.Error, Exception))
