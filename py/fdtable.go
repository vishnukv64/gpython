// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"os"
	"sync"
)

// The raw descriptors handed to Python code - os.open, tempfile.mkstemp -
// stay owned by the *os.File they came from until Python closes or adopts
// them.
//
// Returning f.Fd() and dropping f left the descriptor to f's finalizer: the
// garbage collector closed it behind Python's back, so a later os.close or
// os.fdopen acted on a dead number - or on one the system had since reused.
// On windows that reused handle could be the runtime's own, which killed the
// process with "runtime.preemptM: duplicatehandle failed".
var (
	fdMu    sync.Mutex
	fdFiles = map[uintptr]*os.File{}
)

// OwnFD records f as the owner of its descriptor and returns the descriptor.
func OwnFD(f *os.File) uintptr {
	fd := f.Fd()
	fdMu.Lock()
	fdFiles[fd] = f
	fdMu.Unlock()
	return fd
}

// TakeFD returns the *os.File for a descriptor, releasing it from the table:
// the one this process recorded, or a fresh wrapper for a descriptor it did
// not open (0, 1, 2, or one inherited).
func TakeFD(fd uintptr, name string) *os.File {
	fdMu.Lock()
	f, ok := fdFiles[fd]
	delete(fdFiles, fd)
	fdMu.Unlock()
	if ok {
		return f
	}
	return os.NewFile(fd, name)
}
