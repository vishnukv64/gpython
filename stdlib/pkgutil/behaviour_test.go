// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkgutil_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestPkgutilBehaviour exercises pkgutil through the interpreter, asserting the values
// CPython 3.14 produces (captured with python3 and diffed).
func TestPkgutilBehaviour(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
