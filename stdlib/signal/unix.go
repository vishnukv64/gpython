//go:build !windows

package signal

import (
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const sigStop = int(syscall.SIGSTOP)

func raise(n int) error { return syscall.Kill(syscall.Getpid(), syscall.Signal(n)) }

func writeWake(fd int, b byte) { syscall.Write(fd, []byte{b}) }

var alarmTimer atomic.Pointer[time.Timer]
var alarmAt atomic.Int64

func alarm(self py.Object, arg py.Object) (py.Object, error) {
	s, err := py.GetInt(arg)
	if err != nil {
		return nil, err
	}
	var prev int64
	if t := alarmTimer.Swap(nil); t != nil {
		t.Stop()
		if left := time.Until(time.Unix(0, alarmAt.Load())); left > 0 {
			prev = int64((left + time.Second - 1) / time.Second)
		}
	}
	if s > 0 {
		alarmAt.Store(time.Now().Add(time.Duration(s) * time.Second).UnixNano())
		alarmTimer.Store(time.AfterFunc(time.Duration(s)*time.Second, func() { syscall.Kill(syscall.Getpid(), syscall.SIGALRM) }))
	}
	return py.Int(prev), nil
}

func pause(self py.Object) (py.Object, error) {
	start()
	for !py.SignalPending.Load() {
		time.Sleep(time.Millisecond)
	}
	if err := runHandlers(); err != nil {
		return nil, err
	}
	return py.None, nil
}

func platformMethods() []*py.Method {
	return []*py.Method{
		py.MustNewMethod("alarm", alarm, 0, "Arrange for SIGALRM to arrive after the given number of seconds."),
		py.MustNewMethod("pause", pause, 0, "Wait until a signal arrives."),
	}
}
