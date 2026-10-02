package socket

// Registration of the socket class's methods and of the small file object
// makefile returns.  The module globals and the module level functions are
// registered in socket.go.
//
// The receiver of a method arrives in self, not as the first element of args,
// so every argument index below is relative to the Python signature.

import (
	"time"

	"github.com/vishnukv64/gpython/py"
)

// selfSock extracts the receiver of a method call.
func selfSock(self py.Object, name string) (*sock, error) {
	s, ok := self.(*sock)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "descriptor '%s' requires a 'socket' object", name)
	}
	return s, nil
}

// argc checks the argument count with CPython's wording.
func argc(args py.Tuple, name string, min, max int) error {
	n := len(args)
	if min == max {
		if n != min {
			return py.ExceptionNewf(py.TypeError,
				"%s() takes exactly %d argument%s (%d given)", name, min, plural(min), n)
		}
		return nil
	}
	if n < min {
		return py.ExceptionNewf(py.TypeError,
			"%s() takes at least %d argument%s (%d given)", name, min, plural(min), n)
	}
	if n > max {
		return py.ExceptionNewf(py.TypeError,
			"%s() takes at most %d argument%s (%d given)", name, max, plural(max), n)
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// optIntKw reads an integer keyword, or returns the default.
func optIntKw(kwargs py.StringDict, name string, def int) (int, error) {
	v, ok := kwargs.Get(name)
	if !ok || v == py.None {
		return def, nil
	}
	n, ok := asInt(v)
	if !ok {
		return 0, py.ExceptionNewf(py.TypeError, "'%s' must be an integer, not %s", name, v.Type().Name)
	}
	return n, nil
}

// bytesArg checks that an argument is a bytes-like object.
func bytesArg(o py.Object, name string) (py.Bytes, error) {
	b, ok := o.(py.Bytes)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "%s() argument must be a bytes-like object, not %s", name, o.Type().Name)
	}
	return b, nil
}

func registerSocketMethods() {
	methods := []*py.Method{}

	// connect / bind / listen / accept.
	methods = append(methods,
		py.MustNewMethod("connect", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "connect")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "connect", 1, 1); err != nil {
				return nil, err
			}
			return s.connect(args[0])
		}, 0, "Connect to a remote socket at address."),
		py.MustNewMethod("bind", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "bind")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "bind", 1, 1); err != nil {
				return nil, err
			}
			return s.bind(args[0])
		}, 0, "Bind the socket to address."),
		py.MustNewMethod("listen", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			s, err := selfSock(self, "listen")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "listen", 0, 1); err != nil {
				return nil, err
			}
			backlog := 128
			if len(args) == 1 {
				if n, ok := asInt(args[0]); ok {
					backlog = n
				}
			} else {
				if backlog, err = optIntKw(kwargs, "backlog", 128); err != nil {
					return nil, err
				}
			}
			return s.listen(backlog)
		}, 0, "Enable a server to accept connections."),
		py.MustNewMethod("accept", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "accept")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "accept", 0, 0); err != nil {
				return nil, err
			}
			return s.accept()
		}, 0, "Accept a connection, returning a socket and an address."),
	)

	// send / sendall / recv / recvfrom / sendto.
	methods = append(methods,
		py.MustNewMethod("send", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "send")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "send", 1, 2); err != nil {
				return nil, err
			}
			if _, err := bytesArg(args[0], "send"); err != nil {
				return nil, err
			}
			return s.send(args[0])
		}, 0, "Send data to the socket."),
		py.MustNewMethod("sendall", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "sendall")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "sendall", 1, 2); err != nil {
				return nil, err
			}
			if _, err := bytesArg(args[0], "sendall"); err != nil {
				return nil, err
			}
			// Go's Write already sends everything, so sendall is send.
			if _, err := s.send(args[0]); err != nil {
				return nil, err
			}
			return py.None, nil
		}, 0, "Send data to the socket, and return when all data has been sent."),
		py.MustNewMethod("recv", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "recv")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "recv", 1, 2); err != nil {
				return nil, err
			}
			n, ok := asInt(args[0])
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "recv() argument must be int, not %s", args[0].Type().Name)
			}
			return s.recv(n)
		}, 0, "Receive data from the socket."),
		py.MustNewMethod("recvfrom", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "recvfrom")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "recvfrom", 1, 2); err != nil {
				return nil, err
			}
			n, ok := asInt(args[0])
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "recvfrom() argument must be int, not %s", args[0].Type().Name)
			}
			return s.recvfrom(n)
		}, 0, "Receive data from the socket, returning the data and the address."),
		py.MustNewMethod("sendto", func(self py.Object, args py.Tuple) (py.Object, error) {
			// Both sendto(data, address) and the older
			// sendto(data, flags, address) are accepted.  The flags are
			// ignored because Go's net has no equivalent.
			s, err := selfSock(self, "sendto")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "sendto", 2, 3); err != nil {
				return nil, err
			}
			if _, err := bytesArg(args[0], "sendto"); err != nil {
				return nil, err
			}
			return s.sendto(args[0], args[len(args)-1])
		}, 0, "Send data to the socket, to the given address."),
	)

	// close / shutdown / the name and descriptor accessors.
	methods = append(methods,
		py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "close")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "close", 0, 0); err != nil {
				return nil, err
			}
			return s.close()
		}, 0, "Close the socket."),
		py.MustNewMethod("shutdown", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "shutdown")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "shutdown", 1, 1); err != nil {
				return nil, err
			}
			how, ok := asInt(args[0])
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "shutdown() argument must be int, not %s", args[0].Type().Name)
			}
			return s.shutdown(how)
		}, 0, "Shut down one or both halves of the connection."),
		py.MustNewMethod("getsockname", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "getsockname")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "getsockname", 0, 0); err != nil {
				return nil, err
			}
			return s.getsockname()
		}, 0, "Return the socket's own address."),
		py.MustNewMethod("getpeername", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "getpeername")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "getpeername", 0, 0); err != nil {
				return nil, err
			}
			return s.getpeername()
		}, 0, "Return the remote address to which the socket is connected."),
		py.MustNewMethod("fileno", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "fileno")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "fileno", 0, 0); err != nil {
				return nil, err
			}
			return s.fileno()
		}, 0, "Return the socket's file descriptor."),
	)

	// Timeouts and blocking mode.
	methods = append(methods,
		py.MustNewMethod("settimeout", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "settimeout")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "settimeout", 1, 1); err != nil {
				return nil, err
			}
			switch v := args[0].(type) {
			case py.NoneType:
				s.timeout = py.None
			case py.Int:
				if int64(v) < 0 {
					return nil, py.ExceptionNewf(py.ValueError, "Timeout value out of range")
				}
				s.timeout = v
			case py.Float:
				if float64(v) < 0 {
					return nil, py.ExceptionNewf(py.ValueError, "Timeout value out of range")
				}
				s.timeout = v
			default:
				return nil, py.ExceptionNewf(py.TypeError,
					"'%s' object cannot be interpreted as an integer", args[0].Type().Name)
			}
			return py.None, nil
		}, 0, "Set a timeout on blocking socket operations."),
		py.MustNewMethod("gettimeout", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "gettimeout")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "gettimeout", 0, 0); err != nil {
				return nil, err
			}
			return s.timeout, nil
		}, 0, "Return the timeout in seconds or None."),
		py.MustNewMethod("setblocking", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "setblocking")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "setblocking", 1, 1); err != nil {
				return nil, err
			}
			var block bool
			switch v := args[0].(type) {
			case py.Bool:
				block = bool(v)
			case py.Int:
				block = v != 0
			default:
				return nil, py.ExceptionNewf(py.TypeError, "setblocking() argument must be bool, not %s", args[0].Type().Name)
			}
			if block {
				// Blocking mode is the absence of a deadline.
				s.timeout = py.None
			} else {
				s.timeout = py.Float(0)
			}
			return py.None, nil
		}, 0, "Set the socket to blocking or non-blocking mode."),
	)

	// Socket options.  Only the options Go's net can express are accepted;
	// anything else is reported rather than silently ignored, which is what
	// makes a missing option findable.
	methods = append(methods,
		py.MustNewMethod("setsockopt", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			s, err := selfSock(self, "setsockopt")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "setsockopt", 2, 3); err != nil {
				return nil, err
			}
			level, lOk := asInt(args[0])
			optname, oOk := asInt(args[1])
			if !lOk || !oOk {
				return nil, py.ExceptionNewf(py.TypeError, "setsockopt() level and option must be integers")
			}
			value := 0
			if len(args) == 3 {
				v, ok := asInt(args[2])
				if !ok {
					return nil, py.ExceptionNewf(py.TypeError, "setsockopt() value must be an integer")
				}
				value = v
			}
			switch {
			case level == SOL_SOCKET && optname == SO_REUSEADDR:
				s.opts[SO_REUSEADDR] = value
				return py.None, nil
			case level == SOL_SOCKET && (optname == SO_REUSEPORT || optname == SO_KEEPALIVE ||
				optname == SO_BROADCAST || optname == SO_SNDBUF || optname == SO_RCVBUF):
				s.opts[optname] = value
				return py.None, nil
			}
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"setsockopt(level=%d, optname=%d) is not supported by gpython's socket module", level, optname)
		}, 0, "Set a socket option."),
		py.MustNewMethod("getsockopt", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			s, err := selfSock(self, "getsockopt")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "getsockopt", 2, 3); err != nil {
				return nil, err
			}
			level, lOk := asInt(args[0])
			optname, oOk := asInt(args[1])
			if !lOk || !oOk {
				return nil, py.ExceptionNewf(py.TypeError, "getsockopt() level and option must be integers")
			}
			if v, ok := s.opts[optname]; ok {
				return py.Int(v), nil
			}
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"getsockopt(level=%d, optname=%d) is not supported by gpython's socket module", level, optname)
		}, 0, "Get a socket option."),
	)

	// makefile.
	methods = append(methods,
		py.MustNewMethod("makefile", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			s, err := selfSock(self, "makefile")
			if err != nil {
				return nil, err
			}
			if err := argc(args, "makefile", 0, 1); err != nil {
				return nil, err
			}
			mode := "r"
			if len(args) == 1 {
				m, ok := args[0].(py.String)
				if !ok {
					return nil, py.ExceptionNewf(py.TypeError, "makefile() mode must be str, not %s", args[0].Type().Name)
				}
				mode = string(m)
			} else if v, ok := kwargs.Get("mode"); ok {
				m, ok := v.(py.String)
				if !ok {
					return nil, py.ExceptionNewf(py.TypeError, "makefile() mode must be str, not %s", v.Type().Name)
				}
				mode = string(m)
			}
			if s.conn == nil {
				return nil, py.ExceptionNewf(py.NotImplementedError,
					"makefile() requires a connected stream socket in gpython")
			}
			if len(mode) > 0 {
				switch mode[0] {
				case 'r', 'w', 'a':
				default:
					return nil, py.ExceptionNewf(py.ValueError, "invalid mode: '%s'", mode)
				}
			}
			f := &socketFile{conn: s.conn}
			if containsByte(mode, 'r') || containsByte(mode, '+') {
				f.rd = &bufReader{r: s.conn}
			}
			if containsByte(mode, 'w') || containsByte(mode, 'a') || containsByte(mode, '+') {
				f.w = s.conn
			}
			return f, nil
		}, 0, "Return a file object associated with the socket."),
	)

	methods = append(methods,
		py.MustNewMethod("__repr__", socketRepr, 0, ""),
		py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
			return self, nil
		}, 0, ""),
		py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
			s, err := selfSock(self, "__exit__")
			if err != nil {
				return nil, err
			}
			if _, err := s.close(); err != nil {
				return nil, err
			}
			return py.False, nil
		}, 0, ""),
	)

	d := &SocketType.Dict
	for _, m := range methods {
		d.Set(m.Name, m)
	}

	registerSocketFile()
}

// registerSocketFile wires the small file object makefile returns.
func registerSocketFile() {
	selfFile := func(self py.Object, name string) (*socketFile, error) {
		f, ok := self.(*socketFile)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "descriptor '%s' requires a socket file", name)
		}
		return f, nil
	}

	SocketFileType.Dict.Set("read", py.MustNewMethod("read", func(self py.Object, args py.Tuple) (py.Object, error) {
		f, err := selfFile(self, "read")
		if err != nil {
			return nil, err
		}
		if f.rd == nil {
			return nil, py.ExceptionNewf(py.ValueError, "I/O operation on unreadable socket file")
		}
		if err := argc(args, "read", 0, 1); err != nil {
			return nil, err
		}
		if len(args) == 0 {
			b, err := f.rd.readAll()
			if err != nil {
				return nil, osErr(err)
			}
			return py.Bytes(b), nil
		}
		n, ok := asInt(args[0])
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "read() argument must be int, not %s", args[0].Type().Name)
		}
		if n < 0 {
			b, err := f.rd.readAll()
			if err != nil {
				return nil, osErr(err)
			}
			return py.Bytes(b), nil
		}
		b, err := f.rd.read(n)
		if err != nil {
			return nil, osErr(err)
		}
		return py.Bytes(b), nil
	}, 0, "Read up to n bytes from the socket."))

	SocketFileType.Dict.Set("readline", py.MustNewMethod("readline", func(self py.Object, args py.Tuple) (py.Object, error) {
		f, err := selfFile(self, "readline")
		if err != nil {
			return nil, err
		}
		if f.rd == nil {
			return nil, py.ExceptionNewf(py.ValueError, "I/O operation on unreadable socket file")
		}
		if err := argc(args, "readline", 0, 0); err != nil {
			return nil, err
		}
		b, err := f.rd.readline()
		if err != nil {
			return nil, osErr(err)
		}
		return py.Bytes(b), nil
	}, 0, "Read one line from the socket."))

	SocketFileType.Dict.Set("write", py.MustNewMethod("write", func(self py.Object, args py.Tuple) (py.Object, error) {
		f, err := selfFile(self, "write")
		if err != nil {
			return nil, err
		}
		if f.w == nil {
			return nil, py.ExceptionNewf(py.ValueError, "I/O operation on unwritable socket file")
		}
		if err := argc(args, "write", 1, 1); err != nil {
			return nil, err
		}
		b, ok := args[0].(py.Bytes)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not %s", args[0].Type().Name)
		}
		n, err := f.w.Write([]byte(b))
		if err != nil {
			return nil, osErr(err)
		}
		return py.Int(n), nil
	}, 0, "Write bytes to the socket."))

	SocketFileType.Dict.Set("flush", py.MustNewMethod("flush", func(self py.Object, args py.Tuple) (py.Object, error) {
		// Socket writes are unbuffered, so there is nothing to flush.
		return py.None, nil
	}, 0, "Flush the write buffer; this is a no-op for a socket."))

	SocketFileType.Dict.Set("close", py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
		// Closing the file must not close the socket, as CPython documents
		// for socket.makefile.
		f, err := selfFile(self, "close")
		if err != nil {
			return nil, err
		}
		f.w, f.rd = nil, nil
		return py.None, nil
	}, 0, "Close the file object without closing the socket."))

	SocketFileType.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, ""))
	SocketFileType.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		f, err := selfFile(self, "__exit__")
		if err != nil {
			return nil, err
		}
		f.w, f.rd = nil, nil
		return py.False, nil
	}, 0, ""))
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

func socketRepr(self py.Object) (py.Object, error) {
	s, ok := self.(*sock)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "__repr__ requires a socket")
	}
	state := "unconnected"
	switch {
	case s.closed:
		state = "closed"
	case s.listener != nil:
		state = "listening"
	case s.conn != nil:
		state = "connected"
	case s.packet != nil:
		state = "bound"
	}
	timeout := "None"
	switch v := s.timeout.(type) {
	case py.Int:
		timeout = (time.Duration(int64(v)) * time.Second).String()
	case py.Float:
		timeout = (time.Duration(float64(v) * float64(time.Second))).String()
	}
	return py.String("<socket.socket state=" + state + " timeout=" + timeout + ">"), nil
}
