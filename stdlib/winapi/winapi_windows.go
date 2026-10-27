package winapi

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/mmap"
	"github.com/vishnukv64/gpython/stdlib/overlapped"
)

var (
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	procPeekNamed  = kernel32.NewProc("PeekNamedPipe")
	procWaitNamed  = kernel32.NewProc("WaitNamedPipeW")
	procCreateSem  = kernel32.NewProc("CreateSemaphoreW")
	procReleaseSem = kernel32.NewProc("ReleaseSemaphore")
)

// winErr is the OSError CPython raises for a failed Win32 call.
func winErr(err error) error {
	if en, ok := err.(syscall.Errno); ok {
		return py.WinOSError(int(en), en.Error())
	}
	return py.ExceptionNewf(py.OSError, "%s", err.Error())
}

func handleArg(a py.Tuple, i int) (windows.Handle, error) {
	if i >= len(a) {
		return 0, py.ExceptionNewf(py.TypeError, "missing handle argument")
	}
	n, err := py.IndexInt(a[i])
	return windows.Handle(n), err
}

func uintArg(a py.Tuple, i int) (uint32, error) {
	if i >= len(a) {
		return 0, py.ExceptionNewf(py.TypeError, "missing integer argument")
	}
	n, err := py.MakeGoInt64(a[i])
	return uint32(n), err
}

// opt returns positional i or keyword name, nil when absent.
func opt(a py.Tuple, k py.StringDict, i int, name string) py.Object {
	if i < len(a) {
		return a[i]
	}
	if v, ok := k.Get(name); ok {
		return v
	}
	return nil
}

func truthy(o py.Object) bool {
	if o == nil {
		return false
	}
	b, _ := py.ObjectIsTrue(o)
	return b
}

func utf16Opt(o py.Object) (*uint16, error) {
	if o == nil || o == py.None {
		return nil, nil
	}
	s, err := py.StrAsString(o)
	if err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(s)
}

// Overlapped is _winapi.Overlapped: an OVERLAPPED plus the buffer it fills.
type Overlapped struct {
	ov        windows.Overlapped
	handle    windows.Handle
	event     windows.Handle
	buf       []byte
	completed bool
	n         uint32
}

var OverlappedType = py.NewTypeX("_winapi.Overlapped", "OVERLAPPED structure wrapper", nil, nil)

func (o *Overlapped) Type() *py.Type { return OverlappedType }

func newOverlapped(h windows.Handle) (*Overlapped, error) {
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, winErr(err)
	}
	o := &Overlapped{handle: h, event: ev}
	o.ov.HEvent = ev
	// Released when Python drops the object, as CPython's dealloc does: the
	// event stays valid for as long as Python can wait on o.event.
	runtime.SetFinalizer(o, (*Overlapped).release)
	return o, nil
}

// release is the finalizer.  An operation still in flight would write into
// o.ov and o.buf after they are reclaimed, so it is cancelled and waited for
// first, exactly as CPython's Overlapped dealloc does.
func (o *Overlapped) release() {
	if !o.completed && o.handle != 0 {
		if windows.CancelIoEx(o.handle, &o.ov) == nil {
			var n uint32
			_ = windows.GetOverlappedResult(o.handle, &o.ov, &n, true)
		}
	}
	o.free()
}

func (o *Overlapped) free() {
	if o.event != 0 {
		windows.CloseHandle(o.event)
		o.event = 0
	}
}

// result waits (or polls) and returns bytes transferred and the Win32 status.
func (o *Overlapped) result(wait bool) (py.Object, error) {
	var n uint32
	err := windows.GetOverlappedResult(o.handle, &o.ov, &n, wait)
	code := 0
	if err != nil {
		code = int(err.(syscall.Errno))
	}
	switch code {
	case 0, int(windows.ERROR_MORE_DATA), int(windows.ERROR_OPERATION_ABORTED):
		o.completed, o.n = true, n
	case int(windows.ERROR_IO_INCOMPLETE):
	default:
		return nil, winErr(err)
	}
	return py.Tuple{py.Int(n), py.Int(code)}, nil
}

func init() {
	m := func(name string, f func(o *Overlapped, a py.Tuple) (py.Object, error)) {
		OverlappedType.Dict.Set(name, py.MustNewMethod(name, func(self py.Object, a py.Tuple) (py.Object, error) { return f(self.(*Overlapped), a) }, 0, name))
	}
	m("GetOverlappedResult", func(o *Overlapped, a py.Tuple) (py.Object, error) {
		return o.result(len(a) > 0 && truthy(a[0]))
	})
	m("getbuffer", func(o *Overlapped, a py.Tuple) (py.Object, error) {
		if !o.completed {
			return py.None, nil
		}
		return py.Bytes(append([]byte(nil), o.buf[:o.n]...)), nil
	})
	m("cancel", func(o *Overlapped, a py.Tuple) (py.Object, error) {
		if !o.completed {
			if err := windows.CancelIoEx(o.handle, &o.ov); err != nil && err != windows.ERROR_NOT_FOUND {
				return nil, winErr(err)
			}
		}
		return py.None, nil
	})
	OverlappedType.Dict.Set("event", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*Overlapped).event), nil
	}})
}

func errCode(err error) int {
	if err == nil {
		return 0
	}
	if en, ok := err.(syscall.Errno); ok {
		return int(en)
	}
	return -1
}

func platformFuncs() []py.DictEntry {
	return []py.DictEntry{
		fn("CloseHandle", func(a py.Tuple) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			if overlapped.CloseIOCP(int(h)) {
				return py.None, nil // an emulated completion port, not a kernel handle
			}
			if err := windows.CloseHandle(h); err != nil {
				return nil, winErr(err)
			}
			return py.None, nil
		}),
		fn("GetCurrentProcess", func(a py.Tuple) (py.Object, error) {
			return py.Int(windows.CurrentProcess()), nil
		}),
		fn("GetLastError", func(a py.Tuple) (py.Object, error) {
			return py.Int(mmap.LastError()), nil
		}),
		fn("OpenProcess", func(a py.Tuple) (py.Object, error) {
			access, err := uintArg(a, 0)
			if err != nil {
				return nil, err
			}
			pid, err := uintArg(a, 2)
			if err != nil {
				return nil, err
			}
			h, err := windows.OpenProcess(access, len(a) > 1 && truthy(a[1]), pid)
			if err != nil {
				return nil, winErr(err)
			}
			return py.Int(h), nil
		}),
		fn("DuplicateHandle", func(a py.Tuple) (py.Object, error) {
			src, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			h, err := handleArg(a, 1)
			if err != nil {
				return nil, err
			}
			dst, err := handleArg(a, 2)
			if err != nil {
				return nil, err
			}
			access, err := uintArg(a, 3)
			if err != nil {
				return nil, err
			}
			var opts uint32
			if len(a) > 5 {
				if opts, err = uintArg(a, 5); err != nil {
					return nil, err
				}
			}
			var out windows.Handle
			if err := windows.DuplicateHandle(src, h, dst, &out, access, len(a) > 4 && truthy(a[4]), opts); err != nil {
				return nil, winErr(err)
			}
			return py.Int(out), nil
		}),
		fn("CreatePipe", func(a py.Tuple) (py.Object, error) {
			var size uint32
			if len(a) > 1 {
				size, _ = uintArg(a, 1)
			}
			var r, w windows.Handle
			if err := windows.CreatePipe(&r, &w, nil, size); err != nil {
				return nil, winErr(err)
			}
			return py.Tuple{py.Int(r), py.Int(w)}, nil
		}),
		fn("CreateNamedPipe", func(a py.Tuple) (py.Object, error) {
			if len(a) < 8 {
				return nil, py.ExceptionNewf(py.TypeError, "CreateNamedPipe() takes 8 arguments")
			}
			name, err := utf16Opt(a[0])
			if err != nil {
				return nil, err
			}
			v := [6]uint32{}
			for i := range v {
				if v[i], err = uintArg(a, i+1); err != nil {
					return nil, err
				}
			}
			h, err := windows.CreateNamedPipe(name, v[0], v[1], v[2], v[3], v[4], v[5], nil)
			if err != nil {
				return nil, winErr(err)
			}
			return py.Int(h), nil
		}),
		kfn("ConnectNamedPipe", func(a py.Tuple, k py.StringDict) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			if !truthy(opt(a, k, 1, "overlapped")) {
				if err := windows.ConnectNamedPipe(h, nil); err != nil {
					return nil, winErr(err)
				}
				return py.None, nil
			}
			o, err := newOverlapped(h)
			if err != nil {
				return nil, err
			}
			err = windows.ConnectNamedPipe(h, &o.ov)
			switch errCode(err) {
			case 0, int(windows.ERROR_IO_PENDING):
			case int(windows.ERROR_PIPE_CONNECTED):
				windows.SetEvent(o.event)
			default:
				o.free()
				return nil, winErr(err)
			}
			return o, nil
		}),
		fn("CreateFile", func(a py.Tuple) (py.Object, error) {
			if len(a) < 7 {
				return nil, py.ExceptionNewf(py.TypeError, "CreateFile() takes 7 arguments")
			}
			name, err := utf16Opt(a[0])
			if err != nil {
				return nil, err
			}
			v := [4]uint32{}
			for i, idx := range []int{1, 2, 4, 5} {
				if v[i], err = uintArg(a, idx); err != nil {
					return nil, err
				}
			}
			h, err := windows.CreateFile(name, v[0], v[1], nil, v[2], v[3], 0)
			if err != nil {
				return nil, winErr(err)
			}
			return py.Int(h), nil
		}),
		fn("SetNamedPipeHandleState", func(a py.Tuple) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			var p [3]*uint32
			for i := range p {
				if i+1 < len(a) && a[i+1] != py.None {
					v, err := uintArg(a, i+1)
					if err != nil {
						return nil, err
					}
					p[i] = &v
				}
			}
			if err := windows.SetNamedPipeHandleState(h, p[0], p[1], p[2]); err != nil {
				return nil, winErr(err)
			}
			return py.None, nil
		}),
		fn("WaitNamedPipe", func(a py.Tuple) (py.Object, error) {
			if len(a) < 2 {
				return nil, py.ExceptionNewf(py.TypeError, "WaitNamedPipe() takes exactly 2 arguments (%d given)", len(a))
			}
			name, err := utf16Opt(a[0])
			if err != nil {
				return nil, err
			}
			ms, err := uintArg(a, 1)
			if err != nil {
				return nil, err
			}
			if r, _, e := procWaitNamed.Call(uintptr(unsafe.Pointer(name)), uintptr(ms)); r == 0 {
				return nil, winErr(e)
			}
			return py.None, nil
		}),
		fn("PeekNamedPipe", func(a py.Tuple) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			var avail, left uint32
			if r, _, e := procPeekNamed.Call(uintptr(h), 0, 0, 0, uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&left))); r == 0 {
				return nil, winErr(e)
			}
			return py.Tuple{py.Int(avail), py.Int(left)}, nil
		}),
		kfn("ReadFile", func(a py.Tuple, k py.StringDict) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			size, err := uintArg(a, 1)
			if err != nil {
				return nil, err
			}
			buf := make([]byte, size)
			if !truthy(opt(a, k, 2, "overlapped")) {
				var n uint32
				err := windows.ReadFile(h, buf, &n, nil)
				if code := errCode(err); code != 0 && code != int(windows.ERROR_MORE_DATA) {
					return nil, winErr(err)
				}
				return py.Tuple{py.Bytes(buf[:n]), py.Int(errCode(err))}, nil
			}
			o, err := newOverlapped(h)
			if err != nil {
				return nil, err
			}
			o.buf = buf
			var n uint32
			err = windows.ReadFile(h, buf, &n, &o.ov)
			switch code := errCode(err); code {
			case 0, int(windows.ERROR_IO_PENDING), int(windows.ERROR_MORE_DATA):
				return py.Tuple{o, py.Int(code)}, nil
			}
			o.free()
			return nil, winErr(err)
		}),
		kfn("WriteFile", func(a py.Tuple, k py.StringDict) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			if len(a) < 2 {
				return nil, py.ExceptionNewf(py.TypeError, "WriteFile() requires a buffer")
			}
			data, _, ok := py.BufferBytes(a[1])
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required")
			}
			data = append([]byte(nil), data...)
			if !truthy(opt(a, k, 2, "overlapped")) {
				var n uint32
				if err := windows.WriteFile(h, data, &n, nil); err != nil {
					return nil, winErr(err)
				}
				return py.Tuple{py.Int(n), py.Int(0)}, nil
			}
			o, err := newOverlapped(h)
			if err != nil {
				return nil, err
			}
			o.buf = data
			var n uint32
			err = windows.WriteFile(h, data, &n, &o.ov)
			switch code := errCode(err); code {
			case 0, int(windows.ERROR_IO_PENDING):
				return py.Tuple{o, py.Int(code)}, nil
			}
			o.free()
			return nil, winErr(err)
		}),
		fn("WaitForSingleObject", func(a py.Tuple) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			ms, err := uintArg(a, 1)
			if err != nil {
				return nil, err
			}
			r, err := windows.WaitForSingleObject(h, ms)
			if err != nil {
				return nil, winErr(err)
			}
			return py.Int(r), nil
		}),
		fn("WaitForMultipleObjects", func(a py.Tuple) (py.Object, error) {
			hs, err := handleList(a, 0)
			if err != nil {
				return nil, err
			}
			ms, err := uintArg(a, 2)
			if err != nil {
				return nil, err
			}
			if len(hs) > maxWait {
				return nil, py.ExceptionNewf(py.ValueError, "need at most %d handles, got a sequence of length %d", maxWait, len(hs))
			}
			r, err := windows.WaitForMultipleObjects(hs, len(a) > 1 && truthy(a[1]), ms)
			if err != nil {
				return nil, winErr(err)
			}
			return py.Int(r), nil
		}),
		fn("BatchedWaitForMultipleObjects", batchedWait),
		fn("GetExitCodeProcess", func(a py.Tuple) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			var code uint32
			if err := windows.GetExitCodeProcess(h, &code); err != nil {
				return nil, winErr(err)
			}
			return py.Int(code), nil
		}),
		fn("TerminateProcess", func(a py.Tuple) (py.Object, error) {
			h, err := handleArg(a, 0)
			if err != nil {
				return nil, err
			}
			code, err := uintArg(a, 1)
			if err != nil {
				return nil, err
			}
			if err := windows.TerminateProcess(h, code); err != nil {
				return nil, winErr(err)
			}
			return py.None, nil
		}),
		fn("CreateProcess", createProcess),
	}
}

func handleList(a py.Tuple, i int) ([]windows.Handle, error) {
	if i >= len(a) {
		return nil, py.ExceptionNewf(py.TypeError, "missing handle sequence")
	}
	l, err := py.SequenceList(a[i])
	if err != nil {
		return nil, err
	}
	hs := make([]windows.Handle, len(l.Items))
	for j, it := range l.Items {
		n, err := py.IndexInt(it)
		if err != nil {
			return nil, err
		}
		hs[j] = windows.Handle(n)
	}
	return hs, nil
}

// batchedWait returns the indices of every signalled handle (wait_all=False),
// or [] on timeout.  Up to 64 handles cost one blocking wait; more are polled.
func batchedWait(a py.Tuple) (py.Object, error) {
	hs, err := handleList(a, 0)
	if err != nil {
		return nil, err
	}
	all := len(a) > 1 && truthy(a[1])
	ms := uint32(windows.INFINITE)
	if len(a) > 2 {
		if ms, err = uintArg(a, 2); err != nil {
			return nil, err
		}
	}
	signalled := func() []py.Object {
		var out []py.Object
		for i, h := range hs {
			if r, _ := windows.WaitForSingleObject(h, 0); r == windows.WAIT_OBJECT_0 || r == windows.WAIT_ABANDONED {
				out = append(out, py.Int(i))
			}
		}
		return out
	}
	if len(hs) <= maxWait {
		r, err := windows.WaitForMultipleObjects(hs, all, ms)
		if err != nil {
			return nil, winErr(err)
		}
		if r == uint32(windows.WAIT_TIMEOUT) {
			return &py.List{}, nil
		}
		return &py.List{Items: signalled()}, nil
	}
	deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
	for {
		if s := signalled(); len(s) > 0 && (!all || len(s) == len(hs)) {
			return &py.List{Items: s}, nil
		}
		if ms != windows.INFINITE && time.Now().After(deadline) {
			return &py.List{}, nil
		}
		time.Sleep(time.Millisecond)
	}
}

// CreateProcess(app, cmd, procattr, threadattr, inherit, flags, env, cwd, startupinfo)
func createProcess(a py.Tuple) (py.Object, error) {
	if len(a) < 9 {
		return nil, py.ExceptionNewf(py.TypeError, "CreateProcess() takes 9 arguments")
	}
	app, err := utf16Opt(a[0])
	if err != nil {
		return nil, err
	}
	cmd, err := utf16Opt(a[1])
	if err != nil {
		return nil, err
	}
	flags, err := uintArg(a, 5)
	if err != nil {
		return nil, err
	}
	var env *uint16
	if a[6] != py.None {
		d, ok := a[6].(py.StringDict)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "env must be a dict")
		}
		var blk []uint16
		for _, e := range d.Items() {
			k, v := e.Key, e.Value
			s, err := py.StrAsString(v)
			if err != nil {
				return nil, err
			}
			u, err := windows.UTF16FromString(k + "=" + s)
			if err != nil {
				return nil, py.ExceptionNewf(py.ValueError, "embedded null character")
			}
			blk = append(blk, u...)
		}
		blk = append(blk, 0)
		env = &blk[0]
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}
	cwd, err := utf16Opt(a[7])
	if err != nil {
		return nil, err
	}
	si := windows.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(app, cmd, nil, nil, truthy(a[4]), flags, env, cwd, &si, &pi); err != nil {
		return nil, winErr(err)
	}
	return py.Tuple{py.Int(pi.Process), py.Int(pi.Thread), py.Int(pi.ProcessId), py.Int(pi.ThreadId)}, nil
}

// Semaphores for _multiprocessing.SemLock (see stdlib/mpcore).

// CreateSemaphore makes an unnamed semaphore.
func CreateSemaphore(initial, max int) (windows.Handle, error) {
	h, _, e := procCreateSem.Call(0, uintptr(initial), uintptr(max), 0)
	if h == 0 {
		return 0, e
	}
	return windows.Handle(h), nil
}

// ReleaseSemaphore adds one and returns the previous count.
func ReleaseSemaphore(h windows.Handle) (int, error) {
	var prev int32
	if r, _, e := procReleaseSem.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&prev))); r == 0 {
		return 0, e
	}
	return int(prev), nil
}

const maxWait = 64 // MAXIMUM_WAIT_OBJECTS
