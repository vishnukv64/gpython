// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmath_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// test_cmath_golden.txt is CPython's output for test_cmath.py: every function
// over a grid of finite, huge, tiny, signed-zero, infinite and NaN values.
func TestCmath(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_cmath.py")
}
