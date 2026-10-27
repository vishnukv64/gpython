package inspect_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestAbstract(t *testing.T) {
	pytest.RunScript(t, "./testdata/abstract.py")
}
