import sys
import os

print(sys.modules["os.path"] is os.path)
import os.path as p
from os import path
import posixpath, ntpath
print(p is os.path, path is os.path, os.path in (posixpath, ntpath))
print(os.path.join("a", "b"))
