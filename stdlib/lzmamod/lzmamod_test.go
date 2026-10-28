// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzmamod_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// The golden is CPython's output for the same script.
func Test_lzma(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_lzma.py")
}
