// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mmap

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// Two anonymous mappings with one tagname are the same memory, which is what
// multiprocessing's Windows Arena relies on to share Value/Array with a child.
func TestTagnameMappingsShareMemory(t *testing.T) {
	tag := fmt.Sprintf("gpy-tag-test-%d", os.Getpid())
	a, err := mapMem(-1, 0, 4096, ACCESS_DEFAULT, tag)
	if err != nil {
		t.Fatalf("first mapping: %v", err)
	}
	defer unmapMem(a)
	if got := LastError(); got != 0 {
		t.Fatalf("first mapping LastError = %d, want 0", got)
	}
	b, err := mapMem(-1, 0, 4096, ACCESS_DEFAULT, tag)
	if err != nil {
		t.Fatalf("second mapping: %v", err)
	}
	defer unmapMem(b)
	if got := LastError(); got != uint32(windows.ERROR_ALREADY_EXISTS) {
		t.Fatalf("second mapping LastError = %d, want ERROR_ALREADY_EXISTS", got)
	}
	a[0], a[4095] = 7, 9
	if b[0] != 7 || b[4095] != 9 {
		t.Fatalf("write through one view not seen through the other: %d %d", b[0], b[4095])
	}
}
