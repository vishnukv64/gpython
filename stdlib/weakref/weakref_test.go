package weakref_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestFinalize(t *testing.T) {
	pytest.RunScript(t, "./testdata/finalize.py")
}
