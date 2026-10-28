import io
import zipfile


class W(zipfile.ZipFile):
    def __init__(self, f, mode="r", compression=zipfile.ZIP_DEFLATED):
        self.extra = 5
        zipfile.ZipFile.__init__(self, f, mode, compression=compression, allowZip64=True)
        self.more = 6

    def hello(self):
        return (self.extra, self.more, self.mode, self.compression, self.fp is not None)

    def close(self):
        if self.fp is not None:
            print("closing", type(self).__name__)
        zipfile.ZipFile.close(self)


b = io.BytesIO()
with W(b, "w") as w:
    print(type(w).__name__, w.hello())
    zi = zipfile.ZipInfo("dir/a.txt", date_time=(2021, 2, 3, 4, 5, 6))
    zi.external_attr = 0o100644 << 16
    zi.compress_type = zipfile.ZIP_STORED
    w.writestr(zi, "hi")
    w.writestr("b.txt", b"yo")
print(w.fp)

z = zipfile.ZipFile(io.BytesIO(b.getvalue()))
print(z.namelist())
i = z.getinfo("dir/a.txt")
print(i.date_time, oct(i.external_attr >> 16), i.compress_type, z.read("dir/a.txt"), z.read("b.txt"))
try:
    zipfile.ZipInfo("x", date_time=(1970, 1, 1, 0, 0, 0))
except ValueError as e:
    print(e)
