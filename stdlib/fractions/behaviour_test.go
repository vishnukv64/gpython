// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fractions_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestFractionsBehaviour exercises fractions through the interpreter, asserting the values
// CPython 3.14 produces (captured with python3 and diffed).
func TestFractionsBehaviour(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
