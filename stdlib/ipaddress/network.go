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

// netwNew builds a network.  With strict=True (the default) the address must be
// the network address; with strict=False the host bits are masked off, as
// CPython does.
func netwNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		addrArgObj py.Object
		strict     py.Object = py.True
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:network",
		[]string{"address", "strict"}, &addrArgObj, &strict); err != nil {
		return nil, err
	}
	want := 4
	if metatype == IPv6NetworkType {
		want = 6
	}

	// The argument may itself be a network/interface object.
	if n, ok := addrArgObj.(*netw); ok {
		if versionOf(n.prefix.Addr()) != want {
			return nil, pyErr(AddressValueError, "version mismatch")
		}
		return &netw{prefix: n.prefix, strict: truthy(strict), Dict: py.NewStringDict()}, nil
	}
	if i, ok := addrArgObj.(*iface); ok {
		if i.addr.version != want {
			return nil, pyErr(AddressValueError, "version mismatch")
		}
		return &netw{prefix: i.netobj.prefix, strict: truthy(strict), Dict: py.NewStringDict()}, nil
	}

	s, err := py.StrAsString(addrArgObj)
	if err != nil {
		return nil, err
	}
	// "192.168.1.0/24" carries the prefix in the string.
	ipPart := s
	plen := -1
	if i := strings.LastIndex(s, "/"); i >= 0 {
		ipPart = s[:i]
		plen = -2 // mark: take from the string
	}
	ip, version, err := addrArg(py.String(ipPart))
	if err != nil {
		return nil, err
	}
	if version != want {
		return nil, pyErr(AddressValueError, "%s address/network/interface object is not valid",
			map[int]string{4: "IPv4", 6: "IPv6"}[want])
	}
	if plen == -2 {
		plen, err = prefixArg(py.String(strings.TrimPrefix(s[strings.LastIndex(s, "/"):], "/")), ip)
		if err != nil {
			return nil, err
		}
	} else {
		// A bare address means a full-length prefix.
		plen = 32
		if version == 6 {
			plen = 128
		}
	}

	prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", addrStr(ip), plen))
	if err != nil {
		return nil, pyErr(AddressValueError, "%s", err.Error())
	}
	if truthy(strict) && prefix.Addr() != ip {
		return nil, pyErr(py.ValueError, "%s has host bits set", addrStr(ip)+"/"+fmt.Sprint(plen))
	}
	if !truthy(strict) {
		prefix = prefix.Masked()
	}
	return &netw{prefix: prefix, strict: truthy(strict), Dict: py.NewStringDict()}, nil
}

func truthy(o py.Object) bool {
	b, err := py.MakeBool(o)
	return err == nil && b == py.True
}

func (n *netw) version() int { return versionOf(n.prefix.Addr()) }

func (n *netw) netmask() netip.Addr  { return maskFor(n.prefix.Addr(), n.prefix.Bits()) }
func (n *netw) hostmask() netip.Addr { return hostMaskFor(n.prefix.Addr(), n.prefix.Bits()) }

func (n *netw) broadcast() netip.Addr {
	// The last address in the network: network address with the host bits set.
	last := n.prefix.Masked().Addr()
	host := hostMaskFor(n.prefix.Addr(), n.prefix.Bits())
	if last.Is4() {
		a, h := last.As4(), host.As4()
		return netip.AddrFrom4([4]byte{a[0] | h[0], a[1] | h[1], a[2] | h[2], a[3] | h[3]})
	}
	a, h := last.As16(), host.As16()
	var b [16]byte
	for i := range b {
		b[i] = a[i] | h[i]
	}
	return netip.AddrFrom16(b)
}

func netwRepr(self py.Object, args py.Tuple) (py.Object, error) {
	n := self.(*netw)
	cls := "IPv4Network"
	if n.version() == 6 {
		cls = "IPv6Network"
	}
	return py.String(fmt.Sprintf("%s('%s/%d')", cls, addrStr(n.prefix.Addr()), n.prefix.Bits())), nil
}

func netwStr(self py.Object, args py.Tuple) (py.Object, error) {
	n := self.(*netw)
	return py.String(fmt.Sprintf("%s/%d", addrStr(n.prefix.Addr()), n.prefix.Bits())), nil
}

// netwIter walks every address in the network.  CPython 3.14's network
// classes are iterable but have no __len__, so a caller uses num_addresses.
func netwIter(self py.Object, args py.Tuple) (py.Object, error) {
	n := self.(*netw)
	last := n.broadcast()
	out := []py.Object{}
	for ip := n.prefix.Masked().Addr(); ip.IsValid() && !last.Less(ip); ip = ip.Next() {
		out = append(out, newAddr(ip))
		if len(out) > 1<<20 {
			// A network larger than a million addresses is not materialised;
			// CPython's generator would also be impractical here.
			break
		}
	}
	return py.NewListFromItems(out).M__iter__()
}

func (n *netw) maxPrefixLen() int {
	if n.version() == 4 {
		return 32
	}
	return 128
}

// pow2String renders 2^exp as a decimal string, which the interpreter accepts
// for values beyond int64.
func pow2String(exp int) string {
	if exp == 0 {
		return "1"
	}
	// A decimal big number: start from 1 and double exp times.
	digits := []byte{1}
	for i := 0; i < exp; i++ {
		carry := 0
		for j := 0; j < len(digits); j++ {
			v := int(digits[j])*2 + carry
			digits[j] = byte(v % 10)
			carry = v / 10
		}
		if carry > 0 {
			digits = append(digits, byte(carry))
		}
	}
	var sb strings.Builder
	for i := len(digits) - 1; i >= 0; i-- {
		sb.WriteByte('0' + digits[i])
	}
	return sb.String()
}

func netwContains(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, pyErr(py.TypeError, "__contains__() takes exactly one argument")
	}
	n := self.(*netw)
	ip, ver, err := addrArg(args[0])
	if err != nil {
		return py.False, nil
	}
	if ver != n.version() {
		return py.False, nil
	}
	return py.Bool(n.prefix.Contains(ip)), nil
}

func netwHash(self py.Object, args py.Tuple) (py.Object, error) {
	n := self.(*netw)
	b := packedBytes(n.prefix.Addr())
	var h uint64 = 1469598103934665603
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	h ^= uint64(n.prefix.Bits())
	h *= 1099511628211
	if h > 0x7FFFFFFFFFFFFFFF {
		h -= 0xFFFFFFFFFFFFFFFF
	}
	return py.Int(int64(h)), nil
}

func netwEq(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	o, ok := args[0].(*netw)
	if !ok {
		return py.NotImplemented, nil
	}
	n := self.(*netw)
	return py.Bool(n.prefix == o.prefix), nil
}

func netwHosts(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	n := self.(*netw)
	first := n.prefix.Masked().Addr()
	last := n.broadcast()
	if n.version() == 4 && n.prefix.Bits() <= 30 {
		first = first.Next()
		last = last.Prev()
	}
	hosts := []py.Object{}
	if n.prefix.Bits() == n.maxPrefixLen() {
		// A /32 or /128 has exactly one host: itself.
		return py.NewListFromItems([]py.Object{newAddr(first)}).M__iter__()
	}
	for ip := first; ip.IsValid() && !last.Less(ip); ip = ip.Next() {
		hosts = append(hosts, newAddr(ip))
	}
	return py.NewListFromItems(hosts).M__iter__()
}

func netwSubnets(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var prefixlen py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:subnets",
		[]string{"prefixlen"}, &prefixlen); err != nil {
		return nil, err
	}
	n := self.(*netw)
	newPrefix := n.prefix.Bits() + 1
	if prefixlen != py.None {
		p, err := py.MakeGoInt(prefixlen)
		if err != nil {
			return nil, err
		}
		newPrefix = p
	}
	if newPrefix <= n.prefix.Bits() || newPrefix > n.maxPrefixLen() {
		return nil, pyErr(py.ValueError, "new prefix must be longer")
	}
	out := []py.Object{}
	for i := 0; i < 1<<uint(newPrefix-n.prefix.Bits()); i++ {
		sub, err := n.nthSubnet(i, newPrefix)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return py.NewListFromItems(out).M__iter__()
}

// nthSubnet builds the i'th subnet of length newPrefix.  The network address is
// the base plus i << hostBits, computed as a byte-wise addition so a 128-bit
// value needs no big-int arithmetic.
func (n *netw) nthSubnet(i, newPrefix int) (*netw, error) {
	base := n.prefix.Masked().Addr()
	shift := n.maxPrefixLen() - newPrefix
	offset := uint64(i) << uint(shift)
	var b []byte
	if base.Is4() {
		m := base.As4()
		b = m[:]
	} else {
		m := base.As16()
		b = m[:]
	}
	// Add offset to the low bytes.
	carry := uint64(0)
	for j := len(b) - 1; j >= 0; j-- {
		if shift == 0 && j == len(b)-1 {
			// offset is a full byte addition at position 0.
		}
		add := (offset & 0xFF) + carry
		v := uint64(b[j]) + add
		b[j] = byte(v)
		carry = v >> 8
		offset >>= 8
	}
	var ip netip.Addr
	if len(b) == 4 {
		var arr [4]byte
		copy(arr[:], b)
		ip = netip.AddrFrom4(arr)
	} else {
		var arr [16]byte
		copy(arr[:], b)
		ip = netip.AddrFrom16(arr)
	}
	p, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", addrStr(ip), newPrefix))
	if err != nil {
		return nil, pyErr(py.ValueError, "%s", err.Error())
	}
	return &netw{prefix: p, Dict: py.NewStringDict()}, nil
}

func netwSupernet(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		other  py.Object
		prefix py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:supernet",
		[]string{"other", "prefixlen"}, &other, &prefix); err != nil {
		return nil, err
	}
	n := self.(*netw)
	newPrefix := n.prefix.Bits() - 1
	if prefix != py.None {
		p, err := py.MakeGoInt(prefix)
		if err != nil {
			return nil, err
		}
		newPrefix = p
	}
	if newPrefix < 0 || newPrefix >= n.prefix.Bits() {
		return nil, pyErr(py.ValueError, "new prefix must be shorter")
	}
	// The supernet is the network with the lowest newPrefix bits used.
	ip := n.prefix.Masked().Addr()
	p, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", addrStr(ip), newPrefix))
	if err != nil {
		return nil, pyErr(py.ValueError, "%s", err.Error())
	}
	return &netw{prefix: p.Masked(), Dict: py.NewStringDict()}, nil
}

func netwAddressEx(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// address_exclude(other) yields the networks within self that do not
	// overlap other.
	var other py.Object
	if err := py.UnpackTuple(args, kwargs, "address_exclude", 1, 1, &other); err != nil {
		return nil, err
	}
	o, ok := other.(*netw)
	if !ok {
		return nil, pyErr(py.TypeError, "address_exclude() requires a network")
	}
	n := self.(*netw)
	if !n.prefix.Contains(o.prefix.Addr()) || o.prefix.Bits() < n.prefix.Bits() {
		// Render each network the way netwStr does: "addr/bits".
		ostr := fmt.Sprintf("%s/%d", addrStr(o.prefix.Addr()), o.prefix.Bits())
		nstr := fmt.Sprintf("%s/%d", addrStr(n.prefix.Addr()), n.prefix.Bits())
		return nil, pyErr(py.ValueError, "'%s' is not a subnet of '%s'", ostr, nstr)
	}
	out := []py.Object{}
	cur := n
	for cur.prefix.Bits() < o.prefix.Bits() {
		subs, err := cur.subnetsWithPrefix(cur.prefix.Bits() + 1)
		if err != nil {
			return nil, err
		}
		for _, s := range subs {
			if s.prefix.Contains(o.prefix.Addr()) {
				cur = s
			} else {
				out = append(out, s)
			}
		}
	}
	return py.NewListFromItems(out).M__iter__()
}

func (n *netw) subnetsWithPrefix(newPrefix int) ([]*netw, error) {
	a, err := n.nthSubnet(0, newPrefix)
	if err != nil {
		return nil, err
	}
	b, err := n.nthSubnet(1, newPrefix)
	if err != nil {
		return nil, err
	}
	return []*netw{a, b}, nil
}

func netwIsSupernetOf(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var other py.Object
	if err := py.UnpackTuple(args, kwargs, "supernet_of", 1, 1, &other); err != nil {
		return nil, err
	}
	o, ok := other.(*netw)
	if !ok {
		return nil, pyErr(py.TypeError, "supernet_of() requires a network")
	}
	n := self.(*netw)
	return py.Bool(n.prefix.Bits() <= o.prefix.Bits() && n.prefix.Contains(o.prefix.Addr())), nil
}

func netwIsSubnetOf(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var other py.Object
	if err := py.UnpackTuple(args, kwargs, "subnet_of", 1, 1, &other); err != nil {
		return nil, err
	}
	o, ok := other.(*netw)
	if !ok {
		return nil, pyErr(py.TypeError, "subnet_of() requires a network")
	}
	n := self.(*netw)
	return py.Bool(n.prefix.Bits() >= o.prefix.Bits() && o.prefix.Contains(n.prefix.Addr())), nil
}

func netwOverlaps(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var other py.Object
	if err := py.UnpackTuple(args, kwargs, "overlaps", 1, 1, &other); err != nil {
		return nil, err
	}
	o, ok := other.(*netw)
	if !ok {
		return nil, pyErr(py.TypeError, "overlaps() requires a network")
	}
	n := self.(*netw)
	return py.Bool(n.prefix.Overlaps(o.prefix)), nil
}

// --- interface -----------------------------------------------------------

func ifaceNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, pyErr(py.TypeError, "Interface() takes exactly one argument")
	}
	s, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	want := 4
	if metatype == IPv6InterfaceType {
		want = 6
	}
	slash := strings.LastIndex(s, "/")
	ipPart := s
	plen := -1
	if slash >= 0 {
		ipPart = s[:slash]
	}
	ip, version, err := addrArg(py.String(ipPart))
	if err != nil {
		return nil, err
	}
	if version != want {
		return nil, pyErr(AddressValueError, "version mismatch")
	}
	if slash >= 0 {
		plen, err = prefixArg(py.String(s[slash+1:]), ip)
		if err != nil {
			return nil, err
		}
	} else {
		plen = 32
		if version == 6 {
			plen = 128
		}
	}
	prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", addrStr(ip), plen))
	if err != nil {
		return nil, pyErr(AddressValueError, "%s", err.Error())
	}
	return &iface{
		addr:   newAddr(ip),
		netobj: &netw{prefix: prefix.Masked(), Dict: py.NewStringDict()},
		Dict:   py.NewStringDict(),
	}, nil
}

func ifaceStr(self py.Object, args py.Tuple) (py.Object, error) {
	i := self.(*iface)
	return py.String(fmt.Sprintf("%s/%d", addrStr(i.addr.ip), i.netobj.prefix.Bits())), nil
}

func ifaceRepr(self py.Object, args py.Tuple) (py.Object, error) {
	i := self.(*iface)
	cls := "IPv4Interface"
	if i.addr.version == 6 {
		cls = "IPv6Interface"
	}
	return py.String(fmt.Sprintf("%s('%s/%d')", cls, addrStr(i.addr.ip), i.netobj.prefix.Bits())), nil
}

// --- property plumbing ---------------------------------------------------

// prop registers a read-only property reading from fn.
func prop(t *py.Type, name string, fn func(self py.Object) (py.Object, error)) {
	t.Dict.Set(name, &py.Property{Fget: fn})
}

func addrProp(name string, fn func(netip.Addr) py.Object) func(py.Object) (py.Object, error) {
	return func(self py.Object) (py.Object, error) {
		switch v := self.(type) {
		case *addr:
			return fn(v.ip), nil
		case *netw:
			return fn(v.prefix.Addr()), nil
		case *iface:
			return fn(v.addr.ip), nil
		}
		return nil, pyErr(py.TypeError, "unsupported receiver")
	}
}

func init() {
	IPv4AddressType = py.NewTypeX("ipaddress.IPv4Address", "An IPv4 address.", addrNew, nil)
	IPv6AddressType = py.NewTypeX("ipaddress.IPv6Address", "An IPv6 address.", addrNew, nil)
	IPv4NetworkType = py.NewTypeX("ipaddress.IPv4Network", "An IPv4 network.", netwNew, nil)
	IPv6NetworkType = py.NewTypeX("ipaddress.IPv6Network", "An IPv6 network.", netwNew, nil)
	IPv4InterfaceType = py.NewTypeX("ipaddress.IPv4Interface", "An IPv4 interface.", ifaceNew, nil)
	IPv6InterfaceType = py.NewTypeX("ipaddress.IPv6Interface", "An IPv6 interface.", ifaceNew, nil)

	for _, t := range []*py.Type{IPv4AddressType, IPv6AddressType} {
		t.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", addrGetitem, 0,
			"Return the byte at the given index of the packed address."))
		t.Dict.Set("__len__", py.MustNewMethod("__len__", addrLen, 0, "The number of bytes."))
		t.Dict.Set("__str__", py.MustNewMethod("__str__", addrStrMethod, 0, "The address in its compressed form."))
		t.Dict.Set("__repr__", py.MustNewMethod("__repr__", addrReprMethod, 0, "The address in its compressed form."))
		t.Dict.Set("__hash__", py.MustNewMethod("__hash__", addrHash, 0, "The address hash."))
		t.Dict.Set("__int__", py.MustNewMethod("__int__", addrIntMethod, 0, "The address as an integer."))
		t.Dict.Set("__eq__", py.MustNewMethod("__eq__", addrEq, 0, "Address equality."))
		t.Dict.Set("__lt__", py.MustNewMethod("__lt__", addrLt, 0, "Address ordering."))
		t.Dict.Set("__le__", py.MustNewMethod("__le__", addrLe, 0, "Address ordering."))
		t.Dict.Set("__gt__", py.MustNewMethod("__gt__", addrGt, 0, "Address ordering."))
		t.Dict.Set("__ge__", py.MustNewMethod("__ge__", addrGe, 0, "Address ordering."))

		prop(t, "compressed", func(self py.Object) (py.Object, error) {
			return py.String(addrStr(asAddrIP(self))), nil
		})
		prop(t, "exploded", func(self py.Object) (py.Object, error) {
			return py.String(exploded(asAddrIP(self))), nil
		})
		prop(t, "packed", func(self py.Object) (py.Object, error) {
			return packedBytes(asAddrIP(self)), nil
		})
		prop(t, "reverse_pointer", func(self py.Object) (py.Object, error) {
			return py.String(reversePointer(asAddrIP(self))), nil
		})
		prop(t, "version", func(self py.Object) (py.Object, error) {
			return py.Int(versionOf(asAddrIP(self))), nil
		})
		prop(t, "max_prefixlen", func(self py.Object) (py.Object, error) {
			if versionOf(asAddrIP(self)) == 4 {
				return py.Int(32), nil
			}
			return py.Int(128), nil
		})
		prop(t, "is_private", addrProp("is_private", func(a netip.Addr) py.Object { return py.Bool(isPrivate(a)) }))
		prop(t, "is_global", addrProp("is_global", func(a netip.Addr) py.Object { return py.Bool(isGlobal(a)) }))
		prop(t, "is_reserved", addrProp("is_reserved", func(a netip.Addr) py.Object { return py.Bool(isReserved(a)) }))
		prop(t, "is_loopback", addrProp("is_loopback", func(a netip.Addr) py.Object { return py.Bool(isLoopback(a)) }))
		prop(t, "is_link_local", addrProp("is_link_local", func(a netip.Addr) py.Object { return py.Bool(isLinkLocal(a)) }))
		prop(t, "is_multicast", addrProp("is_multicast", func(a netip.Addr) py.Object { return py.Bool(isMulticast(a)) }))
		prop(t, "is_unspecified", addrProp("is_unspecified", func(a netip.Addr) py.Object { return py.Bool(isUnspecified(a)) }))
		prop(t, "is_site_local", func(self py.Object) (py.Object, error) {
			a := asAddrIP(self)
			if versionOf(a) == 4 {
				return py.False, nil
			}
			return py.Bool(netip.MustParsePrefix("fec0::/10").Contains(a)), nil
		})
		prop(t, "ipv4_mapped", func(self py.Object) (py.Object, error) {
			a := asAddrIP(self)
			if versionOf(a) == 4 {
				return py.None, nil
			}
			if u := a.Unmap(); u.Is4() && netip.MustParsePrefix("::ffff:0:0/96").Contains(a) {
				return newAddr(u), nil
			}
			return py.None, nil
		})
		prop(t, "sixtofour", func(self py.Object) (py.Object, error) {
			a := asAddrIP(self)
			if versionOf(a) == 4 || !netip.MustParsePrefix("2002::/16").Contains(a) {
				return py.None, nil
			}
			b := a.As16()
			return newAddr(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]})), nil
		})
	}

	for _, t := range []*py.Type{IPv4NetworkType, IPv6NetworkType} {
		t.Dict.Set("__str__", py.MustNewMethod("__str__", netwStr, 0, "The network address and prefix length."))
		t.Dict.Set("__repr__", py.MustNewMethod("__repr__", netwRepr, 0, "The network in its compressed form."))
		t.Dict.Set("__iter__", py.MustNewMethod("__iter__", netwIter, 0, "Iterate over the addresses in the network."))
		t.Dict.Set("__contains__", py.MustNewMethod("__contains__", netwContains, 0, "Whether the address is in the network."))
		t.Dict.Set("__hash__", py.MustNewMethod("__hash__", netwHash, 0, "The network hash."))
		t.Dict.Set("__eq__", py.MustNewMethod("__eq__", netwEq, 0, "Network equality."))
		t.Dict.Set("hosts", py.MustNewMethod("hosts", netwHosts, 0, "Iterate over the usable hosts in the network."))
		t.Dict.Set("subnets", py.MustNewMethod("subnets", netwSubnets, 0, "Iterate over the subnets of the network."))
		t.Dict.Set("supernet", py.MustNewMethod("supernet", netwSupernet, 0, "The supernet of the network."))
		t.Dict.Set("address_exclude", py.MustNewMethod("address_exclude", netwAddressEx, 0,
			"Remove a subnetwork from the network, yielding the remaining networks."))
		t.Dict.Set("supernet_of", py.MustNewMethod("supernet_of", netwIsSupernetOf, 0, "Whether other is a supernet."))
		t.Dict.Set("subnet_of", py.MustNewMethod("subnet_of", netwIsSubnetOf, 0, "Whether other is a subnet."))
		t.Dict.Set("overlaps", py.MustNewMethod("overlaps", netwOverlaps, 0, "Whether the two networks overlap."))
		prop(t, "version", func(self py.Object) (py.Object, error) {
			if self.(*netw).version() == 4 {
				return py.Int(4), nil
			}
			return py.Int(6), nil
		})
		prop(t, "network_address", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*netw).prefix.Masked().Addr()), nil
		})
		prop(t, "broadcast_address", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*netw).broadcast()), nil
		})
		prop(t, "netmask", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*netw).netmask()), nil
		})
		prop(t, "hostmask", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*netw).hostmask()), nil
		})
		prop(t, "prefixlen", func(self py.Object) (py.Object, error) {
			return py.Int(self.(*netw).prefix.Bits()), nil
		})
		prop(t, "with_prefixlen", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.String(fmt.Sprintf("%s/%d", addrStr(n.prefix.Addr()), n.prefix.Bits())), nil
		})
		prop(t, "compressed", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.String(fmt.Sprintf("%s/%d", addrStr(n.prefix.Addr()), n.prefix.Bits())), nil
		})
		prop(t, "with_netmask", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.String(fmt.Sprintf("%s/%s", addrStr(n.prefix.Addr()), addrStr(n.netmask()))), nil
		})
		prop(t, "with_hostmask", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.String(fmt.Sprintf("%s/%s", addrStr(n.prefix.Addr()), addrStr(n.hostmask()))), nil
		})
		prop(t, "num_addresses", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.IntFromString(pow2String(n.maxPrefixLen()-n.prefix.Bits()), 10)
		})
		prop(t, "is_private", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.Bool(isPrivate(n.prefix.Addr()) && isPrivate(n.broadcast())), nil
		})
		prop(t, "is_global", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.Bool(isGlobal(n.prefix.Addr()) && isGlobal(n.broadcast())), nil
		})
		prop(t, "is_reserved", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.Bool(isReserved(n.prefix.Addr()) && isReserved(n.broadcast())), nil
		})
		prop(t, "is_loopback", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.Bool(isLoopback(n.prefix.Addr()) && isLoopback(n.broadcast())), nil
		})
		prop(t, "is_link_local", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.Bool(isLinkLocal(n.prefix.Addr()) && isLinkLocal(n.broadcast())), nil
		})
		prop(t, "is_multicast", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.Bool(isMulticast(n.prefix.Addr()) && isMulticast(n.broadcast())), nil
		})
		prop(t, "is_unspecified", func(self py.Object) (py.Object, error) {
			n := self.(*netw)
			return py.Bool(isUnspecified(n.prefix.Addr()) && isUnspecified(n.broadcast())), nil
		})
		prop(t, "max_prefixlen", func(self py.Object) (py.Object, error) {
			if self.(*netw).version() == 4 {
				return py.Int(32), nil
			}
			return py.Int(128), nil
		})
	}

	for _, t := range []*py.Type{IPv4InterfaceType, IPv6InterfaceType} {
		t.Dict.Set("__str__", py.MustNewMethod("__str__", ifaceStr, 0, "The address with its prefix length."))
		t.Dict.Set("__repr__", py.MustNewMethod("__repr__", ifaceRepr, 0, "The interface in its compressed form."))
		prop(t, "ip", func(self py.Object) (py.Object, error) {
			return self.(*iface).addr, nil
		})
		prop(t, "network", func(self py.Object) (py.Object, error) {
			return self.(*iface).netobj, nil
		})
		prop(t, "version", func(self py.Object) (py.Object, error) {
			return py.Int(self.(*iface).addr.version), nil
		})
		prop(t, "with_prefixlen", func(self py.Object) (py.Object, error) {
			i := self.(*iface)
			return py.String(fmt.Sprintf("%s/%d", addrStr(i.addr.ip), i.netobj.prefix.Bits())), nil
		})
		prop(t, "compressed", func(self py.Object) (py.Object, error) {
			i := self.(*iface)
			return py.String(fmt.Sprintf("%s/%d", addrStr(i.addr.ip), i.netobj.prefix.Bits())), nil
		})
		prop(t, "with_netmask", func(self py.Object) (py.Object, error) {
			i := self.(*iface)
			return py.String(fmt.Sprintf("%s/%s", addrStr(i.addr.ip), addrStr(i.netobj.netmask()))), nil
		})
		prop(t, "with_hostmask", func(self py.Object) (py.Object, error) {
			i := self.(*iface)
			return py.String(fmt.Sprintf("%s/%s", addrStr(i.addr.ip), addrStr(i.netobj.hostmask()))), nil
		})
		prop(t, "network_address", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*iface).netobj.prefix.Masked().Addr()), nil
		})
		prop(t, "broadcast_address", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*iface).netobj.broadcast()), nil
		})
		prop(t, "netmask", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*iface).netobj.netmask()), nil
		})
		prop(t, "hostmask", func(self py.Object) (py.Object, error) {
			return newAddr(self.(*iface).netobj.hostmask()), nil
		})
		prop(t, "max_prefixlen", func(self py.Object) (py.Object, error) {
			if self.(*iface).addr.version == 4 {
				return py.Int(32), nil
			}
			return py.Int(128), nil
		})
		prop(t, "is_private", func(self py.Object) (py.Object, error) {
			return py.Bool(isPrivate(self.(*iface).addr.ip)), nil
		})
		prop(t, "is_global", func(self py.Object) (py.Object, error) {
			return py.Bool(isGlobal(self.(*iface).addr.ip)), nil
		})
		prop(t, "is_reserved", func(self py.Object) (py.Object, error) {
			return py.Bool(isReserved(self.(*iface).addr.ip)), nil
		})
		prop(t, "is_loopback", func(self py.Object) (py.Object, error) {
			return py.Bool(isLoopback(self.(*iface).addr.ip)), nil
		})
		prop(t, "is_link_local", func(self py.Object) (py.Object, error) {
			return py.Bool(isLinkLocal(self.(*iface).addr.ip)), nil
		})
		prop(t, "is_multicast", func(self py.Object) (py.Object, error) {
			return py.Bool(isMulticast(self.(*iface).addr.ip)), nil
		})
		prop(t, "is_unspecified", func(self py.Object) (py.Object, error) {
			return py.Bool(isUnspecified(self.(*iface).addr.ip)), nil
		})
	}

	globals := py.NewStringDict()
	globals.Set("IPv4Address", IPv4AddressType)
	globals.Set("IPv6Address", IPv6AddressType)
	globals.Set("IPv4Network", IPv4NetworkType)
	globals.Set("IPv6Network", IPv6NetworkType)
	globals.Set("IPv4Interface", IPv4InterfaceType)
	globals.Set("IPv6Interface", IPv6InterfaceType)
	globals.Set("AddressValueError", AddressValueError)
	globals.Set("NetmaskValueError", NetmaskValueError)
	globals.Set("ip_address", py.MustNewMethod("ip_address", ipAddressFunc, 0,
		"Build an IPv4Address or IPv6Address from a string or integer."))
	globals.Set("ip_network", py.MustNewMethod("ip_network", ipNetworkFunc, 0,
		"Build an IPv4Network or IPv6Network from a string or integer."))
	globals.Set("ip_interface", py.MustNewMethod("ip_interface", ipInterfaceFunc, 0,
		"Build an IPv4Interface or IPv6Interface from a string."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "ipaddress",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// asAddrIP extracts the address from any of the wrapper kinds.
func asAddrIP(self py.Object) netip.Addr {
	switch v := self.(type) {
	case *addr:
		return v.ip
	case *netw:
		return v.prefix.Addr()
	case *iface:
		return v.addr.ip
	}
	return netip.Addr{}
}

func ipAddressFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var address py.Object
	if err := py.UnpackTuple(args, kwargs, "ip_address", 1, 1, &address); err != nil {
		return nil, err
	}
	ip, _, err := addrArg(address)
	if err != nil {
		return nil, err
	}
	return newAddr(ip), nil
}

func ipNetworkFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		address py.Object
		strict  py.Object = py.True
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:ip_network",
		[]string{"address", "strict"}, &address, &strict); err != nil {
		return nil, err
	}
	// ip_network accepts an address with no prefix, which means a full-length
	// network.
	if s, ok := address.(py.String); ok && !strings.Contains(string(s), "/") {
		ip, version, err := addrArg(address)
		if err != nil {
			return nil, err
		}
		plen := 32
		if version == 6 {
			plen = 128
		}
		prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", addrStr(ip), plen))
		if err != nil {
			return nil, pyErr(py.ValueError, "%s", err.Error())
		}
		return &netw{prefix: prefix, strict: truthy(strict), Dict: py.NewStringDict()}, nil
	}
	if n, ok := address.(*netw); ok {
		return &netw{prefix: n.prefix, strict: truthy(strict), Dict: py.NewStringDict()}, nil
	}
	if a, ok := address.(*addr); ok {
		plen := 32
		if a.version == 6 {
			plen = 128
		}
		prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", addrStr(a.ip), plen))
		if err != nil {
			return nil, pyErr(py.ValueError, "%s", err.Error())
		}
		return &netw{prefix: prefix, strict: truthy(strict), Dict: py.NewStringDict()}, nil
	}
	// A string with a slash, or a network/interface.
	s, err := py.StrAsString(address)
	if err != nil {
		return nil, err
	}
	t := IPv4NetworkType
	if strings.Contains(s, ":") {
		t = IPv6NetworkType
	}
	return netwNew(t, py.Tuple{address, strict}, py.NewStringDict())
}

func ipInterfaceFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var address py.Object
	if err := py.UnpackTuple(args, kwargs, "ip_interface", 1, 1, &address); err != nil {
		return nil, err
	}
	s, ok := address.(py.String)
	if !ok {
		return nil, pyErr(py.TypeError, "ip_interface() requires a string")
	}
	t := IPv4InterfaceType
	if strings.Contains(string(s), ":") {
		t = IPv6InterfaceType
	}
	return ifaceNew(t, args, kwargs)
}
