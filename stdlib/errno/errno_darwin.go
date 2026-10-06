// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errno

import "syscall"

// platformErrors are darwin's own errno names.  CPython's errno module has only
// the names its platform defines, so they live here rather than in the shared
// table, where they broke the linux and windows builds.
//
// A package variable, not an init(): init functions run in file-name order, so
// errno.go's would read this before an init here had filled it.
var platformErrors = []errnoEntry{
	{"EPROCLIM", syscall.EPROCLIM},
	{"EBADEXEC", syscall.EBADEXEC},
	{"EBADARCH", syscall.EBADARCH},
	{"ESHLIBVERS", syscall.ESHLIBVERS},
	{"EBADMACHO", syscall.EBADMACHO},
	{"ENOATTR", syscall.ENOATTR},
}
