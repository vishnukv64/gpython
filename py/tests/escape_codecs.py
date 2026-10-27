import pickle, codecs
cases = ["", "plain", "tab\t nl\n cr\r bs\\ q'\"", "\x00\x1f\x7f\xe9\xff", "\u20ac\U0001f600", "\\u0041 \\\\u0041"]
for s in cases:
    u = s.encode("unicode_escape"); r = s.encode("raw_unicode_escape")
    print(repr(u), repr(r), u.decode("unicode_escape") == s, r.decode("raw-unicode-escape") == s)
print(repr(b"\\a\\b\\f\\v\\101\\x41\\u00e9\\q".decode("unicode_escape")))
print(repr(b"\\\\u0041\\u0041\\x41".decode("raw_unicode_escape")))
for bad in (b"\\x4", b"\\u12", b"\\"):
    try:
        bad.decode("unicode_escape")
    except UnicodeDecodeError as e:
        print("err", e)
for p in range(6):
    for o in (b"k", b"\x00\xff", bytearray(b"ab"), "s\u20ac\\u", ["x", b"y"]):
        assert pickle.loads(pickle.dumps(o, p)) == o, (p, o)
print(pickle.dumps("a\u20ac\\", 0))
print(codecs.encode.__module__ in ("codecs", "_codecs"))
import io
b=io.BytesIO(b"abcdef")
ba=bytearray(4)
print(b.readinto(ba), ba, b.readinto(ba), ba, b.readinto(ba))
try: b.readinto(b"xx")
except TypeError as e: print("TE", e)
