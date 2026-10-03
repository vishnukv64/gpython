package logging_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestLogging runs the module's script against the golden output, which is
// CPython 3.14's own output for the same program.  It pins logging.Filter,
// including the child-logger rule and a subclass whose override must win.
func TestLogging(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
