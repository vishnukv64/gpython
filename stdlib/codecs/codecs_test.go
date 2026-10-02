// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codecs_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestCodecs runs the module's script against the golden output, which is
// CPython 3.14's own output for the same program - so the incremental
// decoder's hold-back behaviour, the module-level encode/decode shape and the
// per-encoding modules are pinned rather than merely exercised.
func TestCodecs(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}

// TestRot13 checks the rot13 transform round-trips and matches CPython.
func TestRot13(t *testing.T) {
	pytest.RunScript(t, "./testdata/rot13.py")
}
