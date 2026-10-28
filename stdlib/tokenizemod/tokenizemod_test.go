package tokenizemod_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// The golden is CPython 3.14's output for the same script.
func TestTokenize(t *testing.T) {
	pytest.RunScript(t, "./testdata/tok_corpus.py")
}

// tabnanny, pyclbr and inspect's tokenize-based helpers, against CPython's
// output for the same script.
func TestDevTools(t *testing.T) {
	pytest.RunScript(t, "./testdata/devtools/devtools.py")
}
