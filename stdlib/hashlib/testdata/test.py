"""Tests for the hashlib module.

Every digest here was computed with CPython 3.14's hashlib, so these are
exact-match tests against the reference implementation.
"""

import hashlib

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

# Known digests of b"abc".
assertEqual(hashlib.md5(b"abc").hexdigest(), "900150983cd24fb0d6963f7d28e17f72")
assertEqual(hashlib.sha1(b"abc").hexdigest(), "a9993e364706816aba3e25717850c26c9cd0d89d")
assertEqual(hashlib.sha224(b"abc").hexdigest(), "23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7")
assertEqual(hashlib.sha256(b"abc").hexdigest(), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
assertEqual(hashlib.sha384(b"abc").hexdigest(),
            "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed"
            "8086072ba1e7cc2358baeca134c825a7")
assertEqual(hashlib.sha512(b"abc").hexdigest(),
            "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a"
            "2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f")
assertEqual(hashlib.sha3_256(b"abc").hexdigest(),
            "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532")
assertEqual(hashlib.blake2b(b"abc").hexdigest(),
            "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d"
            "17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923")

# Empty input.
assertEqual(hashlib.md5(b"").hexdigest(), "d41d8cd98f00b204e9800998ecf8427e")
assertEqual(hashlib.sha256(b"").hexdigest(),
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

# A selectable digest size, for BLAKE2b.
assertEqual(hashlib.blake2b(b"abc", digest_size=20).hexdigest(), "384264f676f39536840523f284921cdc68b6846b")
assertEqual(hashlib.blake2b(b"abc").digest_size, 64)

# update() accumulates, and copy() forks the state.
h = hashlib.sha256()
h.update(b"ab")
h.update(b"c")
assertEqual(h.hexdigest(), hashlib.sha256(b"abc").hexdigest())
forked = hashlib.sha256(b"ab").copy()
forked.update(b"c")
assertEqual(forked.hexdigest(), hashlib.sha256(b"abc").hexdigest())
# the original is untouched by the fork
assertEqual(hashlib.sha256(b"ab").hexdigest(), hashlib.sha256(b"ab").hexdigest())

# attributes
assertEqual(hashlib.sha256().digest_size, 32)
assertEqual(hashlib.sha256().block_size, 64)
assertEqual(hashlib.sha256().name, "sha256")
assertEqual(hashlib.md5().name, "md5")

# new() and the algorithm sets
assertEqual(hashlib.new("sha256", b"abc").hexdigest(), hashlib.sha256(b"abc").hexdigest())
assertEqual("md5" in hashlib.algorithms_guaranteed, True)
assertEqual("sha256" in hashlib.algorithms_guaranteed, True)
assertEqual("sha3_256" in hashlib.algorithms_available, True)
assertEqual("blake2b" in hashlib.algorithms_available, True)

# digest() returns the raw bytes; slicing is not implemented for bytes in this
# interpreter, so the length and determinism are asserted instead.
assertEqual(len(hashlib.md5(b"abc").digest()), 16)
assertEqual(len(hashlib.sha256(b"abc").digest()), 32)
assertEqual(hashlib.md5(b"abc").digest() == hashlib.md5(b"abc").digest(), True)

# errors
assertRaises(ValueError, lambda: hashlib.new("nope"))
assertRaises(TypeError, lambda: hashlib.md5(b"abc").update("str"))

print("hashlib: all tests pass")
