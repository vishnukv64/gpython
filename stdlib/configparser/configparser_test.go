package configparser_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestConfigparser runs the module's script against the golden output, which is
// CPython 3.14's own output for the same program - so section listing, default
// inheritance, both key/value separators and the typed getters are pinned.
func TestConfigparser(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
