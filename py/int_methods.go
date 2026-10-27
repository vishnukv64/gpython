package py

import (
	"math/big"
	"math/bits"
)

func toBig(self Object) *big.Int {
	switch v := self.(type) {
	case Int:
		return big.NewInt(int64(v))
	case *BigInt:
		return (*big.Int)(v)
	}
	return nil
}

func intToBytes(self Object, args Tuple, kwargs StringDict) (Object, error) {
	length, order, signed := Object(Int(1)), Object(String("big")), Object(False)
	if err := ParseTupleAndKeywords(args, kwargs, "|OO$O:to_bytes", []string{"length", "byteorder", "signed"}, &length, &order, &signed); err != nil {
		return nil, err
	}
	n, err := IndexInt(length)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, ExceptionNewf(ValueError, "length argument must be non-negative")
	}
	little, err := byteOrderIsLittle(order)
	if err != nil {
		return nil, err
	}
	sg, err := ObjectIsTrue(signed)
	if err != nil {
		return nil, err
	}
	v := toBig(UnwrapScalar(self))
	lim := new(big.Int).Lsh(big.NewInt(1), uint(8*n))
	if sg {
		half := new(big.Int).Rsh(lim, 1)
		if v.Cmp(half) >= 0 || v.Cmp(new(big.Int).Neg(half)) < 0 {
			return nil, ExceptionNewf(OverflowError, "int too big to convert")
		}
		if v.Sign() < 0 {
			v = new(big.Int).Add(v, lim)
		}
	} else if v.Sign() < 0 {
		return nil, ExceptionNewf(OverflowError, "can't convert negative int to unsigned")
	} else if v.Cmp(lim) >= 0 {
		return nil, ExceptionNewf(OverflowError, "int too big to convert")
	}
	out := make([]byte, n)
	v.FillBytes(out)
	if little {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return Bytes(out), nil
}

func byteOrderIsLittle(order Object) (bool, error) {
	switch UnwrapScalar(order) {
	case String("little"):
		return true, nil
	case String("big"):
		return false, nil
	}
	return false, ExceptionNewf(ValueError, "byteorder must be either 'little' or 'big'")
}

func intFromBytes(self Object, args Tuple, kwargs StringDict) (Object, error) {
	var data Object
	order, signed := Object(String("big")), Object(False)
	if err := ParseTupleAndKeywords(args, kwargs, "O|O$O:from_bytes", []string{"bytes", "byteorder", "signed"}, &data, &order, &signed); err != nil {
		return nil, err
	}
	b, err := BytesFromObject(data)
	if err != nil {
		return nil, err
	}
	little, err := byteOrderIsLittle(order)
	if err != nil {
		return nil, err
	}
	sg, err := ObjectIsTrue(signed)
	if err != nil {
		return nil, err
	}
	buf := append([]byte(nil), b...)
	if little {
		for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
			buf[i], buf[j] = buf[j], buf[i]
		}
	}
	v := new(big.Int).SetBytes(buf)
	if sg && len(buf) > 0 && buf[0]&0x80 != 0 {
		v.Sub(v, new(big.Int).Lsh(big.NewInt(1), uint(8*len(buf))))
	}
	var res Object = (*BigInt)(v).MaybeInt()
	if t, ok := self.(*Type); ok {
		res = wrapPayload(t, res)
	}
	return res, nil
}

func init() {
	for _, t := range []*Type{IntType, BigIntType} {
		t.Dict.Set("to_bytes", MustNewMethod("to_bytes", intToBytes, 0, "Return an array of bytes representing an integer."))
		t.Dict.Set("from_bytes", MustNewMethod("from_bytes", intFromBytes, METH_CLASS, "Return the integer represented by the given array of bytes."))
		t.Dict.Set("bit_length", MustNewMethod("bit_length", func(self Object) (Object, error) {
			if v, ok := self.(Int); ok {
				u := uint64(v)
				if v < 0 {
					u = uint64(-int64(v)) // IntMin wraps to 1<<63, still 64 bits
				}
				return Int(bits.Len64(u)), nil
			}
			return Int(toBig(self).BitLen()), nil // BitLen uses |x|
		}, 0, "Number of bits necessary to represent self in binary."))
		self := func(s Object) (Object, error) { return s, nil }
		t.Dict.Set("real", &Property{Fget: self})
		t.Dict.Set("numerator", &Property{Fget: self})
		t.Dict.Set("denominator", &Property{Fget: func(Object) (Object, error) { return Int(1), nil }})
		t.Dict.Set("imag", &Property{Fget: func(Object) (Object, error) { return Int(0), nil }})
		t.Dict.Set("conjugate", MustNewMethod("conjugate", func(s Object) (Object, error) { return s, nil }, 0, "Returns self, the complex conjugate of any int."))
		t.Dict.Set("as_integer_ratio", MustNewMethod("as_integer_ratio", func(s Object) (Object, error) { return Tuple{s, Int(1)}, nil }, 0, "Return (self, 1)."))
		t.Dict.Set("is_integer", MustNewMethod("is_integer", func(Object) (Object, error) { return True, nil }, 0, "Returns True."))
		t.Dict.Set("bit_count", MustNewMethod("bit_count", func(self Object) (Object, error) {
			n := 0
			for _, w := range new(big.Int).Abs(toBig(self)).Bits() {
				n += bits.OnesCount(uint(w))
			}
			return Int(n), nil
		}, 0, "Number of ones in the binary representation of abs(self)."))
	}
}
