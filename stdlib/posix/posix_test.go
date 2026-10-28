// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package posix_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// The script picks posix or nt by os.name; the golden is CPython's output on
// Unix and is the same on Windows.
func TestPosix(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_posix.py")
}
