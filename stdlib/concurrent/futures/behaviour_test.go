// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package futures_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestFuturesBehaviour exercises ThreadPoolExecutor, Future, wait and
// as_completed through the interpreter.  The rendezvous in the script is the
// load-bearing part: it can only complete if the pool really runs its tasks on
// more than one thread of control at a time.
func TestFuturesBehaviour(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
