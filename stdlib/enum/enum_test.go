package enum_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestEnumMethods(t *testing.T) {
	pytest.RunScript(t, "./testdata/methods.py")
}
