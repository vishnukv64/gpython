// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
func StatPath(p string) (fs.FileInfo, error) {
	if name, ok := embeddedName(p); ok {
		return fs.Stat(embeddedLib, name)
	}
	return os.Stat(p)
}

// ReadPath is os.ReadFile, extended to the embedded library.
func ReadPath(p string) ([]byte, error) {
	if name, ok := embeddedName(p); ok {
		return fs.ReadFile(embeddedLib, name)
	}
	return os.ReadFile(p)
}
