// Package socket implements the socket module: low level networking.
//
// The implementation is backed by Go's net package.  Go's net abstraction is
// deliberately higher level than a raw socket, so this is an honest subset
// rather than a full emulation:
//
//   - Stream sockets (SOCK_STREAM) become a net.Conn from Dial, or a
//     net.Listener from Listen and its Accept.
//   - Datagram sockets (SOCK_DGRAM) become a net.PacketConn from ListenPacket,
//     or a connected net.Conn from Dial.
//   - AF_UNIX works through the same calls, because Go's net speaks it.
//   - Options Go's net does not expose (IP_TTL, SO_LINGER and the like) raise
//     NotImplementedError naming the option, rather than being accepted and
//     dropped: a silently ignored setsockopt is a debugging trap.
//
// makefile returns a small buffered reader over the connection rather than a
// real io.BufferedReader, because the interpreter has no general buffered IO
// stack to attach to.
package socket

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `This module provides access to the BSD socket interface.`

// Address families, socket types and options.  Where Go's syscall package
// exposes the value it is used, so the numbers are right on each platform
// (AF_INET6 is 10 on Linux but 30 on Darwin).  The AI_*, NI_* and MSG_*
// families are not in the standard library's syscall, so those keep the POSIX
// values; they are only ever passed back to these functions.
var (
	AF_UNSPEC = syscall.AF_UNSPEC
	AF_UNIX   = syscall.AF_UNIX
	AF_INET   = syscall.AF_INET
	AF_INET6  = syscall.AF_INET6

	SOCK_STREAM   = syscall.SOCK_STREAM
	SOCK_DGRAM    = syscall.SOCK_DGRAM
	SOCK_RAW      = syscall.SOCK_RAW
	SOCK_NONBLOCK = 0x800
	SOCK_CLOEXEC  = 0x80000

	SOL_SOCKET   = syscall.SOL_SOCKET
	SO_REUSEADDR = syscall.SO_REUSEADDR
	SO_REUSEPORT = syscall.SO_REUSEPORT
	SO_KEEPALIVE = syscall.SO_KEEPALIVE
	SO_BROADCAST = syscall.SO_BROADCAST
	SO_SNDBUF    = syscall.SO_SNDBUF
	SO_RCVBUF    = syscall.SO_RCVBUF

	SHUT_RD   = syscall.SHUT_RD
	SHUT_WR   = syscall.SHUT_WR
	SHUT_RDWR = syscall.SHUT_RDWR

	IPPROTO_TCP  = syscall.IPPROTO_TCP
	IPPROTO_UDP  = syscall.IPPROTO_UDP
	IPPROTO_IPV6 = syscall.IPPROTO_IPV6

	// TCP and IPv6 socket options.  urllib3 reads TCP_NODELAY to disable
	// Nagle's algorithm on its connections.
	TCP_NODELAY = 0x1
	IPV6_V6ONLY = 0x1b

	AI_PASSIVE     = 0x0001
	AI_CANONNAME   = 0x0002
	AI_NUMERICHOST = 0x0004
	AI_NUMERICSERV = 0x0400

	NI_NUMERICHOST = 1
	NI_NUMERICSERV = 2
	NI_NOFQDN      = 4
	NI_NAMEREQD    = 8
	NI_DGRAM       = 16

	MSG_PEEK     = 0x0002
	MSG_DONTWAIT = 0x0080

	INADDR_ANY       = 0
	INADDR_BROADCAST = 0xffffffff
)

// Exception classes.  socket.error is OSError itself in CPython, so these
// derive from it and an "except OSError" clause catches them.
var (
	SocketErrorType = py.OSError.NewType("socket.error",
		"A subclass of OSError, this exception is raised for socket-related errors.", nil, nil)
	GaierrorType = SocketErrorType.NewType("socket.gaierror",
		"A subclass of OSError, this exception is raised for address-related errors.", nil, nil)
	HerrorType = SocketErrorType.NewType("socket.herror",
		"A subclass of OSError, this exception is raised for address-related errors.", nil, nil)
)

// osErr maps a Go network error onto the exception class CPython raises.
func osErr(err error) error {
	if err == nil {
		return nil
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return py.ExceptionNewf(py.TimeoutError, "timed out")
	}
	if dnsErr, ok := err.(*net.DNSError); ok {
		return py.ExceptionNewf(GaierrorType, "%s", dnsErr.Err)
	}
	if opErr, ok := err.(*net.OpError); ok {
		if sysErr, ok := opErr.Err.(*os.SyscallError); ok {
			switch sysErr.Err {
			case syscall.ECONNREFUSED:
				return py.ExceptionNewf(py.ConnectionRefusedError, "[Errno 61] Connection refused")
			case syscall.ECONNRESET:
				return py.ExceptionNewf(py.ConnectionResetError, "[Errno 54] Connection reset by peer")
			case syscall.EPIPE:
				return py.ExceptionNewf(py.BrokenPipeError, "[Errno 32] Broken pipe")
			case syscall.EADDRINUSE:
				return py.ExceptionNewf(py.OSError, "[Errno 48] Address already in use")
			case syscall.EADDRNOTAVAIL:
				return py.ExceptionNewf(py.OSError, "[Errno 49] Can't assign requested address")
			}
		}
	}
	return py.ExceptionNewf(SocketErrorType, "%s", err.Error())
}

// asInt reads a Python integer argument, so callers do not have to unpack
// through a *py.Object and then assert.
func asInt(o py.Object) (int, bool) {
	v, ok := o.(py.Int)
	if !ok {
		return 0, false
	}
	return int(v), true
}

// ---------------------------------------------------------------------------
// The socket object
// ---------------------------------------------------------------------------

type sock struct {
	family  int
	typ     int
	proto   int
	timeout py.Object // None means blocking
	opts    map[int]int

	conn     net.Conn       // connected stream socket
	listener net.Listener   // listening stream socket
	packet   net.PacketConn // bound or connected datagram socket
	closed   bool
}

var SocketType = py.NewTypeX("socket.socket",
	"socket(family=AF_INET, type=SOCK_STREAM, proto=0, fileno=None)\n\n"+
		"A network socket, backed by Go's net package.",
	socketNew, nil)

func (s *sock) Type() *py.Type { return SocketType }

func socketNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) > 3 {
		return nil, py.ExceptionNewf(py.TypeError,
			"socket() takes at most 3 arguments (%d given)", len(args))
	}
	var famObj, typObj, protoObj py.Object
	kwlist := []string{"family", "type", "proto"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "|iii:__new__", kwlist,
		&famObj, &typObj, &protoObj); err != nil {
		return nil, err
	}

	family := AF_INET
	if famObj != nil && famObj != py.None {
		v, ok := asInt(famObj)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "family must be an integer")
		}
		family = v
	}
	typ := SOCK_STREAM
	if typObj != nil && typObj != py.None {
		v, ok := asInt(typObj)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "type must be an integer")
		}
		typ = v
	}
	proto := 0
	if protoObj != nil && protoObj != py.None {
		v, ok := asInt(protoObj)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "proto must be an integer")
		}
		proto = v
	}

	switch family {
	case AF_INET, AF_INET6, AF_UNIX, AF_UNSPEC:
	default:
		return nil, py.ExceptionNewf(py.OSError, "address family not supported")
	}
	switch typ &^ (SOCK_NONBLOCK | SOCK_CLOEXEC) {
	case SOCK_STREAM, SOCK_DGRAM:
	default:
		return nil, py.ExceptionNewf(py.OSError, "socket type not supported")
	}
	return &sock{
		family:  family,
		typ:     typ,
		proto:   proto,
		timeout: py.None,
		opts:    map[int]int{},
	}, nil
}

// network maps the socket's family onto the prefix Go's net uses.
func (s *sock) network(base string) (string, error) {
	switch s.family {
	case AF_UNIX:
		if s.typ == SOCK_DGRAM {
			return "unixgram", nil
		}
		return "unix", nil
	case AF_INET:
		if base == "udp" {
			return "udp4", nil
		}
		return "tcp4", nil
	case AF_INET6:
		if base == "udp" {
			return "udp6", nil
		}
		return "tcp6", nil
	case AF_UNSPEC:
		if base == "udp" {
			return "udp", nil
		}
		return "tcp", nil
	}
	return "", py.ExceptionNewf(py.OSError, "address family not supported")
}

// addrString turns a Python address into the string Go's net dials and listens
// on.  A tuple is (host, port); for AF_UNIX a bare str is the path.
func (s *sock) addrString(o py.Object) (string, error) {
	if path, ok := o.(py.String); ok {
		return string(path), nil
	}
	items, ok := o.(py.Tuple)
	if !ok || len(items) < 2 {
		return "", py.ExceptionNewf(py.TypeError, "AF_INET address must be tuple, not %s", o.Type().Name)
	}
	host := ""
	if h, ok := items[0].(py.String); ok {
		host = string(h)
	}
	port, err := portOf(items[1])
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, port), nil
}

// portOf renders the port of an address tuple.
func portOf(o py.Object) (string, error) {
	switch v := o.(type) {
	case py.Int:
		return strconv.FormatInt(int64(v), 10), nil
	case py.String:
		return string(v), nil
	}
	return "", py.ExceptionNewf(py.TypeError, "port must be an integer or a str, not %s", o.Type().Name)
}

// addrTuple turns a net address back into the Python form.
func (s *sock) addrTuple(a net.Addr) (py.Object, error) {
	if a == nil {
		return py.Tuple{py.String(""), py.Int(0)}, nil
	}
	switch v := a.(type) {
	case *net.TCPAddr:
		return py.Tuple{py.String(v.IP.String()), py.Int(v.Port)}, nil
	case *net.UDPAddr:
		return py.Tuple{py.String(v.IP.String()), py.Int(v.Port)}, nil
	case *net.UnixAddr:
		return py.String(v.Name), nil
	}
	host, port, err := net.SplitHostPort(a.String())
	if err != nil {
		return py.String(a.String()), nil
	}
	n, _ := strconv.Atoi(port)
	return py.Tuple{py.String(host), py.Int(n)}, nil
}

// deadline applies the stored timeout, so a socket made with settimeout()
// honours it on every blocking call.
func (s *sock) deadline() time.Time {
	var secs float64
	switch v := s.timeout.(type) {
	case py.Float:
		secs = float64(v)
	case py.Int:
		secs = float64(v)
	default:
		return time.Time{}
	}
	if secs < 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(secs * float64(time.Second)))
}

func (s *sock) checkOpen(call string) error {
	if s.closed {
		// CPython's message names only the errno, like os.write on a bad fd.
		_ = call
		return py.ExceptionNewf(py.OSError, "[Errno 9] Bad file descriptor")
	}
	return nil
}

// ---------------------------------------------------------------------------
// socket methods
// ---------------------------------------------------------------------------

func (s *sock) connect(address py.Object) (py.Object, error) {
	if err := s.checkOpen("connect"); err != nil {
		return nil, err
	}
	target, err := s.addrString(address)
	if err != nil {
		return nil, err
	}
	if s.typ == SOCK_DGRAM {
		network, err := s.network("udp")
		if err != nil {
			return nil, err
		}
		// A connected datagram socket in Python can send() and recv(); Go
		// models that as a connected PacketConn.
		dialer := net.Dialer{}
		if d := s.deadline(); !d.IsZero() {
			dialer.Deadline = d
		}
		conn, err := dialer.Dial(network, target)
		if err != nil {
			return nil, osErr(err)
		}
		if uc, ok := conn.(*net.UDPConn); ok {
			s.packet = uc
			return py.None, nil
		}
		s.conn = conn
		return py.None, nil
	}
	network, err := s.network("tcp")
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	if d := s.deadline(); !d.IsZero() {
		dialer.Deadline = d
	}
	conn, err := dialer.Dial(network, target)
	if err != nil {
		return nil, osErr(err)
	}
	s.conn = conn
	return py.None, nil
}

func (s *sock) bind(address py.Object) (py.Object, error) {
	if err := s.checkOpen("bind"); err != nil {
		return nil, err
	}
	target, err := s.addrString(address)
	if err != nil {
		return nil, err
	}
	if s.typ == SOCK_DGRAM {
		network, err := s.network("udp")
		if err != nil {
			return nil, err
		}
		pc, err := net.ListenPacket(network, target)
		if err != nil {
			return nil, osErr(err)
		}
		s.packet = pc
		return py.None, nil
	}
	// For a stream socket the bind is realised as a listener, because Go's
	// net has no bare listening socket: that is what makes getsockname and
	// accept work on the usual bind/listen/accept sequence.  listen() then
	// only records the backlog.
	network, err := s.network("tcp")
	if err != nil {
		return nil, err
	}
	l, err := listenOn(network, target, s.opts)
	if err != nil {
		return nil, osErr(err)
	}
	s.listener = l
	return py.None, nil
}

// listenOn creates the listener, honouring SO_REUSEADDR where Go exposes it.
func listenOn(network, target string, opts map[int]int) (net.Listener, error) {
	if v, ok := opts[SO_REUSEADDR]; !ok || v == 0 {
		return net.Listen(network, target)
	}
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var serr error
			if err := c.Control(func(fd uintptr) {
				serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			}); err != nil {
				return err
			}
			return serr
		},
	}
	return lc.Listen(context.Background(), network, target)
}

func (s *sock) listen(backlog int) (py.Object, error) {
	if err := s.checkOpen("listen"); err != nil {
		return nil, err
	}
	if s.listener == nil {
		return nil, py.ExceptionNewf(py.OSError, "socket is not bound")
	}
	// Go's net does not expose the accept backlog; the listener is already
	// accepting, so only the intent is recorded.
	return py.None, nil
}

func (s *sock) accept() (py.Object, error) {
	if err := s.checkOpen("accept"); err != nil {
		return nil, err
	}
	if s.listener == nil {
		return nil, py.ExceptionNewf(py.OSError, "socket is not listening")
	}
	if d := s.deadline(); !d.IsZero() {
		if tl, ok := s.listener.(*net.TCPListener); ok {
			_ = tl.SetDeadline(d)
		}
	}
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, osErr(err)
	}
	child := &sock{
		family:  s.family,
		typ:     s.typ,
		proto:   s.proto,
		timeout: s.timeout,
		opts:    map[int]int{},
		conn:    conn,
	}
	addr, err := s.addrTuple(conn.RemoteAddr())
	if err != nil {
		return nil, err
	}
	return py.Tuple{child, addr}, nil
}

func (s *sock) send(data py.Object) (py.Object, error) {
	if err := s.checkOpen("send"); err != nil {
		return nil, err
	}
	b, ok := data.(py.Bytes)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not %s", data.Type().Name)
	}
	if s.conn == nil {
		if s.packet == nil {
			return nil, py.ExceptionNewf(py.OSError, "socket is not connected")
		}
		n, err := s.packet.WriteTo([]byte(b), s.packet.LocalAddr())
		if err != nil {
			return nil, osErr(err)
		}
		return py.Int(n), nil
	}
	if d := s.deadline(); !d.IsZero() {
		_ = s.conn.SetWriteDeadline(d)
	}
	n, err := s.conn.Write([]byte(b))
	if err != nil {
		return nil, osErr(err)
	}
	return py.Int(n), nil
}

func (s *sock) recv(bufsize int) (py.Object, error) {
	if err := s.checkOpen("recv"); err != nil {
		return nil, err
	}
	if s.conn == nil {
		return nil, py.ExceptionNewf(py.OSError, "socket is not connected")
	}
	buf := make([]byte, bufsize)
	if d := s.deadline(); !d.IsZero() {
		_ = s.conn.SetReadDeadline(d)
	}
	n, err := s.conn.Read(buf)
	if n > 0 {
		return py.Bytes(buf[:n]), nil
	}
	if err != nil {
		if err == io.EOF {
			// An orderly shutdown reads as an empty bytes object.
			return py.Bytes(nil), nil
		}
		return nil, osErr(err)
	}
	return py.Bytes(nil), nil
}

func (s *sock) recvfrom(bufsize int) (py.Object, error) {
	if err := s.checkOpen("recvfrom"); err != nil {
		return nil, err
	}
	buf := make([]byte, bufsize)
	if s.packet != nil {
		if d := s.deadline(); !d.IsZero() {
			_ = s.packet.SetReadDeadline(d)
		}
		n, addr, err := s.packet.ReadFrom(buf)
		if err != nil {
			return nil, osErr(err)
		}
		a, err := s.addrTuple(addr)
		if err != nil {
			return nil, err
		}
		return py.Tuple{py.Bytes(buf[:n]), a}, nil
	}
	if s.conn == nil {
		return nil, py.ExceptionNewf(py.OSError, "socket is not connected")
	}
	if d := s.deadline(); !d.IsZero() {
		_ = s.conn.SetReadDeadline(d)
	}
	n, err := s.conn.Read(buf)
	if err != nil && n == 0 {
		return nil, osErr(err)
	}
	a, err := s.addrTuple(s.conn.RemoteAddr())
	if err != nil {
		return nil, err
	}
	return py.Tuple{py.Bytes(buf[:n]), a}, nil
}

func (s *sock) sendto(data py.Object, address py.Object) (py.Object, error) {
	if err := s.checkOpen("sendto"); err != nil {
		return nil, err
	}
	b, ok := data.(py.Bytes)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not %s", data.Type().Name)
	}
	target, err := s.addrString(address)
	if err != nil {
		return nil, err
	}
	if s.packet != nil {
		network := s.packet.LocalAddr().Network()
		var addr net.Addr
		if strings.HasPrefix(network, "udp") {
			a, err := net.ResolveUDPAddr(network, target)
			if err != nil {
				return nil, osErr(err)
			}
			addr = a
		} else {
			a, err := net.ResolveUnixAddr(network, target)
			if err != nil {
				return nil, osErr(err)
			}
			addr = a
		}
		n, err := s.packet.WriteTo([]byte(b), addr)
		if err != nil {
			return nil, osErr(err)
		}
		return py.Int(n), nil
	}
	if s.conn == nil {
		return nil, py.ExceptionNewf(py.OSError, "socket is not connected")
	}
	n, err := s.conn.Write([]byte(b))
	if err != nil {
		return nil, osErr(err)
	}
	return py.Int(n), nil
}

func (s *sock) close() (py.Object, error) {
	if s.closed {
		return py.None, nil
	}
	s.closed = true
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.packet != nil {
		_ = s.packet.Close()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	return py.None, nil
}

func (s *sock) shutdown(how int) (py.Object, error) {
	if err := s.checkOpen("shutdown"); err != nil {
		return nil, err
	}
	if tc, ok := s.conn.(*net.TCPConn); ok {
		var err error
		switch how {
		case SHUT_RD:
			err = tc.CloseRead()
		case SHUT_WR:
			err = tc.CloseWrite()
		default:
			err = tc.Close()
		}
		if err != nil {
			return nil, osErr(err)
		}
		return py.None, nil
	}
	if uc, ok := s.conn.(*net.UnixConn); ok {
		if err := uc.CloseWrite(); err != nil {
			return nil, osErr(err)
		}
		return py.None, nil
	}
	if s.packet != nil {
		return nil, py.ExceptionNewf(py.OSError, "shutdown is not supported on a datagram socket")
	}
	return nil, py.ExceptionNewf(py.OSError, "socket is not connected")
}

func (s *sock) getsockname() (py.Object, error) {
	if s.listener != nil {
		return s.addrTuple(s.listener.Addr())
	}
	if s.packet != nil {
		return s.addrTuple(s.packet.LocalAddr())
	}
	if s.conn != nil {
		return s.addrTuple(s.conn.LocalAddr())
	}
	return s.addrTuple(nil)
}

func (s *sock) getpeername() (py.Object, error) {
	if s.conn != nil {
		return s.addrTuple(s.conn.RemoteAddr())
	}
	return nil, py.ExceptionNewf(py.OSError, "[Errno 57] Socket is not connected")
}

// fileno returns the underlying descriptor without disturbing the socket.  The
// SyscallConn path is used rather than File(), which would hand the descriptor
// to the caller and switch the socket to blocking mode.
func (s *sock) fileno() (py.Object, error) {
	var c syscall.Conn
	switch {
	case s.conn != nil:
		c, _ = s.conn.(syscall.Conn)
	case s.listener != nil:
		if tl, ok := s.listener.(*net.TCPListener); ok {
			c = tl
		}
	case s.packet != nil:
		if uc, ok := s.packet.(*net.UDPConn); ok {
			c = uc
		}
	}
	if c == nil {
		return nil, py.ExceptionNewf(py.OSError, "fileno is not available for this socket")
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return nil, osErr(err)
	}
	var fd uintptr
	if err := raw.Control(func(f uintptr) { fd = f }); err != nil {
		return nil, osErr(err)
	}
	return py.Int(int64(fd)), nil
}

// ---------------------------------------------------------------------------
// socket files
//
// makefile is backed by this small wrapper.  It is not io.BufferedReader, but
// it does read the connection and respects the buffering mode.
// ---------------------------------------------------------------------------

type socketFile struct {
	conn net.Conn
	r    io.Reader
	w    io.Writer
	rd   *bufReader
}

var SocketFileType = py.NewTypeX("socket.SocketIO",
	"A file-like object that reads a socket.", nil, nil)

func (f *socketFile) Type() *py.Type { return SocketFileType }

// bufReader is a one-byte lookahead buffer over the connection, enough for
// readline.
type bufReader struct {
	r   io.Reader
	buf []byte
}

func (r *bufReader) readByte() (byte, error) {
	if len(r.buf) > 0 {
		b := r.buf[0]
		r.buf = r.buf[1:]
		return b, nil
	}
	one := make([]byte, 1)
	n, err := r.r.Read(one)
	if n == 0 {
		if err == nil {
			err = io.EOF
		}
		return 0, err
	}
	return one[0], nil
}

func (r *bufReader) read(n int) ([]byte, error) {
	out := make([]byte, 0, n)
	for len(out) < n {
		b, err := r.readByte()
		if err != nil {
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *bufReader) readAll() ([]byte, error) {
	var out []byte
	for {
		b, err := r.readByte()
		if err != nil {
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		out = append(out, b)
	}
}

func (r *bufReader) readline() ([]byte, error) {
	var out []byte
	for {
		b, err := r.readByte()
		if err != nil {
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		out = append(out, b)
		if b == '\n' {
			return out, nil
		}
	}
}

// ---------------------------------------------------------------------------
// module level functions
// ---------------------------------------------------------------------------

func gethostnameMethod(self py.Object, args py.Tuple) (py.Object, error) {
	name, err := os.Hostname()
	if err != nil {
		return nil, osErr(err)
	}
	return py.String(name), nil
}

func gethostbynameMethod(self py.Object, args py.Tuple) (py.Object, error) {
	var host py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "gethostbyname", 1, 1, &host); err != nil {
		return nil, err
	}
	h, ok := host.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "gethostbyname() argument 1 must be str, not %s", host.Type().Name)
	}
	ips, err := net.LookupIP(string(h))
	if err != nil {
		return nil, osErr(err)
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return py.String(v4.String()), nil
		}
	}
	if len(ips) > 0 {
		return py.String(ips[0].String()), nil
	}
	return nil, py.ExceptionNewf(GaierrorType, "no address associated with hostname")
}

func gethostbyaddrMethod(self py.Object, args py.Tuple) (py.Object, error) {
	var addr py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "gethostbyaddr", 1, 1, &addr); err != nil {
		return nil, err
	}
	a, ok := addr.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "gethostbyaddr() argument 1 must be str, not %s", addr.Type().Name)
	}
	names, err := net.LookupAddr(string(a))
	if err != nil {
		return nil, osErr(err)
	}
	if len(names) == 0 {
		return nil, py.ExceptionNewf(HerrorType, "unknown host")
	}
	aliases := make([]py.Object, 0, len(names)-1)
	for _, n := range names[1:] {
		aliases = append(aliases, py.String(strings.TrimSuffix(n, ".")))
	}
	return py.Tuple{
		py.String(strings.TrimSuffix(names[0], ".")),
		py.NewListFromItems(aliases),
		py.NewListFromItems([]py.Object{py.String(string(a))}),
	}, nil
}

// inetAtonMethod is socket.inet_aton: a dotted quad as four bytes.
func inetAtonMethod(self py.Object, args py.Tuple) (py.Object, error) {
	var addr py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "inet_aton", 1, 1, &addr); err != nil {
		return nil, err
	}
	a, ok := addr.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "inet_aton() argument 1 must be str, not %s", addr.Type().Name)
	}
	ip := net.ParseIP(string(a))
	if ip == nil {
		return nil, py.ExceptionNewf(py.OSError, "illegal IP address string passed to inet_aton")
	}
	v4 := ip.To4()
	if v4 == nil {
		return nil, py.ExceptionNewf(py.OSError, "illegal IP address string passed to inet_aton")
	}
	return py.Bytes(v4), nil
}

func inetNtoaMethod(self py.Object, args py.Tuple) (py.Object, error) {
	var packed py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "inet_ntoa", 1, 1, &packed); err != nil {
		return nil, err
	}
	b, ok := packed.(py.Bytes)
	if !ok || len(b) != 4 {
		return nil, py.ExceptionNewf(py.OSError, "packed IP wrong length for inet_ntoa")
	}
	return py.String(net.IP([]byte(b)).String()), nil
}

// inetPtonMethod is socket.inet_pton(family, ip): the packed bytes of an
// address in the given family.
func inetPtonMethod(self py.Object, args py.Tuple) (py.Object, error) {
	var famObj, addr py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "inet_pton", 2, 2, &famObj, &addr); err != nil {
		return nil, err
	}
	family, ok := asInt(famObj)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "inet_pton() argument 1 must be int, not %s", famObj.Type().Name)
	}
	a, ok := addr.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "inet_pton() argument 2 must be str, not %s", addr.Type().Name)
	}
	ip := net.ParseIP(string(a))
	if ip == nil {
		return nil, py.ExceptionNewf(py.OSError, "illegal IP address string passed to inet_pton")
	}
	switch family {
	case AF_INET:
		v4 := ip.To4()
		if v4 == nil {
			return nil, py.ExceptionNewf(py.OSError, "illegal IP address string passed to inet_pton")
		}
		return py.Bytes(v4), nil
	case AF_INET6:
		if strings.Contains(string(a), ":") {
			return py.Bytes(ip.To16()), nil
		}
		return nil, py.ExceptionNewf(py.OSError, "illegal IP address string passed to inet_pton")
	}
	return nil, py.ExceptionNewf(py.OSError, "unknown address family")
}

// inetNtopMethod is socket.inet_ntop(family, packed).
func inetNtopMethod(self py.Object, args py.Tuple) (py.Object, error) {
	var famObj, packed py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "inet_ntop", 2, 2, &famObj, &packed); err != nil {
		return nil, err
	}
	family, ok := asInt(famObj)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "inet_ntop() argument 1 must be int, not %s", famObj.Type().Name)
	}
	b, ok := packed.(py.Bytes)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "inet_ntop() argument 2 must be bytes, not %s", packed.Type().Name)
	}
	switch family {
	case AF_INET:
		if len(b) != 4 {
			return nil, py.ExceptionNewf(py.ValueError, "invalid length of packed IP address string")
		}
	case AF_INET6:
		if len(b) != 16 {
			return nil, py.ExceptionNewf(py.ValueError, "invalid length of packed IP address string")
		}
	default:
		return nil, py.ExceptionNewf(py.OSError, "unknown address family")
	}
	return py.String(net.IP([]byte(b)).String()), nil
}

// getaddrinfoMethod resolves host/port into the Python 5-tuple list.
func getaddrinfoMethod(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var hostArg, portArg, famObj, typObj, protoObj, flagsObj py.Object
	kwlist := []string{"host", "port", "family", "type", "proto", "flags"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|iiii:getaddrinfo", kwlist,
		&hostArg, &portArg, &famObj, &typObj, &protoObj, &flagsObj); err != nil {
		return nil, err
	}

	host := ""
	switch v := hostArg.(type) {
	case py.String:
		host = string(v)
	case py.NoneType:
		host = ""
	default:
		return nil, py.ExceptionNewf(py.TypeError,
			"getaddrinfo() argument 1 must be str or None, not %s", hostArg.Type().Name)
	}
	port, err := portOf(portArg)
	if err != nil {
		return nil, err
	}
	fam := AF_UNSPEC
	if v, ok := asInt(famObj); ok {
		fam = v
	}
	wantType := 0
	if v, ok := asInt(typObj); ok {
		wantType = v
	}
	flagBits := 0
	if v, ok := asInt(flagsObj); ok {
		flagBits = v
	}

	var ips []net.IP
	canon := ""
	if host == "" {
		// The wildcard address.
		if fam == AF_INET6 {
			ips = []net.IP{net.IPv6unspecified}
		} else {
			ips = []net.IP{net.IPv4zero}
		}
	} else {
		ips, err = net.LookupIP(host)
		if err != nil {
			return nil, osErr(err)
		}
		// A canonname is only produced when asked for, and only for a name
		// that is not already numeric.
		if flagBits&AI_CANONNAME != 0 && net.ParseIP(host) == nil {
			if cname, err := net.LookupCNAME(host); err == nil {
				canon = strings.TrimSuffix(cname, ".")
			}
		}
	}

	n, _ := strconv.Atoi(port)
	out := make([]py.Object, 0, len(ips)*2)
	seen := map[string]bool{}
	for _, ip := range ips {
		ipFam := AF_INET
		ipOut := ip
		if ip.To4() == nil {
			ipFam = AF_INET6
		} else {
			ipOut = ip.To4()
		}
		if fam != AF_UNSPEC && fam != ipFam {
			continue
		}
		// CPython reports one entry per socket type, stream before datagram,
		// unless the caller asked for a particular one.
		types := []int{SOCK_STREAM, SOCK_DGRAM}
		if wantType != 0 {
			types = []int{wantType}
		}
		for _, sockType := range types {
			key := fmt.Sprintf("%d/%d/%s", ipFam, sockType, ipOut.String())
			if seen[key] {
				continue
			}
			seen[key] = true
			proto := IPPROTO_TCP
			if sockType == SOCK_DGRAM {
				proto = IPPROTO_UDP
			}
			out = append(out, py.Tuple{
				py.Int(ipFam),
				py.Int(sockType),
				py.Int(proto),
				py.String(canon),
				py.Tuple{py.String(ipOut.String()), py.Int(n)},
			})
		}
	}
	if len(out) == 0 {
		return nil, py.ExceptionNewf(GaierrorType, "no addresses found for the given family")
	}
	return py.NewListFromItems(out), nil
}

// createConnectionMethod dials and returns a ready socket.
func createConnectionMethod(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var address, timeoutObj, sourceAddr py.Object
	kwlist := []string{"address", "timeout", "source_address"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:create_connection", kwlist,
		&address, &timeoutObj, &sourceAddr); err != nil {
		return nil, err
	}
	timeout := py.Object(py.None)
	if timeoutObj != nil {
		timeout = timeoutObj
	}
	s := &sock{family: AF_INET, typ: SOCK_STREAM, timeout: timeout, opts: map[int]int{}}
	if _, err := s.connect(address); err != nil {
		return nil, err
	}
	return s, nil
}

// socketpairMethod returns two connected sockets, over a real socketpair where
// Go exposes one and otherwise over a loopback listener, which is what CPython
// falls back to as well.
func socketpairMethod(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var famObj, typObj, protoObj py.Object
	kwlist := []string{"family", "type", "proto"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "|iii:socketpair", kwlist,
		&famObj, &typObj, &protoObj); err != nil {
		return nil, err
	}
	family := AF_UNIX
	if v, ok := asInt(famObj); ok {
		family = v
	}
	typ := SOCK_STREAM
	if v, ok := asInt(typObj); ok {
		typ = v
	}
	if family != AF_UNIX {
		return nil, py.ExceptionNewf(py.OSError, "only AF_UNIX is supported by socketpair on this platform")
	}
	if typ != SOCK_STREAM {
		return nil, py.ExceptionNewf(py.NotImplementedError, "socketpair only supports SOCK_STREAM")
	}

	// Unix socketpair in Go: listen on a temporary unix socket, connect to
	// it, and accept, which yields a connected pair.  The listening socket
	// and its file are removed before returning.
	tmp, err := os.CreateTemp("", "gpython-socketpair-*")
	if err != nil {
		return nil, osErr(err)
	}
	path := tmp.Name()
	tmp.Close()
	os.Remove(path)
	defer os.Remove(path)

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, osErr(err)
	}
	defer l.Close()

	done := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			errCh <- err
			return
		}
		done <- c
	}()

	client, err := net.Dial("unix", path)
	if err != nil {
		return nil, osErr(err)
	}
	var server net.Conn
	select {
	case server = <-done:
	case err := <-errCh:
		client.Close()
		return nil, osErr(err)
	case <-time.After(5 * time.Second):
		client.Close()
		return nil, py.ExceptionNewf(SocketErrorType, "socketpair timed out")
	}

	mk := func(c net.Conn) *sock {
		return &sock{
			family:  AF_UNIX,
			typ:     SOCK_STREAM,
			timeout: py.None,
			opts:    map[int]int{},
			conn:    c,
		}
	}
	return py.Tuple{mk(client), mk(server)}, nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func init() {
	methods := []*py.Method{
		py.MustNewMethod("gethostname", gethostnameMethod, 0, "Return the current host name."),
		py.MustNewMethod("gethostbyname", gethostbynameMethod, 0, "Return the IPv4 address of a host name."),
		py.MustNewMethod("gethostbyaddr", gethostbyaddrMethod, 0, "Return the true host name for a given address."),
		py.MustNewMethod("getaddrinfo", getaddrinfoMethod, 0, "Resolve a host/port into a list of address information tuples."),
		py.MustNewMethod("create_connection", createConnectionMethod, 0, "Connect to an address and return a ready socket."),
		py.MustNewMethod("inet_aton", inetAtonMethod, 0, "Convert an IPv4 address from dotted quad to 32-bit packed binary."),
		py.MustNewMethod("inet_ntoa", inetNtoaMethod, 0, "Convert a 32-bit packed IPv4 address to dotted quad."),
		py.MustNewMethod("inet_pton", inetPtonMethod, 0, "Convert an IP address from text to packed binary form."),
		py.MustNewMethod("inet_ntop", inetNtopMethod, 0, "Convert a packed IP address to text form."),
		py.MustNewMethod("socketpair", socketpairMethod, 0, "Return a pair of connected sockets."),
		py.MustNewMethod("getdefaulttimeout", getDefaultTimeoutMethod, 0,
			"Return the default timeout in seconds, or None for no timeout."),
		py.MustNewMethod("setdefaulttimeout", setDefaultTimeoutMethod, 0,
			"Set the default timeout for new sockets; None means no timeout."),
	}

	// The socket class's own methods, and its file object, are registered in
	// socket_module.go.
	registerSocketMethods()

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "socket",
			Doc:  module_doc,
		},
		Methods:          methods,
		Globals:          py.NewStringDictFrom(py.DictEntry{Key: "socket", Value: SocketType}, py.DictEntry{Key: "AF_UNSPEC", Value: py.Int(AF_UNSPEC)}, py.DictEntry{Key: "AF_UNIX", Value: py.Int(AF_UNIX)}, py.DictEntry{Key: "AF_INET", Value: py.Int(AF_INET)}, py.DictEntry{Key: "AF_INET6", Value: py.Int(AF_INET6)}, py.DictEntry{Key: "SOCK_STREAM", Value: py.Int(SOCK_STREAM)}, py.DictEntry{Key: "SOCK_DGRAM", Value: py.Int(SOCK_DGRAM)}, py.DictEntry{Key: "SOCK_RAW", Value: py.Int(SOCK_RAW)}, py.DictEntry{Key: "SOL_SOCKET", Value: py.Int(SOL_SOCKET)}, py.DictEntry{Key: "SO_REUSEADDR", Value: py.Int(SO_REUSEADDR)}, py.DictEntry{Key: "SO_REUSEPORT", Value: py.Int(SO_REUSEPORT)}, py.DictEntry{Key: "SO_KEEPALIVE", Value: py.Int(SO_KEEPALIVE)}, py.DictEntry{Key: "SO_BROADCAST", Value: py.Int(SO_BROADCAST)}, py.DictEntry{Key: "SO_SNDBUF", Value: py.Int(SO_SNDBUF)}, py.DictEntry{Key: "SO_RCVBUF", Value: py.Int(SO_RCVBUF)}, py.DictEntry{Key: "SHUT_RD", Value: py.Int(SHUT_RD)}, py.DictEntry{Key: "SHUT_WR", Value: py.Int(SHUT_WR)}, py.DictEntry{Key: "SHUT_RDWR", Value: py.Int(SHUT_RDWR)}, py.DictEntry{Key: "IPPROTO_TCP", Value: py.Int(IPPROTO_TCP)}, py.DictEntry{Key: "IPPROTO_UDP", Value: py.Int(IPPROTO_UDP)}, py.DictEntry{Key: "IPPROTO_IPV6", Value: py.Int(IPPROTO_IPV6)}, py.DictEntry{Key: "IPPROTO_IP", Value: py.Int(0)}, py.DictEntry{Key: "TCP_NODELAY", Value: py.Int(TCP_NODELAY)}, py.DictEntry{Key: "IPV6_V6ONLY", Value: py.Int(IPV6_V6ONLY)}, py.DictEntry{Key: "TCP_MAXSEG", Value: py.Int(0x2)}, py.DictEntry{Key: "AI_PASSIVE", Value: py.Int(AI_PASSIVE)}, py.DictEntry{Key: "AI_CANONNAME", Value: py.Int(AI_CANONNAME)}, py.DictEntry{Key: "AI_NUMERICHOST", Value: py.Int(AI_NUMERICHOST)}, py.DictEntry{Key: "AI_NUMERICSERV", Value: py.Int(AI_NUMERICSERV)}, py.DictEntry{Key: "NI_NUMERICHOST", Value: py.Int(NI_NUMERICHOST)}, py.DictEntry{Key: "NI_NUMERICSERV", Value: py.Int(NI_NUMERICSERV)}, py.DictEntry{Key: "NI_NOFQDN", Value: py.Int(NI_NOFQDN)}, py.DictEntry{Key: "NI_NAMEREQD", Value: py.Int(NI_NAMEREQD)}, py.DictEntry{Key: "NI_DGRAM", Value: py.Int(NI_DGRAM)}, py.DictEntry{Key: "MSG_PEEK", Value: py.Int(MSG_PEEK)}, py.DictEntry{Key: "MSG_DONTWAIT", Value: py.Int(MSG_DONTWAIT)}, py.DictEntry{Key: "INADDR_ANY", Value: py.Int(INADDR_ANY)}, py.DictEntry{Key: "INADDR_BROADCAST", Value: py.Int(INADDR_BROADCAST)}, py.DictEntry{Key: "error", Value: SocketErrorType}, py.DictEntry{Key: "gaierror", Value: GaierrorType}, py.DictEntry{Key: "herror", Value: HerrorType}, py.DictEntry{Key: // socket.timeout is TimeoutError as of CPython 3.10.
		"timeout", Value: py.TimeoutError}, py.DictEntry{Key: "has_ipv6", Value: py.True}),
	})
}

// defaultTimeout is the process-wide default timeout for new sockets, set by
// setdefaulttimeout.  None means no timeout, matching CPython; urllib3 reads
// it through getdefaulttimeout.
var defaultTimeout py.Object = py.None

func getDefaultTimeoutMethod(self py.Object, args py.Tuple) (py.Object, error) {
	return defaultTimeout, nil
}

func setDefaultTimeoutMethod(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "setdefaulttimeout() takes exactly one argument")
	}
	v := args[0]
	if v == py.None {
		defaultTimeout = py.None
		return py.None, nil
	}
	f, err := toFloat(v)
	if err != nil {
		return nil, err
	}
	if f < 0 {
		return nil, py.ExceptionNewf(py.ValueError, "Timeout value out of range")
	}
	defaultTimeout = py.Float(f)
	return py.None, nil
}

// toFloat converts a number-like object to a float.
func toFloat(v py.Object) (float64, error) {
	switch x := v.(type) {
	case py.Int:
		return float64(x), nil
	case py.Float:
		return float64(x), nil
	case py.Bool:
		if bool(x) {
			return 1, nil
		}
		return 0, nil
	}
	return 0, py.ExceptionNewf(py.TypeError, "a number is required")
}
