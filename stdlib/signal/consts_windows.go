//go:build windows

package signal

const nsig = 23

var consts = []struct {
	name string
	num  int
}{
	{"SIGINT", 2}, {"SIGILL", 4}, {"SIGFPE", 8}, {"SIGSEGV", 11}, {"SIGTERM", 15},
	{"SIGBREAK", 21}, {"SIGABRT", 22}, {"CTRL_C_EVENT", 0}, {"CTRL_BREAK_EVENT", 1},
}

const rtMin, rtMax = 1, 0
