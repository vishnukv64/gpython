"""hashlib: cryptographic digests.

Run with:  /tmp/gpy examples/stdlib/hashlib_demo.py

Covers md5, sha1, the sha2 family, sha3 and blake2, plus new(), update()
streaming and file_digest().

Interpreter note: hashlib.file_digest() is present but crashes the interpreter
with a Go nil-pointer panic (not a Python exception) when called, so it is NOT
demonstrated here -- see examples/KNOWN_ISSUES.md.
"""

import hashlib

print("--- the algorithm set ---")
print("guaranteed:", sorted(hashlib.algorithms_guaranteed))
print("available: ", sorted(hashlib.algorithms_available))

print()
print("--- a digest of known input ---")
TEXT = b"the quick brown fox"
for name in ["md5", "sha1", "sha224", "sha256", "sha384", "sha512"]:
    digest = getattr(hashlib, name)(TEXT).hexdigest()
    print("  %-8s %s" % (name, digest))

print()
print("--- sha3 and blake2 ---")
for name in ["sha3_224", "sha3_256", "sha3_384", "sha3_512"]:
    print("  %-9s %s" % (name, getattr(hashlib, name)(TEXT).hexdigest()))
print("  blake2b   %s" % (hashlib.blake2b(TEXT).hexdigest(),))
print("  blake2s   %s" % (hashlib.blake2s(TEXT).hexdigest(),))

print()
print("--- digest() gives raw bytes, hexdigest() gives hex ---")
md5 = hashlib.md5(TEXT)
print("hexdigest:", md5.hexdigest())
print("digest:   ", md5.digest())
print("digest length in bytes:", len(md5.digest()))

print()
print("--- update() streams data in pieces ---")
chunked = hashlib.sha256()
for chunk in [b"the ", b"quick ", b"brown ", b"fox"]:
    chunked.update(chunk)
whole = hashlib.sha256(TEXT)
print("streamed:   ", chunked.hexdigest())
print("in one go:  ", whole.hexdigest())
print("they agree: ", chunked.hexdigest() == whole.hexdigest())

print()
print("--- new() selects by name at runtime ---")
for name in ["md5", "sha256"]:
    print("  new(%r) -> %s" % (name, hashlib.new(name, TEXT).hexdigest()))
print("  new with no data:", hashlib.new("sha256").hexdigest(), "(digest of empty input)")
print("  empty md5 is the well-known d41d8cd9...:", hashlib.md5(b"").hexdigest())

print()
print("--- a worked example: content-addressed file storage ---")


class ContentStore:
    """Names every object by the sha256 of its contents."""

    def __init__(self):
        self.objects = {}

    def digest_of(self, data):
        return hashlib.sha256(data).hexdigest()

    def put(self, data):
        key = self.digest_of(data)
        self.objects[key] = data
        return key

    def get(self, key):
        return self.objects.get(key)

    def short(self, key):
        return key[:12]


store = ContentStore()
for blob in [b"first document", b"second document", b"first document"]:
    key = store.put(blob)
    print("  put %-16r -> %s" % (blob, store.short(key)))

print("distinct objects stored:", len(store.objects))
first = store.digest_of(b"first document")
print("the duplicate stored once under one key:", store.get(first))
print("a key that is not present:", store.get("0" * 64))

print()
print("--- file_digest() is not usable here ---")
print("hashlib.file_digest exists:", hasattr(hashlib, "file_digest"))
print("but calling it crashes the interpreter with a Go panic,")
print("so this demo hashes file contents itself instead:")
PATH = "/tmp/gpy_hashlib_demo.txt"
with open(PATH, "wb") as handle:
    handle.write(TEXT)
with open(PATH, "rb") as handle:
    data = handle.read()
print("  read %d bytes and hashed them: %s" % (len(data), hashlib.sha256(data).hexdigest()))
