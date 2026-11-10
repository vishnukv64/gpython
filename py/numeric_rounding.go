// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import "math"

// float.__trunc__/__ceil__/__floor__ and the int versions.  math.trunc, ceil
// and floor already work on the values; these make the unbound attributes
// exist so a subclass can alias them (tomlkit: "__trunc__ = float.__trunc__").
// self may be a float subclass instance, whose value is its payload.
func init() {
	floatRound := func(name string, f func(float64) float64) {
		FloatType.Dict.Set(name, MustNewMethod(name, func(self Object, args Tuple) (Object, error) {
			if p, ok := payloadOf(self); ok {
				self = p
			}
			v, ok := self.(Float)
			if !ok {
				return nil, ExceptionNewf(TypeError, "descriptor '%s' requires a 'float' object but received a '%s'", name, self.Type().Name)
			}
			if math.IsNaN(float64(v)) {
				return nil, ExceptionNewf(ValueError, "cannot convert float NaN to integer")
			}
			if math.IsInf(float64(v), 0) {
				return nil, ExceptionNewf(OverflowError, "cannot convert float infinity to integer")
			}
			return Float(f(float64(v))).M__int__()
		}, 0, name+"(self) -> Integral"))
	}
	floatRound("__trunc__", math.Trunc)
	floatRound("__ceil__", math.Ceil)
	floatRound("__floor__", math.Floor)

	for _, t := range []*Type{IntType, BigIntType} {
		for _, name := range []string{"__trunc__", "__ceil__", "__floor__"} {
			t.Dict.Set(name, MustNewMethod(name, func(self Object, args Tuple) (Object, error) {
				if p, ok := payloadOf(self); ok {
					return p, nil
				}
				return self, nil
			}, 0, name+"(self) -> self"))
		}
	}
}
