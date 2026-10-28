// Package signal implements the _signal module on top of os/signal.
//
// ponytail: handlers run in whichever goroutine's VM loop polls first, not
// strictly the main thread; fix by tagging the main Context if threads matter.
package signal

import (
	"os"
	ossignal "os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const maxSig = 65

var (
	mu       sync.Mutex
	handlers [maxSig]py.Object // nil == SIG_DFL
	tripped  [maxSig]atomic.Bool
	ch       = make(chan os.Signal, 64)
	once     sync.Once
	wakeFd   int64 = -1
)

func sigNum(o py.Object) (int, error) {
	if v, err := py.GetAttrString(o, "value"); err == nil {
		o = v // IntEnum member
	}
	i, err := py.IndexInt(o)
	if err != nil {
		return 0, err
	}
	n := int(i)
	if n < 1 || n >= maxSig {
		return 0, py.ExceptionNewf(py.ValueError, "signal number out of range")
	}
	return n, nil
}

func start() {
	once.Do(func() {
		go func() {
			for s := range ch {
				if n, ok := s.(syscall.Signal); ok && int(n) < maxSig {
					tripped[n].Store(true)
					py.SetSignalPending(true)
					if fd := atomic.LoadInt64(&wakeFd); fd >= 0 {
						writeWake(int(fd), byte(n))
					}
				}
			}
		}()
		py.RunSignalHandlers = runHandlers
		py.SignalSent = waitTripped
	})
}

func runHandlers() error {
	py.SetSignalPending(false)
	for n := 1; n < maxSig; n++ {
		if !tripped[n].Swap(false) {
			continue
		}
		mu.Lock()
		h := handlers[n]
		mu.Unlock()
		if _, ok := h.(py.I__call__); ok {
			if _, err := py.Call(h, py.Tuple{py.Int(n), py.None}, py.NewStringDict()); err != nil {
				return err
			}
		}
	}
	return nil
}

// waitTripped gives the notify goroutine a moment to queue a Python handler,
// so it runs before the sending call returns, as in CPython.
func waitTripped(n int) {
	for i := 0; i < 200 && !tripped[n].Load(); i++ {
		mu.Lock()
		h := handlers[n]
		mu.Unlock()
		if h == nil || h == py.Int(1) {
			return
		}
		time.Sleep(100 * time.Microsecond)
	}
}

func current(n int) py.Object {
	if h := handlers[n]; h != nil {
		return h
	}
	return py.Int(0)
}

func signal(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "signal expected 2 arguments, got %d", len(args))
	}
	n, err := sigNum(args[0])
	if err != nil {
		return nil, err
	}
	h := args[1]
	if !isCallable(h) {
		if v, err := py.GetAttrString(h, "value"); err == nil {
			h = v // Handlers member
		}
	}
	isInt := false
	if i, ok := h.(py.Int); ok && (i == 0 || i == 1) {
		isInt = true
	} else if !isCallable(h) {
		return nil, py.ExceptionNewf(py.TypeError, "signal handler must be signal.SIG_IGN, signal.SIG_DFL, or a callable object")
	}
	if n == int(syscall.SIGKILL) || n == sigStop {
		return nil, py.ExceptionNewf(py.OSError, "[Errno 22] Invalid argument")
	}
	start()
	mu.Lock()
	defer mu.Unlock()
	old := current(n)
	sig := syscall.Signal(n)
	switch {
	case isInt && h.(py.Int) == 0:
		ossignal.Reset(sig)
		handlers[n] = nil
	case isInt:
		ossignal.Ignore(sig)
		handlers[n] = h
	default:
		ossignal.Notify(ch, sig)
		handlers[n] = h
	}
	return old, nil
}

func getsignal(self py.Object, arg py.Object) (py.Object, error) {
	n, err := sigNum(arg)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	return current(n), nil
}

func defaultIntHandler(self py.Object, args py.Tuple) (py.Object, error) {
	return nil, py.ExceptionNewf(py.KeyboardInterrupt, "")
}

func raiseSignal(self py.Object, arg py.Object) (py.Object, error) {
	n, err := sigNum(arg)
	if err != nil {
		return nil, err
	}
	start()
	if err := raise(n); err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%v", err)
	}
	waitTripped(n)
	return py.None, nil
}

func strsignal(self py.Object, arg py.Object) (py.Object, error) {
	n, err := sigNum(arg)
	if err != nil {
		return nil, err
	}
	s := syscall.Signal(n).String()
	if len(s) > 7 && s[:7] == "signal " {
		return py.None, nil
	}
	if s != "" {
		s = string(s[0]-32*boolB(s[0] >= 'a')) + s[1:]
	}
	return py.String(s), nil
}

func boolB(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func setWakeupFd(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "set_wakeup_fd() missing required argument 'fd'")
	}
	i, err := py.GetInt(args[0])
	if err != nil {
		return nil, err
	}
	start()
	return py.Int(atomic.SwapInt64(&wakeFd, int64(i))), nil
}

func initSIGINT(g py.StringDict) {
	// Like CPython, a signal-aware interpreter turns SIGINT into KeyboardInterrupt.
	mu.Lock()
	defer mu.Unlock()
	if handlers[syscall.SIGINT] == nil {
		start()
		handlers[syscall.SIGINT] = g.GetOrNil("default_int_handler")
		ossignal.Notify(ch, syscall.SIGINT)
	}
}

var defaultIntHandlerObj = py.MustNewMethod("default_int_handler", defaultIntHandler, 0, "default_int_handler(signalnum, frame) -> raise KeyboardInterrupt")

func init() {
	methods := []*py.Method{
		py.MustNewMethod("signal", signal, 0, "signal(signalnum, handler) -> previous handler"),
		py.MustNewMethod("getsignal", getsignal, 0, "getsignal(signalnum) -> current handler"),
		py.MustNewMethod("raise_signal", raiseSignal, 0, "Send a signal to the executing process."),
		py.MustNewMethod("strsignal", strsignal, 0, "Return the system description of the signal."),
		py.MustNewMethod("set_wakeup_fd", setWakeupFd, 0, "set_wakeup_fd(fd) -> old fd"),
		defaultIntHandlerObj,
	}
	g := py.NewStringDict()
	g.Set("SIG_DFL", py.Int(0))
	g.Set("SIG_IGN", py.Int(1))
	g.Set("NSIG", py.Int(nsig))
	for _, c := range consts {
		g.Set(c.name, py.Int(c.num))
	}
	methods = append(methods, platformMethods()...)
	methods = append(methods, py.MustNewMethod("valid_signals", validSignals, 0, "Return a set of valid signal numbers on this platform."))
	py.RegisterModule(&py.ModuleImpl{
		Info:          py.ModuleInfo{Name: "_signal", Doc: "This module provides mechanisms to use signal handlers in Python."},
		Methods:       methods,
		Globals:       g,
		PreSetGlobals: initSIGINT,
	})
}

func validSignals(self py.Object) (py.Object, error) {
	s := py.NewSet()
	for _, c := range consts {
		if len(c.name) > 3 && c.name[:3] == "SIG" && c.name != "SIGCLD" && c.name != "SIGPOLL" && c.name != "SIGIOT" && c.name != "SIGRTMAX" && c.name != "SIGRTMIN" && c.num != sigStop && c.num != int(syscall.SIGKILL) || c.name == "SIGSTOP" || c.name == "SIGKILL" {
			s.Add(py.Int(c.num))
		}
	}
	for n := rtMin; n <= rtMax; n++ {
		s.Add(py.Int(n))
	}
	return s, nil
}

func isCallable(o py.Object) bool { _, ok := o.(py.I__call__); return ok }
