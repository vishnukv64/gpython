package traceback_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestTraceback runs the module's script and compares it against the golden
// output, which is CPython 3.14's own output for the same program - so the
// rendering of an exception, and the exception state around a handler, are
// pinned rather than merely exercised.
func TestTraceback(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
