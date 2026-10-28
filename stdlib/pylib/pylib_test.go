package pylib_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestPylib runs each testdata script against output recorded from CPython 3.14.
func TestPylib(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
