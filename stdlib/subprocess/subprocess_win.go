//go:build windows || gpython_simwin

package subprocess

import (
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/types"
)

// platformEntries: the names CPython's subprocess defines on win32 and that
// multiprocessing.popen_spawn_win32 imports.  Popen here is os/exec, so
// STARTUPINFO and the creation flags are accepted but not interpreted.
func platformEntries() []py.DictEntry {
	e := []py.DictEntry{{Key: "STARTUPINFO", Value: types.SimpleNamespaceType}}
	for _, c := range []struct {
		n string
		v int64
	}{
		{"STARTF_USESHOWWINDOW", 0x1}, {"STARTF_USESTDHANDLES", 0x100},
		{"STARTF_FORCEONFEEDBACK", 0x40}, {"STARTF_FORCEOFFFEEDBACK", 0x80},
		{"SW_HIDE", 0}, {"STD_INPUT_HANDLE", -10}, {"STD_OUTPUT_HANDLE", -11}, {"STD_ERROR_HANDLE", -12},
		{"CREATE_NEW_CONSOLE", 0x10}, {"CREATE_NEW_PROCESS_GROUP", 0x200},
		{"ABOVE_NORMAL_PRIORITY_CLASS", 0x8000}, {"BELOW_NORMAL_PRIORITY_CLASS", 0x4000},
		{"HIGH_PRIORITY_CLASS", 0x80}, {"IDLE_PRIORITY_CLASS", 0x40},
		{"NORMAL_PRIORITY_CLASS", 0x20}, {"REALTIME_PRIORITY_CLASS", 0x100},
		{"CREATE_NO_WINDOW", 0x08000000}, {"DETACHED_PROCESS", 0x8},
		{"CREATE_DEFAULT_ERROR_MODE", 0x04000000}, {"CREATE_BREAKAWAY_FROM_JOB", 0x01000000},
	} {
		e = append(e, py.DictEntry{Key: c.n, Value: py.Int(c.v)})
	}
	return e
}
