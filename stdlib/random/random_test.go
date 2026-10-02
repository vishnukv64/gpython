package random_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestRandom(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
