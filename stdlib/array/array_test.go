// Copyright 2023 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package array_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestArray(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
