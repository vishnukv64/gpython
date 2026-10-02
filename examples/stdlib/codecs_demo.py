"""codecs: encode str -> bytes and decode bytes -> str.

Run with:  /tmp/gpy examples/stdlib/codecs_demo.py

codecs.encode(text, encoding) returns a (bytes, length) *tuple* in this
interpreter, not a bare bytes object as in CPython, so the value you want is at
index [0]. codecs.decode(blob, encoding) likewise returns (str, length).

This is also the way to convert between str and bytes here, because
str.encode() and bytes.decode() do not exist.
"""

import codecs

print("--- encoding text to bytes ---")
result = codecs.encode("café", "utf-8")
print("raw return value:", result)
print("it is a tuple:   ", type(result).__name__ == "tuple")
print("the bytes:       ", result[0])
print("the length:      ", result[1])

print()
print("--- decoding bytes back to text ---")
decoded = codecs.decode(b"caf\xc3\xa9", "utf-8")
print("raw return value:", decoded)
print("the text:        ", decoded[0])
print("the length:      ", decoded[1])
print("round trip equal:", codecs.decode(codecs.encode("café", "utf-8")[0], "utf-8")[0] == "café")

print()
print("--- ascii and latin-1 are available too ---")
print("encode ascii:   ", codecs.encode("abc", "ascii")[0])
print("decode ascii:   ", codecs.decode(b"abc", "ascii")[0])
print("decode latin-1: ", codecs.decode(b"\xe9", "latin-1")[0])
print("utf-16 leaves a BOM:", codecs.encode("a", "utf-16")[0])

print()
print("--- a codec that is not compiled in raises LookupError ---")
try:
    codecs.encode(b"hi", "hex")
except codecs.LookupError as err:
    print("codecs.encode(b'hi', 'hex') ->", type(err).__name__, err)

print()
print("--- lookup() gives you a CodecInfo object ---")
info = codecs.lookup("utf-8")
print("lookup('utf-8'):", info)
print("its name:       ", info.name)
print("it can encode:  ", info.encode("hi"))

print()
print("--- a worked example: read a latin-1 file that is really utf-8 ---")


class TextFile:
    def __init__(self, path, encoding):
        self.path = path
        self.encoding = encoding

    def read_text(self):
        with open(self.path, "rb") as handle:
            raw = handle.read()
        return codecs.decode(raw, self.encoding)[0]

    def read_mojibake(self):
        """Decode as latin-1 -- every byte maps to a character, never fails."""
        with open(self.path, "rb") as handle:
            raw = handle.read()
        return codecs.decode(raw, "latin-1")[0]


PATH = "/tmp/gpy_codecs_demo.txt"
with open(PATH, "wb") as handle:
    handle.write(b"c\xc3\xa9dille")

print("wrote bytes:      ", b"c\xc3\xa9dille")
print("read as utf-8:    ", TextFile(PATH, "utf-8").read_text())
print("read as latin-1:  ", TextFile(PATH, "latin-1").read_mojibake())
print("(the latin-1 read is the classic 'mojibake' bug: cÃ©dille)")
