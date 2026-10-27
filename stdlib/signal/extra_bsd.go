//go:build !linux && !windows

package signal

import "syscall"

const nsig = 32

var extra = []struct {
	name string
	num  int
}{{"SIGEMT", int(syscall.SIGEMT)}, {"SIGINFO", int(syscall.SIGINFO)}}

const rtMin, rtMax = 1, 0
