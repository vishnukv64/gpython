// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ipaddress provides the implementation of python's 'ipaddress' module:
// IPv4 and IPv6 addresses, networks and interfaces.
//
// The arithmetic is Go's net/netip.  The classification predicates
// (is_private, is_global, is_reserved, ...) follow CPython's tables from the
// IANA special-purpose address registries, because netip's own IsPrivate and
// friends use different (and narrower) definitions.
package ipaddress

import (
	"fmt"
	"math/bits"
	"net/netip"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `ipaddress - IPv4/IPv6 manipulation library.

ip_address, ip_network and ip_interface construct IPv4Address/IPv6Address,
IPv4Network/IPv6Network and IPv4Interface/IPv6Interface objects.  The address
and network classes expose the classification properties (is_private,
is_global, is_loopback, is_multicast, is_reserved, is_link_local, ...) that
network code tests.`

// AddressValueError and NetmaskValueError are the two exception classes the
// constructors raise.
var (
	AddressValueError = py.ValueError.NewType("ipaddress.AddressValueError",
		"Address is not valid.", nil, nil)
	NetmaskValueError = py.ValueError.NewType("ipaddress.NetmaskValueError",
		"Netmask is not valid.", nil, nil)
)

func init() {
	AddressValueError.Base = py.ValueError
	NetmaskValueError.Base = py.ValueError
}

// addr wraps a single address together with the version-specific behaviour.
type addr struct {
	ip      netip.Addr
	version int
	// Dict carries attributes assigned from python.
	Dict py.StringDict
}

func (a *addr) Type() *py.Type { return addrType(a.version) }

func (a *addr) GetDict() py.StringDict { return a.Dict }

// netw wraps a network (an address plus a prefix length).
type netw struct {
	prefix netip.Prefix
	strict bool
	Dict   py.StringDict
}

func (n *netw) Type() *py.Type { return netType(n.prefix.Addr().Is4()) }

func (n *netw) GetDict() py.StringDict { return n.Dict }

// iface wraps an interface: an address paired with the network it is on.
type iface struct {
	addr   *addr
	netobj *netw
	Dict   py.StringDict
}

func (i *iface) Type() *py.Type { return ifaceType(i.addr.version) }

func (i *iface) GetDict() py.StringDict { return i.Dict }

var (
	IPv4AddressType   *py.Type
	IPv6AddressType   *py.Type
	IPv4NetworkType   *py.Type
	IPv6NetworkType   *py.Type
	IPv4InterfaceType *py.Type
	IPv6InterfaceType *py.Type
)

func addrType(v int) *py.Type {
	if v == 4 {
		return IPv4AddressType
	}
	return IPv6AddressType
}

func netType(is4 bool) *py.Type {
	if is4 {
		return IPv4NetworkType
	}
	return IPv6NetworkType
}

func ifaceType(v int) *py.Type {
	if v == 4 {
		return IPv4InterfaceType
	}
	return IPv6InterfaceType
}

// --- parsing helpers -----------------------------------------------------

// parseAddrString parses one address.  A scope id (%eth0) is accepted and
// dropped: Go's netip keeps it but CPython's ipaddress rejects a zone on
// anything but a link-local address, which is not the case worth carrying.
func parseAddrString(s string) (netip.Addr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, fmt.Errorf("Address cannot be empty")
	}
	if ip, err := netip.ParseAddr(s); err == nil {
		return ip, nil
	}
	return netip.Addr{}, fmt.Errorf("'%s' does not appear to be an IPv4 or IPv6 address", s)
}

func netmaskErr(s string) error {
	return fmt.Errorf("'%s' is not a valid netmask", s)
}

// --- Python conversion helpers -------------------------------------------

func pyErr(t *py.Type, format string, a ...interface{}) error {
	return py.ExceptionNewf(t, format, a...)
}

// addrInt returns the address as a python int.  IPv6 needs 128 bits, which is
// wider than py.Int, so the value is built from its decimal string and the
// interpreter promotes it to a big int.
func addrInt(a netip.Addr) (py.Object, error) {
	n, err := addrBig(a)
	if err != nil {
		return nil, err
	}
	return py.IntFromString(n.String(), 10)
}

func addrBig(a netip.Addr) (*bigInt, error) {
	if a.Is4() {
		b := a.As4()
		v := uint64(b[0])<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
		return newBigInt(v), nil
	}
	b := a.As16()
	hi := uint64(0)
	lo := uint64(0)
	for i := 0; i < 8; i++ {
		hi = hi<<8 | uint64(b[i])
	}
	for i := 8; i < 16; i++ {
		lo = lo<<8 | uint64(b[i])
	}
	return newBig128(hi, lo), nil
}

// bigInt is a minimal unsigned big integer: enough for the 128-bit values an
// IPv6 address needs, with decimal conversion for python's int().
type bigInt struct {
	words []uint64 // little-endian
}

func newBigInt(v uint64) *bigInt { return &bigInt{words: []uint64{v}} }

func newBig128(hi, lo uint64) *bigInt {
	if hi == 0 {
		return &bigInt{words: []uint64{lo}}
	}
	return &bigInt{words: []uint64{lo, hi}}
}

func (b *bigInt) String() string {
	if len(b.words) == 0 {
		return "0"
	}
	// Repeated division by 10^19.
	digits := []string{}
	cur := append([]uint64(nil), b.words...)
	base := uint64(10000000000000000000)
	for !b.isZero(cur) {
		var rem uint64
		cur, rem = divmodSmall(cur, base)
		if b.isZero(cur) {
			digits = append(digits, fmt.Sprintf("%d", rem))
		} else {
			digits = append(digits, fmt.Sprintf("%019d", rem))
		}
	}
	var sb strings.Builder
	for i := len(digits) - 1; i >= 0; i-- {
		sb.WriteString(digits[i])
	}
	return sb.String()
}

func (b *bigInt) isZero(cur []uint64) bool {
	for _, w := range cur {
		if w != 0 {
			return false
		}
	}
	return true
}

func divmodSmall(in []uint64, d uint64) ([]uint64, uint64) {
	out := make([]uint64, len(in))
	var rem uint64
	for i := len(in) - 1; i >= 0; i-- {
		quo, r := bits.Div64(rem, in[i], d)
		out[i] = quo
		rem = r
	}
	for len(out) > 1 && out[len(out)-1] == 0 {
		out = out[:len(out)-1]
	}
	return out, rem
}

// intToAddr converts a python int back to an address of the right version.
func intToAddr(v py.Object, version int) (netip.Addr, error) {
	n, err := py.MakeGoInt64(v)
	if err == nil {
		return int64ToAddr(n, version)
	}
	// Fall back to the decimal string, which big ints support.
	s, serr := py.StrAsString(v)
	if serr != nil {
		return netip.Addr{}, pyErr(AddressValueError, "an integer is required")
	}
	var u uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &u); err != nil {
		return netip.Addr{}, pyErr(AddressValueError, "'%s' does not appear to be an IPv4 or IPv6 address", s)
	}
	return int64ToAddr(int64(u), version)
}

func int64ToAddr(n int64, version int) (netip.Addr, error) {
	max := int64(0xFFFFFFFF)
	if version == 6 {
		// A 64-bit int cannot hold a full IPv6 address; only the low bits
		// are representable here.
		max = int64(0x7FFFFFFFFFFFFFFF)
	}
	if n < 0 || n > max {
		return netip.Addr{}, pyErr(AddressValueError, "Address has invalid length")
	}
	if version == 4 {
		b := [4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
		return netip.AddrFrom4(b), nil
	}
	var b [16]byte
	for i := 15; i >= 0; i-- {
		b[i] = byte(n)
		n >>= 8
	}
	return netip.AddrFrom16(b), nil
}

// --- constructor argument parsing ----------------------------------------

// addrArg turns the single argument of an address/network/interface
// constructor into an address and version.
func addrArg(v py.Object) (netip.Addr, int, error) {
	switch x := v.(type) {
	case py.String:
		ip, err := parseAddrString(string(x))
		if err != nil {
			return netip.Addr{}, 0, pyErr(AddressValueError, "%s", err.Error())
		}
		if ip.Is4() {
			return ip, 4, nil
		}
		return ip, 6, nil
	case *addr:
		return x.ip, x.version, nil
	case *netw:
		return x.prefix.Addr(), versionOf(x.prefix.Addr()), nil
	case *iface:
		return x.addr.ip, x.addr.version, nil
	}
	// An integer selects the version by magnitude.
	if n, err := py.MakeGoInt64(v); err == nil {
		if n < 0 {
			return netip.Addr{}, 0, pyErr(AddressValueError, "Address cannot be negative")
		}
		if n <= 0xFFFFFFFF {
			ip, err := int64ToAddr(n, 4)
			return ip, 4, err
		}
		ip, err := int64ToAddr(n, 6)
		return ip, 6, err
	}
	return netip.Addr{}, 0, pyErr(AddressValueError,
		"'%s' does not appear to be an IPv4 or IPv6 address", pyRepr(v))
}

func pyRepr(o py.Object) string {
	r, err := py.Repr(o)
	if err != nil {
		return "?"
	}
	s, _ := py.StrAsString(r)
	return s
}

func versionOf(a netip.Addr) int {
	if a.Is4() {
		return 4
	}
	return 6
}

// prefixArg parses the second argument of a network: a prefix length, a
// netmask address, or an integer.
func prefixArg(v py.Object, ip netip.Addr) (int, error) {
	maxlen := 128
	if ip.Is4() {
		maxlen = 32
	}
	switch x := v.(type) {
	case py.String:
		s := strings.TrimSpace(string(x))
		if strings.Contains(s, ".") || strings.Contains(s, ":") {
			// A netmask address.
			mask, err := parseAddrString(s)
			if err != nil {
				return 0, pyErr(NetmaskValueError, "%s", err.Error())
			}
			if mask.Is4() != ip.Is4() {
				return 0, pyErr(NetmaskValueError, "Invalid netmask")
			}
			plen := maskLen(mask)
			if plen < 0 {
				return 0, pyErr(NetmaskValueError, "'%s' is not a valid netmask", s)
			}
			return plen, nil
		}
		n, err := py.MakeGoInt(py.String(s))
		if err != nil {
			return 0, pyErr(NetmaskValueError, "'%s' is not a valid netmask", s)
		}
		return prefixRange(n, maxlen, s)
	}
	n, err := py.MakeGoInt(v)
	if err != nil {
		return 0, pyErr(NetmaskValueError, "'%s' is not a valid netmask", pyRepr(v))
	}
	return prefixRange(n, maxlen, pyRepr(v))
}

func prefixRange(n, maxlen int, s string) (int, error) {
	if n < 0 || n > maxlen {
		return 0, pyErr(NetmaskValueError, "Netmask is not valid: '%s'", s)
	}
	return n, nil
}

// maskLen returns the prefix length of a netmask, or -1 when the mask is not
// contiguous.
func maskLen(mask netip.Addr) int {
	var b []byte
	if mask.Is4() {
		m := mask.As4()
		b = m[:]
	} else {
		m := mask.As16()
		b = m[:]
	}
	seenZero := false
	plen := 0
	for _, c := range b {
		for bit := 7; bit >= 0; bit-- {
			if c&(1<<uint(bit)) != 0 {
				if seenZero {
					return -1
				}
				plen++
			} else {
				seenZero = true
			}
		}
	}
	return plen
}

// maskFor builds the netmask address for a prefix length.
func maskFor(ip netip.Addr, plen int) netip.Addr {
	if ip.Is4() {
		var m uint32
		if plen > 0 {
			m = ^uint32(0) << uint(32-plen)
		}
		return netip.AddrFrom4([4]byte{byte(m >> 24), byte(m >> 16), byte(m >> 8), byte(m)})
	}
	hi, lo := uint64(0), uint64(0)
	if plen > 0 {
		if plen <= 64 {
			hi = ^uint64(0) << uint(64-plen)
		} else {
			hi = ^uint64(0)
			lo = ^uint64(0) << uint(128-plen)
		}
	}
	var b [16]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(hi >> uint(56-8*i))
	}
	for i := 0; i < 8; i++ {
		b[8+i] = byte(lo >> uint(56-8*i))
	}
	return netip.AddrFrom16(b)
}

func hostMaskFor(ip netip.Addr, plen int) netip.Addr {
	// The complement of the netmask.
	mask := maskFor(ip, plen)
	if mask.Is4() {
		m := mask.As4()
		return netip.AddrFrom4([4]byte{^m[0], ^m[1], ^m[2], ^m[3]})
	}
	m := mask.As16()
	var b [16]byte
	for i := range b {
		b[i] = ^m[i]
	}
	return netip.AddrFrom16(b)
}
