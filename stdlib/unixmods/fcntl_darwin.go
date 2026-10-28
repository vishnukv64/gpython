// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package unixmods

import "golang.org/x/sys/unix"

var osFcntlConsts = []namedConst{
	{"F_FULLFSYNC", unix.F_FULLFSYNC}, {"F_NOCACHE", unix.F_NOCACHE}, {"F_GETPATH", unix.F_GETPATH},
	{"F_RDAHEAD", unix.F_RDAHEAD}, {"F_GETNOSIGPIPE", 74}, {"F_SETNOSIGPIPE", 73},
	{"F_GETLEASE", 107}, {"F_SETLEASE", 106},
	{"F_OFD_GETLK", 92}, {"F_OFD_SETLK", 90}, {"F_OFD_SETLKW", 91},
}
