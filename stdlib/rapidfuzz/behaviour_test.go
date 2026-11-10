package rapidfuzz_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// The golden is the output of this script under CPython with the compiled
// rapidfuzz 3.14.6 (PYTHONPATH=<pip install --target dir> python3 test.py).
func TestRapidfuzzBehaviour(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
