// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package time_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestTime(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}

func TestStrftime(t *testing.T) {
	pytest.RunScript(t, "./testdata/strftime.py")
}

// TestStructSeq covers time.struct_time and sys.version_info, the two struct
// sequences: tuples whose elements also have names.  It is registered by hand
// because this package drives RunScript per file rather than scanning the
// directory, so a new script there would otherwise never run.
func TestStructSeq(t *testing.T) {
	pytest.RunScript(t, "./testdata/structseq.py")
}
