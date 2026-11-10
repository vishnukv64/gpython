print(b"com%d" % 3, b"%s-%s" % (b"a", b"b"), b"%b" % b"xy", b"%5d|%-5d|%05d" % (1, 2, 3))
print(b"%x %X %o %c %c" % (255, 255, 8, 65, b"z"), b"%r %a" % (b"q", b"q"))
print(b"%(a)s %(b)d" % {b"a": b"A", b"b": 2} if False else b"%%|%.2f|%e" % (1.5, 2.5))
print(b"%s" % bytearray(b"ba"), b"" % (), b"no args" % ())
print(bytearray(b"n=%d") % 4 if False else b"n=%d" % 4)
for bad in (lambda: b"%d" % (1, 2), lambda: b"%d %d" % (1,), lambda: b"%z" % 1):
    try: bad()
    except Exception as e: print(type(e).__name__)
