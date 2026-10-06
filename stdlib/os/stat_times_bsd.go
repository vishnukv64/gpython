// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || freebsd || netbsd

package os

import "syscall"

// statTimes is access, modification and change time.  These systems name the
// fields Atimespec/Mtimespec/Ctimespec; linux and openbsd call them Atim/Mtim/
// Ctim, which is why this is its own file.
func statTimes(st *syscall.Stat_t) (a, m, c syscall.Timespec) {
	return st.Atimespec, st.Mtimespec, st.Ctimespec
}
