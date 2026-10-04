// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestHeader runs testdata/header.py against its CPython-generated golden.
func TestHeader(t *testing.T) {
	pytest.RunScript(t, "./testdata/header.py")
}
