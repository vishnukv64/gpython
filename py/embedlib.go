// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EmbeddedLibRoot is the sys.path entry for the pure-Python standard library
// compiled into the binary (stdlib/pylib): CPython's own sources for modules
// that are written in Python there, so they behave exactly as CPython's do.
// It is not a real directory; StatPath and ReadPath serve paths beneath it
// from the embedded files.
const EmbeddedLibRoot = "<gpython-lib>"

var embeddedLib fs.FS

// RegisterEmbeddedLib installs the files behind EmbeddedLibRoot.
func RegisterEmbeddedLib(f fs.FS) { embeddedLib = f }

// embeddedName is the name of p inside the embedded files, if p is beneath
// EmbeddedLibRoot.  filepath.Join gives backslashes on windows, and fs.FS
// names always use "/".
func embeddedName(p string) (string, bool) {
	if embeddedLib == nil {
		return "", false
	}
	p = filepath.ToSlash(p)
	if p == EmbeddedLibRoot {
		return ".", true
	}
	if rest, ok := strings.CutPrefix(p, EmbeddedLibRoot+"/"); ok {
		return rest, true
	}
	return "", false
}

// IsEmbeddedPath reports whether p names something in the embedded library.
// Such a path is complete on its own, like an absolute one, and must not be
// joined onto other search paths.
func IsEmbeddedPath(p string) bool {
	_, ok := embeddedName(p)
	return ok
}

// StatPath is os.Stat, extended to the embedded library.
// zipEntry splits "x.whl/pkg/mod.py" into an archive on disk and the name
// inside it, so a wheel or zip on sys.path imports like a directory (what
// ensurepip relies on to run the bundled pip).
func zipEntry(p string) (archive, name string, ok bool) {
	p = filepath.ToSlash(p)
	for _, ext := range []string{".whl/", ".zip/", ".egg/"} {
		if i := strings.Index(p, ext); i >= 0 {
			archive, name = p[:i+len(ext)-1], p[i+len(ext):]
			if st, err := os.Stat(archive); err == nil && !st.IsDir() {
				return archive, strings.TrimSuffix(name, "/"), true
			}
		}
	}
	return "", "", false
}

type zipCacheEntry struct {
	mtime time.Time
	r     *zip.ReadCloser
}

var (
	zipMu    sync.Mutex
	zipCache = map[string]zipCacheEntry{}
)

// zipFS opens an archive once and reopens it when the file changes.
func zipFS(archive string) (fs.FS, error) {
	st, err := os.Stat(archive)
	if err != nil {
		return nil, err
	}
	zipMu.Lock()
	defer zipMu.Unlock()
	if e, ok := zipCache[archive]; ok {
		if e.mtime.Equal(st.ModTime()) {
			return e.r, nil
		}
		_ = e.r.Close()
	}
	r, err := zip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	zipCache[archive] = zipCacheEntry{st.ModTime(), r}
	return r, nil
}

func StatPath(p string) (fs.FileInfo, error) {
	if name, ok := embeddedName(p); ok {
		return fs.Stat(embeddedLib, name)
	}
	if archive, name, ok := zipEntry(p); ok {
		z, err := zipFS(archive)
		if err != nil {
			return nil, err
		}
		return fs.Stat(z, name)
	}
	return os.Stat(p)
}

// ReadPath is os.ReadFile, extended to the embedded library and to archives.
func ReadPath(p string) ([]byte, error) {
	if name, ok := embeddedName(p); ok {
		return fs.ReadFile(embeddedLib, name)
	}
	if archive, name, ok := zipEntry(p); ok {
		z, err := zipFS(archive)
		if err != nil {
			return nil, err
		}
		return fs.ReadFile(z, name)
	}
	return os.ReadFile(p)
}

// LstatPath is os.Lstat, extended like StatPath (an archive or the embedded
// library has no symlinks, so the two agree there).
func LstatPath(p string) (fs.FileInfo, error) {
	if IsEmbeddedPath(p) {
		return StatPath(p)
	}
	if _, _, ok := zipEntry(p); ok {
		return StatPath(p)
	}
	return os.Lstat(p)
}

// ReadDirPath is os.ReadDir, extended like StatPath.
func ReadDirPath(p string) ([]fs.DirEntry, error) {
	if name, ok := embeddedName(p); ok {
		return fs.ReadDir(embeddedLib, name)
	}
	if archive, name, ok := zipEntry(p); ok {
		z, err := zipFS(archive)
		if err != nil {
			return nil, err
		}
		if name == "" {
			name = "."
		}
		return fs.ReadDir(z, name)
	}
	return os.ReadDir(p)
}

// OpenReadonly opens a file for reading.  A file inside the embedded library
// or an archive is copied to an unlinked temporary file, so that it is an
// ordinary *os.File to the caller.
// ponytail: copies per open; a read-only fs.File adapter if this gets hot.
func OpenReadonly(p string, flag int) (*os.File, error) {
	if !IsEmbeddedPath(p) {
		if _, _, ok := zipEntry(p); !ok {
			return os.OpenFile(p, flag, 0)
		}
	}
	data, err := ReadPath(p)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "gpython-embed-")
	if err != nil {
		return nil, err
	}
	_ = os.Remove(f.Name()) // fails while open on Windows; a leftover temp file
	if _, err := f.Write(data); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
