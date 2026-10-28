import io
import os
import pickle
import pickletools
import quopri
import stringprep
import shutil
import tempfile
import wave
import zipapp
import zipfile

# quopri
raw = b"caf\xc3\xa9 = ok \t\nline two with trailing space \n" + b"x" * 80 + b"\n"
enc = quopri.encodestring(raw)
print(enc)
print(quopri.decodestring(enc) == raw)
print(quopri.encodestring(b"a b\tc", quotetabs=True))
out = io.BytesIO()
quopri.encode(io.BytesIO(b"h\xe9llo\n"), out, False)
print(out.getvalue())

# stringprep (RFC 3454 tables)
print(stringprep.in_table_a1("\u0221"), stringprep.in_table_a1("A"))
print(stringprep.in_table_b1("\u00ad"), stringprep.map_table_b2("\u00df"), stringprep.map_table_b2("A"))
print(stringprep.in_table_c12("\u00a0"), stringprep.in_table_c21("\x07"), stringprep.in_table_c3("\ue000"))
print(stringprep.in_table_c4("\uffff"), stringprep.in_table_c6("\ufff9"), stringprep.in_table_c7("\u2ff0"))
print(stringprep.in_table_d1("\u05d0"), stringprep.in_table_d2("A"))

# pickletools
data = pickle.dumps({"a": [1, 2]}, protocol=2)
dis = io.StringIO()
pickletools.dis(data, dis)
print(dis.getvalue())
print([op.name for op, arg, pos in pickletools.genops(data)])
print(pickletools.optimize(pickle.dumps((1, "x"), protocol=2)) == pickle.dumps((1, "x"), protocol=2))

# wave: write then read a tiny file
d = tempfile.mkdtemp()
try:
    p = os.path.join(d, "t.wav")
    w = wave.open(p, "wb")
    w.setnchannels(1)
    w.setsampwidth(2)
    w.setframerate(8000)
    w.writeframes(bytes(range(16)))
    w.close()
    r = wave.open(p, "rb")
    print(r.getparams())
    print(r.readframes(8) == bytes(range(16)), r.getnframes())
    r.close()

    # zipapp
    src = os.path.join(d, "app")
    os.mkdir(src)
    with open(os.path.join(src, "__main__.py"), "w") as f:
        f.write("print('app ran')\n")
    target = os.path.join(d, "app.pyz")
    zipapp.create_archive(src, target, interpreter="/usr/bin/env python3")
    with open(target, "rb") as f:
        print(f.readline())
    print(zipapp.get_interpreter(target))
    with zipfile.ZipFile(target) as z:
        print(z.namelist())
finally:
    shutil.rmtree(d)
