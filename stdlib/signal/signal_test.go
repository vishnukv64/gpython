//go:build !windows

package signal_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestSignal(t *testing.T) { pytest.RunScript(t, "./testdata/test.py") }
