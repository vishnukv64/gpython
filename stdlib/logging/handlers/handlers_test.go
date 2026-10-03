package handlers_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestHandlers runs the module's script against the golden output, which is
// CPython 3.14's own output for the same program - so rotation, the buffering
// handler, the re-exported parent classes and the refusal of the handlers that
// need a platform service are all pinned.
func TestHandlers(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
