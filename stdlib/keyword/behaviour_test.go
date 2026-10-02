// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package keyword_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestKeywordBehaviour exercises the module the way a caller does, through the
// interpreter, so that the predicates are reached by name as a library reaches
// them.
func TestKeywordBehaviour(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
