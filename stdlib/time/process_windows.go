// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package time

import "syscall"

// processTime is user plus kernel CPU seconds, from GetProcessTimes - what
// CPython uses on windows, which has no getrusage.  The times are in 100ns
// units.
func processTime() (float64, error) {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, err
	}
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, err
	}
	ticks := func(f syscall.Filetime) float64 {
		return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) * 1e-7
	}
	return ticks(kernel) + ticks(user), nil
}
