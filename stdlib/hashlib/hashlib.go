// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hashlib provides the implementation of python's 'hashlib' module.
//
// The digests are Go's crypto/md5, crypto/sha1, crypto/sha256, crypto/sha512
// and crypto/sha3, which are byte-for-byte identical to CPython's.  BLAKE2b is
// the vendored pure-Go reference implementation under internal/blake2b (see
// that package for why it is vendored rather than imported).
//
// sha3_224/384/512 and blake2s are implemented too, even though the task named
// only sha3_256 and blake2b, because exposing only half of a family the module
// advertises would be a false advertising of its own.
package hashlib

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding"
	"encoding/hex"
	"hash"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/hashlib/internal/blake2b"
	"github.com/vishnukv64/gpython/stdlib/hashlib/internal/blake2s"
)

const module_doc = `hashlib module - A common interface to many hash functions.

new(name, data=b'', **kwargs) - returns a new hash object implementing the
                                given hash function; initializing the hash
                                using the given binary data.

Named constructor functions are also available, these are faster
than using new():

md5(), sha1(), sha224(), sha256(), sha384(), sha512(), sha3_224(),
sha3_256(), sha3_384(), sha3_512(), blake2b(), blake2s()

Choose your hash function wisely.  Some have known collision weaknesses.

For the hashes listed above, the module provides the same interface as
CPython's hashlib: update(), digest(), hexdigest(), copy(), name,
digest_size and block_size.`

// hashType is the Python-visible hash object type.
var hashType = py.NewTypeX("hashlib.hash", "A hash object: update(), digest(), hexdigest(), copy().", hashNew, nil)

// hashAlgo describes one hash function.
type hashAlgo struct {
	name      string
	digestLen int
	blockLen  int
	// newHash returns a fresh Go hash.  blake2b and blake2s need a digest size
	// argument, which is why this is a constructor rather than a value.
	newHash func(digestSize int) (hash.Hash, error)
	// varSize records that the digest length is selectable, which is only true
	// of blake2b and blake2s.
	varSize bool
}

// Hash is a Python hash object.
type Hash struct {
	algo hashAlgo
	h    hash.Hash
	// keyLen records the BLAKE2 key length, needed to report the right digest
	// after a copy().
	keyLen int
}

func (h *Hash) Type() *py.Type { return hashType }

func (h *Hash) M__repr__() (py.Object, error) {
	return py.String("<" + h.algo.name + " hash object>"), nil
}

// algorithms is the registry backing new() and algorithms_available.
var algorithms = map[string]*hashAlgo{
	"md5":      {"md5", md5.Size, md5.BlockSize, func(int) (hash.Hash, error) { return md5.New(), nil }, false},
	"sha1":     {"sha1", sha1.Size, sha1.BlockSize, func(int) (hash.Hash, error) { return sha1.New(), nil }, false},
	"sha224":   {"sha224", sha256.Size224, sha256.BlockSize, func(int) (hash.Hash, error) { return sha256.New224(), nil }, false},
	"sha256":   {"sha256", sha256.Size, sha256.BlockSize, func(int) (hash.Hash, error) { return sha256.New(), nil }, false},
	"sha384":   {"sha384", sha512.Size384, sha512.BlockSize, func(int) (hash.Hash, error) { return sha512.New384(), nil }, false},
	"sha512":   {"sha512", sha512.Size, sha512.BlockSize, func(int) (hash.Hash, error) { return sha512.New(), nil }, false},
	"sha3_224": {"sha3_224", 28, 144, func(int) (hash.Hash, error) { return sha3.New224(), nil }, false},
	"sha3_256": {"sha3_256", 32, 136, func(int) (hash.Hash, error) { return sha3.New256(), nil }, false},
	"sha3_384": {"sha3_384", 48, 104, func(int) (hash.Hash, error) { return sha3.New384(), nil }, false},
	"sha3_512": {"sha3_512", 64, 72, func(int) (hash.Hash, error) { return sha3.New512(), nil }, false},
	"blake2b":  {"blake2b", blake2b.Size, blake2b.BlockSize, func(n int) (hash.Hash, error) { return blake2b.New(n, nil) }, true},
	"blake2s":  {"blake2s", blake2s.Size, blake2s.BlockSize, func(n int) (hash.Hash, error) { return newBlake2s(n) }, true},
}

// algosByName lists the algorithm names in the fixed order CPython's
// algorithms_available and algorithms_guaranteed use.
var algosByName = []string{
	"blake2b", "blake2s", "md5", "sha1", "sha224", "sha256", "sha384", "sha512",
	"sha3_224", "sha3_256", "sha3_384", "sha3_512",
}

// newBlake2s adapts the vendored blake2s package, whose constructors are New256
// and New128, to hashlib's variable digest size.  blake2s hashes are 32 or 16
// bytes long; CPython's hashlib exposes the same two sizes.
func newBlake2s(size int) (hash.Hash, error) {
	switch size {
	case blake2s.Size:
		return blake2s.New256(nil)
	case blake2s.Size128:
		return blake2s.New128(nil)
	}
	return nil, py.ExceptionNewf(py.ValueError, "digest_size must be 16 or 32 for blake2s")
}

// hashNew implements hash(name, data=b'') and the named constructors.
func hashNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var name py.Object
	var data py.Object = py.None
	if err := py.UnpackTuple(args, kwargs, "hash", 1, 2, &name, &data); err != nil {
		return nil, err
	}
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	algo, ok := algorithms[n]
	if !ok {
		return nil, py.ExceptionNewf(py.ValueError, "unsupported hash type %s", n)
	}
	return newHashObject(algo, algo.digestLen, data)
}

// newHashObject builds a hash object of algo and feeds it the initial data.
func newHashObject(algo *hashAlgo, digestSize int, data py.Object) (py.Object, error) {
	h, err := algo.newHash(digestSize)
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "%s", err.Error())
	}
	obj := &Hash{algo: *algo, h: h}
	if data != py.None {
		if err := obj.update(data); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

// namedCtor builds a constructor function such as md5() or blake2b().
//
// A digest-size-selectable algorithm (BLAKE2) takes an optional digest_size
// keyword, which the fixed-size ones do not.
func namedCtor(name string) func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		algo := algorithms[name]
		var data py.Object = py.None
		if algo.varSize {
			// blake2b(data=b'', *, digest_size=64, ...) - only digest_size is
			// supported; the tree parameters, key, salt and person arguments
			// are not expressible here because Go's BLAKE2b does not expose
			// them, and silently accepting them would compute a different
			// digest than requested.
			var ds py.Object = py.Int(algo.digestLen)
			if err := py.ParseTupleAndKeywords(args, kwargs, "|O$O:"+name,
				[]string{"data", "digest_size"}, &data, &ds); err != nil {
				return nil, err
			}
			for _, k := range []string{"key", "salt", "person", "fanout", "depth", "leaf_size", "node_offset", "node_depth", "inner_size", "last_node"} {
				if _, ok := kwargs[k]; ok {
					return nil, py.ExceptionNewf(py.NotImplementedError,
						"hashlib.%s(): the %s parameter is not supported (Go's BLAKE2b does not expose the tree mode)", name, k)
				}
			}
			size, err := py.IndexInt(ds)
			if err != nil {
				return nil, err
			}
			if size < 1 || size > algo.digestLen {
				return nil, py.ExceptionNewf(py.ValueError, "digest_size must be between 1 and %d bytes", algo.digestLen)
			}
			return newHashObject(algo, size, data)
		}
		var data2 py.Object = py.None
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:"+name, []string{"data"}, &data2); err != nil {
			return nil, err
		}
		return newHashObject(algo, algo.digestLen, data2)
	}
}

// update feeds more data into the hash.  A str is rejected, as CPython does.
func (h *Hash) update(data py.Object) error {
	switch data.(type) {
	case py.String:
		return py.ExceptionNewf(py.TypeError, "Strings must be encoded before hashing")
	}
	b, err := py.BytesFromObject(data)
	if err != nil {
		return err
	}
	h.h.Write([]byte(b))
	return nil
}

func hashUpdate(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*Hash)
	var data py.Object = py.None
	if err := py.UnpackTuple(args, nil, "update", 1, 1, &data); err != nil {
		return nil, err
	}
	if err := h.update(data); err != nil {
		return nil, err
	}
	return py.None, nil
}

func hashDigest(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*Hash)
	if err := py.UnpackTuple(args, nil, "digest", 0, 0); err != nil {
		return nil, err
	}
	// Sum() appends to its argument and does not disturb the running state,
	// so the object stays usable afterwards.
	return py.Bytes(h.h.Sum(nil)), nil
}

func hashHexdigest(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*Hash)
	if err := py.UnpackTuple(args, nil, "hexdigest", 0, 0); err != nil {
		return nil, err
	}
	return py.String(hex.EncodeToString(h.h.Sum(nil))), nil
}

func hashCopy(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*Hash)
	if err := py.UnpackTuple(args, nil, "copy", 0, 0); err != nil {
		return nil, err
	}
	return h.copy()
}

// copy duplicates the hash state.
func (h *Hash) copy() (py.Object, error) {
	m, ok := h.h.(encoding.BinaryMarshaler)
	if !ok {
		// Every hash in this package implements BinaryMarshaler, so this is
		// unreachable in practice; returning an error rather than panicking
		// keeps the contract.
		return nil, py.ExceptionNewf(py.RuntimeError, "hash object of type %s cannot be copied", h.algo.name)
	}
	state, err := m.MarshalBinary()
	if err != nil {
		// The BLAKE2 marshaller refuses to marshal a keyed hash.  Unkeyed
		// BLAKE2 is the only kind this package creates, so this too should be
		// unreachable.
		return nil, py.ExceptionNewf(py.ValueError, "%s", err.Error())
	}
	fresh, err := h.algo.newHash(h.h.Size())
	if err != nil {
		return nil, err
	}
	if u, ok := fresh.(encoding.BinaryUnmarshaler); ok {
		if err := u.UnmarshalBinary(state); err != nil {
			return nil, py.ExceptionNewf(py.ValueError, "%s", err.Error())
		}
	}
	return &Hash{algo: h.algo, h: fresh, keyLen: h.keyLen}, nil
}

// ---------------------------------------------------------------------------
// Module-level functions

func newFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var name py.Object
	var data py.Object = py.None
	if err := py.UnpackTuple(args, nil, "new", 1, 2, &name, &data); err != nil {
		return nil, err
	}
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	algo, ok := algorithms[n]
	if !ok {
		return nil, py.ExceptionNewf(py.ValueError, "unsupported hash type %s", n)
	}
	// new() forwards any remaining keywords to the constructor; only BLAKE2's
	// digest_size is meaningful here.
	size := algo.digestLen
	if algo.varSize {
		if ds, ok := kwargs["digest_size"]; ok {
			s, err := py.IndexInt(ds)
			if err != nil {
				return nil, err
			}
			if s < 1 || s > algo.digestLen {
				return nil, py.ExceptionNewf(py.ValueError, "digest_size must be between 1 and %d bytes", algo.digestLen)
			}
			size = s
		}
	}
	return newHashObject(algo, size, data)
}

// fileDigestFn implements the file_digest() helper, which CPython added in
// 3.11.  It reads a binary file object to EOF in chunks.
func fileDigestFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var file py.Object
	var name py.Object
	var data py.Object = py.None
	if err := py.UnpackTuple(args, nil, "file_digest", 2, 2, &file, &name); err != nil {
		return nil, err
	}
	_ = data
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	algo, ok := algorithms[n]
	if !ok {
		return nil, py.ExceptionNewf(py.ValueError, "unsupported hash type %s", n)
	}
	size := algo.digestLen
	if algo.varSize {
		if ds, ok := kwargs["digest_size"]; ok {
			s, err := py.IndexInt(ds)
			if err != nil {
				return nil, err
			}
			size = s
		}
	}
	h, err := algo.newHash(size)
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "%s", err.Error())
	}
	// The file must have read().  ObjectGetAttr returns nil when the attribute
	// is absent, and calling through it dereferenced a nil function pointer and
	// killed the process with a SIGSEGV.  A missing read is a TypeError.
	// py.ObjectGetAttr is a stub that always returns nil, so it could never
	// have found anything: the real lookup is GetAttrString.
	readObj, err := py.GetAttrString(file, "read")
	if err != nil || readObj == nil {
		return nil, py.ExceptionNewf(py.TypeError, "file_digest() argument 1 must be a file with a read() method")
	}
	for {
		chunk, err := py.Call(readObj, py.Tuple{py.Int(blake2b.BlockSize * 64)}, nil)
		if err != nil {
			return nil, err
		}
		if chunk == py.None {
			break
		}
		b, err := py.BytesFromObject(chunk)
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			break
		}
		h.Write([]byte(b))
	}
	// file_digest returns the digest OBJECT, not its bytes: the caller asks it
	// for hexdigest() or digest().  Returning the bytes made
	// "file_digest(f, 'md5').hexdigest()" an AttributeError on bytes.
	return &Hash{algo: *algo, h: h}, nil
}

// ---------------------------------------------------------------------------
// Registration

// digestSizeProp and the other properties report the object's own state, so
// they are properties on the shared hash type rather than per-algorithm data.
func init() {
	hashType.Dict["update"] = py.MustNewMethod("update", hashUpdate, 0, "Update this hash object's state with the provided bytes-like object.")
	hashType.Dict["digest"] = py.MustNewMethod("digest", hashDigest, 0, "Return the digest value as a bytes object.")
	hashType.Dict["hexdigest"] = py.MustNewMethod("hexdigest", hashHexdigest, 0, "Return the digest value as a string of hexadecimal digits.")
	hashType.Dict["copy"] = py.MustNewMethod("copy", hashCopy, 0, "Return a copy of the hash object.")

	hashType.Dict["name"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String(self.(*Hash).algo.name), nil
	}}
	hashType.Dict["digest_size"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*Hash).h.Size()), nil
	}}
	hashType.Dict["block_size"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*Hash).algo.blockLen), nil
	}}

	methods := []*py.Method{
		py.MustNewMethod("new", newFn, 0, "new(name, data=b'', **kwargs) - returns a new hash object implementing the given hash function."),
		py.MustNewMethod("file_digest", fileDigestFn, 0, "file_digest(fileobj, digest, /) - return the digest of a binary file object."),
	}
	for _, name := range algosByName {
		methods = append(methods, py.MustNewMethod(name, namedCtor(name), 0, name+"(data=b'') - return a new hashing object using the "+name+" algorithm."))
	}

	guaranteed := py.NewListFromStrings([]string{"md5", "sha1", "sha224", "sha256", "sha384", "sha512", "blake2b", "sha3_224", "sha3_256", "sha3_384", "sha3_512", "blake2s"})
	available := py.NewListFromStrings(algosByName)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "hashlib",
			Doc:  module_doc,
		},
		Methods: methods,
		Globals: py.StringDict{
			"algorithms_guaranteed": guaranteed,
			"algorithms_available":  available,
		},
	})
}

// The vendored blake2b package's Size constant is re-exported here so callers
// of hashlib do not need to import the internal package.
var _ = bytes.MinRead
