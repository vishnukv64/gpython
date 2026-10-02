"""glob: find files by wildcard pattern.

Run with:  /tmp/gpy examples/stdlib/glob_demo.py

Patterns: * matches within a path segment, ? a single character, [] a set.
The demo creates its own scratch tree and removes exactly the entries it
created, so the output is the same everywhere.

Interpreter note: glob() accepts only the pattern -- `recursive=` and every
other keyword argument raise TypeError, so there is no ** support.
"""

import glob
import os

print("--- set up a scratch tree ---")
ROOT = "/tmp/gpy_glob_demo"
DIRECTORIES = ["", "/reports", "/logs", "/reports/archive"]
FILENAMES = [
    "notes.txt", "data.csv", "readme.md",
    "reports/q1.txt", "reports/q2.txt", "reports/archive/old.txt",
    "logs/run.log",
    "a1.txt", "a2.txt", "ab.txt", "abc.txt",
    "file_a.txt", "file_b.txt", "file_c.txt", "file_d.txt",
]

for sub in DIRECTORIES:
    path = ROOT + sub
    if not os.path.isdir(path):
        os.makedirs(path)

for name in FILENAMES:
    with open(os.path.join(ROOT, name), "w") as handle:
        handle.write(name + "\n")
print("created", len(DIRECTORIES), "directories and", len(FILENAMES), "files under", ROOT)

print()
print("--- * matches any run of characters in one segment ---")
for pattern in ["*.txt", "*.csv", "*.log", "readme.*"]:
    matches = glob.glob(os.path.join(ROOT, pattern))
    print("  %-12s -> %s" % (pattern, sorted([os.path.basename(m) for m in matches])))

print()
print("--- ? matches exactly one character ---")
matches = glob.glob(os.path.join(ROOT, "a?.txt"))
print("  pattern a?.txt ->", sorted([os.path.basename(m) for m in matches]))

print()
print("--- [abc] matches a set, [!abc] excludes one ---")
print("  file_[ab].txt  ->", sorted([os.path.basename(m)
      for m in glob.glob(os.path.join(ROOT, "file_[ab].txt"))]))
print("  file_[!ab].txt ->", sorted([os.path.basename(m)
      for m in glob.glob(os.path.join(ROOT, "file_[!ab].txt"))]))

print()
print("--- a pattern does not cross a directory boundary ---")
for pattern in ["reports/*.txt", "reports/*/*.txt"]:
    matches = glob.glob(os.path.join(ROOT, pattern))
    print("  %-20s -> %s" % (pattern, sorted([m[len(ROOT) + 1:] for m in matches])))

print()
print("--- no matches gives an empty list, not an error ---")
print("  '*.nothing' ->", glob.glob(os.path.join(ROOT, "*.nothing")))
print("  a whole path that does not exist ->", glob.glob("/no/such/dir/*"))

print()
print("--- the pattern can be an absolute or relative path ---")
cwd = os.getcwd()
os.chdir(ROOT)
try:
    print("  from inside the tree, '*.md' ->", glob.glob("*.md"))
    print("  and 'reports/*.txt'         ->", sorted(glob.glob("reports/*.txt")))
finally:
    os.chdir(cwd)

print()
print("--- keyword arguments are rejected ---")
try:
    glob.glob(os.path.join(ROOT, "**", "*.txt"), recursive=True)
except TypeError as err:
    print("glob(..., recursive=True) raises TypeError:", err)
    print("(so there is no ** recursive walk; the loop below does it by hand)")

print()
print("--- a walked search over the same tree ---")
found = []
pending = [ROOT]
while pending:
    current = pending.pop()
    for entry in os.listdir(current):
        path = os.path.join(current, entry)
        if os.path.isdir(path):
            pending.append(path)
        elif entry.endswith(".txt"):
            found.append(path)
print("every .txt below the tree:")
for path in sorted(found):
    print("  ", path[len(ROOT) + 1:])
print("count:", len(found))
print("glob only saw the top level:", len(glob.glob(os.path.join(ROOT, "*.txt"))))

print()
print("--- clean up exactly what was created ---")
for name in FILENAMES:
    os.remove(os.path.join(ROOT, name))
# Deepest directories first, so no rmdir ever meets a non-empty directory.
for sub in ["/reports/archive", "/reports", "/logs", ""]:
    os.rmdir(ROOT + sub)
print("removed", len(FILENAMES), "files and", len(DIRECTORIES), "directories")
print("root still exists:", os.path.exists(ROOT))
