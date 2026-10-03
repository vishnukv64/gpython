package importlib_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestEntryPoints runs the script against the golden output, which is CPython
// 3.14's own output.  It pins importlib.metadata.entry_points and the module
// __dict__ that pygments rewrites itself through.
func TestEntryPoints(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
