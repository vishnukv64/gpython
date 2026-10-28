# glob is CPython's own glob.py, embedded; this builds a tree and checks it.
import glob, os, tempfile

root = tempfile.mkdtemp()
os.mkdir(os.path.join(root, "testdata"))
for rel in ["glob.go", "glob_test.go", "testdata/test.py", "testdata/test_golden.txt", "testdata/.hidden"]:
    open(os.path.join(root, rel), "w").close()
old = os.getcwd()
os.chdir(root)

def show(label, got):
    print(label, sorted(got))

for pat in ["*", "*test*", "*/test*", "*/test*_*", "*/t??t*_*", "*/t[e]?t*_*", "*/t[oe]?t*_*", "*/t[o]?t*_*", "*/t[!o]?t*_*", "*/*", "nope*"]:
    show(pat, glob.glob(pat))
    show(repr(pat.encode()), glob.glob(pat.encode()))
show("recursive", glob.glob("**/*.py", recursive=True))
show("hidden", glob.glob("*/*", include_hidden=True))
show("root_dir", glob.glob("*.go", root_dir=root))
show("iglob", list(glob.iglob("*/test.py")))
print(glob.escape("a*b?[c]"), glob.has_magic("a*"), glob.has_magic("ab"))
os.chdir(old)
