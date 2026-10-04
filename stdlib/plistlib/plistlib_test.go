package plistlib_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestPlistlib(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
