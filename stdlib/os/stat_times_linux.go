// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || openbsd

package os

import "syscall"

// statTimes is access, modification and change time, under the Atim/Mtim/Ctim
// names these systems use (darwin and the other BSDs say Atimespec).
func statTimes(st *syscall.Stat_t) (a, m, c syscall.Timespec) {
	return st.Atim, st.Mtim, st.Ctim
}
