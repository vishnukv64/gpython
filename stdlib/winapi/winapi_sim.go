//go:build gpython_simwin && !windows

package winapi

import (
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/overlapped"
)

// platformFuncs: every function exists (modules bind them at class-definition
// time) but fails, since there is no Windows underneath.
func platformFuncs() []py.DictEntry {
	var out []py.DictEntry
	for _, n := range []string{
		"GetCurrentProcess", "GetLastError", "OpenProcess", "DuplicateHandle",
		"CreatePipe", "CreateNamedPipe", "ConnectNamedPipe", "CreateFile", "SetNamedPipeHandleState",
		"WaitNamedPipe", "PeekNamedPipe", "ReadFile", "WriteFile", "WaitForSingleObject",
		"WaitForMultipleObjects", "BatchedWaitForMultipleObjects", "GetExitCodeProcess",
		"TerminateProcess", "CreateProcess",
	} {
		n := n
		out = append(out, fn(n, func(py.Tuple) (py.Object, error) {
			return nil, py.ExceptionNewf(py.NotImplementedError, "_winapi.%s needs Windows", n)
		}))
	}
	out = append(out, fn("CloseHandle", func(a py.Tuple) (py.Object, error) {
		if len(a) == 1 {
			if h, err := py.IndexInt(a[0]); err == nil && overlapped.CloseIOCP(h) {
				return py.None, nil
			}
		}
		return nil, py.ExceptionNewf(py.NotImplementedError, "_winapi.CloseHandle needs Windows")
	}))
	return out
}
