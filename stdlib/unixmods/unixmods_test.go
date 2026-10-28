// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package unixmods_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// The goldens in testdata are CPython's own output for the same scripts
// (python3 test_x.py > test_x_golden.txt), so each module is compared with
// CPython's behaviour, not with itself.  The scripts only print what is the
// same on Linux and macOS.
func TestUnixModules(t *testing.T) {
	for _, name := range []string{"fcntl", "pwdgrp", "termios", "pty", "resource", "syslog"} {
		t.Run(name, func(t *testing.T) {
			pytest.RunScript(t, "./testdata/test_"+name+".py")
		})
	}
}
