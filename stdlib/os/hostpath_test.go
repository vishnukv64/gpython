// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"runtime"
	"testing"
)

// os.path must be the host flavour.  os.go binds it during init, so a host
// choice made in an init() that ran later left Windows with posixpath.
func TestOsPathIsHostFlavour(t *testing.T) {
	want := "posixpath"
	if runtime.GOOS == "windows" {
		want = "ntpath"
	}
	if got := PathModule().ModuleImpl.Info.Name; got != want {
		t.Fatalf("os.path is %s, want %s", got, want)
	}
}
