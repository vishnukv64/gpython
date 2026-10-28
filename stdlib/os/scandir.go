// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

// scandirSrc is os.scandir and os.DirEntry, built on listdir and lstat/stat
// (the body runs in the os module's namespace).  CPython answers is_dir() and
// friends from the directory entry's d_type without a stat; this one stats on
// first use and caches the result, which is the same observable behaviour.
const scandirSrc = `
import stat as _stat


class DirEntry:
    __slots__ = ("name", "path", "_lstat", "_stat")

    def __init__(self, name, path):
        self.name = name
        self.path = path
        self._lstat = None
        self._stat = None

    def _get_lstat(self):
        if self._lstat is None:
            self._lstat = lstat(self.path)
        return self._lstat

    def stat(self, *, follow_symlinks=True):
        if not follow_symlinks:
            return self._get_lstat()
        if self._stat is None:
            if self.is_symlink():
                self._stat = stat(self.path)
            else:
                self._stat = self._get_lstat()
        return self._stat

    def is_symlink(self):
        try:
            return _stat.S_ISLNK(self._get_lstat().st_mode)
        except OSError:
            return False

    def _is_mode(self, check, follow_symlinks):
        try:
            return check(self.stat(follow_symlinks=follow_symlinks).st_mode)
        except OSError:
            return False

    def is_dir(self, *, follow_symlinks=True):
        return self._is_mode(_stat.S_ISDIR, follow_symlinks)

    def is_file(self, *, follow_symlinks=True):
        return self._is_mode(_stat.S_ISREG, follow_symlinks)

    def is_junction(self):
        return False

    def inode(self):
        return self._get_lstat().st_ino

    def __fspath__(self):
        return self.path

    def __repr__(self):
        return "<DirEntry %r>" % (self.name,)

    __class_getitem__ = classmethod(lambda cls, item: cls)


class _ScandirIterator:
    def __init__(self, names, path):
        self._it = iter(names)
        self._path = path

    def __iter__(self):
        return self

    def __next__(self):
        name = next(self._it)
        return DirEntry(name, path.join(self._path, name))

    def close(self):
        self._it = iter(())

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()


def scandir(path=None):
    if path is None:
        path = "."
    path = fspath(path)
    return _ScandirIterator(listdir(path), path)
`
