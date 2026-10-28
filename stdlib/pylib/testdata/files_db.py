import dbm
import dbm.dumb
import filecmp
import fileinput
import genericpath
import mailbox
import os
import shelve
import shutil
import tempfile

d = tempfile.mkdtemp()
try:
    def w(*parts, text):
        p = os.path.join(d, *parts)
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w") as f:
            f.write(text)
        return p

    # filecmp
    a1 = w("a", "same.txt", text="hello\n")
    b1 = w("b", "same.txt", text="hello\n")
    a2 = w("a", "diff.txt", text="one\n")
    b2 = w("b", "diff.txt", text="two\n")
    # Shallow dircmp compares (type, size, mtime); the two diff.txt are the
    # same size and can share an mtime tick, so set them apart explicitly.
    t = os.stat(a2).st_mtime
    os.utime(b2, (t + 10, t + 10))
    w("a", "only_a.txt", text="x\n")
    w("b", "only_b.txt", text="y\n")
    print(filecmp.cmp(a1, b1, shallow=False), filecmp.cmp(a2, b2, shallow=False))
    dc = filecmp.dircmp(os.path.join(d, "a"), os.path.join(d, "b"))
    print(dc.left_only, dc.right_only, dc.common, dc.same_files, dc.diff_files)
    print(filecmp.cmpfiles(os.path.join(d, "a"), os.path.join(d, "b"), ["same.txt", "diff.txt"], shallow=False))

    # fileinput
    f1 = w("in1.txt", text="alpha\nbeta\n")
    f2 = w("in2.txt", text="gamma\n")
    with fileinput.input(files=(f1, f2)) as fi:
        for line in fi:
            print(os.path.basename(fi.filename()), fi.lineno(), fi.filelineno(), fi.isfirstline(), line.rstrip())
    with fileinput.input(files=(f1,), inplace=True) as fi:
        for line in fi:
            print(line.upper(), end="")
    with open(f1) as fh:
        print(fh.read().split())

    # genericpath
    print(genericpath.exists(f1), genericpath.isfile(f1), genericpath.isdir(d), genericpath.getsize(f2))
    print(genericpath.commonprefix(["/usr/lib", "/usr/local"]))

    # dbm.dumb + shelve
    dumb = dbm.dumb.open(os.path.join(d, "dumbdb"), "c")
    dumb[b"k"] = b"v"
    dumb[b"k2"] = b"v2"
    print(sorted(dumb.keys()), dumb[b"k"])
    dumb.close()
    with dbm.dumb.open(os.path.join(d, "dumbdb"), "r") as ro:
        print(len(ro), ro[b"k2"])

    with shelve.open(os.path.join(d, "shelf")) as db:
        db["list"] = [1, 2, 3]
        db["dict"] = {"x": (1, 2)}
    with shelve.open(os.path.join(d, "shelf"), flag="r") as db:
        print(sorted(db.keys()), db["list"], db["dict"], "list" in db)

    # mailbox (mbox): string messages in, strings out
    path = os.path.join(d, "box.mbox")
    box = mailbox.mbox(path)
    k1 = box.add("From: a@b.c\nSubject: one\n\nfirst body\n")
    k2 = box.add("From: d@e.f\nSubject: two\n\nsecond\nFrom here\n")
    box.flush()
    print(len(box), k1, k2)
    print(box.get_string(k1))
    print(repr(box.get_bytes(k2)))
    box.remove(k1)
    box.flush()
    print(list(box.keys()))
    box.close()
    box = mailbox.mbox(path)
    print(len(box), repr(box.get_string(0)))
    box.close()
finally:
    shutil.rmtree(d)
