package textwrap_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestTextwrap(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
