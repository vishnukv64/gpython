// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package shutil

import (
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// diskUsageQuery mirrors CPython's nt._getdiskusage: GetDiskFreeSpaceExW's
// total and total-free byte counts; used is total minus free.
func diskUsageQuery(path string) (total, used, free int64, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, oserr(err)
	}
	var avail, tot, fr uint64
	r, _, e := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&tot)), uintptr(unsafe.Pointer(&fr)))
	if r == 0 {
		return 0, 0, 0, oserr(e)
	}
	return int64(tot), int64(tot - fr), int64(fr), nil
}
