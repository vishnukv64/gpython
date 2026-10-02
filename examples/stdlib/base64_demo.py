"""base64: encode binary data as printable ASCII.

Run with:  /tmp/gpy examples/stdlib/base64_demo.py

Every encoder here takes bytes and returns bytes. b64decode also accepts a str.

Interpreter notes:
  * str.encode() does not exist in this interpreter, so text is converted to
    bytes with the two-argument bytes(text, encoding) builtin instead.
  * b64encode(str) raises TypeError, exactly as in CPython 3.
"""

import base64
import codecs


def to_text(blob):
    """bytes -> str. This interpreter has no bytes.decode(), so codecs does it."""
    return codecs.decode(blob, "ascii")[0]


data = bytes("hello gpython", "utf-8")
print("--- the input ---")
print("data:", data)

print()
print("--- standard Base64 ---")
encoded = base64.b64encode(data)
print("b64encode:          ", encoded)
print("b64decode (bytes):  ", base64.b64decode(encoded))
print("b64decode (str):    ", base64.b64decode(to_text(encoded)))
print("standard_b64encode: ", base64.standard_b64encode(data))

print()
print("--- URL-safe Base64 replaces + and / with - and _ ---")
raw = b"\xfb\xff\xfe"
print("input:              ", raw)
print("b64encode:          ", base64.b64encode(raw))
print("urlsafe_b64encode:  ", base64.urlsafe_b64encode(raw))
print("urlsafe round trip: ", base64.urlsafe_b64decode(base64.urlsafe_b64encode(raw)))

print()
print("--- Base32 (alphabet A-Z and 2-7) ---")
b32 = base64.b32encode(data)
print("b32encode:", b32)
print("b32decode:", base64.b32decode(b32))

print()
print("--- Base16 / hex ---")
b16 = base64.b16encode(data)
print("b16encode:", b16)
print("b16decode:", base64.b16decode(b16))

print()
print("--- Base85 and Ascii85 ---")
for name, encode, decode in [
    ("b85", base64.b85encode, base64.b85decode),
    ("a85", base64.a85encode, base64.a85decode),
]:
    text = encode(data)
    print("%s encode: %s" % (name, text))
    print("%s decode: %s" % (name, decode(text)))

print()
print("--- a worked example: embedding binary data in a text config ---")
payload = b"\x00\x01\x02\xfd\xfe\xff"
print("binary payload:  ", payload)

token = base64.b64encode(payload)
print("as base64 text:  ", token)


class Config:
    def __init__(self, blob):
        self.blob = blob

    def __repr__(self):
        return "Config(blob=%r)" % (self.blob,)

    def to_text(self):
        return "blob: " + to_text(self.blob)

    @classmethod
    def from_text(cls, line):
        # str.partition() does not exist here; split() does.
        value = line.split(": ", 1)[1]
        # bytes(text, encoding) is how this interpreter spells str.encode().
        return cls(base64.b64decode(bytes(value, "utf-8")))


config = Config(token)
text = config.to_text()
print("serialised line: ", text)
restored = Config.from_text(text)
print("restored blob:   ", restored.blob)
print("round trip equal:", restored.blob == payload)

print()
print("--- mistakes ---")
try:
    base64.b64encode("a string is not bytes")
except TypeError as err:
    print("b64encode(str) raises TypeError:", err)
