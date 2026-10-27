package contextvars_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// Goldens are CPython 3.14 output.
func TestContextvars(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
