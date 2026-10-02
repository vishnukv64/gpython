// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hmac provides the implementation of python's 'hmac' module.
//
// HMAC is implemented here directly per RFC 2104 rather than through Go's
// crypto/hmac, because crypto/hmac exposes neither the key nor a way to clone
// an in-progress MAC, and CPython's hmac.HMAC has a working copy().  Building
// it from the digest constructors in crypto/md5, crypto/sha1, crypto/sha256,
// crypto/sha512 and crypto/sha3 gives byte-for-byte the same MACs as CPython
// and makes copy() possible: each of those digests implements
// encoding.BinaryMarshaler, so the inner state can be marshalled into a fresh
// hash.
package hmac

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"crypto/subtle"
	"hash"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `HMAC (Keyed-Hashing for Message Authentication).

This module implements the HMAC algorithm as described by RFC 2104.

    new(key, msg=None, digestmod='')
        Return a new hmac object.

    digest(key, msg, digestmod)
        Return the digest of the bytes given as msg.

    compare_digest(a, b)
        Return a == b, in a way that is not vulnerable to timing analysis.
`

// hmacType is the Python-visible HMAC object type.
var hmacType = py.NewTypeX("hmac.HMAC",
	"An HMAC object: update(), digest(), hexdigest(), copy().", nil, nil)

// digestAlgos maps a digestmod name to its constructor.  CPython accepts any
// hash name hashlib knows; these are the ones the interpreter can compute.
var digestAlgos = map[string]func() hash.Hash{
	"md5":      func() hash.Hash { return md5.New() },
	"sha1":     func() hash.Hash { return sha1.New() },
	"sha224":   func() hash.Hash { return sha256.New224() },
	"sha256":   func() hash.Hash { return sha256.New() },
	"sha384":   func() hash.Hash { return sha512.New384() },
	"sha512":   func() hash.Hash { return sha512.New() },
	"sha3_224": func() hash.Hash { return sha3.New224() },
	"sha3_256": func() hash.Hash { return sha3.New256() },
	"sha3_384": func() hash.Hash { return sha3.New384() },
	"sha3_512": func() hash.Hash { return sha3.New512() },
}

// hmacObject is a Python HMAC object.  key is kept because copy() rebuilds the
// outer hash from it and the marshalled inner state.
type hmacObject struct {
	ctor  func() hash.Hash
	name  string
	key   []byte
	inner hash.Hash
	outer hash.Hash
}

func (h *hmacObject) Type() *py.Type { return hmacType }

func (h *hmacObject) M__repr__() (py.Object, error) {
	return py.String("<hmac.HMAC object>"), nil
}

// newHMAC builds the ipad/opad pair and folds msg (when given) into the inner
// digest, exactly as RFC 2104.
func newHMAC(ctor func() hash.Hash, name string, key, msg []byte) *hmacObject {
	h := &hmacObject{ctor: ctor, name: name, key: append([]byte(nil), key...)}
	blockSize := ctor().BlockSize()
	k := h.key
	if len(k) > blockSize {
		kh := ctor()
		kh.Write(k)
		k = kh.Sum(nil)
	}
	pad := make([]byte, blockSize)
	copy(pad, k)
	ipad := make([]byte, blockSize)
	opad := make([]byte, blockSize)
	for i := 0; i < blockSize; i++ {
		ipad[i] = pad[i] ^ 0x36
		opad[i] = pad[i] ^ 0x5c
	}
	h.inner = ctor()
	h.inner.Write(ipad)
	h.outer = ctor()
	h.outer.Write(opad)
	if len(msg) > 0 {
		h.inner.Write(msg)
	}
	return h
}

// clone returns an independent copy with the same in-progress state.  Each
// supported digest implements encoding.BinaryMarshaler, so the inner state
// round-trips; the outer hash has absorbed only the opad and is rebuilt.
func (h *hmacObject) clone() (*hmacObject, error) {
	mar, ok := h.inner.(interface{ MarshalBinary() ([]byte, error) })
	if !ok {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"hmac.HMAC.copy: the %s digest cannot serialise its state, so the copy cannot be built", h.name)
	}
	state, err := mar.MarshalBinary()
	if err != nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "cannot copy this HMAC object: %s", err)
	}
	c := &hmacObject{ctor: h.ctor, name: h.name, key: append([]byte(nil), h.key...)}
	c.inner = h.ctor()
	unmar, ok := c.inner.(interface{ UnmarshalBinary([]byte) error })
	if !ok {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"hmac.HMAC.copy: the %s digest cannot restore its state, so the copy cannot be built", h.name)
	}
	if err := unmar.UnmarshalBinary(state); err != nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "cannot copy this HMAC object: %s", err)
	}
	c.outer = h.ctor()
	blockSize := h.ctor().BlockSize()
	k := c.key
	if len(k) > blockSize {
		kh := h.ctor()
		kh.Write(k)
		k = kh.Sum(nil)
	}
	opad := make([]byte, blockSize)
	for i := 0; i < blockSize; i++ {
		v := byte(0)
		if i < len(k) {
			v = k[i]
		}
		opad[i] = v ^ 0x5c
	}
	c.outer.Write(opad)
	return c, nil
}

// sum returns the MAC for the current inner state without disturbing it.
func (h *hmacObject) sum() ([]byte, error) {
	cl, err := h.clone()
	if err != nil {
		return nil, err
	}
	cl.outer.Write(cl.inner.Sum(nil))
	return cl.outer.Sum(nil), nil
}

// lookupAlgo resolves digestmod to a digest constructor.  A string names the
// algorithm; a callable hashlib-style constructor is called to discover the
// algorithm; a module with a "new" attribute is consulted.
func lookupAlgo(digestmod py.Object) (func() hash.Hash, string, error) {
	switch d := digestmod.(type) {
	case py.String:
		name := string(d)
		f, ok := digestAlgos[name]
		if !ok {
			return nil, "", py.ExceptionNewf(py.ValueError, "unsupported hash type %s", name)
		}
		return f, name, nil
	case *py.Module:
		fn, err := py.GetAttrString(d, "new")
		if err != nil {
			return nil, "", py.ExceptionNewf(py.ValueError, "unsupported digestmod")
		}
		res, err := py.Call(fn, py.Tuple{py.String("md5")}, py.NewStringDict())
		if err != nil {
			return nil, "", err
		}
		return lookupAlgo(res)
	}
	// A callable (a hashlib constructor, or a class): call it and read .name.
	if _, err := py.GetAttrString(digestmod, "__call__"); err == nil {
		res, err := py.Call(digestmod, py.Tuple{}, py.NewStringDict())
		if err != nil {
			return nil, "", err
		}
		nameObj, err := py.GetAttrString(res, "name")
		if err != nil {
			return nil, "", py.ExceptionNewf(py.ValueError, "unsupported digestmod")
		}
		return lookupAlgo(nameObj)
	}
	return nil, "", py.ExceptionNewf(py.TypeError,
		"digestmod must be a string naming a hash algorithm")
}

func hmacNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var key py.Object
	var msg py.Object
	var digestmod py.Object = py.String("")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:new",
		[]string{"key", "msg", "digestmod"}, &key, &msg, &digestmod); err != nil {
		return nil, err
	}
	keyBytes, err := py.BytesFromObject(key)
	if err != nil {
		return nil, err
	}
	if digestmod == py.None || digestmod == py.String("") {
		return nil, py.ExceptionNewf(py.TypeError,
			"new() missing required argument: 'digestmod' (this interpreter has no default digest "+
				"registry, so digestmod must be given explicitly)")
	}
	ctor, name, err := lookupAlgo(digestmod)
	if err != nil {
		return nil, err
	}
	var msgBytes []byte
	if msg != nil && msg != py.None {
		b, err := py.BytesFromObject(msg)
		if err != nil {
			return nil, err
		}
		msgBytes = b
	}
	return newHMAC(ctor, name, keyBytes, msgBytes), nil
}

func hmacUpdate(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*hmacObject)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "update() takes exactly one argument")
	}
	b, err := py.BytesFromObject(args[0])
	if err != nil {
		return nil, err
	}
	h.inner.Write(b)
	return py.None, nil
}

func hmacDigest(self py.Object, args py.Tuple) (py.Object, error) {
	sum, err := self.(*hmacObject).sum()
	if err != nil {
		return nil, err
	}
	return py.Bytes(sum), nil
}

func hmacHexdigest(self py.Object, args py.Tuple) (py.Object, error) {
	sum, err := self.(*hmacObject).sum()
	if err != nil {
		return nil, err
	}
	const hex = "0123456789abcdef"
	out := make([]byte, 0, len(sum)*2)
	for _, b := range sum {
		out = append(out, hex[b>>4], hex[b&0x0f])
	}
	return py.String(string(out)), nil
}

func hmacCopy(self py.Object, args py.Tuple) (py.Object, error) {
	c, err := self.(*hmacObject).clone()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func hmacDigestFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var key, msg, digestmod py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOO:digest",
		[]string{"key", "msg", "digestmod"}, &key, &msg, &digestmod); err != nil {
		return nil, err
	}
	h, err := hmacNew(nil, py.Tuple{key, msg, digestmod}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	return hmacDigest(h, nil)
}

func hmacCompareDigest(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "compare_digest() takes exactly two arguments")
	}
	a, err := py.BytesFromObject(args[0])
	if err != nil {
		return nil, err
	}
	b, err := py.BytesFromObject(args[1])
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 {
		return py.Bool(true), nil
	}
	return py.Bool(false), nil
}

func init() {
	hmacType.Dict.Set("update", py.MustNewMethod("update", hmacUpdate, 0,
		"Update this hashing object with the string msg."))
	hmacType.Dict.Set("digest", py.MustNewMethod("digest", hmacDigest, 0,
		"Return the digest value as a bytes object."))
	hmacType.Dict.Set("hexdigest", py.MustNewMethod("hexdigest", hmacHexdigest, 0,
		"Return the digest value as a string of hexadecimal digits."))
	hmacType.Dict.Set("copy", py.MustNewMethod("copy", hmacCopy, 0,
		"Return a copy of the HMAC object."))
	hmacType.Dict.Set("name", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String("hmac-" + self.(*hmacObject).name), nil
	}})
	hmacType.Dict.Set("digest_size", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*hmacObject).ctor().Size()), nil
	}})
	hmacType.Dict.Set("block_size", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*hmacObject).ctor().BlockSize()), nil
	}})

	globals := py.NewStringDict()
	globals.Set("new", py.MustNewMethod("new", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return hmacNew(hmacType, args, kw)
	}, 0, "new(key, msg=None, digestmod='') - return a new hmac object."))
	globals.Set("HMAC", hmacType)
	globals.Set("digest", py.MustNewMethod("digest", hmacDigestFn, 0,
		"digest(key, msg, digestmod) - return the digest of msg."))
	globals.Set("compare_digest", py.MustNewMethod("compare_digest", hmacCompareDigest, 0,
		"compare_digest(a, b) - return a == b in constant time."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "hmac",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
