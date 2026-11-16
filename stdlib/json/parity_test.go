package json_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestJsonParity runs CPython 3.12 parity cases for json.dumps and
// json.loads; the golden file is CPython's own output.
func TestJsonParity(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_parity.py")
}

// TestStrAndDictParity covers the str.center fill order and the dict.pop
// KeyError text, which already match CPython and are kept as regressions.
func TestStrAndDictParity(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_parity_strings.py")
}
