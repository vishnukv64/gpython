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

func init() {
	for _, t := range []*Type{IntType, BigIntType} {
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
