//go:build windows || gpython_simwin

// Package overlapped is _overlapped, the C module behind asyncio's Windows
// ProactorEventLoop.
//
// gpython's sockets are Go net values, not winsock handles, so real IOCP
// cannot drive them.  This emulates the part asyncio uses: an Overlapped
// operation runs in a goroutine on the socket's net.Conn and, when done, posts
// a completion to the emulated port that GetQueuedCompletionStatus drains.
// Sockets are found by the number fileno() returned (socket.Lookup).
//
// Not emulated, each raising NotImplementedError: named-pipe I/O, events,
// RegisterWait* and TransmitFile.  They serve asyncio subprocesses, pipe
// servers and sendfile, which the Go-backed subprocess/sockets cannot back.
package overlapped

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/socket"
)

const (
	errorInvalidHandle   = 6
	errorIOIncomplete    = 996
	errorIOPending       = 997
	errorOperationAbort  = 995
	errorNetnameDeleted  = 64
	errorConnRefused     = 1225
	errorConnAborted     = 1236
	errorGenFailure      = 31
	errorPipeBusy        = 231
	errorPortUnreachable = 1234
	errorSemTimeout      = 121
	wsaeNotSock          = 10038
	infinite             = 0xFFFFFFFF
	// portBase keeps emulated port numbers clear of real kernel handles.
	portBase = 1 << 28
)

type status struct{ err, transferred, key, addr int }

type port struct{ q chan status }

var (
	mu     sync.Mutex
	ports  = map[int]*port{}
	assoc  = map[any]assocEntry{} // socket -> port it was registered with
	nextID atomic.Int64
)

type assocEntry struct {
	p   *port
	key int
}

// sockKey identifies a socket across fileno() changes (a socket's number
// differs before and after AcceptEx hands it a connection).  Callers hold mu.
func sockKey(fd int) any {
	if r, ok := socket.Lookup(fd); ok {
		return r.Key()
	}
	return fd
}

// CloseIOCP releases an emulated port; false means h is not one.
func CloseIOCP(h int) bool {
	mu.Lock()
	defer mu.Unlock()
	_, ok := ports[h]
	delete(ports, h)
	return ok
}

func notImpl(what string) error {
	return py.ExceptionNewf(py.NotImplementedError, "_overlapped.%s is not supported by gpython", what)
}

// winErr maps an error from Go's net onto the Win32 code asyncio tests for.
func winErr(err error, canceled bool) int {
	if err == nil {
		return 0
	}
	if canceled || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
		return errorOperationAbort
	}
	var en syscall.Errno
	raw := 0
	if errors.As(err, &en) {
		raw = int(en)
	}
	switch {
	case raw == 10054 || raw == 10053 || raw == 10058 || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE):
		return errorNetnameDeleted
	case raw == 10061 || errors.Is(err, syscall.ECONNREFUSED):
		return errorConnRefused
	case errors.Is(err, syscall.ECONNABORTED):
		return errorConnAborted
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ETIMEDOUT):
		return 10060
	}
	return errorGenFailure
}

func winMsg(code int) string {
	switch code {
	case errorIOIncomplete:
		return "Overlapped I/O event is not in a signaled state"
	case errorOperationAbort:
		return "The I/O operation has been aborted because of either a thread exit or an application request"
	case errorNetnameDeleted:
		return "The specified network name is no longer available"
	case errorConnRefused:
		return "The remote computer refused the network connection"
	case errorConnAborted:
		return "The network connection was aborted by the local system"
	case errorInvalidHandle, wsaeNotSock:
		return "The handle is invalid"
	case 10060:
		return "A connection attempt failed because the connected party did not properly respond after a period of time"
	}
	return "Windows Error"
}

// Overlapped is _overlapped.Overlapped.
type Overlapped struct {
	id    int
	event py.Object

	mu        sync.Mutex
	pending   bool
	done      chan struct{}
	started   bool
	canceled  bool
	interrupt func()
	port      *port
	key       int
	result    py.Object
	n         int
	werr      int
}

var OverlappedType = py.NewTypeX("_overlapped.Overlapped", "OVERLAPPED structure wrapper",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var ev py.Object = py.Int(0)
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:Overlapped", []string{"event"}, &ev); err != nil {
			return nil, err
		}
		return &Overlapped{id: int(nextID.Add(1)) << 4, event: ev}, nil
	}, nil)

func (o *Overlapped) Type() *py.Type { return OverlappedType }

// start runs fn in a goroutine and completes o with its outcome.  fn gets the
// context-free handle to register an interrupt that cancel() calls.
func (o *Overlapped) start(fd int, fn func() (py.Object, int, error)) error {
	o.mu.Lock()
	if o.started {
		o.mu.Unlock()
		return py.WinOSError(errorIOPending, "Overlapped I/O operation is in progress")
	}
	o.started, o.pending = true, true
	o.done = make(chan struct{})
	mu.Lock()
	if a, ok := assoc[sockKey(fd)]; ok {
		o.port, o.key = a.p, a.key
	}
	mu.Unlock()
	o.mu.Unlock()
	go func() {
		res, n, err := fn()
		o.mu.Lock()
		o.result, o.n = res, n
		o.werr = winErr(err, o.canceled)
		o.pending = false
		close(o.done)
		p, key := o.port, o.key
		o.mu.Unlock()
		if p != nil {
			p.q <- status{o.werr, n, key, o.id}
		}
	}()
	return nil
}

func (o *Overlapped) setInterrupt(f func()) {
	o.mu.Lock()
	o.interrupt = f
	if o.canceled {
		f()
	}
	o.mu.Unlock()
}

func (o *Overlapped) cancel() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.pending || o.canceled {
		return
	}
	o.canceled = true
	if o.interrupt != nil {
		o.interrupt()
	}
}

func (o *Overlapped) getresult(wait bool) (py.Object, error) {
	o.mu.Lock()
	if !o.started {
		o.mu.Unlock()
		return nil, py.WinOSError(errorInvalidHandle, winMsg(errorInvalidHandle))
	}
	done := o.done
	pending := o.pending
	o.mu.Unlock()
	if pending {
		if !wait {
			return nil, py.WinOSError(errorIOIncomplete, winMsg(errorIOIncomplete))
		}
		<-done
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.werr != 0 {
		return nil, py.WinOSError(o.werr, winMsg(o.werr))
	}
	if o.result != nil {
		return o.result, nil
	}
	return py.Int(o.n), nil
}

func sockOf(args py.Tuple, i int) (*socket.Raw, int, error) {
	if len(args) <= i {
		return nil, 0, py.ExceptionNewf(py.TypeError, "missing handle argument")
	}
	fd, err := py.IndexInt(args[i])
	if err != nil {
		return nil, 0, err
	}
	r, ok := socket.Lookup(fd)
	if !ok {
		return nil, 0, py.WinOSError(wsaeNotSock, winMsg(wsaeNotSock))
	}
	return r, fd, nil
}

func intArg(args py.Tuple, i int) (int, error) {
	if len(args) <= i {
		return 0, py.ExceptionNewf(py.TypeError, "missing integer argument")
	}
	return py.IndexInt(args[i])
}

func bufArg(args py.Tuple, i int) ([]byte, bool, error) {
	if len(args) <= i {
		return nil, false, py.ExceptionNewf(py.TypeError, "missing buffer argument")
	}
	b, w, ok := py.BufferBytes(args[i])
	if !ok {
		return nil, false, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", args[i].Type().Name)
	}
	return b, w, nil
}

func (o *Overlapped) recv(r *socket.Raw, fd int, buf []byte, into bool) error {
	c := r.Conn()
	if c == nil {
		return py.WinOSError(errorNetnameDeleted, winMsg(errorNetnameDeleted))
	}
	return o.start(fd, func() (py.Object, int, error) {
		o.setInterrupt(func() { _ = c.SetReadDeadline(time.Unix(1, 0)) })
		n, err := c.Read(buf)
		o.mu.Lock()
		if o.canceled {
			_ = c.SetReadDeadline(time.Time{})
		}
		o.mu.Unlock()
		if err != nil && n == 0 && errors.Is(err, io.EOF) {
			err = nil
		}
		if err != nil {
			return nil, 0, err
		}
		if into {
			return py.Int(n), n, nil
		}
		return py.Bytes(append([]byte(nil), buf[:n]...)), n, nil
	})
}

func (o *Overlapped) send(r *socket.Raw, fd int, data []byte) error {
	c := r.Conn()
	if c == nil {
		return py.WinOSError(errorNetnameDeleted, winMsg(errorNetnameDeleted))
	}
	data = append([]byte(nil), data...)
	return o.start(fd, func() (py.Object, int, error) {
		n, err := c.Write(data)
		return py.Int(n), n, err
	})
}

func init() {
	m := func(name string, f func(o *Overlapped, args py.Tuple) (py.Object, error)) {
		OverlappedType.Dict.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			return f(self.(*Overlapped), args)
		}, 0, name))
	}
	recvFn := func(into bool) func(o *Overlapped, args py.Tuple) (py.Object, error) {
		return func(o *Overlapped, args py.Tuple) (py.Object, error) {
			r, fd, err := sockOf(args, 0)
			if err != nil {
				return nil, err
			}
			var buf []byte
			if into {
				if buf, _, err = bufArg(args, 1); err != nil {
					return nil, err
				}
			} else {
				n, err := intArg(args, 1)
				if err != nil {
					return nil, err
				}
				if n < 0 {
					return nil, py.ExceptionNewf(py.ValueError, "negative size")
				}
				buf = make([]byte, n)
			}
			return py.None, o.recv(r, fd, buf, into)
		}
	}
	m("WSARecv", recvFn(false))
	m("ReadFile", recvFn(false))
	m("WSARecvInto", recvFn(true))
	m("ReadFileInto", recvFn(true))
	sendFn := func(o *Overlapped, args py.Tuple) (py.Object, error) {
		r, fd, err := sockOf(args, 0)
		if err != nil {
			return nil, err
		}
		b, _, err := bufArg(args, 1)
		if err != nil {
			return nil, err
		}
		return py.None, o.send(r, fd, b)
	}
	m("WSASend", sendFn)
	m("WriteFile", sendFn)
	m("AcceptEx", func(o *Overlapped, args py.Tuple) (py.Object, error) {
		l, fd, err := sockOf(args, 0)
		if err != nil {
			return nil, err
		}
		acc, _, err := sockOf(args, 1)
		if err != nil {
			return nil, err
		}
		ln := l.Listener()
		if ln == nil {
			return nil, py.WinOSError(wsaeNotSock, winMsg(wsaeNotSock))
		}
		return py.None, o.start(fd, func() (py.Object, int, error) {
			if d, ok := ln.(interface{ SetDeadline(time.Time) error }); ok {
				o.setInterrupt(func() { _ = d.SetDeadline(time.Unix(1, 0)) })
				defer func() {
					o.mu.Lock()
					if o.canceled {
						_ = d.SetDeadline(time.Time{})
					}
					o.mu.Unlock()
				}()
			}
			c, err := ln.Accept()
			if err != nil {
				return nil, 0, err
			}
			acc.Attach(c)
			return py.Int(0), 0, nil
		})
	})
	m("ConnectEx", func(o *Overlapped, args py.Tuple) (py.Object, error) {
		r, fd, err := sockOf(args, 0)
		if err != nil {
			return nil, err
		}
		if len(args) < 2 {
			return nil, py.ExceptionNewf(py.TypeError, "ConnectEx() requires an address")
		}
		addr := args[1]
		return py.None, o.start(fd, func() (py.Object, int, error) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			o.setInterrupt(stop)
			return py.Int(0), 0, r.Dial(ctx, addr)
		})
	})
	m("DisconnectEx", func(o *Overlapped, args py.Tuple) (py.Object, error) {
		r, fd, err := sockOf(args, 0)
		if err != nil {
			return nil, err
		}
		return py.None, o.start(fd, func() (py.Object, int, error) {
			if c := r.Conn(); c != nil {
				return py.Int(0), 0, c.Close()
			}
			return py.Int(0), 0, nil
		})
	})
	from := func(into bool) func(o *Overlapped, args py.Tuple) (py.Object, error) {
		return func(o *Overlapped, args py.Tuple) (py.Object, error) {
			r, fd, err := sockOf(args, 0)
			if err != nil {
				return nil, err
			}
			var buf []byte
			if into {
				buf, _, err = bufArg(args, 1)
				if err != nil {
					return nil, err
				}
			} else {
				n, err := intArg(args, 1)
				if err != nil {
					return nil, err
				}
				buf = make([]byte, n)
			}
			pc := r.Packet()
			if pc == nil {
				return nil, py.WinOSError(wsaeNotSock, winMsg(wsaeNotSock))
			}
			return py.None, o.start(fd, func() (py.Object, int, error) {
				o.setInterrupt(func() { _ = pc.SetReadDeadline(time.Unix(1, 0)) })
				n, a, err := pc.ReadFrom(buf)
				o.mu.Lock()
				if o.canceled {
					_ = pc.SetReadDeadline(time.Time{})
				}
				o.mu.Unlock()
				if err != nil {
					return nil, 0, err
				}
				at, perr := r.AddrTuple(a)
				if perr != nil {
					return nil, 0, perr
				}
				var first py.Object = py.Int(n)
				if !into {
					first = py.Bytes(append([]byte(nil), buf[:n]...))
				}
				return py.Tuple{first, at}, n, nil
			})
		}
	}
	m("WSARecvFrom", from(false))
	m("WSARecvFromInto", from(true))
	m("WSASendTo", func(o *Overlapped, args py.Tuple) (py.Object, error) {
		r, fd, err := sockOf(args, 0)
		if err != nil {
			return nil, err
		}
		b, _, err := bufArg(args, 1)
		if err != nil {
			return nil, err
		}
		if len(args) < 4 {
			return nil, py.ExceptionNewf(py.TypeError, "WSASendTo() requires an address")
		}
		pc := r.Packet()
		if pc == nil {
			return nil, py.WinOSError(wsaeNotSock, winMsg(wsaeNotSock))
		}
		to, err := r.ResolveUDP(args[3])
		if err != nil {
			return nil, socket.WrapErr(err)
		}
		b = append([]byte(nil), b...)
		return py.None, o.start(fd, func() (py.Object, int, error) {
			n, err := pc.WriteTo(b, to)
			return py.Int(n), n, err
		})
	})
	for _, name := range []string{"ConnectNamedPipe", "TransmitFile"} {
		name := name
		m(name, func(o *Overlapped, args py.Tuple) (py.Object, error) { return nil, notImpl(name) })
	}
	m("cancel", func(o *Overlapped, args py.Tuple) (py.Object, error) {
		o.cancel()
		return py.None, nil
	})
	m("getresult", func(o *Overlapped, args py.Tuple) (py.Object, error) {
		wait := false
		if len(args) > 0 {
			var err error
			if wait, err = py.ObjectIsTrue(args[0]); err != nil {
				return nil, err
			}
		}
		return o.getresult(wait)
	})
	prop := func(name string, get func(o *Overlapped) py.Object) {
		OverlappedType.Dict.Set(name, &py.Property{Fget: func(self py.Object) (py.Object, error) { return get(self.(*Overlapped)), nil }})
	}
	prop("address", func(o *Overlapped) py.Object { return py.Int(o.id) })
	prop("event", func(o *Overlapped) py.Object { return o.event })
	prop("pending", func(o *Overlapped) py.Object {
		o.mu.Lock()
		defer o.mu.Unlock()
		return py.NewBool(o.pending)
	})

	fn := func(name string, f func(args py.Tuple) (py.Object, error)) py.DictEntry {
		return py.DictEntry{Key: name, Value: py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) { return f(args) }, 0, name)}
	}
	unsupported := func(name string) py.DictEntry {
		return fn(name, func(py.Tuple) (py.Object, error) { return nil, notImpl(name) })
	}
	c := func(name string, v int) py.DictEntry { return py.DictEntry{Key: name, Value: py.Int(v)} }
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_overlapped", Doc: "Support for overlapped IO (emulated over Go sockets)"},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "Overlapped", Value: OverlappedType},
			c("INVALID_HANDLE_VALUE", -1),
			c("NULL", 0),
			c("ERROR_IO_PENDING", errorIOPending),
			c("ERROR_NETNAME_DELETED", errorNetnameDeleted),
			c("ERROR_OPERATION_ABORTED", errorOperationAbort),
			c("ERROR_PIPE_BUSY", errorPipeBusy),
			c("ERROR_PORT_UNREACHABLE", errorPortUnreachable),
			c("ERROR_SEM_TIMEOUT", errorSemTimeout),
			c("SO_UPDATE_ACCEPT_CONTEXT", 0x700b),
			c("SO_UPDATE_CONNECT_CONTEXT", 0x7010),
			fn("CreateIoCompletionPort", func(args py.Tuple) (py.Object, error) {
				if len(args) != 4 {
					return nil, py.ExceptionNewf(py.TypeError, "CreateIoCompletionPort() takes 4 arguments")
				}
				handle, err := py.IndexInt(args[0])
				if err != nil {
					return nil, err
				}
				existing, err := py.IndexInt(args[1])
				if err != nil {
					return nil, err
				}
				key, err := py.IndexInt(args[2])
				if err != nil {
					return nil, err
				}
				mu.Lock()
				defer mu.Unlock()
				if existing == 0 {
					id := portBase + int(nextID.Add(1))
					p := &port{q: make(chan status, 4096)}
					ports[id] = p
					if handle != -1 {
						assoc[sockKey(handle)] = assocEntry{p, key}
					}
					return py.Int(id), nil
				}
				p, ok := ports[existing]
				if !ok {
					return nil, py.WinOSError(errorInvalidHandle, winMsg(errorInvalidHandle))
				}
				assoc[sockKey(handle)] = assocEntry{p, key}
				return py.Int(existing), nil
			}),
			fn("GetQueuedCompletionStatus", func(args py.Tuple) (py.Object, error) {
				h, err := intArg(args, 0)
				if err != nil {
					return nil, err
				}
				if len(args) < 2 {
					return nil, py.ExceptionNewf(py.TypeError, "GetQueuedCompletionStatus() takes 2 arguments")
				}
				ms, err := py.MakeGoInt64(args[1])
				if err != nil {
					return nil, err
				}
				mu.Lock()
				p, ok := ports[h]
				mu.Unlock()
				if !ok {
					return nil, py.WinOSError(errorInvalidHandle, winMsg(errorInvalidHandle))
				}
				var st status
				select {
				case st = <-p.q:
				default:
					if ms == 0 {
						return py.None, nil
					}
					var timeout <-chan time.Time
					if uint32(ms) != infinite {
						timeout = time.After(time.Duration(ms) * time.Millisecond)
					}
					select {
					case st = <-p.q:
					case <-timeout:
						return py.None, nil
					}
				}
				return py.Tuple{py.Int(st.err), py.Int(st.transferred), py.Int(st.key), py.Int(st.addr)}, nil
			}),
			fn("PostQueuedCompletionStatus", func(args py.Tuple) (py.Object, error) {
				v := [4]int{}
				for i := range v {
					var err error
					if v[i], err = intArg(args, i); err != nil {
						return nil, err
					}
				}
				mu.Lock()
				p, ok := ports[v[0]]
				mu.Unlock()
				if !ok {
					return nil, py.WinOSError(errorInvalidHandle, winMsg(errorInvalidHandle))
				}
				p.q <- status{0, v[1], v[2], v[3]}
				return py.None, nil
			}),
			fn("BindLocal", func(args py.Tuple) (py.Object, error) { return py.None, nil }),
			fn("WSAConnect", func(args py.Tuple) (py.Object, error) {
				r, _, err := sockOf(args, 0)
				if err != nil {
					return nil, err
				}
				if len(args) < 2 {
					return nil, py.ExceptionNewf(py.TypeError, "WSAConnect() requires an address")
				}
				return py.None, r.Connect(args[1])
			}),
			fn("FormatMessage", func(args py.Tuple) (py.Object, error) {
				code, err := intArg(args, 0)
				if err != nil {
					return nil, err
				}
				return py.String(winMsg(code)), nil
			}),
			unsupported("CreateEvent"),
			unsupported("SetEvent"),
			unsupported("ResetEvent"),
			unsupported("RegisterWaitWithQueue"),
			unsupported("UnregisterWait"),
			unsupported("UnregisterWaitEx"),
			unsupported("ConnectPipe"),
		),
	})
}
