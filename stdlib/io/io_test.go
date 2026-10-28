// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestTextIOWrapper compares io.TextIOWrapper over a BytesIO with CPython's output.
func TestTextIOWrapper(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_textio.py")
}

// TestStdBuffer checks sys.std*.buffer and file.writelines against CPython.
func TestStdBuffer(t *testing.T) {
	pytest.RunScript(t, "./testdata/stdbuffer.py")
}
