//go:build linux

package signal

import "syscall"

const nsig = 65

var extra = []struct {
	name string
	num  int
}{
	{"SIGCLD", int(syscall.SIGCHLD)}, {"SIGPOLL", int(syscall.SIGIO)}, {"SIGPWR", int(syscall.SIGPWR)},
	{"SIGSTKFLT", int(syscall.SIGSTKFLT)}, {"SIGRTMIN", 34}, {"SIGRTMAX", 64},
}

const rtMin, rtMax = 34, 64
