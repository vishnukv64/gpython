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
