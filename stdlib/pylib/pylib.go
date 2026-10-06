// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pylib embeds the pure-Python standard library modules gpython takes
// from CPython unchanged: argparse, unittest, decimal, statistics, difflib,
// tarfile, gzip, heapq, xml, tomllib, numbers, timeit, asyncio and the modules
// they import.  Running CPython's own source keeps their behaviour, messages
// and edge cases identical to CPython's, where a port would drift.
//
// The files under lib/ are CPython 3.14.7's Lib/, copied verbatim; they are
// covered by LICENSE-PSF.txt.  A module gpython implements in Go always wins:
// the embedded root is the LAST sys.path entry.
package pylib

import (
	"embed"
	"io/fs"

	"github.com/vishnukv64/gpython/py"
)

// "all:" because a directory embed silently skips names starting with "_" -
// every __init__.py, and _pydecimal.py.
//
//go:embed all:lib
var files embed.FS

func init() {
	lib, err := fs.Sub(files, "lib")
	if err != nil {
		panic(err)
	}
	py.RegisterEmbeddedLib(lib)
}
