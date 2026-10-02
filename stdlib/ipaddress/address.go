// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipaddress

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// newAddr builds an address object.
func newAddr(ip netip.Addr) *addr {
	return &addr{ip: ip, version: versionOf(ip), Dict: py.NewStringDict()}
}

// --- address properties --------------------------------------------------

// privateNetworksV4 is CPython's _IPv4Constants._private_networks.
var privateNetworksV4 = []string{
	"0.0.0.0/8", "10.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.0.0.170/31", "192.0.2.0/24",
	"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
	"240.0.0.0/4",
}

var privateExceptionsV4 = []string{"192.0.0.9/32", "192.0.0.10/32"}

// privateNetworksV6 is CPython's _IPv6Constants._private_networks.
var privateNetworksV6 = []string{
	"::1/128", "::/128", "::ffff:0:0/96", "64:ff9b:1::/48", "100::/64",
	"2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20", "fc00::/7",
	"fe80::/10",
}

var privateExceptionsV6 = []string{
	"2001:1::1/128", "2001:1::2/128", "2001:3::/32", "2001:4:112::/48",
	"2001:20::/28", "2001:30::/28",
}

var reservedNetworksV6 = []string{
	"::/8", "100::/8", "200::/7", "400::/6", "800::/5", "1000::/4",
	"4000::/3", "6000::/3", "8000::/3", "A000::/3", "C000::/3",
	"E000::/4", "F000::/5", "F800::/6", "FE00::/9",
}

func inAny(ip netip.Addr, list []string) bool {
	for _, cidr := range list {
		p := netip.MustParsePrefix(cidr)
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func isPrivate(ip netip.Addr) bool {
	if ip.Is4() {
		return inAny(ip, privateNetworksV4) && !inAny(ip, privateExceptionsV4)
	}
	// The IPv4-mapped form takes its classification from the IPv4 address.
	if u := ip.Unmap(); u.Is4() {
		return inAny(u, privateNetworksV4) && !inAny(u, privateExceptionsV4)
	}
	return inAny(ip, privateNetworksV6) && !inAny(ip, privateExceptionsV6)
}

func isGlobal(ip netip.Addr) bool {
	if ip.Is4() {
		// 100.64.0.0/10 is neither private nor global.
		if netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
			return false
		}
	}
	return !isPrivate(ip)
}

func isReserved(ip netip.Addr) bool {
	if ip.Is4() {
		return netip.MustParsePrefix("240.0.0.0/4").Contains(ip)
	}
	return inAny(ip, reservedNetworksV6)
}

func isLoopback(ip netip.Addr) bool {
	if ip.Is4() {
		return netip.MustParsePrefix("127.0.0.0/8").Contains(ip)
	}
	return ip == netip.IPv6Loopback() || netip.MustParseAddr("::1") == ip
}

func isLinkLocal(ip netip.Addr) bool {
	if ip.Is4() {
		return netip.MustParsePrefix("169.254.0.0/16").Contains(ip)
	}
	return netip.MustParsePrefix("fe80::/10").Contains(ip)
}

func isMulticast(ip netip.Addr) bool {
	if ip.Is4() {
		return netip.MustParsePrefix("224.0.0.0/4").Contains(ip)
	}
	return netip.MustParsePrefix("ff00::/8").Contains(ip)
}

func isUnspecified(ip netip.Addr) bool { return ip.IsUnspecified() }

// packed returns the address as bytes.
func packedBytes(ip netip.Addr) py.Bytes {
	if ip.Is4() {
		b := ip.As4()
		return py.Bytes(b[:])
	}
	b := ip.As16()
	return py.Bytes(b[:])
}

// reversePointer is the in-addr.arpa / ip6.arpa name, as CPython builds it.
func reversePointer(ip netip.Addr) string {
	if ip.Is4() {
		b := ip.As4()
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", b[3], b[2], b[1], b[0])
	}
	b := ip.As16()
	parts := make([]string, 0, 32)
	for i := len(b) - 1; i >= 0; i-- {
		parts = append(parts, fmt.Sprintf("%x", b[i]&0xf))
		parts = append(parts, fmt.Sprintf("%x", b[i]>>4))
	}
	return strings.Join(parts, ".") + ".ip6.arpa"
}

func exploded(ip netip.Addr) string {
	if ip.Is4() {
		b := ip.As4()
		return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
	}
	b := ip.As16()
	parts := make([]string, 8)
	for i := 0; i < 8; i++ {
		parts[i] = fmt.Sprintf("%x", uint16(b[2*i])<<8|uint16(b[2*i+1]))
	}
	return strings.Join(parts, ":")
}

// compressed is CPython's compressed form: lower-case, longest run of zero
// groups replaced by "::".
func compressed(ip netip.Addr) string {
	if ip.Is4() {
		return exploded(ip)
	}
	b := ip.As16()
	groups := make([]uint16, 8)
	for i := 0; i < 8; i++ {
		groups[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	// Longest run of zeros (must be more than one group to be elided).
	bestStart, bestLen := -1, 0
	curStart, curLen := -1, 0
	for i, g := range groups {
		if g == 0 {
			if curStart < 0 {
				curStart = i
			}
			curLen++
			if curLen > bestLen {
				bestStart, bestLen = curStart, curLen
			}
		} else {
			curStart, curLen = -1, 0
		}
	}
	if bestLen < 2 {
		bestStart, bestLen = -1, 0
	}
	var sb strings.Builder
	for i := 0; i < 8; {
		if i == bestStart {
			sb.WriteString("::")
			i += bestLen
			continue
		}
		if sb.Len() > 0 && !strings.HasSuffix(sb.String(), ":") {
			sb.WriteString(":")
		}
		fmt.Fprintf(&sb, "%x", groups[i])
		i++
	}
	out := sb.String()
	if out == "" {
		return "::"
	}
	return out
}

func addrStr(ip netip.Addr) string {
	if ip.Is4() {
		return exploded(ip)
	}
	return compressed(ip)
}

// --- address methods -----------------------------------------------------

func addrNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, pyErr(py.TypeError, "Address() takes exactly one argument")
	}
	ip, version, err := addrArg(args[0])
	if err != nil {
		return nil, err
	}
	want := 4
	if metatype == IPv6AddressType {
		want = 6
	}
	if version != want {
		return nil, pyErr(AddressValueError, "%s address/network/interface object is not valid",
			map[int]string{4: "IPv4", 6: "IPv6"}[want])
	}
	_ = version
	if metatype == IPv4AddressType && !ip.Is4() {
		return nil, pyErr(AddressValueError, "IPv6 address is not valid IPv4 address")
	}
	if metatype == IPv6AddressType && ip.Is4() {
		// CPython wraps a plain IPv4 address into ::ffff:a.b.c.d only for
		// the interface form; a bare v4 string is an error for IPv6Address.
		return nil, pyErr(AddressValueError, "IPv4 address is not valid IPv6 address")
	}
	return newAddr(ip), nil
}

func addrGetitem(self py.Object, args py.Tuple) (py.Object, error) {
	a := self.(*addr)
	if len(args) != 1 {
		return nil, pyErr(py.TypeError, "__getitem__() takes exactly one argument")
	}
	i, err := py.MakeGoInt(args[0])
	if err != nil {
		return nil, err
	}
	b := packedBytes(a.ip)
	if i < 0 || i >= len(b) {
		return nil, pyErr(py.IndexError, "index out of range")
	}
	return py.Int(b[i]), nil
}

func addrLen(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(len(packedBytes(self.(*addr).ip))), nil
}

// addrIntMethod is __int__, the address as an integer.
func addrIntMethod(self py.Object, args py.Tuple) (py.Object, error) {
	return addrInt(self.(*addr).ip)
}

// M__int__ is what int() calls: MakeInt consults the I__int__ interface, so
// __int__ registered in the type dict is not enough.
func (a *addr) M__int__() (py.Object, error) {
	return addrInt(a.ip)
}

// The comparison operators go through the I__eq__/I__lt__ interfaces rather
// than the type dictionary, so each wrapper needs the Go-level method too.
func (a *addr) M__eq__(other py.Object) (py.Object, error) {
	return addrCompare(a, other, func(c int) bool { return c == 0 })
}

func (a *addr) M__lt__(other py.Object) (py.Object, error) {
	return addrCompare(a, other, func(c int) bool { return c < 0 })
}

func (a *addr) M__le__(other py.Object) (py.Object, error) {
	return addrCompare(a, other, func(c int) bool { return c <= 0 })
}

func (a *addr) M__gt__(other py.Object) (py.Object, error) {
	return addrCompare(a, other, func(c int) bool { return c > 0 })
}

func (a *addr) M__ge__(other py.Object) (py.Object, error) {
	return addrCompare(a, other, func(c int) bool { return c >= 0 })
}

func (n *netw) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*netw)
	if !ok {
		return py.NotImplemented, nil
	}
	return py.Bool(n.prefix == o.prefix), nil
}

// M__str__ and M__repr__ are the Go-level methods the interpreter looks for
// before the type dictionary.
func (a *addr) M__str__() (py.Object, error) {
	return py.String(addrStr(a.ip)), nil
}

func (a *addr) M__repr__() (py.Object, error) {
	cls := "IPv4Address"
	if a.version == 6 {
		cls = "IPv6Address"
	}
	return py.String(fmt.Sprintf("%s('%s')", cls, addrStr(a.ip))), nil
}

func addrStrMethod(self py.Object, args py.Tuple) (py.Object, error) {
	return py.String(addrStr(self.(*addr).ip)), nil
}

func addrReprMethod(self py.Object, args py.Tuple) (py.Object, error) {
	a := self.(*addr)
	cls := "IPv4Address"
	if a.version == 6 {
		cls = "IPv6Address"
	}
	return py.String(fmt.Sprintf("%s('%s')", cls, addrStr(a.ip))), nil
}

func addrHash(self py.Object, args py.Tuple) (py.Object, error) {
	a := self.(*addr)
	// A hash that agrees with __eq__: the address bytes are unique per value.
	b := packedBytes(a.ip)
	var h uint64 = 1469598103934665603
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	if h > 0x7FFFFFFFFFFFFFFF {
		h -= 0xFFFFFFFFFFFFFFFF
	}
	return py.Int(int64(h)), nil
}

func addrEq(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	return addrCompare(self.(*addr), args[0], func(c int) bool { return c == 0 })
}

func addrLt(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	return addrCompare(self.(*addr), args[0], func(c int) bool { return c < 0 })
}

func addrLe(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	return addrCompare(self.(*addr), args[0], func(c int) bool { return c <= 0 })
}

func addrGt(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	return addrCompare(self.(*addr), args[0], func(c int) bool { return c > 0 })
}

func addrGe(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	return addrCompare(self.(*addr), args[0], func(c int) bool { return c >= 0 })
}

// addrCompare orders two objects as CPython does: addresses compare with
// addresses, and an address compares with a network/interface holding the
// same version; anything else is NotImplemented.
func addrCompare(a *addr, other py.Object, pred func(int) bool) (py.Object, error) {
	var otherIP netip.Addr
	var otherVer int
	switch o := other.(type) {
	case *addr:
		otherIP, otherVer = o.ip, o.version
	case *netw:
		otherIP, otherVer = o.prefix.Addr(), versionOf(o.prefix.Addr())
	case *iface:
		otherIP, otherVer = o.addr.ip, o.addr.version
	default:
		return py.NotImplemented, nil
	}
	// CPython orders by version first.
	if a.version != otherVer {
		return py.Bool(pred(a.version - otherVer)), nil
	}
	return py.Bool(pred(compareAddr(a.ip, otherIP))), nil
}

func compareAddr(a, b netip.Addr) int {
	switch {
	case a == b:
		return 0
	case a.Less(b):
		return -1
	default:
		return 1
	}
}
