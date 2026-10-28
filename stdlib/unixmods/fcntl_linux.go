//go:build linux

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unixmods

import "golang.org/x/sys/unix"

var osFcntlConsts = []namedConst{
	{"F_ADD_SEALS", unix.F_ADD_SEALS}, {"F_GET_SEALS", unix.F_GET_SEALS},
	{"F_GETLEASE", unix.F_GETLEASE}, {"F_SETLEASE", unix.F_SETLEASE},
	{"F_GETPIPE_SZ", unix.F_GETPIPE_SZ}, {"F_SETPIPE_SZ", unix.F_SETPIPE_SZ},
	{"F_GETSIG", unix.F_GETSIG}, {"F_SETSIG", unix.F_SETSIG}, {"F_NOTIFY", unix.F_NOTIFY},
	{"F_EXLCK", unix.F_EXLCK}, {"F_SHLCK", unix.F_SHLCK},
	{"F_OFD_GETLK", unix.F_OFD_GETLK}, {"F_OFD_SETLK", unix.F_OFD_SETLK}, {"F_OFD_SETLKW", unix.F_OFD_SETLKW},
	{"F_SEAL_SEAL", unix.F_SEAL_SEAL}, {"F_SEAL_SHRINK", unix.F_SEAL_SHRINK},
	{"F_SEAL_GROW", unix.F_SEAL_GROW}, {"F_SEAL_WRITE", unix.F_SEAL_WRITE},
	{"DN_ACCESS", 1}, {"DN_MODIFY", 2}, {"DN_CREATE", 4},
	{"DN_DELETE", 8}, {"DN_RENAME", 16}, {"DN_ATTRIB", 32},
	{"DN_MULTISHOT", 2147483648},
}
