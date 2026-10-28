// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && (386 || amd64 || arm || arm64 || riscv64)

package unixmods

// Linux termios constants golang.org/x/sys/unix does not export, from the
// kernel's include/uapi/asm-generic/termbits.h and ioctls.h, which these
// architectures use (powerpc, mips, sparc and alpha number them differently).
// CPython's termios exposes them, and fcntl.ioctl users pass FIONREAD.
//
// Appended in a variable initialiser, not init(): termios.go's init builds
// the module from termiosConsts and runs first, by file order.
var _ = appendGenericTermiosConsts()

func appendGenericTermiosConsts() bool {
	termiosConsts = append(termiosConsts,
		namedConst{"TCSANOW", 0},
		namedConst{"TCSADRAIN", 1},
		namedConst{"FIONREAD", 0x541B},
		namedConst{"FIONBIO", 0x5421},
		namedConst{"FIONCLEX", 0x5450},
		namedConst{"FIOCLEX", 0x5451},
		namedConst{"FIOASYNC", 0x5452},
	)
	return true
}
