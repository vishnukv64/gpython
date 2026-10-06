import mmap

path = "/tmp/gpython_mmap_test_data.bin"
fp = open(path, "wb")
fp.write(b"hello world\nsecond line\nthird")
fp.close()

fp = open(path, "rb")
m = mmap.mmap(fp.fileno(), 0, access=mmap.ACCESS_READ)

print("len", len(m))
print("tell", m.tell())
print("read5", m.read(5))
print("tell2", m.tell())
print("readline", m.readline())
print("readline2", m.readline())
print("readline3", m.readline())
print("readline_eof", m.readline())
m.seek(0)
print("idx0", m[0], type(m[0]).__name__)
print("idxneg", m[-1])
print("slice", m[0:5], type(m[0:5]).__name__)
print("step", m[::-1][-5:])
print("find", m.find(b"world"), m.find(b"nope"))
print("find_start", m.find(b"o", 10))
print("rfind", m.rfind(b"o"))
print("contains_int", 119 in m, 63 in m)
print("contains_bytes", b"hello" in m)
m.seek(0)
print("iter", [b for b in m][:3])
print("size", m.size())
print("eq_bytes", m[:] == b"hello world\nsecond line\nthird")
print("eq_neg", m[:] == b"zzz")
m.seek(0)
print("read_byte", m.read_byte())
print("pos_after_byte", m.tell())
m.seek(-5, 2)
print("seek_end", m.read())

try:
    m.resize(10)
except Exception as e:
    print("resize_err", type(e).__name__)
try:
    m[0] = 88
except TypeError as e:
    print("readonly_setitem", str(e))
try:
    mmap.mmap(fp.fileno(), 0, access=99)
except ValueError as e:
    print("bad_access", str(e))

with m:
    print("with_open", m.tell())

m.close()
try:
    len(m)
except ValueError as e:
    print("closed_len", str(e))
try:
    m.read(1)
except ValueError as e:
    print("closed_read", str(e))
try:
    with m:
        pass
except ValueError as e:
    print("closed_enter", str(e))
m.close()
print("double_close ok")
fp.close()

# writable mapping
fw = open(path, "r+b")
mw = mmap.mmap(fw.fileno(), 0)
mw[0:5] = b"HELLO"
mw[5] = ord('!')
mw.seek(0)
print("write_back", mw.read(8))
mw[0:0 + 8] = b"hello! w"
fw.close()
mw.close()

# anonymous mapping
a = mmap.mmap(-1, 8)
print("anon_len", len(a))
a[:] = b"12345678"
print("anon_rw", a[0:4], a[7])
a.close()
print("done")
