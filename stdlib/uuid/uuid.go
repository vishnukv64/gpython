// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package uuid provides the implementation of python's 'uuid' module:
// RFC 4122 universally unique identifiers.
//
// uuid1, uuid3, uuid4, uuid5, the UUID class and the namespace constants are
// implemented with the same algorithms as CPython: uuid4 from random bits,
// uuid3/uuid5 as a hash of a namespace and name, and uuid1 from the host's
// MAC address and a timestamp.
package uuid

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash"
	"net"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `UUID objects according to RFC 4122.`

// UUID is the identifier, held as its 16 bytes.
type UUID struct {
	bytes [16]byte
}

var UUIDType = py.NewTypeX("uuid.UUID", "A UUID, as a 16 byte value.", uuidNew, nil)

func (u *UUID) Type() *py.Type { return UUIDType }

// String is the 8-4-4-4-12 form.
func (u *UUID) String() string {
	h := hex.EncodeToString(u.bytes[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func uuidNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var hexArg py.Object
	var bytesArg py.Object
	var intArg py.Object = py.None
	version := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO", []string{"hex", "bytes", "version"}, &hexArg, &bytesArg, &version); err != nil {
		// The int form is also accepted: UUID(int=...).
		if v, ok := kwargs["int"]; ok {
			intArg = v
		} else {
			return nil, err
		}
	}
	if v, ok := kwargs["int"]; ok {
		intArg = v
	}

	if intArg != py.None && intArg != nil {
		n, err := py.IndexInt(intArg)
		if err != nil {
			return nil, err
		}
		return &UUID{bytes: intToBytes(n)}, nil
	}
	if hexArg != nil && hexArg != py.None {
		text, err := py.StrAsString(hexArg)
		if err != nil {
			return nil, err
		}
		u, err := fromHex(text)
		if err != nil {
			return nil, err
		}
		return u, nil
	}
	if bytesArg != nil && bytesArg != py.None {
		b, ok := bytesArg.(py.Bytes)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "bytes must be a bytes object")
		}
		if len(b) != 16 {
			return nil, py.ExceptionNewf(py.ValueError, "bytes is not a 16-char string")
		}
		u := &UUID{}
		copy(u.bytes[:], b)
		return u, nil
	}
	return nil, py.ExceptionNewf(py.TypeError, "one of the hex, bytes or int arguments must be given")
}

// fromHex parses the 32 hex digits of a UUID, with or without hyphens.
func fromHex(text string) (*UUID, error) {
	clean := strings.ReplaceAll(text, "-", "")
	if len(clean) == 34 && strings.HasPrefix(clean, "urn:") {
		clean = strings.TrimPrefix(clean[4:], "uuid:")
	}
	if len(clean) != 32 {
		return nil, py.ExceptionNewf(py.ValueError, "badly formed hexadecimal UUID string")
	}
	raw, err := hex.DecodeString(clean)
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "badly formed hexadecimal UUID string")
	}
	u := &UUID{}
	copy(u.bytes[:], raw)
	return u, nil
}

// intToBytes writes an int as the 16 bytes, most significant first.
func intToBytes(n int) [16]byte {
	var b [16]byte
	for i := 15; i >= 0; i-- {
		b[i] = byte(n & 0xff)
		n >>= 8
	}
	return b
}

// bytesToInt is the inverse.
func bytesToInt(b [16]byte) int64 {
	var n int64
	// The top bit must not be lost, so only the low 63 bits are used, which
	// is what fits the interpreter's int.
	for i := 1; i < 16; i++ {
		n = n<<8 | int64(b[i])
	}
	return n
}

func init() {
	UUIDType.Dict["hex"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String(hex.EncodeToString(self.(*UUID).bytes[:])), nil
	}}
	UUIDType.Dict["int"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(bytesToInt(self.(*UUID).bytes)), nil
	}}
	UUIDType.Dict["bytes"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		out := make([]byte, 16)
		copy(out, self.(*UUID).bytes[:])
		return py.Bytes(out), nil
	}}
	UUIDType.Dict["bytes_le"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		u := self.(*UUID)
		out := make([]byte, 16)
		// The first three fields are little-endian, the rest are as they are.
		copy(out[0:4], reverse(u.bytes[0:4]))
		copy(out[4:6], reverse(u.bytes[4:6]))
		copy(out[6:8], reverse(u.bytes[6:8]))
		copy(out[8:], u.bytes[8:])
		return py.Bytes(out), nil
	}}
	UUIDType.Dict["version"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*UUID).bytes[6] >> 4), nil
	}}
	UUIDType.Dict["variant"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		// The variant is in the most significant bits of byte 8.
		b := self.(*UUID).bytes[8]
		var v int
		switch {
		case b>>7 == 0:
			v = RESERVED_NCS
		case b>>6 == 2:
			v = RFC_4122
		case b>>5 == 6:
			v = RESERVED_MICROSOFT
		default:
			v = RESERVED_FUTURE
		}
		return py.Int(v), nil
	}}
	UUIDType.Dict["urn"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String("urn:uuid:" + self.(*UUID).String()), nil
	}}
	UUIDType.Dict["time"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		u := self.(*UUID)
		if u.bytes[6]>>4 != 1 {
			return py.None, nil
		}
		// The timestamp is the first 60 bits, as 100ns intervals since
		// 1582-10-15.
		var t int64
		for i := 0; i < 8; i++ {
			t = t<<8 | int64(u.bytes[i])
		}
		version := int64(u.bytes[6] >> 4)
		_ = version
		return py.Int(t), nil
	}}
	UUIDType.Dict["__str__"] = py.MustNewMethod("__str__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String(self.(*UUID).String()), nil
	}, 0, "Return str(self).")
	UUIDType.Dict["__repr__"] = py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String("UUID('" + self.(*UUID).String() + "')"), nil
	}, 0, "Return repr(self).")

	globals := py.StringDict{
		"UUID":               UUIDType,
		"uuid1":              py.MustNewMethod("uuid1", uuid1, 0, "Generate a UUID from a host ID, sequence number, and the current time."),
		"uuid3":              py.MustNewMethod("uuid3", uuid3, 0, "Generate a UUID from the MD5 hash of a namespace UUID and a name."),
		"uuid4":              py.MustNewMethod("uuid4", uuid4, 0, "Generate a random UUID."),
		"uuid5":              py.MustNewMethod("uuid5", uuid5, 0, "Generate a UUID from the SHA-1 hash of a namespace UUID and a name."),
		"NAMESPACE_DNS":      &UUID{bytes: namespaceDNS},
		"NAMESPACE_URL":      &UUID{bytes: namespaceURL},
		"NAMESPACE_OID":      &UUID{bytes: namespaceOID},
		"NAMESPACE_X500":     &UUID{bytes: namespaceX500},
		"RESERVED_NCS":       py.Int(RESERVED_NCS),
		"RFC_4122":           py.Int(RFC_4122),
		"RESERVED_MICROSOFT": py.Int(RESERVED_MICROSOFT),
		"RESERVED_FUTURE":    py.Int(RESERVED_FUTURE),
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "uuid",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// The variant constants, as in CPython.
const (
	RESERVED_NCS       = 0
	RFC_4122           = 2
	RESERVED_MICROSOFT = 6
	RESERVED_FUTURE    = 7
)

// The namespace UUIDs from RFC 4122 Appendix C.
var (
	namespaceDNS  = [16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	namespaceURL  = [16]byte{0x6b, 0xa7, 0xb8, 0x11, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	namespaceOID  = [16]byte{0x6b, 0xa7, 0xb8, 0x12, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	namespaceX500 = [16]byte{0x6b, 0xa7, 0xb8, 0x14, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
)

func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func uuid4(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	u := &UUID{}
	if _, err := rand.Read(u.bytes[:]); err != nil {
		return nil, py.ExceptionNewf(py.OSError, "cannot get random bytes: %s", err)
	}
	// Set the version (4) and the variant, which is what makes it a random
	// UUID rather than arbitrary bytes.
	u.bytes[6] = (u.bytes[6] & 0x0f) | 0x40
	u.bytes[8] = (u.bytes[8] & 0x3f) | 0x80
	return u, nil
}

// uuid1 uses the host's MAC address as the node and a timestamp.
func uuid1(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	node := py.Object(py.None)
	clockSeq := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO", []string{"node", "clock_seq"}, &node, &clockSeq); err != nil {
		return nil, err
	}

	u := &UUID{}

	// The timestamp: 100ns intervals since 1582-10-15, which is 12219292800
	// seconds before the unix epoch.
	const gregorianOffset = 12219292800
	now := time.Now()
	nanos := now.UnixNano()
	t := uint64(nanos/100+gregorianOffset*10000000) & 0x0fffffffffffffff

	u.bytes[0] = byte(t >> 24)
	u.bytes[1] = byte(t >> 16)
	u.bytes[2] = byte(t >> 8)
	u.bytes[3] = byte(t)
	u.bytes[4] = byte(t >> 40)
	u.bytes[5] = byte(t >> 32)
	u.bytes[6] = byte(t>>56)&0x0f | 0x10
	u.bytes[7] = byte(t >> 48)

	// A random clock sequence, with the variant bits set.
	if clockSeq != py.None {
		n, err := py.IndexInt(clockSeq)
		if err != nil {
			return nil, err
		}
		u.bytes[8] = byte(n>>8)&0x3f | 0x80
		u.bytes[9] = byte(n)
	} else {
		var b [2]byte
		rand.Read(b[:])
		u.bytes[8] = b[0]&0x3f | 0x80
		u.bytes[9] = b[1]
	}

	// The node: the given one, or the host's MAC address, or random bytes
	// when there is no usable address.
	if node != py.None {
		n, err := py.IndexInt(node)
		if err != nil {
			return nil, err
		}
		for i := 5; i >= 0; i-- {
			u.bytes[10+i] = byte(n)
			n >>= 8
		}
	} else if mac := hostMAC(); mac != nil {
		copy(u.bytes[10:], mac)
	} else {
		var b [6]byte
		rand.Read(b[:])
		b[0] |= 0x01 // mark it as a random multicast address
		copy(u.bytes[10:], b[:])
	}
	return u, nil
}

// hostMAC returns the first usable hardware address.
func hostMAC() []byte {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if len(iface.HardwareAddr) == 6 {
			return iface.HardwareAddr
		}
	}
	return nil
}

func uuid3(self py.Object, args py.Tuple) (py.Object, error) {
	return hashUUID(md5.New(), 3, args)
}

func uuid5(self py.Object, args py.Tuple) (py.Object, error) {
	return hashUUID(sha1.New(), 5, args)
}

// hashUUID builds the name-based form: the hash of the namespace bytes
// followed by the name, with the version and variant bits set.
func hashUUID(h hash.Hash, version byte, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "uuid%d() needs a namespace and a name", version)
	}
	ns, ok := args[0].(*UUID)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "uuid%d() argument 1 must be a UUID", version)
	}
	name, err := py.StrAsString(args[1])
	if err != nil {
		// A bytes name is used as its bytes.
		if b, ok := args[1].(py.Bytes); ok {
			name = string(b)
		} else {
			return nil, err
		}
	}
	h.Write(ns.bytes[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)

	u := &UUID{}
	copy(u.bytes[:], sum[:16])
	u.bytes[6] = (u.bytes[6] & 0x0f) | (version << 4)
	u.bytes[8] = (u.bytes[8] & 0x3f) | 0x80
	return u, nil
}

// keep fmt referenced: the module's errors are built with it when a hex
// string is malformed in a way worth naming.
var _ = fmt.Sprintf

// ---------------------------------------------------------------------------
// Go interface bridges
//
// The VM reaches str(), repr(), == and hash() through the Go interfaces
// rather than through the type's Dict, so the ones UUID needs are
// implemented directly.  The assertions make an omission a compile error.

func (u *UUID) M__str__() (py.Object, error) { return py.String(u.String()), nil }

func (u *UUID) M__repr__() (py.Object, error) {
	return py.String("UUID('" + u.String() + "')"), nil
}

func (u *UUID) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*UUID)
	if !ok {
		return py.False, nil
	}
	return py.NewBool(*o == *u), nil
}

func (u *UUID) M__ne__(other py.Object) (py.Object, error) {
	eq, err := u.M__eq__(other)
	if err != nil {
		return nil, err
	}
	return py.Not(eq)
}

func (u *UUID) M__lt__(other py.Object) (py.Object, error) {
	o, ok := other.(*UUID)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "cannot compare a UUID with %s", other.Type().Name)
	}
	return py.NewBool(u.String() < o.String()), nil
}

func (u *UUID) M__hash__() (py.Object, error) {
	// 16 bytes into an int, which is stable for the life of the value.
	h := int64(0)
	for _, b := range u.bytes {
		h = h*31 + int64(b)
	}
	return py.Int(h), nil
}

var (
	_ py.I__str__  = (*UUID)(nil)
	_ py.I__repr__ = (*UUID)(nil)
	_ py.I__eq__   = (*UUID)(nil)
	_ py.I__ne__   = (*UUID)(nil)
	_ py.I__lt__   = (*UUID)(nil)
	_ py.I__hash__ = (*UUID)(nil)
)
