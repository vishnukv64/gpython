// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmath

import (
	"math"

	"github.com/vishnukv64/gpython/py"
)

const (
	largeDouble = math.MaxFloat64 / 4
	minNormal   = 0x1p-1022
	scaleUp     = 2*(53/2) + 1
	scaleDown   = -(scaleUp + 1) / 2
	ln2         = 0.6931471805599453094
	ln10        = 2.302585092994045684
	e           = math.E
)

var (
	sqrtLarge  = math.Sqrt(largeDouble)
	logLarge   = math.Log(largeDouble)
	sqrtDblMin = math.Sqrt(minNormal)
)

func isinf(x float64) bool { return math.IsInf(x, 0) }
func isnan(x float64) bool { return math.IsNaN(x) }
func cneg(z cx) cx         { return cx{-z.re, -z.im} }

// special returns the table entry when either part is non-finite.
func special(z cx, t *[7][7]cx) (cx, bool) {
	if !finite(z.re) || !finite(z.im) {
		return t[specialType(z.re)][specialType(z.im)], true
	}
	return cx{}, false
}

func acos(z cx) (cx, int) {
	if r, ok := special(z, &acosSpecial); ok {
		return r, 0
	}
	var r cx
	if math.Abs(z.re) > largeDouble || math.Abs(z.im) > largeDouble {
		r.re = atan2(math.Abs(z.im), z.re)
		r.im = -math.Copysign(math.Log(math.Hypot(z.re/2, z.im/2))+ln2*2, z.im)
	} else {
		s1, _ := sqrt(cx{1 - z.re, -z.im})
		s2, _ := sqrt(cx{1 + z.re, z.im})
		r.re = 2 * atan2(s1.re, s2.re)
		r.im = math.Asinh(s2.re*s1.im - s2.im*s1.re)
	}
	return r, 0
}

func acosh(z cx) (cx, int) {
	if r, ok := special(z, &acoshSpecial); ok {
		return r, 0
	}
	var r cx
	if math.Abs(z.re) > largeDouble || math.Abs(z.im) > largeDouble {
		r.re = math.Log(math.Hypot(z.re/2, z.im/2)) + ln2*2
		r.im = atan2(z.im, z.re)
	} else {
		s1, _ := sqrt(cx{z.re - 1, z.im})
		s2, _ := sqrt(cx{z.re + 1, z.im})
		r.re = math.Asinh(s1.re*s2.re + s1.im*s2.im)
		r.im = 2 * atan2(s1.im, s2.re)
	}
	return r, 0
}

func asin(z cx) (cx, int) {
	s, _ := asinh(cx{-z.im, z.re})
	return cx{s.im, -s.re}, 0
}

func asinh(z cx) (cx, int) {
	if r, ok := special(z, &asinhSpecial); ok {
		return r, 0
	}
	var r cx
	if math.Abs(z.re) > largeDouble || math.Abs(z.im) > largeDouble {
		l := math.Log(math.Hypot(z.re/2, z.im/2)) + ln2*2
		if z.im >= 0 {
			r.re = math.Copysign(l, z.re)
		} else {
			r.re = -math.Copysign(l, -z.re)
		}
		r.im = atan2(z.im, math.Abs(z.re))
	} else {
		s1, _ := sqrt(cx{1 + z.im, -z.re})
		s2, _ := sqrt(cx{1 - z.im, z.re})
		r.re = math.Asinh(s1.re*s2.im - s2.re*s1.im)
		r.im = atan2(z.im, s1.re*s2.re-s1.im*s2.im)
	}
	return r, 0
}

func atan(z cx) (cx, int) {
	s, err := atanh(cx{-z.im, z.re})
	return cx{s.im, -s.re}, err
}

func atanh(z cx) (cx, int) {
	if r, ok := special(z, &atanhSpecial); ok {
		return r, 0
	}
	if z.re < 0 {
		r, err := atanh(cneg(z))
		return cneg(r), err
	}
	ay := math.Abs(z.im)
	var r cx
	switch {
	case z.re > sqrtLarge || ay > sqrtLarge:
		h := math.Hypot(z.re/2, z.im/2)
		r.re = z.re / 4 / h / h
		r.im = math.Copysign(math.Pi/2, z.im)
	case z.re == 1 && ay < sqrtDblMin:
		if ay == 0 {
			return cx{inf, z.im}, edom
		}
		r.re = -math.Log(math.Sqrt(ay) / math.Sqrt(math.Hypot(ay, 2)))
		r.im = math.Copysign(atan2(2, -ay)/2, z.im)
	default:
		r.re = math.Log1p(4*z.re/((1-z.re)*(1-z.re)+ay*ay)) / 4
		r.im = -atan2(-2*z.im, (1-z.re)*(1+z.re)-ay*ay) / 2
	}
	return r, 0
}

func cos(z cx) (cx, int) { return cosh(cx{-z.im, z.re}) }

func cosh(z cx) (cx, int) {
	if !finite(z.re) || !finite(z.im) {
		var r cx
		if isinf(z.re) && finite(z.im) && z.im != 0 {
			if z.re > 0 {
				r = cx{math.Copysign(inf, math.Cos(z.im)), math.Copysign(inf, math.Sin(z.im))}
			} else {
				r = cx{math.Copysign(inf, math.Cos(z.im)), -math.Copysign(inf, math.Sin(z.im))}
			}
		} else {
			r = coshSpecial[specialType(z.re)][specialType(z.im)]
		}
		if isinf(z.im) && !isnan(z.re) {
			return r, edom
		}
		return r, 0
	}
	var r cx
	if math.Abs(z.re) > logLarge {
		x := z.re - math.Copysign(1, z.re)
		r.re = math.Cos(z.im) * math.Cosh(x) * e
		r.im = math.Sin(z.im) * math.Sinh(x) * e
	} else {
		r.re = math.Cos(z.im) * math.Cosh(z.re)
		r.im = math.Sin(z.im) * math.Sinh(z.re)
	}
	if isinf(r.re) || isinf(r.im) {
		return r, erange
	}
	return r, 0
}

func exp(z cx) (cx, int) {
	if !finite(z.re) || !finite(z.im) {
		var r cx
		if isinf(z.re) && finite(z.im) && z.im != 0 {
			if z.re > 0 {
				r = cx{math.Copysign(inf, math.Cos(z.im)), math.Copysign(inf, math.Sin(z.im))}
			} else {
				r = cx{math.Copysign(0, math.Cos(z.im)), math.Copysign(0, math.Sin(z.im))}
			}
		} else {
			r = expSpecial[specialType(z.re)][specialType(z.im)]
		}
		if isinf(z.im) && (finite(z.re) || (isinf(z.re) && z.re > 0)) {
			return r, edom
		}
		return r, 0
	}
	var r cx
	if z.re > logLarge {
		l := math.Exp(z.re - 1)
		r = cx{l * math.Cos(z.im) * e, l * math.Sin(z.im) * e}
	} else {
		l := math.Exp(z.re)
		r = cx{l * math.Cos(z.im), l * math.Sin(z.im)}
	}
	if isinf(r.re) || isinf(r.im) {
		return r, erange
	}
	return r, 0
}

func log(z cx) (cx, int) {
	if r, ok := special(z, &logSpecial); ok {
		return r, 0
	}
	ax, ay := math.Abs(z.re), math.Abs(z.im)
	var r cx
	switch {
	case ax > largeDouble || ay > largeDouble:
		r.re = math.Log(math.Hypot(ax/2, ay/2)) + ln2
	case ax < minNormal && ay < minNormal:
		if ax > 0 || ay > 0 {
			r.re = math.Log(math.Hypot(math.Ldexp(ax, 53), math.Ldexp(ay, 53))) - 53*ln2
		} else {
			return cx{-inf, atan2(z.im, z.re)}, edom
		}
	default:
		h := math.Hypot(ax, ay)
		if 0.71 <= h && h <= 1.73 {
			am, an := math.Max(ax, ay), math.Min(ax, ay)
			r.re = math.Log1p((am-1)*(am+1)+an*an) / 2
		} else {
			r.re = math.Log(h)
		}
	}
	r.im = atan2(z.im, z.re)
	return r, 0
}

func log10(z cx) (cx, int) {
	r, err := log(z)
	return cx{r.re / ln10, r.im / ln10}, err
}

func sin(z cx) (cx, int) {
	s, err := sinh(cx{-z.im, z.re})
	return cx{s.im, -s.re}, err
}

func sinh(z cx) (cx, int) {
	if !finite(z.re) || !finite(z.im) {
		var r cx
		if isinf(z.re) && finite(z.im) && z.im != 0 {
			if z.re > 0 {
				r = cx{math.Copysign(inf, math.Cos(z.im)), math.Copysign(inf, math.Sin(z.im))}
			} else {
				r = cx{-math.Copysign(inf, math.Cos(z.im)), math.Copysign(inf, math.Sin(z.im))}
			}
		} else {
			r = sinhSpecial[specialType(z.re)][specialType(z.im)]
		}
		if isinf(z.im) && !isnan(z.re) {
			return r, edom
		}
		return r, 0
	}
	var r cx
	if math.Abs(z.re) > logLarge {
		x := z.re - math.Copysign(1, z.re)
		r.re = math.Cos(z.im) * math.Sinh(x) * e
		r.im = math.Sin(z.im) * math.Cosh(x) * e
	} else {
		r.re = math.Cos(z.im) * math.Sinh(z.re)
		r.im = math.Sin(z.im) * math.Cosh(z.re)
	}
	if isinf(r.re) || isinf(r.im) {
		return r, erange
	}
	return r, 0
}

func sqrt(z cx) (cx, int) {
	if r, ok := special(z, &sqrtSpecial); ok {
		return r, 0
	}
	if z.re == 0 && z.im == 0 {
		return cx{0, z.im}, 0
	}
	ax, ay := math.Abs(z.re), math.Abs(z.im)
	var s float64
	if ax < minNormal && ay < minNormal {
		ax = math.Ldexp(ax, scaleUp)
		s = math.Ldexp(math.Sqrt(ax+math.Hypot(ax, math.Ldexp(ay, scaleUp))), scaleDown)
	} else {
		ax /= 8
		s = 2 * math.Sqrt(ax+math.Hypot(ax, ay/8))
	}
	d := ay / (2 * s)
	if z.re >= 0 {
		return cx{s, math.Copysign(d, z.im)}, 0
	}
	return cx{d, math.Copysign(s, z.im)}, 0
}

func tan(z cx) (cx, int) {
	s, err := tanh(cx{-z.im, z.re})
	return cx{s.im, -s.re}, err
}

func tanh(z cx) (cx, int) {
	if !finite(z.re) || !finite(z.im) {
		var r cx
		if isinf(z.re) && finite(z.im) && z.im != 0 {
			r.re = 1
			if z.re < 0 {
				r.re = -1
			}
			r.im = math.Copysign(0, 2*math.Sin(z.im)*math.Cos(z.im))
		} else {
			r = tanhSpecial[specialType(z.re)][specialType(z.im)]
		}
		if isinf(z.im) && finite(z.re) {
			return r, edom
		}
		return r, 0
	}
	var r cx
	if math.Abs(z.re) > logLarge {
		r.re = math.Copysign(1, z.re)
		r.im = 4 * math.Sin(z.im) * math.Cos(z.im) * math.Exp(-2*math.Abs(z.re))
	} else {
		tx, ty := math.Tanh(z.re), math.Tan(z.im)
		cx_ := 1 / math.Cosh(z.re)
		txty := tx * ty
		denom := 1 + txty*txty
		r.re = tx * (1 + ty*ty) / denom
		r.im = ((ty / denom) * cx_) * cx_
	}
	return r, 0
}

func rect(r, phi float64) (cx, int) {
	var z cx
	switch {
	case !finite(r) || !finite(phi):
		if isinf(r) && finite(phi) && phi != 0 {
			if r > 0 {
				z = cx{math.Copysign(inf, math.Cos(phi)), math.Copysign(inf, math.Sin(phi))}
			} else {
				z = cx{-math.Copysign(inf, math.Cos(phi)), -math.Copysign(inf, math.Sin(phi))}
			}
		} else {
			z = rectSpecial[specialType(r)][specialType(phi)]
		}
		if r != 0 && !isnan(r) && isinf(phi) {
			return z, edom
		}
	case phi == 0:
		z = cx{r, r * phi}
	default:
		z = cx{r * math.Cos(phi), r * math.Sin(phi)}
	}
	return z, 0
}

// Python glue.

func toErr(code int) error {
	switch code {
	case edom:
		return py.ExceptionNewf(py.ValueError, "math domain error")
	case erange:
		return py.ExceptionNewf(py.OverflowError, "math range error")
	}
	return nil
}

func unary(name string, f func(cx) (cx, int), doc string) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		z, err := argComplex(name, args)
		if err != nil {
			return nil, err
		}
		r, code := f(z)
		if err := toErr(code); err != nil {
			return nil, err
		}
		return py.Complex(complex(r.re, r.im)), nil
	}, 0, doc)
}

func boolean(name, doc string, f func(cx) bool) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		z, err := argComplex(name, args)
		if err != nil {
			return nil, err
		}
		return py.NewBool(f(z)), nil
	}, 0, doc)
}

func argComplex(name string, args py.Tuple) (cx, error) {
	if len(args) != 1 {
		return cx{}, py.ExceptionNewf(py.TypeError, "cmath.%s() takes exactly one argument (%d given)", name, len(args))
	}
	return toComplex(args[0])
}

// toComplex follows PyComplex_AsCComplex: __complex__, then __float__, then __index__.
func toComplex(o py.Object) (cx, error) {
	if c, ok := o.(py.Complex); ok {
		return cx{real(c), imag(c)}, nil
	}
	if _, isStr := o.(py.String); isStr {
		return cx{}, py.ExceptionNewf(py.TypeError, "must be real number, not str")
	}
	if b, ok := o.(py.Bool); ok {
		if b {
			return cx{1, 0}, nil
		}
		return cx{}, nil
	}
	c, err := py.MakeComplex(o)
	if err != nil {
		// No __complex__: __float__ then __index__, as PyComplex_AsCComplex does.
		f, ferr := py.MakeFloat(o)
		if ferr != nil {
			return cx{}, py.ExceptionNewf(py.TypeError, "must be real number, not %s", o.Type().Name)
		}
		return cx{float64(f.(py.Float)), 0}, nil
	}
	cc := c.(py.Complex)
	return cx{real(cc), imag(cc)}, nil
}

func toFloat(o py.Object) (float64, error) {
	f, err := py.MakeFloat(o)
	if err != nil {
		return 0, err
	}
	return float64(f.(py.Float)), nil
}

func hypotAbs(z cx) (float64, int) {
	if isinf(z.re) || isinf(z.im) {
		return inf, 0
	}
	if isnan(z.re) || isnan(z.im) {
		return nan, 0
	}
	r := math.Hypot(z.re, z.im)
	if isinf(r) {
		return r, erange
	}
	return r, 0
}

func init() {
	g := py.NewStringDict()
	for k, v := range map[string]py.Object{
		"pi": py.Float(math.Pi), "e": py.Float(math.E), "tau": py.Float(2 * math.Pi),
		"inf": py.Float(inf), "infj": py.Complex(complex(0, inf)),
		"nan": py.Float(nan), "nanj": py.Complex(complex(0, nan)),
	} {
		g.Set(k, v)
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "cmath", Doc: "This module provides access to mathematical functions for complex\nnumbers."},
		Globals: g,
		Methods: []*py.Method{
			unary("acos", acos, "Return the arc cosine of z."),
			unary("acosh", acosh, "Return the inverse hyperbolic cosine of z."),
			unary("asin", asin, "Return the arc sine of z."),
			unary("asinh", asinh, "Return the inverse hyperbolic sine of z."),
			unary("atan", atan, "Return the arc tangent of z."),
			unary("atanh", atanh, "Return the inverse hyperbolic tangent of z."),
			unary("cos", cos, "Return the cosine of z."),
			unary("cosh", cosh, "Return the hyperbolic cosine of z."),
			unary("exp", exp, "Return the exponential value e**z."),
			unary("log10", log10, "Return the base-10 logarithm of z."),
			unary("sin", sin, "Return the sine of z."),
			unary("sinh", sinh, "Return the hyperbolic sine of z."),
			unary("sqrt", sqrt, "Return the square root of z."),
			unary("tan", tan, "Return the tangent of z."),
			unary("tanh", tanh, "Return the hyperbolic tangent of z."),
			boolean("isfinite", "Return True if both the real and imaginary parts of z are finite, else False.", func(z cx) bool { return finite(z.re) && finite(z.im) }),
			boolean("isnan", "Checks if the real or imaginary part of z not a number (NaN).", func(z cx) bool { return isnan(z.re) || isnan(z.im) }),
			boolean("isinf", "Checks if the real or imaginary part of z is infinite.", func(z cx) bool { return isinf(z.re) || isinf(z.im) }),
			py.MustNewMethod("phase", func(self py.Object, args py.Tuple) (py.Object, error) {
				z, err := argComplex("phase", args)
				if err != nil {
					return nil, err
				}
				return py.Float(atan2(z.im, z.re)), nil
			}, 0, "Return argument, also known as the phase angle, of a complex."),
			py.MustNewMethod("polar", func(self py.Object, args py.Tuple) (py.Object, error) {
				z, err := argComplex("polar", args)
				if err != nil {
					return nil, err
				}
				r, code := hypotAbs(z)
				if err := toErr(code); err != nil {
					return nil, err
				}
				return py.Tuple{py.Float(r), py.Float(atan2(z.im, z.re))}, nil
			}, 0, "Convert a complex from rectangular coordinates to polar coordinates.\n\nr is the distance from 0 and phi the phase angle."),
			py.MustNewMethod("rect", func(self py.Object, args py.Tuple) (py.Object, error) {
				if len(args) != 2 {
					return nil, py.ExceptionNewf(py.TypeError, "rect expected 2 arguments, got %d", len(args))
				}
				r, err := toFloat(args[0])
				if err != nil {
					return nil, err
				}
				phi, err := toFloat(args[1])
				if err != nil {
					return nil, err
				}
				z, code := rect(r, phi)
				if err := toErr(code); err != nil {
					return nil, err
				}
				return py.Complex(complex(z.re, z.im)), nil
			}, 0, "Convert from polar coordinates to rectangular coordinates."),
			py.MustNewMethod("log", func(self py.Object, args py.Tuple) (py.Object, error) {
				if len(args) < 1 || len(args) > 2 {
					return nil, py.ExceptionNewf(py.TypeError, "log expected at least 1 argument, got %d", len(args))
				}
				x, err := toComplex(args[0])
				if err != nil {
					return nil, err
				}
				r, code := log(x)
				if len(args) == 2 {
					y, err := toComplex(args[1])
					if err != nil {
						return nil, err
					}
					ly, c2 := log(y)
					code = c2 // as in C, the second c_log resets errno
					q, err := py.TrueDiv(py.Complex(complex(r.re, r.im)), py.Complex(complex(ly.re, ly.im)))
					if err != nil {
						return nil, err
					}
					qc := q.(py.Complex)
					r = cx{real(qc), imag(qc)}
				}
				if err := toErr(code); err != nil {
					return nil, err
				}
				return py.Complex(complex(r.re, r.im)), nil
			}, 0, "log(z[, base]) -> the logarithm of z to the given base.\n\nIf the base is not specified, returns the natural logarithm (base e) of z."),
			py.MustNewMethod("isclose", isclose, 0, "Determine whether two complex numbers are close in value."),
		},
	})
}

func isclose(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "isclose() takes exactly 2 positional arguments (%d given)", len(args))
	}
	rel, abs := 1e-09, 0.0
	for _, k := range kwargs.Keys() {
		v, _ := kwargs.Get(k)
		f, err := toFloat(v)
		if err != nil {
			return nil, err
		}
		switch k {
		case "rel_tol":
			rel = f
		case "abs_tol":
			abs = f
		default:
			return nil, py.ExceptionNewf(py.TypeError, "isclose() got an unexpected keyword argument '%s'", k)
		}
	}
	a, err := toComplex(args[0])
	if err != nil {
		return nil, err
	}
	b, err := toComplex(args[1])
	if err != nil {
		return nil, err
	}
	if rel < 0 || abs < 0 {
		return nil, py.ExceptionNewf(py.ValueError, "tolerances must be non-negative")
	}
	if a == b {
		return py.True, nil
	}
	if isinf(a.re) || isinf(a.im) || isinf(b.re) || isinf(b.im) {
		return py.False, nil
	}
	diff := math.Hypot(a.re-b.re, a.im-b.im)
	return py.NewBool(diff <= rel*math.Hypot(b.re, b.im) || diff <= rel*math.Hypot(a.re, a.im) || diff <= abs), nil
}

// atan2 is math.Atan2 with C's answer when y/x underflows to zero for x < 0:
// Go returns +Pi whatever the sign of y there.
func atan2(y, x float64) float64 {
	if x < 0 && y != 0 && finite(y) && finite(x) && y/x == 0 {
		return math.Copysign(math.Pi, y)
	}
	return math.Atan2(y, x)
}
