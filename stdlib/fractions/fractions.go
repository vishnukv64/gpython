// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fractions provides the implementation of python's 'fractions'
// module: exact rational arithmetic.
//
// Fraction is a port of CPython's Lib/fractions.py.  All of its arithmetic is
// done with Go's math/big integers, never with Go's fixed-width types and never
// through float64: a Fraction that silently lost precision would be worse than
// no Fraction at all, so every intermediate value is an arbitrary-precision
// big.Int until the moment a float is explicitly asked for.
//
// The module follows CPython's rules for mixed arithmetic.  With an int or
// another Fraction the result stays exact; with a float the result is a float
// (or a complex for a float power of a negative Fraction), computed by
// converting through float64 as CPython does.  The exactness guarantee is about
// Fraction-to-Fraction operations, not about those.
package fractions

import (
	"math"
	"math/big"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Fraction, Rational number implementation.`

func init() {
	fractionType.Dict.Set("numerator", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return bigToObject(self.(*Fraction).Num), nil
	}})
	fractionType.Dict.Set("denominator", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return bigToObject(self.(*Fraction).Den), nil
	}})

	// Constructors and conversions.
	setMethod("from_float", fractionFromFloat, "Create a Fraction from a float.")
	setMethod("from_decimal", fractionFromDecimal, "Create a Fraction from a Decimal.")
	setMethod("from_number", fractionFromNumber, "Create a Fraction from a numbers.Rational or a float.")
	setMethod("as_integer_ratio", fractionAsIntegerRatio, "Return (numerator, denominator) in lowest terms with denominator > 0.")
	setMethod("is_integer", fractionIsInteger, "Return True if the Fraction is an integer.")
	setMethod("limit_denominator", fractionLimitDenominator, "Closest Fraction to self with denominator at most max_denominator.")
	setMethod("__reduce__", fractionReduce, "Pickle support: (Fraction, (numerator, denominator)).")
	setMethod("__copy__", fractionCopy, "Fraction is immutable, so it is its own copy.")
	setMethod("__deepcopy__", fractionDeepCopy, "Fraction is immutable, so it is its own deep copy.")

	globals := py.NewStringDict()
	globals.Set("Fraction", fractionType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "fractions",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// Fraction is a rational number held as a pair of coprime big.Ints, with a
// positive denominator.  Both fields are only ever written by newFraction, so
// that invariant holds for every Fraction the module hands out.
type Fraction struct {
	Num *big.Int
	Den *big.Int
}

const fraction_doc = `Fraction(numerator=0, denominator=None)

Constructs a Rational.  Takes a string like '3/2' or '1.5', another
Rational instance, a numerator/denominator pair, or a float.`

var fractionType = py.NewTypeX("fractions.Fraction", fraction_doc, fractionNew, nil)

func (f *Fraction) Type() *py.Type { return fractionType }

// ---------------------------------------------------------------------------
// construction

func fractionNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var numArg py.Object = py.Int(0)
	var denArg py.Object = py.None
	var hasDen bool
	if err := py.UnpackTuple(args, kwargs, "Fraction", 0, 2, &numArg, &denArg); err != nil {
		return nil, err
	}
	hasDen = denArg != py.None

	switch numerator := numArg.(type) {
	case py.Int:
		num := big.NewInt(int64(numerator))
		if !hasDen {
			return newFraction(num, big.NewInt(1)), nil
		}
		den, err := rationalArgument(denArg)
		if err != nil {
			return nil, err
		}
		return makeFraction(num, den)
	case *py.BigInt:
		num := new(big.Int).Set((*big.Int)(numerator))
		if !hasDen {
			return newFraction(num, big.NewInt(1)), nil
		}
		den, err := rationalArgument(denArg)
		if err != nil {
			return nil, err
		}
		return makeFraction(num, den)
	case py.Bool:
		num := big.NewInt(0)
		if numerator {
			num = big.NewInt(1)
		}
		if !hasDen {
			return newFraction(num, big.NewInt(1)), nil
		}
		den, err := rationalArgument(denArg)
		if err != nil {
			return nil, err
		}
		return makeFraction(num, den)
	case *Fraction:
		if !hasDen {
			// A Fraction is already normalized, so it can be shared or
			// copied; copy so the two are independent values.
			return newFraction(new(big.Int).Set(numerator.Num), new(big.Int).Set(numerator.Den)), nil
		}
		den, err := rationalArgument(denArg)
		if err != nil {
			return nil, err
		}
		return makeFraction(new(big.Int).Set(numerator.Num), new(big.Int).Mul(numerator.Den, den))
	case py.Float:
		num, den, err := floatRatio(float64(numerator))
		if err != nil {
			return nil, err
		}
		if !hasDen {
			return newFraction(num, den), nil
		}
		dd, err := rationalArgument(denArg)
		if err != nil {
			return nil, err
		}
		return makeFraction(num, new(big.Int).Mul(den, dd))
	case py.String:
		num, den, err := parseFractionString(string(numerator))
		if err != nil {
			return nil, err
		}
		if !hasDen {
			return makeFraction(num, den)
		}
		dd, err := rationalArgument(denArg)
		if err != nil {
			return nil, err
		}
		return makeFraction(num, new(big.Int).Mul(den, dd))
	}

	// An object with as_integer_ratio (a Decimal, say) converts exactly.
	if hasRatio, err := callAsIntegerRatio(numArg); err != nil {
		return nil, err
	} else if hasRatio != nil {
		num, den := hasRatio[0], hasRatio[1]
		if !hasDen {
			return makeFraction(num, den)
		}
		dd, err := rationalArgument(denArg)
		if err != nil {
			return nil, err
		}
		return makeFraction(num, new(big.Int).Mul(den, dd))
	}

	return nil, py.ExceptionNewf(py.TypeError,
		"argument should be a string or a Rational instance or have the as_integer_ratio() method")
}

// rationalArgument converts the denominator argument, which must be an int or
// a Fraction, to a big.Int.
func rationalArgument(o py.Object) (*big.Int, error) {
	switch v := o.(type) {
	case py.Int:
		return big.NewInt(int64(v)), nil
	case *py.BigInt:
		return new(big.Int).Set((*big.Int)(v)), nil
	case py.Bool:
		if v {
			return big.NewInt(1), nil
		}
		return big.NewInt(0), nil
	case *Fraction:
		return new(big.Int).Set(v.Num), nil
	}
	return nil, py.ExceptionNewf(py.TypeError, "both arguments should be Rational instances")
}

// callAsIntegerRatio invokes obj.as_integer_ratio() if present, returning a
// two-element big.Int pair, or nil if the method is absent.
func callAsIntegerRatio(o py.Object) ([]*big.Int, error) {
	method, err := py.GetAttrString(o, "as_integer_ratio")
	if err != nil {
		return nil, nil
	}
	if method == py.None {
		return nil, nil
	}
	res, err := py.Call(method, py.Tuple{}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	pair, ok := res.(py.Tuple)
	if !ok || len(pair) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "as_integer_ratio() returned an invalid result")
	}
	num, ok := objectToBig(pair[0])
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "as_integer_ratio() returned a non-integer numerator")
	}
	den, ok := objectToBig(pair[1])
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "as_integer_ratio() returned a non-integer denominator")
	}
	return []*big.Int{num, den}, nil
}

func fractionFromFloat(self py.Object, args py.Tuple) (py.Object, error) {
	var fObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "from_float", 1, 1, &fObj); err != nil {
		return nil, err
	}
	f, ok := fObj.(py.Float)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "from_float() requires a float")
	}
	num, den, err := floatRatio(float64(f))
	if err != nil {
		return nil, err
	}
	return newFraction(num, den), nil
}

func fractionFromDecimal(self py.Object, args py.Tuple) (py.Object, error) {
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "from_decimal", 1, 1, &o); err != nil {
		return nil, err
	}
	ratio, err := callAsIntegerRatio(o)
	if err != nil {
		return nil, err
	}
	if ratio == nil {
		return nil, py.ExceptionNewf(py.TypeError, "from_decimal() requires a decimal.Decimal")
	}
	return makeFraction(ratio[0], ratio[1])
}

func fractionFromNumber(self py.Object, args py.Tuple) (py.Object, error) {
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "from_number", 1, 1, &o); err != nil {
		return nil, err
	}
	switch v := o.(type) {
	case py.Int:
		return newFraction(big.NewInt(int64(v)), big.NewInt(1)), nil
	case *py.BigInt:
		return newFraction(new(big.Int).Set((*big.Int)(v)), big.NewInt(1)), nil
	case *Fraction:
		return newFraction(new(big.Int).Set(v.Num), new(big.Int).Set(v.Den)), nil
	case py.Float:
		num, den, err := floatRatio(float64(v))
		if err != nil {
			return nil, err
		}
		return newFraction(num, den), nil
	}
	if ratio, err := callAsIntegerRatio(o); err != nil {
		return nil, err
	} else if ratio != nil {
		return makeFraction(ratio[0], ratio[1])
	}
	return nil, py.ExceptionNewf(py.TypeError, "from_number() requires a Rational or a float")
}

// floatRatio is CPython's float.as_integer_ratio: an exact ratio of a binary64.
//
// big.Rat's SetFloat64 gives a normalized ratio directly, and its error
// handling matches CPython's - NaN and infinities are the two cases CPython
// refuses.
func floatRatio(f float64) (*big.Int, *big.Int, error) {
	r := new(big.Rat).SetFloat64(f)
	if r == nil {
		if f != f {
			return nil, nil, py.ExceptionNewf(py.ValueError, "cannot convert NaN to integer ratio")
		}
		return nil, nil, py.ExceptionNewf(py.OverflowError, "cannot convert Infinity to integer ratio")
	}
	return new(big.Int).Set(r.Num()), new(big.Int).Set(r.Denom()), nil
}

// parseFractionString parses CPython's _RATIONAL_FORMAT: optional whitespace, an
// optional sign, then either "num/den" or "num[.decimal][Eexp]", then optional
// trailing whitespace.  It returns the raw numerator and denominator, not yet
// reduced.
//
// The regex allows digit runs with single underscores between digits, and the
// two tails are alternatives, so "1.0/2" is not a valid literal.
func parseFractionString(s string) (*big.Int, *big.Int, error) {
	invalid := func() error {
		return py.ExceptionNewf(py.ValueError, "Invalid literal for Fraction: %s", pyStringRepr(s))
	}

	// \A\s*
	rest := strings.TrimLeft(s, whitespaceChars)
	// (?P<sign>[-+]?)
	neg := false
	if len(rest) > 0 && (rest[0] == '+' || rest[0] == '-') {
		neg = rest[0] == '-'
		rest = rest[1:]
	}
	// (?=\d|\.\d)
	if !(len(rest) > 0 && (isDigit(rest[0]) || (rest[0] == '.' && len(rest) > 1 && isDigit(rest[1])))) {
		return nil, nil, invalid()
	}

	// The two tails are mutually exclusive: a '/' selects the num/den form.
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		// (?P<num>\d+(_\d+)*) then \s*/\s*(?P<denom>\d+(_\d+)*) then \s*\z
		numText := rest[:slash]
		denText := strings.TrimLeft(rest[slash+1:], whitespaceChars)
		// The numerator must be a bare digit run here; the empty numerator
		// alternative only pairs with the decimal tail.
		num, ok := bigFromDigitRun(numText)
		if !ok || numText == "" {
			return nil, nil, invalid()
		}
		denText = strings.TrimRight(denText, whitespaceChars)
		den, ok := bigFromDigitRun(denText)
		if !ok || denText == "" {
			return nil, nil, invalid()
		}
		if neg {
			num.Neg(num)
		}
		return num, den, nil
	}

	// num[.decimal][eE exp] form, with a \s*\z tail.
	body := rest
	expText := ""
	if idx := strings.IndexAny(body, "eE"); idx >= 0 {
		expText = body[idx+1:]
		body = body[:idx]
	}
	decText := ""
	hasDecimal := false
	if idx := strings.IndexByte(body, '.'); idx >= 0 {
		decText = body[idx+1:]
		body = body[:idx]
		hasDecimal = true
	}
	numText := body

	expText = strings.TrimRight(expText, whitespaceChars)
	decText = strings.TrimRight(decText, whitespaceChars)
	numText = strings.TrimRight(numText, whitespaceChars)

	num, ok := bigFromDigitRun(numText)
	if !ok {
		return nil, nil, invalid()
	}
	if numText == "" {
		num = big.NewInt(0)
	}
	den := big.NewInt(1)
	if hasDecimal {
		frac, ok := bigFromDigitRun(decText)
		if !ok {
			return nil, nil, invalid()
		}
		if decText == "" {
			// An empty decimal part still requires the digit-run shape to be
			// valid, which it is; it just contributes nothing.
			frac = big.NewInt(0)
		} else {
			// The scale counts digits, not underscores.
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digitCount(decText))), nil)
			num.Mul(num, scale)
			num.Add(num, frac)
			den.Mul(den, scale)
		}
	}
	if expText != "" {
		sign := 1
		if expText[0] == '+' || expText[0] == '-' {
			if expText[0] == '-' {
				sign = -1
			}
			expText = expText[1:]
		}
		if !allDigitsUnderscore(expText) {
			return nil, nil, invalid()
		}
		exp, ok := parseSmallInt(expText)
		if !ok {
			return nil, nil, invalid()
		}
		exp *= sign
		if exp != 0 {
			ten := big.NewInt(10)
			if exp >= 0 {
				num.Mul(num, new(big.Int).Exp(ten, big.NewInt(int64(exp)), nil))
			} else {
				den.Mul(den, new(big.Int).Exp(ten, big.NewInt(int64(-exp)), nil))
			}
		}
	}
	if neg {
		num.Neg(num)
	}
	return num, den, nil
}

// whitespaceChars is the ASCII set CPython's \s matches for these literals.
const whitespaceChars = " \t\n\v\f\r"

// digitCount counts digits, ignoring the underscores between them.
func digitCount(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if isDigit(s[i]) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// normalization

// newFraction builds a Fraction from an already-normalized numerator and
// positive denominator.
func newFraction(num, den *big.Int) *Fraction {
	return &Fraction{Num: num, Den: den}
}

// makeFraction reduces num/den to lowest terms with a positive denominator and
// checks for a zero denominator.
func makeFraction(num, den *big.Int) (*Fraction, error) {
	if den.Sign() == 0 {
		return nil, py.ExceptionNewf(py.ZeroDivisionError, "Fraction(%s, 0)", num.String())
	}
	g := new(big.Int).GCD(nil, nil, new(big.Int).Abs(num), new(big.Int).Abs(den))
	if g.Sign() > 0 {
		num = new(big.Int).Quo(num, g)
		den = new(big.Int).Quo(den, g)
	}
	if den.Sign() < 0 {
		num.Neg(num)
		den.Neg(den)
	}
	return &Fraction{Num: num, Den: den}, nil
}

// fromCoprime is CPython's Fraction._from_coprime_ints: the caller guarantees
// the pair is already in lowest terms with a positive denominator.
func fromCoprime(num, den *big.Int) *Fraction {
	return &Fraction{Num: num, Den: den}
}

// ---------------------------------------------------------------------------
// conversions and helpers

func bigToObject(i *big.Int) py.Object {
	if i.IsInt64() {
		return py.Int(i.Int64())
	}
	return (*py.BigInt)(new(big.Int).Set(i))
}

// objectToBig converts an int-like object to a big.Int.
func objectToBig(o py.Object) (*big.Int, bool) {
	switch v := o.(type) {
	case py.Int:
		return big.NewInt(int64(v)), true
	case *py.BigInt:
		return new(big.Int).Set((*big.Int)(v)), true
	case py.Bool:
		if v {
			return big.NewInt(1), true
		}
		return big.NewInt(0), true
	}
	return nil, false
}

// asFraction converts the operands of a rational monomorphic operator to a
// Fraction pair, mirroring CPython's isinstance(other, (int, Fraction)).
func asFraction(o py.Object) (*Fraction, bool) {
	switch v := o.(type) {
	case *Fraction:
		return v, true
	case py.Int:
		return &Fraction{Num: big.NewInt(int64(v)), Den: big.NewInt(1)}, true
	case *py.BigInt:
		return &Fraction{Num: new(big.Int).Set((*big.Int)(v)), Den: big.NewInt(1)}, true
	case py.Bool:
		n := int64(0)
		if v {
			n = 1
		}
		return &Fraction{Num: big.NewInt(n), Den: big.NewInt(1)}, true
	}
	return nil, false
}

func asFloatValue(f *Fraction) float64 {
	num := new(big.Float).SetInt(f.Num)
	den := new(big.Float).SetInt(f.Den)
	q := new(big.Float).Quo(num, den)
	v, _ := q.Float64()
	return v
}

func (f *Fraction) repr() string {
	return "Fraction(" + f.Num.String() + ", " + f.Den.String() + ")"
}

// pyStringRepr formats s the way CPython's %r does for a str literal: inside
// single quotes, with backslash and single-quote escaped.
func pyStringRepr(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func isSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// parseSmallInt parses a decimal exponent.
func parseSmallInt(s string) (int, bool) {
	clean := strings.ReplaceAll(s, "_", "")
	if clean == "" {
		return 0, false
	}
	v, ok := new(big.Int).SetString(clean, 10)
	if !ok || !v.IsInt64() {
		return 0, false
	}
	return int(v.Int64()), true
}

// bigFromDigitRun parses the digit-run shape \d+(_\d+)* (or the empty string,
// which the num group allows), returning zero for the empty run.  ok is false
// when the text is not a valid run.
func bigFromDigitRun(s string) (*big.Int, bool) {
	if s == "" {
		return big.NewInt(0), true
	}
	if !allDigitsUnderscore(s) {
		return nil, false
	}
	clean := strings.ReplaceAll(s, "_", "")
	v, ok := new(big.Int).SetString(clean, 10)
	return v, ok
}

// allDigitsUnderscore accepts \d+(_\d+)*: a leading digit, underscores only
// between digits, and no trailing underscore.
func allDigitsUnderscore(s string) bool {
	if s == "" {
		return false
	}
	if !isDigit(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if isDigit(s[i]) {
			continue
		}
		if s[i] == '_' && i+1 < len(s) && isDigit(s[i+1]) {
			continue
		}
		return false
	}
	return true
}

func setMethod(name string, fn interface{}, doc string) {
	fractionType.Dict.Set(name, py.MustNewMethod(name, fn, 0, doc))
}

// ---------------------------------------------------------------------------
// arithmetic
//
// The monomorphic operators below are CPython's _add, _mul, _div and friends:
// they work only on two Fractions, and the two-argument entry points apply
// CPython's _operator_fallbacks rules to decide whether the operation stays
// exact (int, Fraction), becomes a float, or is a complex.

// addFractions is CPython's _add.
func addFractions(a, b *Fraction) *Fraction {
	na, da := a.Num, a.Den
	nb, db := b.Num, b.Den
	g := new(big.Int).GCD(nil, nil, da, db)
	if g.Cmp(big.NewInt(1)) == 0 {
		return fromCoprime(new(big.Int).Add(new(big.Int).Mul(na, db), new(big.Int).Mul(da, nb)),
			new(big.Int).Mul(da, db))
	}
	s := new(big.Int).Quo(da, g)
	t := new(big.Int).Add(new(big.Int).Mul(na, new(big.Int).Quo(db, g)), new(big.Int).Mul(nb, s))
	g2 := new(big.Int).GCD(nil, nil, t, g)
	if g2.Cmp(big.NewInt(1)) == 0 {
		return fromCoprime(t, new(big.Int).Mul(s, db))
	}
	return fromCoprime(new(big.Int).Quo(t, g2), new(big.Int).Mul(s, new(big.Int).Quo(db, g2)))
}

// subFractions is CPython's _sub.
func subFractions(a, b *Fraction) *Fraction {
	na, da := a.Num, a.Den
	nb, db := b.Num, b.Den
	g := new(big.Int).GCD(nil, nil, da, db)
	if g.Cmp(big.NewInt(1)) == 0 {
		return fromCoprime(new(big.Int).Sub(new(big.Int).Mul(na, db), new(big.Int).Mul(da, nb)),
			new(big.Int).Mul(da, db))
	}
	s := new(big.Int).Quo(da, g)
	t := new(big.Int).Sub(new(big.Int).Mul(na, new(big.Int).Quo(db, g)), new(big.Int).Mul(nb, s))
	g2 := new(big.Int).GCD(nil, nil, t, g)
	if g2.Cmp(big.NewInt(1)) == 0 {
		return fromCoprime(t, new(big.Int).Mul(s, db))
	}
	return fromCoprime(new(big.Int).Quo(t, g2), new(big.Int).Mul(s, new(big.Int).Quo(db, g2)))
}

// mulFractions is CPython's _mul.
func mulFractions(a, b *Fraction) *Fraction {
	na, da := new(big.Int).Set(a.Num), new(big.Int).Set(a.Den)
	nb, db := new(big.Int).Set(b.Num), new(big.Int).Set(b.Den)
	g1 := new(big.Int).GCD(nil, nil, na, db)
	if g1.Cmp(big.NewInt(1)) > 0 {
		na.Quo(na, g1)
		db.Quo(db, g1)
	}
	g2 := new(big.Int).GCD(nil, nil, nb, da)
	if g2.Cmp(big.NewInt(1)) > 0 {
		nb.Quo(nb, g2)
		da.Quo(da, g2)
	}
	return fromCoprime(new(big.Int).Mul(na, nb), new(big.Int).Mul(db, da))
}

// divFractions is CPython's _div.
func divFractions(a, b *Fraction) *Fraction {
	na, da := new(big.Int).Set(a.Num), new(big.Int).Set(a.Den)
	nb, db := new(big.Int).Set(b.Num), new(big.Int).Set(b.Den)
	g1 := new(big.Int).GCD(nil, nil, na, nb)
	if g1.Cmp(big.NewInt(1)) > 0 {
		na.Quo(na, g1)
		nb.Quo(nb, g1)
	}
	g2 := new(big.Int).GCD(nil, nil, db, da)
	if g2.Cmp(big.NewInt(1)) > 0 {
		da.Quo(da, g2)
		db.Quo(db, g2)
	}
	n := new(big.Int).Mul(na, db)
	d := new(big.Int).Mul(nb, da)
	if d.Sign() < 0 {
		n.Neg(n)
		d.Neg(d)
	}
	return fromCoprime(n, d)
}

// floordivFractions is CPython's _floordiv: the result is an int.
func floordivFractions(a, b *Fraction) *big.Int {
	num := new(big.Int).Mul(a.Num, b.Den)
	den := new(big.Int).Mul(a.Den, b.Num)
	return floorDiv(num, den)
}

func modFractions(a, b *Fraction) *Fraction {
	da, db := a.Den, b.Den
	x := new(big.Int).Mul(a.Num, db)
	y := new(big.Int).Mul(b.Num, da)
	m := new(big.Int).Mod(x, y)
	return &Fraction{Num: m, Den: new(big.Int).Mul(da, db)}
}

// divmodFractions is CPython's _divmod: (a // b, a % b).
func divmodFractions(a, b *Fraction) (*big.Int, *Fraction) {
	da, db := a.Den, b.Den
	x := new(big.Int).Mul(a.Num, db)
	y := new(big.Int).Mul(da, b.Num)
	div := floorDiv(x, y)
	nMod := new(big.Int).Mod(x, y)
	return div, &Fraction{Num: nMod, Den: new(big.Int).Mul(da, db)}
}

// floorDiv is Go's Euclidean floor division on big.Ints: it matches Python's
// // for the sign combinations reached here (the denominator can be negative
// only in an internal intermediate, and the public operators normalize it).
func floorDiv(n, d *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	if r.Sign() != 0 && (r.Sign() < 0) != (d.Sign() < 0) {
		q.Sub(q, big.NewInt(1))
	}
	return q
}

// binaryFraction dispatches a monomorphic operator over the mixed-type rules of
// CPython's _operator_fallbacks.forward: int and Fraction stay exact, float and
// complex fall back to the mixed float operator, anything else is NotImplemented.
func binaryFraction(op string, a *Fraction, other py.Object) (py.Object, error) {
	switch v := other.(type) {
	case *Fraction:
		return applyMono(op, a, v)
	case py.Int:
		return applyMono(op, a, &Fraction{Num: big.NewInt(int64(v)), Den: big.NewInt(1)})
	case *py.BigInt:
		return applyMono(op, a, &Fraction{Num: new(big.Int).Set((*big.Int)(v)), Den: big.NewInt(1)})
	case py.Bool:
		n := int64(0)
		if v {
			n = 1
		}
		return applyMono(op, a, &Fraction{Num: big.NewInt(n), Den: big.NewInt(1)})
	case py.Float:
		return applyFloat(op, asFloatValue(a), float64(v))
	}
	return py.NotImplemented, nil
}

// reverseFraction is CPython's reverse() half: the left operand has already
// failed its own operator, so it is anything but a Fraction here.
func reverseFraction(op string, self *Fraction, other py.Object) (py.Object, error) {
	switch v := other.(type) {
	case py.Int:
		return applyMono(op, &Fraction{Num: big.NewInt(int64(v)), Den: big.NewInt(1)}, self)
	case *py.BigInt:
		return applyMono(op, &Fraction{Num: new(big.Int).Set((*big.Int)(v)), Den: big.NewInt(1)}, self)
	case py.Bool:
		n := int64(0)
		if v {
			n = 1
		}
		return applyMono(op, &Fraction{Num: big.NewInt(n), Den: big.NewInt(1)}, self)
	case py.Float:
		return applyFloat(op, float64(v), asFloatValue(self))
	}
	return py.NotImplemented, nil
}

// applyMono runs a monomorphic operator on two normalized Fractions.
func applyMono(op string, a, b *Fraction) (py.Object, error) {
	switch op {
	case "+":
		return addFractions(a, b), nil
	case "-":
		return subFractions(a, b), nil
	case "*":
		return mulFractions(a, b), nil
	case "/":
		if b.Num.Sign() == 0 {
			return nil, py.ExceptionNewf(py.ZeroDivisionError, "Fraction(%s, 0)", b.Den.String())
		}
		return divFractions(a, b), nil
	case "//":
		if b.Num.Sign() == 0 {
			return nil, py.ExceptionNewf(py.ZeroDivisionError, "Fraction(%s, 0)", b.Den.String())
		}
		return bigToObject(floordivFractions(a, b)), nil
	case "%":
		if b.Num.Sign() == 0 {
			return nil, py.ExceptionNewf(py.ZeroDivisionError, "Fraction(%s, 0)", b.Den.String())
		}
		return modFractions(a, b), nil
	case "divmod":
		if b.Num.Sign() == 0 {
			return nil, py.ExceptionNewf(py.ZeroDivisionError, "Fraction(%s, 0)", b.Den.String())
		}
		div, rem := divmodFractions(a, b)
		return py.Tuple{bigToObject(div), rem}, nil
	}
	return py.NotImplemented, nil
}

// applyFloat runs a monomorphic operator on two float64s, as CPython's
// fallback operator (operator.add and friends) does.
func applyFloat(op string, a, b float64) (py.Object, error) {
	switch op {
	case "+":
		return py.Float(a + b), nil
	case "-":
		return py.Float(a - b), nil
	case "*":
		return py.Float(a * b), nil
	case "/":
		if b == 0 {
			return nil, py.ExceptionNewf(py.ZeroDivisionError, "division by zero")
		}
		return py.Float(a / b), nil
	case "//":
		if b == 0 {
			return nil, py.ExceptionNewf(py.ZeroDivisionError, "division by zero")
		}
		return py.Float(mathFloor(a / b)), nil
	case "%":
		return py.Float(mathMod(a, b)), nil
	}
	return py.NotImplemented, nil
}

// mathFloor is Python's float // (floor division), which Go's math.Floor of the
// quotient gives for the finite values reached here.
func mathFloor(x float64) float64 {
	return math.Floor(x)
}

// mathMod is Python's float %.
func mathMod(a, b float64) float64 {
	m := math.Mod(a, b)
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}

// The exported operator methods on Fraction.

func (f *Fraction) M__add__(other py.Object) (py.Object, error) {
	return binaryFraction("+", f, other)
}
func (f *Fraction) M__radd__(other py.Object) (py.Object, error) {
	return reverseFraction("+", f, other)
}
func (f *Fraction) M__sub__(other py.Object) (py.Object, error) {
	return binaryFraction("-", f, other)
}
func (f *Fraction) M__rsub__(other py.Object) (py.Object, error) {
	return reverseFraction("-", f, other)
}
func (f *Fraction) M__mul__(other py.Object) (py.Object, error) {
	return binaryFraction("*", f, other)
}
func (f *Fraction) M__rmul__(other py.Object) (py.Object, error) {
	return reverseFraction("*", f, other)
}
func (f *Fraction) M__truediv__(other py.Object) (py.Object, error) {
	return binaryFraction("/", f, other)
}
func (f *Fraction) M__rtruediv__(other py.Object) (py.Object, error) {
	return reverseFraction("/", f, other)
}
func (f *Fraction) M__floordiv__(other py.Object) (py.Object, error) {
	return binaryFraction("//", f, other)
}
func (f *Fraction) M__rfloordiv__(other py.Object) (py.Object, error) {
	return reverseFraction("//", f, other)
}
func (f *Fraction) M__mod__(other py.Object) (py.Object, error) {
	return binaryFraction("%", f, other)
}
func (f *Fraction) M__rmod__(other py.Object) (py.Object, error) {
	return reverseFraction("%", f, other)
}

func (f *Fraction) M__divmod__(other py.Object) (py.Object, py.Object, error) {
	res, err := binaryFraction("divmod", f, other)
	if err != nil {
		return nil, nil, err
	}
	if res == py.NotImplemented {
		return py.NotImplemented, py.NotImplemented, nil
	}
	pair := res.(py.Tuple)
	return pair[0], pair[1], nil
}

func (f *Fraction) M__rdivmod__(other py.Object) (py.Object, py.Object, error) {
	res, err := reverseFraction("divmod", f, other)
	if err != nil {
		return nil, nil, err
	}
	if res == py.NotImplemented {
		return py.NotImplemented, py.NotImplemented, nil
	}
	pair := res.(py.Tuple)
	return pair[0], pair[1], nil
}

// ---------------------------------------------------------------------------
// power

func (f *Fraction) M__pow__(other, modulo py.Object) (py.Object, error) {
	if modulo != py.None {
		return py.NotImplemented, nil
	}
	switch b := other.(type) {
	case *Fraction:
		return f.powFraction(b)
	case py.Int:
		return f.powFraction(&Fraction{Num: big.NewInt(int64(b)), Den: big.NewInt(1)})
	case *py.BigInt:
		return f.powFraction(&Fraction{Num: new(big.Int).Set((*big.Int)(b)), Den: big.NewInt(1)})
	case py.Bool:
		n := int64(0)
		if b {
			n = 1
		}
		return f.powFraction(&Fraction{Num: big.NewInt(n), Den: big.NewInt(1)})
	case py.Float:
		return floatPow(asFloatValue(f), float64(b))
	}
	return py.NotImplemented, nil
}

// powFraction is CPython's Fraction.__pow__ for a Rational exponent.
func (f *Fraction) powFraction(b *Fraction) (py.Object, error) {
	if b.Den.Cmp(big.NewInt(1)) == 0 {
		power := b.Num
		if power.Sign() >= 0 {
			return fromCoprime(new(big.Int).Exp(f.Num, power, nil), new(big.Int).Exp(f.Den, power, nil)), nil
		}
		np := new(big.Int).Neg(power)
		switch f.Num.Sign() {
		case 1:
			return fromCoprime(new(big.Int).Exp(f.Den, np, nil), new(big.Int).Exp(f.Num, np, nil)), nil
		case 0:
			den := new(big.Int).Exp(f.Den, np, nil)
			return nil, py.ExceptionNewf(py.ZeroDivisionError, "Fraction(%s, 0)", den.String())
		default:
			dn := new(big.Int).Neg(f.Den)
			nn := new(big.Int).Neg(f.Num)
			return fromCoprime(new(big.Int).Exp(dn, np, nil), new(big.Int).Exp(nn, np, nil)), nil
		}
	}
	// A fractional power is generally irrational, so CPython falls back to a
	// float.
	return floatPow(asFloatValue(f), asFloatValue(b))
}

func (f *Fraction) M__rpow__(other py.Object) (py.Object, error) {
	if self, ok := asFraction(other); ok {
		return self.powFraction(f)
	}
	// other is a float or complex: CPython computes float(other) ** self.
	base, ok := other.(py.Float)
	if !ok {
		return py.NotImplemented, nil
	}
	return floatPow(float64(base), asFloatValue(f))
}

// floatPow is CPython's `float ** b`.  For a negative base and a non-integral
// exponent the result is complex, but this interpreter has no cmath and its
// float ** already yields NaN, so the value is passed straight through; the
// difference is recorded on the __pow__ doc string.
func floatPow(base, exp float64) (py.Object, error) {
	return py.Float(math.Pow(base, exp)), nil
}

// ---------------------------------------------------------------------------
// comparisons

func (f *Fraction) M__eq__(other py.Object) (py.Object, error) {
	return f.richCompare(other, "eq")
}
func (f *Fraction) M__ne__(other py.Object) (py.Object, error) {
	return f.richCompare(other, "ne")
}
func (f *Fraction) M__lt__(other py.Object) (py.Object, error) {
	return f.richCompare(other, "lt")
}
func (f *Fraction) M__le__(other py.Object) (py.Object, error) {
	return f.richCompare(other, "le")
}
func (f *Fraction) M__gt__(other py.Object) (py.Object, error) {
	return f.richCompare(other, "gt")
}
func (f *Fraction) M__ge__(other py.Object) (py.Object, error) {
	return f.richCompare(other, "ge")
}

func (f *Fraction) richCompare(other py.Object, op string) (py.Object, error) {
	if b, ok := asFraction(other); ok {
		return py.NewBool(compareBig(f.Num, f.Den, b.Num, b.Den, op)), nil
	}
	if fl, ok := other.(py.Float); ok {
		v := float64(fl)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			// CPython compares 0.0 with the special value, so any finite
			// Fraction orders the same way relative to it.
			return py.NewBool(compareFloat(0, v, op)), nil
		}
		num, den, err := floatRatio(v)
		if err != nil {
			return nil, err
		}
		return py.NewBool(compareBig(f.Num, f.Den, num, den, op)), nil
	}
	return py.NotImplemented, nil
}

// compareBig compares a/b with c/d exactly.
func compareBig(a, b, c, d *big.Int, op string) bool {
	l := new(big.Int).Mul(a, d)
	r := new(big.Int).Mul(c, b)
	return compareBigInt(l, r, op)
}

func compareBigInt(l, r *big.Int, op string) bool {
	switch op {
	case "eq":
		return l.Cmp(r) == 0
	case "ne":
		return l.Cmp(r) != 0
	case "lt":
		return l.Cmp(r) < 0
	case "le":
		return l.Cmp(r) <= 0
	case "gt":
		return l.Cmp(r) > 0
	case "ge":
		return l.Cmp(r) >= 0
	}
	return false
}

// compareFloat compares a/b with a float, used for the NaN/infinity case.
func compareFloat(a, b float64, op string) bool {
	switch op {
	case "eq":
		return a == b
	case "ne":
		return a != b
	case "lt":
		return a < b
	case "le":
		return a <= b
	case "gt":
		return a > b
	case "ge":
		return a >= b
	}
	return false
}

// ---------------------------------------------------------------------------
// unary, conversion and hash

func (f *Fraction) M__bool__() (py.Object, error) {
	return py.NewBool(f.Num.Sign() != 0), nil
}

func (f *Fraction) M__neg__() (py.Object, error) {
	return fromCoprime(new(big.Int).Neg(f.Num), new(big.Int).Set(f.Den)), nil
}

func (f *Fraction) M__pos__() (py.Object, error) {
	return fromCoprime(new(big.Int).Set(f.Num), new(big.Int).Set(f.Den)), nil
}

func (f *Fraction) M__abs__() (py.Object, error) {
	return fromCoprime(new(big.Int).Abs(f.Num), new(big.Int).Set(f.Den)), nil
}

func (f *Fraction) M__float__() (py.Object, error) {
	return py.Float(asFloatValue(f)), nil
}

func (f *Fraction) M__int__() (py.Object, error) {
	// CPython truncates toward zero.
	return bigToObject(truncBig(f.Num, f.Den)), nil
}

func (f *Fraction) M__trunc__() (py.Object, error) {
	return bigToObject(truncBig(f.Num, f.Den)), nil
}

func (f *Fraction) M__floor__() (py.Object, error) {
	return bigToObject(floorDiv(f.Num, f.Den)), nil
}

func (f *Fraction) M__ceil__() (py.Object, error) {
	// CPython's -(-n // d).
	negNum := new(big.Int).Neg(f.Num)
	return bigToObject(new(big.Int).Neg(floorDiv(negNum, f.Den))), nil
}

// truncBig truncates toward zero.
func truncBig(n, d *big.Int) *big.Int {
	return new(big.Int).Quo(n, d)
}

func (f *Fraction) M__round__(ndigits py.Object) (py.Object, error) {
	if ndigits == py.None {
		d := f.Den
		floor, remainder := new(big.Int).QuoRem(f.Num, d, new(big.Int))
		twice := new(big.Int).Lsh(remainder, 1)
		switch twice.Cmp(d) {
		case -1:
			return bigToObject(floor), nil
		case 1:
			return bigToObject(new(big.Int).Add(floor, big.NewInt(1))), nil
		}
		// The half case rounds to even.
		if new(big.Int).And(floor, big.NewInt(1)).Sign() == 0 {
			return bigToObject(floor), nil
		}
		return bigToObject(new(big.Int).Add(floor, big.NewInt(1))), nil
	}
	nd, err := toInt(ndigits)
	if err != nil {
		return nil, err
	}
	if nd > 0 {
		shift := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(nd)), nil)
		scaled := mulFractions(f, &Fraction{Num: shift, Den: big.NewInt(1)})
		rounded, err := roundToInt(scaled)
		if err != nil {
			return nil, err
		}
		return makeFraction(rounded, shift)
	}
	shift := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-nd)), nil)
	scaled := divFractions(f, &Fraction{Num: shift, Den: big.NewInt(1)})
	rounded, err := roundToInt(scaled)
	if err != nil {
		return nil, err
	}
	return makeFraction(new(big.Int).Mul(rounded, shift), big.NewInt(1))
}

// roundToInt is round(f) for an integral-ish Fraction, returning a big.Int.
func roundToInt(f *Fraction) (*big.Int, error) {
	floor, remainder := new(big.Int).QuoRem(f.Num, f.Den, new(big.Int))
	twice := new(big.Int).Lsh(remainder, 1)
	switch twice.Cmp(f.Den) {
	case -1:
		return floor, nil
	case 1:
		return new(big.Int).Add(floor, big.NewInt(1)), nil
	}
	if new(big.Int).And(floor, big.NewInt(1)).Sign() == 0 {
		return floor, nil
	}
	return new(big.Int).Add(floor, big.NewInt(1)), nil
}

func toInt(o py.Object) (int, error) {
	switch v := o.(type) {
	case py.Int:
		return int(v), nil
	case py.Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case *py.BigInt:
		i, err := v.GoInt()
		if err != nil {
			return 0, err
		}
		return i, nil
	}
	return 0, py.ExceptionNewf(py.TypeError, "an integer is required")
}

// hashModulus is _PyHASH_MODULUS, (1 << 61) - 1, and hashInf is _PyHASH_INF.
var (
	hashModulus = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 61), big.NewInt(1))
	hashInf     = big.NewInt(314159)
)

func (f *Fraction) M__hash__() (py.Object, error) {
	// CPython's _hash_algorithm: dinv is the modular inverse of the
	// denominator modulo _PyHASH_MODULUS; when it does not exist the hash is
	// _PyHASH_INF.  The result is signed by the numerator and -1 maps to -2.
	dinv := new(big.Int).ModInverse(f.Den, hashModulus)
	var h *big.Int
	if dinv == nil {
		h = new(big.Int).Set(hashInf)
	} else {
		nAbs := new(big.Int).Abs(f.Num)
		nAbs.Mod(nAbs, hashModulus)
		h = new(big.Int).Mul(nAbs, dinv)
		h.Mod(h, hashModulus)
	}
	if f.Num.Sign() < 0 {
		h.Neg(h)
	}
	if h.Cmp(big.NewInt(-1)) == 0 {
		h.SetInt64(-2)
	}
	return bigToObject(h), nil
}

func (f *Fraction) M__repr__() (py.Object, error) {
	return py.String(f.repr()), nil
}

func (f *Fraction) M__str__() (py.Object, error) {
	// str is not repr: an integer-valued Fraction prints as a bare integer.
	if f.Den.Cmp(big.NewInt(1)) == 0 {
		return py.String(f.Num.String()), nil
	}
	return py.String(f.Num.String() + "/" + f.Den.String()), nil
}

// ---------------------------------------------------------------------------
// the named public methods

func fractionAsIntegerRatio(self py.Object, args py.Tuple) (py.Object, error) {
	f := self.(*Fraction)
	return py.Tuple{bigToObject(new(big.Int).Set(f.Num)), bigToObject(new(big.Int).Set(f.Den))}, nil
}

func fractionIsInteger(self py.Object, args py.Tuple) (py.Object, error) {
	return py.NewBool(self.(*Fraction).Den.Cmp(big.NewInt(1)) == 0), nil
}

func fractionReduce(self py.Object, args py.Tuple) (py.Object, error) {
	f := self.(*Fraction)
	return py.Tuple{fractionType, py.Tuple{bigToObject(f.Num), bigToObject(f.Den)}}, nil
}

func fractionCopy(self py.Object, args py.Tuple) (py.Object, error) {
	return self, nil
}

func fractionDeepCopy(self py.Object, args py.Tuple) (py.Object, error) {
	return self, nil
}

const limitDenominator_doc = `Closest Fraction to self with denominator at most max_denominator.`

func fractionLimitDenominator(self py.Object, args py.Tuple) (py.Object, error) {
	f := self.(*Fraction)
	maxDen := int64(1000000)
	if len(args) > 0 && args[0] != py.None {
		v, err := toInt(args[0])
		if err != nil {
			return nil, err
		}
		maxDen = int64(v)
	}
	if maxDen < 1 {
		return nil, py.ExceptionNewf(py.ValueError, "max_denominator should be at least 1")
	}
	if f.Den.Cmp(big.NewInt(maxDen)) <= 0 {
		return fromCoprime(new(big.Int).Set(f.Num), new(big.Int).Set(f.Den)), nil
	}
	// CPython's continued-fraction algorithm.
	p0, q0, p1, q1 := big.NewInt(0), big.NewInt(1), big.NewInt(1), big.NewInt(0)
	n, d := new(big.Int).Set(f.Num), new(big.Int).Set(f.Den)
	maxD := big.NewInt(maxDen)
	for {
		a := new(big.Int).Quo(n, d)
		q2 := new(big.Int).Add(q0, new(big.Int).Mul(a, q1))
		if q2.Cmp(maxD) > 0 {
			break
		}
		p0, q0, p1, q1 = p1, q1,
			new(big.Int).Add(p0, new(big.Int).Mul(a, p1)),
			q2
		n, d = d, new(big.Int).Sub(n, new(big.Int).Mul(a, d))
	}
	k := new(big.Int).Quo(new(big.Int).Sub(maxD, q0), q1)
	qk := new(big.Int).Add(q0, new(big.Int).Mul(k, q1)) // q0 + k*q1
	// Compare 2*d*(q0+k*q1) with self._denominator: if the first is <=, then
	// p1/q1 is at least as close as the other candidate.
	lhs := new(big.Int).Mul(new(big.Int).Lsh(d, 1), qk)
	if lhs.Cmp(f.Den) <= 0 {
		return fromCoprime(new(big.Int).Set(p1), new(big.Int).Set(q1)), nil
	}
	return fromCoprime(new(big.Int).Add(p0, new(big.Int).Mul(k, p1)), qk), nil
}
