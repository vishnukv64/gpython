package functools_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestCachedProperty(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
