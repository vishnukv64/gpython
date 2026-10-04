package imp_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestImp(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
