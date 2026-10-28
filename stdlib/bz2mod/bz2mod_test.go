// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bz2mod_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// The golden is CPython's output for the same script.
func Test_bz2(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_bz2.py")
}
