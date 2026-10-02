// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutil_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestShutilBehaviour exercises the module through the interpreter: the PATH
// search, the copy family, copytree, move, rmtree (including its refusal of a
// symlink to a directory) and disk_usage, on a real tree under /tmp.
func TestShutilBehaviour(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
