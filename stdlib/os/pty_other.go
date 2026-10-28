//go:build !windows && !plan9 && !js && !wasip1 && !linux && !darwin

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import "syscall"

func openptyFds() (int, int, error) { return 0, 0, syscall.ENOSYS }
