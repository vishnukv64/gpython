package thread_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestThread(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
