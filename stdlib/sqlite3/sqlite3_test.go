package sqlite3_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestSqlite3(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
