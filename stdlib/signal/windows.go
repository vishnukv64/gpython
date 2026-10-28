//go:build windows

package signal

import (
	"github.com/vishnukv64/gpython/py"
)

const sigStop = -1

// raise: on Windows only the C-runtime signals can be raised in-process;
// deliver them straight to the notify path.
func raise(n int) error {
	tripped[n].Store(true)
	py.SetSignalPending(true)
	return nil
}

func writeWake(fd int, b byte) {}

func platformMethods() []*py.Method {
	return nil
}
