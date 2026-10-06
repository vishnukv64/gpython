//go:build !windows

package mmap_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestMmap(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}

func TestMmapPipLine(t *testing.T) {
	pytest.RunScript(t, "./testdata/pip_line.py")
}
