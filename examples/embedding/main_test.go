// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestEmbeddedExample(t *testing.T) {
	pytest.RunScript(t, "./testdata/mylib-demo.py")
}
