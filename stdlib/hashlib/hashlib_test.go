package hashlib_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestHashlib(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
