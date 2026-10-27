//go:build windows || gpython_simwin

package socket

import (
	"context"
	"net"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

// Windows' asyncio drives sockets by file number through _overlapped, which
// is emulated over Go's net package (stdlib/overlapped).  fileno() records
// which socket a number belongs to, and the emulation looks it up here.

var (
	byFileno   = map[int]*sock{}
	byFilenoMu sync.Mutex
)

func noteFileno(fd int, s *sock) {
	byFilenoMu.Lock()
	byFileno[fd] = s
	byFilenoMu.Unlock()
}

// forgetSock drops every number recorded for s once it is closed, so the
// table does not keep closed sockets alive.
func forgetSock(s *sock) {
	byFilenoMu.Lock()
	for fd, have := range byFileno {
		if have == s {
			delete(byFileno, fd)
		}
	}
	byFilenoMu.Unlock()
}

// SO_UPDATE_ACCEPT_CONTEXT and SO_UPDATE_CONNECT_CONTEXT: the connection is
// already complete in Go's net, so there is nothing to update.
func ignoredSockopt(level, optname int) bool {
	return level == SOL_SOCKET && (optname == 0x700b || optname == 0x7010)
}

// Raw is a socket as the _overlapped emulation sees it.
type Raw struct{ s *sock }

// Lookup finds the socket whose fileno() returned fd.
func Lookup(fd int) (*Raw, bool) {
	byFilenoMu.Lock()
	s, ok := byFileno[fd]
	byFilenoMu.Unlock()
	if !ok || s.closed {
		return nil, false
	}
	return &Raw{s}, true
}

// Key identifies the socket regardless of its current file number.
func (r *Raw) Key() any { return r.s }

func (r *Raw) Conn() net.Conn         { return r.s.conn }
func (r *Raw) Listener() net.Listener { return r.s.listener }
func (r *Raw) Packet() net.PacketConn { return r.s.packet }
func (r *Raw) Closed() bool           { return r.s.closed }
func (r *Raw) Attach(c net.Conn)      { r.s.conn = c }
func (r *Raw) Connect(addr py.Object) error {
	_, err := r.s.connect(addr)
	return err
}

// AddrTuple renders a net address as Python's (host, port).
func (r *Raw) AddrTuple(a net.Addr) (py.Object, error) { return r.s.addrTuple(a) }

// ResolveUDP turns a Python address into a net address for WriteTo.
func (r *Raw) ResolveUDP(addr py.Object) (net.Addr, error) {
	target, err := r.s.addrString(addr)
	if err != nil {
		return nil, err
	}
	network, err := r.s.network("udp")
	if err != nil {
		return nil, err
	}
	return net.ResolveUDPAddr(network, target)
}

// WrapErr converts a net error into the OSError the socket module raises.
func WrapErr(err error) error { return osErr(err) }

// Dial connects the socket like connect(), but hands back Go's own error and
// honours ctx, so a pending ConnectEx can be cancelled.
func (r *Raw) Dial(ctx context.Context, addr py.Object) error {
	s := r.s
	target, err := s.addrString(addr)
	if err != nil {
		return err
	}
	network, err := s.network("tcp")
	if err != nil {
		return err
	}
	var d net.Dialer
	c, err := d.DialContext(ctx, network, target)
	if err != nil {
		return err
	}
	s.dropPlaceholder()
	s.conn = c
	return nil
}
