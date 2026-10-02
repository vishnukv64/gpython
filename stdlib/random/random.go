// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package random provides the implementation of python's 'random' module.
//
// The generator is Go's math/rand.  Seeding is deterministic: seed(n) installs
// a fresh rand.Rand built from a fixed source, so the same seed followed by the
// same calls gives the same results on every run.  The values are NOT the ones
// CPython produces - its Mersenne twister and Go's generator are different
// algorithms and neither is reproducible from the other.  What is guaranteed
// is the property programs actually rely on: a seeded sequence repeats.
//
// Every function is also a method of the Random class; the module-level
// functions operate on one module-global instance, as CPython's do.
package random

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/big"
	mathrand "math/rand"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Random variable generators.

    bytes
           uniform bytes (2**32 bits)

    int
           uniform within range

    sequences
           pick random element
           pick random sample
           pick weighted random sample
           generate random permutation

    distributions on the real line:
           uniform
           normal (Gaussian)
           negative exponential

This implementation uses Go's math/rand generator.  A seeded generator is
deterministic and repeats exactly, but it does not reproduce CPython's sequence
of values: the two generators are different algorithms.`

// ---------------------------------------------------------------------------
// Generator state

// Random is the Python-visible generator object.
//
// math/rand's Rand is not safe for concurrent use and this interpreter runs
// Python threads on real goroutines, so a generator that is reachable from more
// than one thread needs its own lock.  The module-global one has a mutex; an
// individual Random object created by a script is used from one thread in
// practice, so the cost of locking every draw is not paid there.
type Random struct {
	src *mathrand.Rand
	// gauss() caches the second deviate of a Box-Muller pair, so two
	// successive gauss() calls are not independent.  This mirrors CPython,
	// whose gauss() does the same.
	haveGauss bool
	nextGauss float64
}

var RandomType = py.NewTypeX("random.Random", "Random() -> create a random number generator with its own internal state.", randomNew, nil)

func (r *Random) Type() *py.Type { return RandomType }

func (r *Random) M__repr__() (py.Object, error) {
	return py.String("<random.Random object>"), nil
}

func newRNG(seed int64) *Random {
	return &Random{src: mathrand.New(mathrand.NewSource(seed))}
}

// float64 returns a float in [0.0, 1.0).  Go's Float64 is documented to return
// [0.0, 1.0) and CPython's random() is also [0.0, 1.0), so the ranges agree.
func (r *Random) float64() float64 { return r.src.Float64() }

// seedFromString derives a deterministic seed from a str/bytes/bytearray in the
// same spirit as CPython, where every byte of the value contributes.
func seedFromString(s string) int64 {
	h := sha256.Sum256([]byte(s))
	return int64(binary.LittleEndian.Uint64(h[:8]))
}

// randomNew implements Random([seed]).
func randomNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var x py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:Random", []string{"x"}, &x); err != nil {
		return nil, err
	}
	r := newRNG(0)
	if x != py.None {
		if err := r.seed(x); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// seed applies one of the seeding forms CPython accepts.
func (r *Random) seed(x py.Object) error {
	switch v := x.(type) {
	case py.NoneType:
		r.src = mathrand.New(mathrand.NewSource(unpredictableSeed()))
	case py.Int:
		r.src = mathrand.New(mathrand.NewSource(int64(v)))
	case py.Bool:
		// bool is an int subclass, so True/False seed 1/0.
		if bool(v) {
			r.src = mathrand.New(mathrand.NewSource(1))
		} else {
			r.src = mathrand.New(mathrand.NewSource(0))
		}
	case py.String:
		r.src = mathrand.New(mathrand.NewSource(seedFromString(string(v))))
	case py.Bytes:
		r.src = mathrand.New(mathrand.NewSource(seedFromString(string(v))))
	case *py.BigInt:
		i, err := v.Int()
		if err != nil {
			// A seed too large for an int64 still seeds deterministically, via
			// its decimal form.
			text, terr := py.ReprAsString(v)
			if terr != nil {
				return terr
			}
			r.src = mathrand.New(mathrand.NewSource(seedFromString(text)))
			return nil
		}
		r.src = mathrand.New(mathrand.NewSource(int64(i)))
	case py.Float:
		// CPython accepts a float seed and hashes it; hashing the float bits is
		// deterministic and just as arbitrary.
		r.src = mathrand.New(mathrand.NewSource(int64(math.Float64bits(float64(v)))))
	default:
		return py.ExceptionNewf(py.TypeError, "The only supported seed types are: None, int, float, str, bytes, and bytearray.")
	}
	// A reseed invalidates the cached gauss value so the next gauss() is
	// independent of the previous stream, as in CPython.
	r.haveGauss = false
	return nil
}

// unpredictableSeed obtains entropy for the no-argument seeding form from Go's
// global source rather than a syscall, so this package stays platform-neutral.
func unpredictableSeed() int64 { return mathrand.Int63() }

// index returns a uniform value in [0, n) by rejection sampling on 63 bits, so
// no value is favoured by a modulo bias.
func (r *Random) index(n int64) int64 {
	if n <= 1 {
		return 0
	}
	// Int63n already rejects the incomplete top range, so it is exactly
	// uniform for any n that fits in an int63.
	return r.src.Int63n(n)
}

// gaussValue returns one standard normal deviate, caching the other half of the
// pair exactly as CPython's gauss() does.
func (r *Random) gaussValue() float64 {
	if r.haveGauss {
		r.haveGauss = false
		return r.nextGauss
	}
	// Marsaglia's polar method.  Two uniforms are consumed; the second normal
	// is cached so the next call costs nothing.
	for {
		x1 := 2*r.float64() - 1
		x2 := 2*r.float64() - 1
		w := x1*x1 + x2*x2
		if w >= 1 || w == 0 {
			continue
		}
		scale := math.Sqrt(-2 * math.Log(w) / w)
		r.nextGauss = x2 * scale
		r.haveGauss = true
		return x1 * scale
	}
}

// ---------------------------------------------------------------------------
// Implementation of each operation, shared by the class methods and the
// module-level functions.

func rndSeed(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var x py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:seed", []string{"a"}, &x); err != nil {
		return nil, err
	}
	if err := r.seed(x); err != nil {
		return nil, err
	}
	return py.None, nil
}

func rndRandom(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := py.UnpackTuple(args, py.StringDict{}, "random", 0, 0); err != nil {
		return nil, err
	}
	return py.Float(r.float64()), nil
}

func rndRandbytes(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var nObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "randbytes", 1, 1, &nObj); err != nil {
		return nil, err
	}
	n, err := py.IndexInt(nObj)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, py.ExceptionNewf(py.ValueError, "Cannot convert negative int")
	}
	// CPython draws whole 32 bit words and truncates, so it consumes
	// ceil(n/4) words.  Match that shape so a seeded sequence advances alike.
	out := make([]byte, n)
	for i := 0; i < n; i += 4 {
		w := r.src.Uint32()
		for j := 0; j < 4 && i+j < n; j++ {
			out[i+j] = byte(w >> (8 * uint(j)))
		}
	}
	return py.Bytes(out), nil
}

// rndGetrandbits returns a uniformly distributed int with exactly k random
// bits.
//
// For k <= 64 the result is built from whole 32 bit words and the top word is
// drawn repeatedly until it is below the requested bound, which removes the
// modulo bias a bare mask-and-return would introduce in the top bits.
func rndGetrandbits(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var kObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "getrandbits", 1, 1, &kObj); err != nil {
		return nil, err
	}
	k, err := py.IndexInt(kObj)
	if err != nil {
		return nil, err
	}
	if k < 0 {
		return nil, py.ExceptionNewf(py.ValueError, "number of bits must be non-negative")
	}
	if k == 0 {
		return py.Int(0), nil
	}

	// words holds ceil(k/32) 32 bit words, most significant last so that the
	// assembly loop below is a plain little-endian accumulation.
	nwords := (k + 31) / 32
	words := make([]uint32, nwords)
	topBits := k - (nwords-1)*32 // 1..32

	for i := 0; i < nwords; i++ {
		words[i] = r.src.Uint32()
	}
	// Mask the top word to topBits and reject until it is a full topBits-bit
	// value, so every bit position is equally likely.
	mask := uint32(0xFFFFFFFF)
	if topBits < 32 {
		mask = uint32(1)<<uint(topBits) - 1
	}
	words[nwords-1] &= mask
	// Rejection: redraw the top word until it is below the exact power-of-two
	// bound.  When topBits is 32 the mask is already the full word and the loop
	// body never repeats.
	if topBits < 32 {
		limit := uint32(1) << uint(topBits)
		for words[nwords-1] >= limit {
			words[nwords-1] = r.src.Uint32() & mask
		}
	}

	if k <= 64 {
		// Assemble little-endian into a uint64.  A value at or above 2**63 does
		// not fit an Int (which is a signed 64 bit value), so it is handed back as
		// a big integer; CPython's getrandbits always returns a non-negative int,
		// and casting the uint64 to int64 would wrap it negative.
		var acc uint64
		for i, w := range words {
			acc |= uint64(w) << (32 * uint(i))
		}
		if acc <= math.MaxInt64 {
			return py.Int(int64(acc)), nil
		}
		return (*py.BigInt)(new(big.Int).SetUint64(acc)), nil
	}

	// More than 64 bits: accumulate into a big integer, most significant word
	// first.
	acc := new(big.Int)
	for i := nwords - 1; i >= 0; i-- {
		acc = acc.Lsh(acc, 32)
		acc = acc.Or(acc, new(big.Int).SetUint64(uint64(words[i])))
	}
	return (*py.BigInt)(acc), nil
}

func rndRandint(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var aObj, bObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "randint", 2, 2, &aObj, &bObj); err != nil {
		return nil, err
	}
	a, err := py.IndexInt(aObj)
	if err != nil {
		return nil, err
	}
	b, err := py.IndexInt(bObj)
	if err != nil {
		return nil, err
	}
	if int64(a) > int64(b) {
		return nil, py.ExceptionNewf(py.ValueError, "empty range in randint(%d, %d)", a, b)
	}
	span := int64(b) - int64(a) + 1
	return py.Int(int64(a) + r.index(span)), nil
}

// randrangeParts resolves the one/two/three argument forms of randrange.
func randrangeParts(args py.Tuple) (start, stop, step int64, err error) {
	step = 1
	toInt64 := func(o py.Object) (int64, error) {
		n, err := py.IndexInt(o)
		if err != nil {
			return 0, err
		}
		return int64(n), nil
	}
	switch len(args) {
	case 1:
		stop, err = toInt64(args[0])
	case 2:
		if start, err = toInt64(args[0]); err != nil {
			return
		}
		stop, err = toInt64(args[1])
	case 3:
		if start, err = toInt64(args[0]); err != nil {
			return
		}
		if stop, err = toInt64(args[1]); err != nil {
			return
		}
		step, err = toInt64(args[2])
	default:
		err = py.ExceptionNewf(py.TypeError, "randrange expected at most 3 arguments, got %d", len(args))
	}
	return
}

// rangeLen is the number of integers in range(start, stop, step), or a
// non-positive number when the arguments describe an empty range.
func rangeLen(start, stop, step int64) int64 {
	if step > 0 {
		if start >= stop {
			return 0
		}
		return (stop - start + step - 1) / step
	}
	if start <= stop {
		return 0
	}
	return (start - stop - step - 1) / (-step)
}

func rndRandrange(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	start, stop, step, err := randrangeParts(args)
	if err != nil {
		return nil, err
	}
	if step == 0 {
		return nil, py.ExceptionNewf(py.ValueError, "zero step for randrange()")
	}
	n := rangeLen(start, stop, step)
	if n <= 0 {
		return nil, py.ExceptionNewf(py.ValueError, "empty range for randrange()")
	}
	return py.Int(start + r.index(n)*step), nil
}

func rndChoice(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var seq py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "choice", 1, 1, &seq); err != nil {
		return nil, err
	}
	items, err := py.SequenceList(seq)
	if err != nil {
		return nil, err
	}
	if len(items.Items) == 0 {
		return nil, py.ExceptionNewf(py.IndexError, "Cannot choose from an empty sequence")
	}
	return items.Items[r.index(int64(len(items.Items)))], nil
}

func rndChoices(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var population py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "choices", 1, 1, &population); err != nil {
		return nil, err
	}
	var weights py.Object = py.None
	var cumWeights py.Object = py.None
	var k py.Object = py.Int(1)
	if err := py.ParseTupleAndKeywords(py.Tuple{}, kwargs, "|OOO:choices",
		[]string{"weights", "cum_weights", "k"}, &weights, &cumWeights, &k); err != nil {
		return nil, err
	}
	if weights != py.None && cumWeights != py.None {
		return nil, py.ExceptionNewf(py.TypeError, "Cannot specify both weights and cumulative weights")
	}
	items, err := py.SequenceList(population)
	if err != nil {
		return nil, err
	}
	n := len(items.Items)
	kn, err := py.IndexInt(k)
	if err != nil {
		return nil, err
	}
	if kn < 0 {
		return nil, py.ExceptionNewf(py.ValueError, "number of choices must be non-negative")
	}
	if n == 0 {
		if kn > 0 {
			return nil, py.ExceptionNewf(py.IndexError, "Cannot choose from an empty sequence")
		}
		return py.NewListFromItems(nil), nil
	}

	// cum holds the running total; every index is then found by a scan for the
	// first entry strictly greater than a uniform draw in [0, total).
	cum := make([]float64, n)
	switch {
	case weights != py.None:
		ws, err := py.SequenceList(weights)
		if err != nil {
			return nil, err
		}
		if len(ws.Items) != n {
			return nil, py.ExceptionNewf(py.ValueError, "The number of weights does not match the population")
		}
		total := 0.0
		for i, w := range ws.Items {
			f, err := py.FloatAsFloat64(w)
			if err != nil {
				return nil, err
			}
			if f < 0 {
				return nil, py.ExceptionNewf(py.ValueError, "Total of weights must be greater than zero")
			}
			total += f
			cum[i] = total
		}
		if total <= 0 {
			return nil, py.ExceptionNewf(py.ValueError, "Total of weights must be greater than zero")
		}
	case cumWeights != py.None:
		ws, err := py.SequenceList(cumWeights)
		if err != nil {
			return nil, err
		}
		if len(ws.Items) != n {
			return nil, py.ExceptionNewf(py.ValueError, "The number of weights does not match the population")
		}
		last := 0.0
		for i, w := range ws.Items {
			f, err := py.FloatAsFloat64(w)
			if err != nil {
				return nil, err
			}
			if f < 0 || f < last {
				return nil, py.ExceptionNewf(py.ValueError, "Cumulative weights must be non-negative and non-decreasing")
			}
			last = f
			cum[i] = f
		}
		if last <= 0 {
			return nil, py.ExceptionNewf(py.ValueError, "Total of weights must be greater than zero")
		}
	default:
		// With no weights every element is equally likely.
		for i := range cum {
			cum[i] = float64(i + 1)
		}
	}
	total := cum[n-1]

	out := make([]py.Object, kn)
	for i := 0; i < kn; i++ {
		x := r.float64() * total
		// The first entry strictly greater than x, matching CPython's
		// bisect_right on the cumulative table.
		idx := 0
		for idx < n-1 && cum[idx] <= x {
			idx++
		}
		out[i] = items.Items[idx]
	}
	return py.NewListFromItems(out), nil
}

func rndUniform(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var aObj, bObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "uniform", 2, 2, &aObj, &bObj); err != nil {
		return nil, err
	}
	a, err := py.FloatAsFloat64(aObj)
	if err != nil {
		return nil, err
	}
	b, err := py.FloatAsFloat64(bObj)
	if err != nil {
		return nil, err
	}
	// CPython computes a + (b-a)*random(), which can return b when rounding
	// pushes the product to 1.0.  Reproduce that rather than using a safer
	// closed-interval formula.
	return py.Float(a + (b-a)*r.float64()), nil
}

func rndShuffle(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var x py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "shuffle", 1, 1, &x); err != nil {
		return nil, err
	}
	l, ok := x.(*py.List)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "Random.shuffle() can only be called on a list")
	}
	// Fisher-Yates from the end, in place.
	for i := len(l.Items) - 1; i > 0; i-- {
		j := r.index(int64(i + 1))
		l.Items[i], l.Items[j] = l.Items[j], l.Items[i]
	}
	return py.None, nil
}

func rndSample(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var population, kObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "sample", 2, 2, &population, &kObj); err != nil {
		return nil, err
	}
	items, err := py.SequenceList(population)
	if err != nil {
		return nil, err
	}
	n := len(items.Items)
	k, err := py.IndexInt(kObj)
	if err != nil {
		return nil, err
	}
	if k < 0 || k > n {
		return nil, py.ExceptionNewf(py.ValueError, "Sample larger than population or is negative")
	}
	// Selection sampling: walk the population once, taking each element with
	// the probability the remaining draws require.  This yields a uniform
	// subset in O(n) without materialising a full shuffle.
	out := make([]py.Object, 0, k)
	remaining := n
	for _, item := range items.Items {
		if int64(k-len(out)) > 0 && r.index(int64(remaining)) < int64(k-len(out)) {
			out = append(out, item)
		}
		remaining--
		if len(out) == k {
			break
		}
	}
	return py.NewListFromItems(out), nil
}

func rndGauss(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var muObj, sigmaObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "gauss", 2, 2, &muObj, &sigmaObj); err != nil {
		return nil, err
	}
	mu, err := py.FloatAsFloat64(muObj)
	if err != nil {
		return nil, err
	}
	sigma, err := py.FloatAsFloat64(sigmaObj)
	if err != nil {
		return nil, err
	}
	if sigma == 0 {
		// CPython returns mu unchanged and draws nothing when sigma is zero.
		return py.Float(mu), nil
	}
	return py.Float(mu + sigma*r.gaussValue()), nil
}

func rndNormalvariate(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var muObj, sigmaObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "normalvariate", 2, 2, &muObj, &sigmaObj); err != nil {
		return nil, err
	}
	mu, err := py.FloatAsFloat64(muObj)
	if err != nil {
		return nil, err
	}
	sigma, err := py.FloatAsFloat64(sigmaObj)
	if err != nil {
		return nil, err
	}
	if sigma == 0 {
		return py.Float(mu), nil
	}
	// Unlike gauss(), CPython's normalvariate discards the second normal of
	// the pair rather than caching it.  Do the same so the two functions
	// consume different amounts of the stream, as in CPython.
	d := r.gaussValue()
	r.haveGauss = false
	return py.Float(mu + sigma*d), nil
}

func rndExpovariate(r *Random, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var lambdObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "expovariate", 1, 1, &lambdObj); err != nil {
		return nil, err
	}
	lambd, err := py.FloatAsFloat64(lambdObj)
	if err != nil {
		return nil, err
	}
	if lambd == 0 {
		// -log(u)/lambd with lambd == 0 divides by zero, as in CPython.
		return nil, py.ExceptionNewf(py.ZeroDivisionError, "division by zero")
	}
	// CPython draws -log(1.0 - random()) / lambd.
	return py.Float(-math.Log(1.0-r.float64()) / lambd), nil
}

// ---------------------------------------------------------------------------
// Registration

// classMethod wraps an operation as a method of Random.
func classMethod(name, doc string, fn func(*Random, py.Tuple, py.StringDict) (py.Object, error)) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return fn(self.(*Random), args, kwargs)
	}, 0, doc)
}

// moduleFn wraps an operation as a module-level function acting on the
// module-global generator.
func moduleFn(name, doc string, fn func(*Random, py.Tuple, py.StringDict) (py.Object, error)) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return fn(globalRNG, args, kwargs)
	}, 0, doc)
}

// globalRNG backs the module-level functions.  It is seeded with a fixed value
// so that, absent an explicit seed() call, importing the module is reproducible;
// seed() replaces it with an entropy-seeded generator when called with no
// argument.
var globalRNG = newRNG(1)

// methods is the single table both the class and the module are built from.
var methods = []struct {
	name, doc string
	fn        func(*Random, py.Tuple, py.StringDict) (py.Object, error)
}{
	{"seed", "Initialize internal state from a seed.", rndSeed},
	{"random", "random() -> x in the interval [0, 1).", rndRandom},
	{"randbytes", "Generate n random bytes.", rndRandbytes},
	{"getrandbits", "getrandbits(k) -> x.  Generates an int with k random bits.", rndGetrandbits},
	{"randint", "Return random integer in range [a, b], including both end points.", rndRandint},
	{"randrange", "Choose a random item from range(start, stop[, step]).", rndRandrange},
	{"choice", "Choose a random element from a non-empty sequence.", rndChoice},
	{"choices", "Returns a k sized list of elements chosen from the population with replacement.", rndChoices},
	{"uniform", "Get a random number in the range [a, b) or [a, b] depending on rounding.", rndUniform},
	{"shuffle", "Shuffle list x in place, and return None.", rndShuffle},
	{"sample", "Chooses k unique random elements from a population sequence or set.", rndSample},
	{"gauss", "Gaussian distribution.", rndGauss},
	{"normalvariate", "Normal distribution.", rndNormalvariate},
	{"expovariate", "Exponential distribution.", rndExpovariate},
}

func init() {
	moduleMethods := make([]*py.Method, 0, len(methods))
	for _, m := range methods {
		RandomType.Dict.Set(m.name, classMethod(m.name, m.doc, m.fn))
		moduleMethods = append(moduleMethods, moduleFn(m.name, m.doc, m.fn))
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "random",
			Doc:  module_doc,
		},
		Methods: moduleMethods,
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "Random", Value: RandomType},
		),
	})
}
