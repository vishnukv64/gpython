package importlibabc_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestImportlibABC(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
