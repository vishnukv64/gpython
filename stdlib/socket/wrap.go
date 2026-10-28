package socket

import (
	"net"
	"time"

	"github.com/vishnukv64/gpython/py"
)

// Rewrap is how ssl wraps a connected stream socket: it returns a new socket
// of Python type t whose connection is wrap(conn, deadline), and detaches the
// original (as CPython's wrap_socket does) without closing the connection.
// deadline is zero for a blocking socket.
func Rewrap(o py.Object, t *py.Type, wrap func(c net.Conn, deadline time.Time) (net.Conn, error)) (py.Object, error) {
	s, ok := o.(*sock)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "expected a socket, not %s", o.Type().Name)
	}
	if s.closed || s.conn == nil {
		return nil, py.ExceptionNewf(py.OSError, "[Errno 57] Socket is not connected")
	}
	conn, err := wrap(s.conn, s.deadline())
	if err != nil {
		return nil, err
	}
	ns := &sock{family: s.family, typ: s.typ, proto: s.proto, timeout: s.timeout,
		opts: s.opts, conn: conn, pytype: t}
	s.conn = nil
	s.closed = true
	s.dropPlaceholder()
	forgetSock(s)
	return ns, nil
}

// ConnOf returns the connection behind a socket, for ssl to inspect its
// TLS state.
func ConnOf(o py.Object) (net.Conn, bool) {
	s, ok := o.(*sock)
	if !ok || s.conn == nil {
		return nil, false
	}
	return s.conn, true
}
