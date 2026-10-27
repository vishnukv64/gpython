//go:build !windows

package signal

import "syscall"

var consts = append([]struct {
	name string
	num  int
}{
	{"SIGHUP", int(syscall.SIGHUP)}, {"SIGINT", int(syscall.SIGINT)}, {"SIGQUIT", int(syscall.SIGQUIT)},
	{"SIGILL", int(syscall.SIGILL)}, {"SIGTRAP", int(syscall.SIGTRAP)}, {"SIGABRT", int(syscall.SIGABRT)},
	{"SIGIOT", int(syscall.SIGABRT)}, {"SIGBUS", int(syscall.SIGBUS)}, {"SIGFPE", int(syscall.SIGFPE)},
	{"SIGKILL", int(syscall.SIGKILL)}, {"SIGUSR1", int(syscall.SIGUSR1)}, {"SIGSEGV", int(syscall.SIGSEGV)},
	{"SIGUSR2", int(syscall.SIGUSR2)}, {"SIGPIPE", int(syscall.SIGPIPE)}, {"SIGALRM", int(syscall.SIGALRM)},
	{"SIGTERM", int(syscall.SIGTERM)}, {"SIGCHLD", int(syscall.SIGCHLD)}, {"SIGCONT", int(syscall.SIGCONT)},
	{"SIGSTOP", int(syscall.SIGSTOP)}, {"SIGTSTP", int(syscall.SIGTSTP)}, {"SIGTTIN", int(syscall.SIGTTIN)},
	{"SIGTTOU", int(syscall.SIGTTOU)}, {"SIGURG", int(syscall.SIGURG)}, {"SIGXCPU", int(syscall.SIGXCPU)},
	{"SIGXFSZ", int(syscall.SIGXFSZ)}, {"SIGVTALRM", int(syscall.SIGVTALRM)}, {"SIGPROF", int(syscall.SIGPROF)},
	{"SIGWINCH", int(syscall.SIGWINCH)}, {"SIGIO", int(syscall.SIGIO)}, {"SIGSYS", int(syscall.SIGSYS)},
}, extra...)
