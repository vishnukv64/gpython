// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows && !plan9 && !js && !wasip1

package os

import "syscall"

const hasMkfifo = true

func mkfifoSys(path string, mode uint32) error { return syscall.Mkfifo(path, mode) }
