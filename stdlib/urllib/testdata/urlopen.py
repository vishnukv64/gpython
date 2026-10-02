# urlopen against a Go test server whose address is in the environment.
import os
import urllib.error
import urllib.request

base = os.environ["GPY_URLOPEN_BASE"]

# A plain GET: read walks the body, and the metadata matches the response.
r = urllib.request.urlopen(base + "/ok")
assert r.status == 200, r.status
assert r.getcode() == 200, r.getcode()
assert r.geturl() == base + "/ok", r.geturl()
body = r.read()
assert body == b"hello world\nsecond line\n", body
assert r.readline() == b""  # already at the end
r.close()

# The context-manager protocol, and readlines splitting on newlines.
with urllib.request.urlopen(base + "/ok") as r:
    assert r.readlines() == [b"hello world\n", b"second line\n"]

# info() answers the headers, and get("Content-Type") works on them.
with urllib.request.urlopen(base + "/ok") as r:
    info = r.info()
    assert info.get("Content-Type") == "text/plain", info

# A POST carries the data and gets it echoed.
r = urllib.request.urlopen(base + "/echo", data=b"abc")
assert r.read() == b"abc"
r.close()

# An error status raises HTTPError, whose fields a caller reads.
try:
    urllib.request.urlopen(base + "/missing")
    raise AssertionError("expected HTTPError")
except urllib.error.HTTPError as e:
    assert e.code == 404, e.code
    assert "404" in e.reason, e.reason

# A connection that cannot be made raises URLError, not a fake response.
try:
    urllib.request.urlopen("http://127.0.0.1:1/nope")
    raise AssertionError("expected URLError")
except urllib.error.URLError as e:
    assert e.reason is not None, e

# An unsupported scheme is a URLError too.
try:
    urllib.request.urlopen("ftp://example.invalid/x")
    raise AssertionError("expected URLError")
except urllib.error.URLError:
    pass

assert issubclass(urllib.error.HTTPError, urllib.error.URLError)
assert issubclass(urllib.error.URLError, OSError)

print("OK")
