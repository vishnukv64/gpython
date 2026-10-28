// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cmath is a port of CPython's Modules/cmathmodule.c: the C99
// Annex G special-value tables and the overflow-safe algorithms, so that
// branch cuts, signed zeros, infinities and NaNs come out as in CPython.
package cmath

import "math"

const (
	pi  = math.Pi
	p14 = 0.25 * math.Pi
	p12 = 0.5 * math.Pi
	p34 = 0.75 * math.Pi
	u   = -9.5426319407711027e33 // placeholder for unreachable cells
)

var (
	inf = math.Inf(1)
	nan = math.NaN()
	nz  = math.Copysign(0, -1)
)

type cx struct{ re, im float64 }

// special_types, in table order.
const (
	stNInf = iota
	stNeg
	stNZero
	stPZero
	stPos
	stPInf
	stNaN
)

func specialType(d float64) int {
	switch {
	case !math.IsInf(d, 0) && !math.IsNaN(d):
		if d != 0 {
			if math.Signbit(d) {
				return stNeg
			}
			return stPos
		}
		if math.Signbit(d) {
			return stNZero
		}
		return stPZero
	case math.IsNaN(d):
		return stNaN
	case math.Signbit(d):
		return stNInf
	}
	return stPInf
}

func finite(x float64) bool { return !math.IsInf(x, 0) && !math.IsNaN(x) }

// Error codes standing in for C errno.
const (
	ok = iota
	edom
	erange
)

var (
	acosSpecial = [7][7]cx{
		{{p34, inf}, {pi, inf}, {pi, inf}, {pi, -inf}, {pi, -inf}, {p34, -inf}, {nan, inf}},
		{{p12, inf}, {u, u}, {u, u}, {u, u}, {u, u}, {p12, -inf}, {nan, nan}},
		{{p12, inf}, {u, u}, {p12, 0.}, {p12, nz}, {u, u}, {p12, -inf}, {p12, nan}},
		{{p12, inf}, {u, u}, {p12, 0.}, {p12, nz}, {u, u}, {p12, -inf}, {p12, nan}},
		{{p12, inf}, {u, u}, {u, u}, {u, u}, {u, u}, {p12, -inf}, {nan, nan}},
		{{p14, inf}, {0., inf}, {0., inf}, {0., -inf}, {0., -inf}, {p14, -inf}, {nan, inf}},
		{{nan, inf}, {nan, nan}, {nan, nan}, {nan, nan}, {nan, nan}, {nan, -inf}, {nan, nan}},
	}
	acoshSpecial = [7][7]cx{
		{{inf, -p34}, {inf, -pi}, {inf, -pi}, {inf, pi}, {inf, pi}, {inf, p34}, {inf, nan}},
		{{inf, -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p12}, {u, u}, {0., -p12}, {0., p12}, {u, u}, {inf, p12}, {nan, p12}},
		{{inf, -p12}, {u, u}, {0., -p12}, {0., p12}, {u, u}, {inf, p12}, {nan, p12}},
		{{inf, -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p14}, {inf, nz}, {inf, nz}, {inf, 0.}, {inf, 0.}, {inf, p14}, {inf, nan}},
		{{inf, nan}, {nan, nan}, {nan, nan}, {nan, nan}, {nan, nan}, {inf, nan}, {nan, nan}},
	}
	asinhSpecial = [7][7]cx{
		{{-inf, -p14}, {-inf, nz}, {-inf, nz}, {-inf, 0.}, {-inf, 0.}, {-inf, p14}, {-inf, nan}},
		{{-inf, -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {-inf, p12}, {nan, nan}},
		{{-inf, -p12}, {u, u}, {nz, nz}, {nz, 0.}, {u, u}, {-inf, p12}, {nan, nan}},
		{{inf, -p12}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p14}, {inf, nz}, {inf, nz}, {inf, 0.}, {inf, 0.}, {inf, p14}, {inf, nan}},
		{{inf, nan}, {nan, nan}, {nan, nz}, {nan, 0.}, {nan, nan}, {inf, nan}, {nan, nan}},
	}
	atanhSpecial = [7][7]cx{
		{{nz, -p12}, {nz, -p12}, {nz, -p12}, {nz, p12}, {nz, p12}, {nz, p12}, {nz, nan}},
		{{nz, -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {nz, p12}, {nan, nan}},
		{{nz, -p12}, {u, u}, {nz, nz}, {nz, 0.}, {u, u}, {nz, p12}, {nz, nan}},
		{{0., -p12}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {0., p12}, {0., nan}},
		{{0., -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {0., p12}, {nan, nan}},
		{{0., -p12}, {0., -p12}, {0., -p12}, {0., p12}, {0., p12}, {0., p12}, {0., nan}},
		{{0., -p12}, {nan, nan}, {nan, nan}, {nan, nan}, {nan, nan}, {0., p12}, {nan, nan}},
	}
	coshSpecial = [7][7]cx{
		{{inf, nan}, {u, u}, {inf, 0.}, {inf, nz}, {u, u}, {inf, nan}, {inf, nan}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{nan, 0.}, {u, u}, {1., 0.}, {1., nz}, {u, u}, {nan, 0.}, {nan, 0.}},
		{{nan, 0.}, {u, u}, {1., nz}, {1., 0.}, {u, u}, {nan, 0.}, {nan, 0.}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{inf, nan}, {u, u}, {inf, nz}, {inf, 0.}, {u, u}, {inf, nan}, {inf, nan}},
		{{nan, nan}, {nan, nan}, {nan, 0.}, {nan, 0.}, {nan, nan}, {nan, nan}, {nan, nan}},
	}
	expSpecial = [7][7]cx{
		{{0., 0.}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {0., 0.}, {0., 0.}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{nan, nan}, {u, u}, {1., nz}, {1., 0.}, {u, u}, {nan, nan}, {nan, nan}},
		{{nan, nan}, {u, u}, {1., nz}, {1., 0.}, {u, u}, {nan, nan}, {nan, nan}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{inf, nan}, {u, u}, {inf, nz}, {inf, 0.}, {u, u}, {inf, nan}, {inf, nan}},
		{{nan, nan}, {nan, nan}, {nan, nz}, {nan, 0.}, {nan, nan}, {nan, nan}, {nan, nan}},
	}
	logSpecial = [7][7]cx{
		{{inf, -p34}, {inf, -pi}, {inf, -pi}, {inf, pi}, {inf, pi}, {inf, p34}, {inf, nan}},
		{{inf, -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p12}, {u, u}, {-inf, -pi}, {-inf, pi}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p12}, {u, u}, {-inf, nz}, {-inf, 0.}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p12}, {u, u}, {u, u}, {u, u}, {u, u}, {inf, p12}, {nan, nan}},
		{{inf, -p14}, {inf, nz}, {inf, nz}, {inf, 0.}, {inf, 0.}, {inf, p14}, {inf, nan}},
		{{inf, nan}, {nan, nan}, {nan, nan}, {nan, nan}, {nan, nan}, {inf, nan}, {nan, nan}},
	}
	sinhSpecial = [7][7]cx{
		{{inf, nan}, {u, u}, {-inf, nz}, {-inf, 0.}, {u, u}, {inf, nan}, {inf, nan}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{0., nan}, {u, u}, {nz, nz}, {nz, 0.}, {u, u}, {0., nan}, {0., nan}},
		{{0., nan}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {0., nan}, {0., nan}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{inf, nan}, {u, u}, {inf, nz}, {inf, 0.}, {u, u}, {inf, nan}, {inf, nan}},
		{{nan, nan}, {nan, nan}, {nan, nz}, {nan, 0.}, {nan, nan}, {nan, nan}, {nan, nan}},
	}
	sqrtSpecial = [7][7]cx{
		{{inf, -inf}, {0., -inf}, {0., -inf}, {0., inf}, {0., inf}, {inf, inf}, {nan, inf}},
		{{inf, -inf}, {u, u}, {u, u}, {u, u}, {u, u}, {inf, inf}, {nan, nan}},
		{{inf, -inf}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {inf, inf}, {nan, nan}},
		{{inf, -inf}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {inf, inf}, {nan, nan}},
		{{inf, -inf}, {u, u}, {u, u}, {u, u}, {u, u}, {inf, inf}, {nan, nan}},
		{{inf, -inf}, {inf, nz}, {inf, nz}, {inf, 0.}, {inf, 0.}, {inf, inf}, {inf, nan}},
		{{inf, -inf}, {nan, nan}, {nan, nan}, {nan, nan}, {nan, nan}, {inf, inf}, {nan, nan}},
	}
	tanhSpecial = [7][7]cx{
		{{-1., 0.}, {u, u}, {-1., nz}, {-1., 0.}, {u, u}, {-1., 0.}, {-1., 0.}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{nz, nan}, {u, u}, {nz, nz}, {nz, 0.}, {u, u}, {nz, nan}, {nz, nan}},
		{{0.0, nan}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {0.0, nan}, {0., nan}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{1., 0.}, {u, u}, {1., nz}, {1., 0.}, {u, u}, {1., 0.}, {1., 0.}},
		{{nan, nan}, {nan, nan}, {nan, nz}, {nan, 0.}, {nan, nan}, {nan, nan}, {nan, nan}},
	}
	rectSpecial = [7][7]cx{
		{{inf, nan}, {u, u}, {-inf, 0.}, {-inf, nz}, {u, u}, {inf, nan}, {inf, nan}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{0., 0.}, {u, u}, {nz, 0.}, {nz, nz}, {u, u}, {0., 0.}, {0., 0.}},
		{{0., 0.}, {u, u}, {0., nz}, {0., 0.}, {u, u}, {0., 0.}, {0., 0.}},
		{{nan, nan}, {u, u}, {u, u}, {u, u}, {u, u}, {nan, nan}, {nan, nan}},
		{{inf, nan}, {u, u}, {inf, nz}, {inf, 0.}, {u, u}, {inf, nan}, {inf, nan}},
		{{nan, nan}, {nan, nan}, {nan, 0.}, {nan, 0.}, {nan, nan}, {nan, nan}, {nan, nan}},
	}
)
