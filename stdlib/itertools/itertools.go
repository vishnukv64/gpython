// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package itertools provides the implementation of python's 'itertools'
// module.
//
// These are the iterator building blocks.  Each returns a lazy iterator: the
// next value is produced only when it is asked for, which is the whole point
// of the module and what lets it work on an infinite source.
package itertools

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Functional tools for creating and using iterators.

Infinite iterators:
    count(start=0, step=1) --> start, start+step, start+2*step, ...
    cycle(p) --> p0, p1, ... plast, p0, p1, ...
    repeat(elem [,n]) --> elem, elem, elem, ... endlessly or up to n times

Iterators terminating on the shortest input sequence:
    accumulate(p[, func]) --> p0, p0+p1, p0+p1+p2
    chain(p, q, ...) --> p0, p1, ... plast, q0, q1, ...
    compress(data, selectors) --> (d[0] if s[0]), (d[1] if s[1]), ...
    dropwhile(pred, seq) --> seq[n], seq[n+1], starting when pred fails
    filterfalse(pred, seq) --> elements of seq where pred(elem) is False
    groupby(iterable[, keyfunc]) --> sub-iterators grouped by value of keyfunc(v)
    islice(seq, [start,] stop [, step]) --> elements from seq[start:stop:step]
    pairwise(iterable) --> (s[0],s[1]), (s[1],s[2]), ...
    starmap(fun, seq) --> fun(*seq[0]), fun(*seq[1]), ...
    takewhile(pred, seq) --> seq[0], seq[1], until pred fails
    tee(it, n=2) --> (it1, it2, ... itn) splits one iterator into n
    zip_longest(p, q, ...) --> (p[0],q[0]), (p[1],q[1]), ...`

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "count", Value: py.MustNewMethod("count", count, 0, "count(start=0, step=1) --> count object")},
		py.DictEntry{Key: "repeat", Value: py.MustNewMethod("repeat", repeat, 0, "repeat(object [,times]) -> create an iterator which returns the object for the specified number of times.")},
		py.DictEntry{Key: "cycle", Value: py.MustNewMethod("cycle", cycle, 0, "cycle(iterable) --> cycle object: repeat the elements of the iterable forever.")},
		py.DictEntry{Key: "chain", Value: py.MustNewMethod("chain", chain, 0, "chain(*iterables) --> chain object: chain from the first until the last is exhausted.")},
		py.DictEntry{Key: "islice", Value: py.MustNewMethod("islice", islice, 0, "islice(iterable, stop) --> islice object")},
		py.DictEntry{Key: "starmap", Value: py.MustNewMethod("starmap", starmap, 0, "starmap(function, sequence) --> starmap object")},
		py.DictEntry{Key: "accumulate", Value: py.MustNewMethod("accumulate", accumulate, 0, "accumulate(iterable[, func, *, initial=None]) --> accumulate object")},
		py.DictEntry{Key: "takewhile", Value: py.MustNewMethod("takewhile", takewhile, 0, "takewhile(predicate, iterable) --> takewhile object")},
		py.DictEntry{Key: "dropwhile", Value: py.MustNewMethod("dropwhile", dropwhile, 0, "dropwhile(predicate, iterable) --> dropwhile object")},
		py.DictEntry{Key: "filterfalse", Value: py.MustNewMethod("filterfalse", filterfalse, 0, "filterfalse(function or None, sequence) --> filterfalse object")},
		py.DictEntry{Key: "compress", Value: py.MustNewMethod("compress", compress, 0, "compress(data, selectors) --> iterator over selected data")},
		py.DictEntry{Key: "groupby", Value: py.MustNewMethod("groupby", groupby, 0, "groupby(iterable[, keyfunc]) -> create an iterator which returns (key, sub-iterator) grouped by each value of key(value).")},
		py.DictEntry{Key: "zip_longest", Value: py.MustNewMethod("zip_longest", zipLongest, 0, "zip_longest(iter1 [,iter2 [...]], [fillvalue=None]) --> zip_longest object")},
		py.DictEntry{Key: "tee", Value: py.MustNewMethod("tee", tee, 0, "tee(iterable, n=2) --> tuple of n independent iterators.")},
		py.DictEntry{Key: "pairwise", Value: py.MustNewMethod("pairwise", pairwise, 0, "pairwise(iterable) --> pairwise object")},
		py.DictEntry{Key: "product", Value: py.MustNewMethod("product", product, 0, "product(*iterables, repeat=1) --> product object")},
		py.DictEntry{Key: "permutations", Value: py.MustNewMethod("permutations", permutations, 0, "permutations(iterable[, r]) --> permutations object")},
		py.DictEntry{Key: "combinations", Value: py.MustNewMethod("combinations", combinations, 0, "combinations(iterable, r) --> combinations object")},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "itertools",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// collector is a simple iterator over a slice, used where a result is
// materialised rather than being produced lazily.
type collector struct {
	items []py.Object
	pos   int
}

var collectorType = py.NewType("itertools._collector", "An iterator over a materialised sequence.")

func (c *collector) Type() *py.Type { return collectorType }

func (c *collector) M__iter__() (py.Object, error) { return c, nil }

func (c *collector) M__next__() (py.Object, error) {
	if c.pos >= len(c.items) {
		return nil, py.StopIteration
	}
	item := c.items[c.pos]
	c.pos++
	return item, nil
}

func collect(items []py.Object) *collector { return &collector{items: items} }

var _ py.I__iter__ = (*collector)(nil)
var _ py.I__next__ = (*collector)(nil)

// iterOf returns an iterator over an argument.
func iterOf(v py.Object) (py.Object, error) { return py.Iter(v) }

// nextOf advances an iterator, reporting exhaustion.
func nextOf(it py.Object) (py.Object, bool, error) {
	v, err := py.Next(it)
	if err != nil {
		if py.IsException(py.StopIteration, err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return v, true, nil
}

// sequenceOf materialises an iterable, for the operators whose result cannot
// be produced in one pass.
func sequenceOf(v py.Object) ([]py.Object, error) {
	items, err := py.SequenceList(v)
	if err != nil {
		return nil, err
	}
	return items.Items, nil
}

// ---------------------------------------------------------------------------
// infinite iterators

func count(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	start := py.Object(py.Int(0))
	step := py.Object(py.Int(1))
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:count", []string{"start", "step"}, &start, &step); err != nil {
		return nil, err
	}
	s, err := py.IndexInt(start)
	if err != nil {
		return nil, err
	}
	st, err := py.IndexInt(step)
	if err != nil {
		return nil, err
	}
	return &countIter{value: s, step: st}, nil
}

type countIter struct {
	value int
	step  int
}

var countType = py.NewType("itertools.count", "An iterator counting up.")

func (c *countIter) Type() *py.Type { return countType }

func (c *countIter) M__iter__() (py.Object, error) { return c, nil }

func (c *countIter) M__next__() (py.Object, error) {
	v := c.value
	c.value += c.step
	return py.Int(v), nil
}

var (
	_ py.I__iter__ = (*countIter)(nil)
	_ py.I__next__ = (*countIter)(nil)
)

func repeat(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "repeat() needs at least one argument")
	}
	n := -1
	if len(args) >= 2 {
		v, err := py.IndexInt(args[1])
		if err != nil {
			return nil, err
		}
		n = v
	}
	return &repeatIter{value: args[0], remaining: n}, nil
}

type repeatIter struct {
	value     py.Object
	remaining int
}

var repeatType = py.NewType("itertools.repeat", "An iterator repeating one value.")

func (r *repeatIter) Type() *py.Type { return repeatType }

func (r *repeatIter) M__iter__() (py.Object, error) { return r, nil }

func (r *repeatIter) M__next__() (py.Object, error) {
	if r.remaining == 0 {
		return nil, py.StopIteration
	}
	if r.remaining > 0 {
		r.remaining--
	}
	return r.value, nil
}

var (
	_ py.I__iter__ = (*repeatIter)(nil)
	_ py.I__next__ = (*repeatIter)(nil)
)

func cycle(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "cycle() needs an iterable")
	}
	items, err := sequenceOf(args[0])
	if err != nil {
		return nil, err
	}
	return &cycleIter{items: items}, nil
}

type cycleIter struct {
	items []py.Object
	pos   int
}

var cycleType = py.NewType("itertools.cycle", "An iterator repeating a sequence.")

func (c *cycleIter) Type() *py.Type { return cycleType }

func (c *cycleIter) M__iter__() (py.Object, error) { return c, nil }

func (c *cycleIter) M__next__() (py.Object, error) {
	if len(c.items) == 0 {
		return nil, py.StopIteration
	}
	item := c.items[c.pos]
	c.pos = (c.pos + 1) % len(c.items)
	return item, nil
}

var (
	_ py.I__iter__ = (*cycleIter)(nil)
	_ py.I__next__ = (*cycleIter)(nil)
)

// ---------------------------------------------------------------------------
// terminating iterators

func chain(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &chainIter{sources: args, index: 0}, nil
}

type chainIter struct {
	sources py.Tuple
	index   int
	current py.Object
}

var chainType = py.NewType("itertools.chain", "An iterator chaining several iterables.")

func (c *chainIter) Type() *py.Type { return chainType }

func (c *chainIter) M__iter__() (py.Object, error) { return c, nil }

func (c *chainIter) M__next__() (py.Object, error) {
	for {
		if c.current == nil {
			if c.index >= len(c.sources) {
				return nil, py.StopIteration
			}
			it, err := iterOf(c.sources[c.index])
			if err != nil {
				return nil, err
			}
			c.current = it
			c.index++
		}
		v, ok, err := nextOf(c.current)
		if err != nil {
			return nil, err
		}
		if ok {
			return v, nil
		}
		c.current = nil
	}
}

var (
	_ py.I__iter__ = (*chainIter)(nil)
	_ py.I__next__ = (*chainIter)(nil)
)

func islice(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "islice() needs an iterable and a stop")
	}
	it, err := iterOf(args[0])
	if err != nil {
		return nil, err
	}
	start, stop, step := 0, -1, 1
	switch len(args) {
	case 2:
		stop, err = py.IndexInt(args[1])
	case 3:
		start, err = py.IndexInt(args[1])
		if err == nil {
			stop, err = py.IndexInt(args[2])
		}
	case 4:
		start, err = py.IndexInt(args[1])
		if err == nil {
			stop, err = py.IndexInt(args[2])
		}
		if err == nil {
			step, err = py.IndexInt(args[3])
		}
	}
	if err != nil {
		return nil, err
	}
	if step <= 0 {
		return nil, py.ExceptionNewf(py.ValueError, "Step for islice() must be a positive integer or None.")
	}
	return &isliceIter{it: it, start: start, stop: stop, step: step, index: 0}, nil
}

type isliceIter struct {
	it      py.Object
	start   int
	stop    int
	step    int
	index   int
	dropped bool
}

var isliceType = py.NewType("itertools.islice", "An iterator over a slice of another iterator.")

func (s *isliceIter) Type() *py.Type { return isliceType }

func (s *isliceIter) M__iter__() (py.Object, error) { return s, nil }

func (s *isliceIter) M__next__() (py.Object, error) {
	if !s.dropped {
		for i := 0; i < s.start; i++ {
			if _, ok, err := nextOf(s.it); err != nil || !ok {
				s.dropped = true
				return nil, py.StopIteration
			}
		}
		s.dropped = true
	}
	for {
		if s.stop >= 0 && s.index >= s.stop-s.start {
			return nil, py.StopIteration
		}
		v, ok, err := nextOf(s.it)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.StopIteration
		}
		s.index++
		// Take every step-th item, counting from the start of the slice.
		if (s.index-1)%s.step == 0 {
			return v, nil
		}
	}
}

var (
	_ py.I__iter__ = (*isliceIter)(nil)
	_ py.I__next__ = (*isliceIter)(nil)
)

func starmap(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "starmap() needs a function and a sequence")
	}
	it, err := iterOf(args[1])
	if err != nil {
		return nil, err
	}
	return &starmapIter{fn: args[0], it: it}, nil
}

type starmapIter struct {
	fn py.Object
	it py.Object
}

var starmapType = py.NewType("itertools.starmap", "An iterator applying a function to unpacked arguments.")

func (s *starmapIter) Type() *py.Type { return starmapType }

func (s *starmapIter) M__iter__() (py.Object, error) { return s, nil }

func (s *starmapIter) M__next__() (py.Object, error) {
	v, ok, err := nextOf(s.it)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, py.StopIteration
	}
	items, err := sequenceOf(v)
	if err != nil {
		return nil, err
	}
	res, err := py.Call(s.fn, py.Tuple(items), py.StringDict{})
	if err != nil {
		return nil, err
	}
	return res, nil
}

var (
	_ py.I__iter__ = (*starmapIter)(nil)
	_ py.I__next__ = (*starmapIter)(nil)
)

func accumulate(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "accumulate() needs an iterable")
	}
	it, err := iterOf(args[0])
	if err != nil {
		return nil, err
	}
	var fn py.Object
	if len(args) >= 2 && args[1] != py.None {
		fn = args[1]
	}
	var initial py.Object
	hasInitial := false
	if v, ok := kwargs.Get("initial"); ok && v != py.None {
		initial = v
		hasInitial = true
	}
	return &accumulateIter{it: it, fn: fn, total: initial, hasTotal: hasInitial}, nil
}

type accumulateIter struct {
	it       py.Object
	fn       py.Object
	total    py.Object
	hasTotal bool
}

var accumulateType = py.NewType("itertools.accumulate", "An iterator of accumulated sums.")

func (a *accumulateIter) Type() *py.Type { return accumulateType }

func (a *accumulateIter) M__iter__() (py.Object, error) { return a, nil }

func (a *accumulateIter) M__next__() (py.Object, error) {
	if a.hasTotal {
		// Yield the running total, then fold the next value in.
		current := a.total
		v, ok, err := nextOf(a.it)
		if err != nil {
			return nil, err
		}
		if !ok {
			a.hasTotal = false
			return current, nil
		}
		next, err := a.combine(current, v)
		if err != nil {
			return nil, err
		}
		a.total = next
		return current, nil
	}
	v, ok, err := nextOf(a.it)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, py.StopIteration
	}
	if a.total == nil {
		a.total = v
		return v, nil
	}
	next, err := a.combine(a.total, v)
	if err != nil {
		return nil, err
	}
	a.total = next
	return next, nil
}

func (a *accumulateIter) combine(left, right py.Object) (py.Object, error) {
	if a.fn != nil {
		return py.Call(a.fn, py.Tuple{left, right}, py.StringDict{})
	}
	return py.Add(left, right)
}

var (
	_ py.I__iter__ = (*accumulateIter)(nil)
	_ py.I__next__ = (*accumulateIter)(nil)
)

func takewhile(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "takewhile() needs a predicate and an iterable")
	}
	it, err := iterOf(args[1])
	if err != nil {
		return nil, err
	}
	return &takewhileIter{pred: args[0], it: it}, nil
}

type takewhileIter struct {
	pred py.Object
	it   py.Object
	done bool
}

var takewhileType = py.NewType("itertools.takewhile", "An iterator taking while the predicate holds.")

func (t *takewhileIter) Type() *py.Type { return takewhileType }

func (t *takewhileIter) M__iter__() (py.Object, error) { return t, nil }

func (t *takewhileIter) M__next__() (py.Object, error) {
	if t.done {
		return nil, py.StopIteration
	}
	v, ok, err := nextOf(t.it)
	if err != nil {
		return nil, err
	}
	if !ok {
		t.done = true
		return nil, py.StopIteration
	}
	res, err := py.Call(t.pred, py.Tuple{v}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	b, err := py.MakeBool(res)
	if err != nil {
		return nil, err
	}
	if b == py.False {
		t.done = true
		return nil, py.StopIteration
	}
	return v, nil
}

var (
	_ py.I__iter__ = (*takewhileIter)(nil)
	_ py.I__next__ = (*takewhileIter)(nil)
)

func dropwhile(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "dropwhile() needs a predicate and an iterable")
	}
	it, err := iterOf(args[1])
	if err != nil {
		return nil, err
	}
	return &dropwhileIter{pred: args[0], it: it}, nil
}

type dropwhileIter struct {
	pred    py.Object
	it      py.Object
	dropped bool
}

var dropwhileType = py.NewType("itertools.dropwhile", "An iterator dropping while the predicate holds.")

func (d *dropwhileIter) Type() *py.Type { return dropwhileType }

func (d *dropwhileIter) M__iter__() (py.Object, error) { return d, nil }

func (d *dropwhileIter) M__next__() (py.Object, error) {
	for {
		v, ok, err := nextOf(d.it)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.StopIteration
		}
		if !d.dropped {
			res, err := py.Call(d.pred, py.Tuple{v}, py.StringDict{})
			if err != nil {
				return nil, err
			}
			b, err := py.MakeBool(res)
			if err != nil {
				return nil, err
			}
			if b == py.True {
				continue
			}
			// The first value the predicate rejects ends the dropping phase
			// and is itself yielded.
			d.dropped = true
		}
		return v, nil
	}
}

var (
	_ py.I__iter__ = (*dropwhileIter)(nil)
	_ py.I__next__ = (*dropwhileIter)(nil)
)

func filterfalse(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "filterfalse() needs a predicate and an iterable")
	}
	it, err := iterOf(args[1])
	if err != nil {
		return nil, err
	}
	return &filterfalseIter{pred: args[0], it: it}, nil
}

type filterfalseIter struct {
	pred py.Object
	it   py.Object
}

var filterfalseType = py.NewType("itertools.filterfalse", "An iterator yielding the items the predicate rejects.")

func (f *filterfalseIter) Type() *py.Type { return filterfalseType }

func (f *filterfalseIter) M__iter__() (py.Object, error) { return f, nil }

func (f *filterfalseIter) M__next__() (py.Object, error) {
	for {
		v, ok, err := nextOf(f.it)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.StopIteration
		}
		var keep bool
		if f.pred == py.None {
			// With None the test is the truth of the item itself.
			b, err := py.MakeBool(v)
			if err != nil {
				return nil, err
			}
			keep = b == py.False
		} else {
			res, err := py.Call(f.pred, py.Tuple{v}, py.StringDict{})
			if err != nil {
				return nil, err
			}
			b, err := py.MakeBool(res)
			if err != nil {
				return nil, err
			}
			keep = b == py.False
		}
		if keep {
			return v, nil
		}
	}
}

var (
	_ py.I__iter__ = (*filterfalseIter)(nil)
	_ py.I__next__ = (*filterfalseIter)(nil)
)

func compress(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "compress() needs data and selectors")
	}
	data, err := iterOf(args[0])
	if err != nil {
		return nil, err
	}
	sel, err := iterOf(args[1])
	if err != nil {
		return nil, err
	}
	return &compressIter{data: data, sel: sel}, nil
}

type compressIter struct {
	data py.Object
	sel  py.Object
}

var compressType = py.NewType("itertools.compress", "An iterator selecting data by a selector sequence.")

func (c *compressIter) Type() *py.Type { return compressType }

func (c *compressIter) M__iter__() (py.Object, error) { return c, nil }

func (c *compressIter) M__next__() (py.Object, error) {
	for {
		v, ok, err := nextOf(c.data)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.StopIteration
		}
		s, ok, err := nextOf(c.sel)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.StopIteration
		}
		b, err := py.MakeBool(s)
		if err != nil {
			return nil, err
		}
		if b == py.True {
			return v, nil
		}
	}
}

var (
	_ py.I__iter__ = (*compressIter)(nil)
	_ py.I__next__ = (*compressIter)(nil)
)

func pairwise(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "pairwise() needs an iterable")
	}
	it, err := iterOf(args[0])
	if err != nil {
		return nil, err
	}
	return &pairwiseIter{it: it}, nil
}

type pairwiseIter struct {
	it   py.Object
	prev py.Object
	has  bool
}

var pairwiseType = py.NewType("itertools.pairwise", "An iterator of overlapping pairs.")

func (p *pairwiseIter) Type() *py.Type { return pairwiseType }

func (p *pairwiseIter) M__iter__() (py.Object, error) { return p, nil }

func (p *pairwiseIter) M__next__() (py.Object, error) {
	if !p.has {
		v, ok, err := nextOf(p.it)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.StopIteration
		}
		p.prev = v
		p.has = true
	}
	v, ok, err := nextOf(p.it)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, py.StopIteration
	}
	pair := py.Tuple{p.prev, v}
	p.prev = v
	return pair, nil
}

var (
	_ py.I__iter__ = (*pairwiseIter)(nil)
	_ py.I__next__ = (*pairwiseIter)(nil)
)

// zipLongest pads with a fill value to the longest input.
func zipLongest(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	fill := py.Object(py.None)
	var seqs []py.Object
	// The last positional argument may be the fillvalue.
	if len(args) > 0 {
		if kw, ok := kwargs.Get("fillvalue"); ok {
			fill = kw
		} else {
			fill = args[len(args)-1]
			args = args[:len(args)-1]
		}
	}
	for _, a := range args {
		seqs = append(seqs, a)
	}
	iters := make([]py.Object, len(seqs))
	for i, s := range seqs {
		it, err := iterOf(s)
		if err != nil {
			return nil, err
		}
		iters[i] = it
	}
	return &zipLongestIter{iters: iters, fill: fill}, nil
}

type zipLongestIter struct {
	iters []py.Object
	fill  py.Object
}

var zipLongestType = py.NewType("itertools.zip_longest", "An iterator zipping to the longest input.")

func (z *zipLongestIter) Type() *py.Type { return zipLongestType }

func (z *zipLongestIter) M__iter__() (py.Object, error) { return z, nil }

func (z *zipLongestIter) M__next__() (py.Object, error) {
	row := make(py.Tuple, len(z.iters))
	any := false
	for i, it := range z.iters {
		if it == nil {
			row[i] = z.fill
			continue
		}
		v, ok, err := nextOf(it)
		if err != nil {
			return nil, err
		}
		if !ok {
			// This iterator is finished; it becomes padding.
			z.iters[i] = nil
			row[i] = z.fill
			continue
		}
		any = true
		row[i] = v
	}
	if !any {
		return nil, py.StopIteration
	}
	return row, nil
}

var (
	_ py.I__iter__ = (*zipLongestIter)(nil)
	_ py.I__next__ = (*zipLongestIter)(nil)
)

// tee splits an iterator, buffering what the copies have not consumed yet.
func tee(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "tee() needs an iterable")
	}
	n := 2
	if len(args) >= 2 {
		v, err := py.IndexInt(args[1])
		if err != nil {
			return nil, err
		}
		n = v
	}
	// The shared source and buffer are what make the copies independent.
	shared := &teeShared{source: args[0], buffer: []py.Object{}}
	copies := make(py.Tuple, n)
	for i := 0; i < n; i++ {
		copies[i] = &teeIter{shared: shared, pos: 0}
	}
	return copies, nil
}

type teeShared struct {
	source py.Object
	it     py.Object
	buffer []py.Object
	done   bool
}

type teeIter struct {
	shared *teeShared
	pos    int
}

var teeType = py.NewType("itertools._tee", "One of the iterators tee() produced.")

func (t *teeIter) Type() *py.Type { return teeType }

func (t *teeIter) M__iter__() (py.Object, error) { return t, nil }

func (t *teeIter) M__next__() (py.Object, error) {
	s := t.shared
	if s.it == nil {
		it, err := iterOf(s.source)
		if err != nil {
			return nil, err
		}
		s.it = it
	}
	// Serve from the buffer if this copy is behind the furthest one.
	if t.pos < len(s.buffer) {
		v := s.buffer[t.pos]
		t.pos++
		return v, nil
	}
	if s.done {
		return nil, py.StopIteration
	}
	v, ok, err := nextOf(s.it)
	if err != nil {
		return nil, err
	}
	if !ok {
		s.done = true
		return nil, py.StopIteration
	}
	s.buffer = append(s.buffer, v)
	t.pos++
	return v, nil
}

var (
	_ py.I__iter__ = (*teeIter)(nil)
	_ py.I__next__ = (*teeIter)(nil)
)

// ---------------------------------------------------------------------------
// groupby

func groupby(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "groupby() needs an iterable")
	}
	it, err := iterOf(args[0])
	if err != nil {
		return nil, err
	}
	var keyFn py.Object
	if len(args) >= 2 && args[1] != py.None {
		keyFn = args[1]
	}
	return &groupbyIter{it: it, keyFn: keyFn}, nil
}

type groupbyIter struct {
	it      py.Object
	keyFn   py.Object
	current py.Object
	// pending is an item read while closing a group that turned out to
	// belong to the next one; it is yielded first when that group starts.
	pending py.Object
	has     bool
	done    bool
}

var groupbyType = py.NewType("itertools.groupby", "An iterator grouping consecutive equal keys.")

func (g *groupbyIter) Type() *py.Type { return groupbyType }

// keyOf applies the key function, or the identity when there is none.
func (g *groupbyIter) keyOf(v py.Object) (py.Object, error) {
	if g.keyFn != nil {
		return py.Call(g.keyFn, py.Tuple{v}, py.StringDict{})
	}
	return v, nil
}

func (g *groupbyIter) M__iter__() (py.Object, error) { return g, nil }

func (g *groupbyIter) M__next__() (py.Object, error) {
	if g.done {
		return nil, py.StopIteration
	}
	// The first item of the group: the pending one, or the next from the
	// source when a previous group was consumed.
	if !g.has {
		// An item stashed by the previous group's iterator comes first.
		var v py.Object
		if g.pending != nil {
			v = g.pending
			g.pending = nil
		} else {
			var ok bool
			var err error
			v, ok, err = nextOf(g.it)
			if err != nil {
				return nil, err
			}
			if !ok {
				g.done = true
				return nil, py.StopIteration
			}
		}
		key, err := g.keyOf(v)
		if err != nil {
			return nil, err
		}
		g.current = key
		g.has = true
		return py.Tuple{key, &groupIter{parent: g, first: v}}, nil
	}
	// The group's iterator exhausted; take the next item, which begins a new
	// group.
	v, ok, err := nextOf(g.it)
	if err != nil {
		return nil, err
	}
	if !ok {
		g.done = true
		return nil, py.StopIteration
	}
	key, err := g.keyOf(v)
	if err != nil {
		return nil, err
	}
	g.current = key
	return py.Tuple{key, &groupIter{parent: g, first: v}}, nil
}

// groupIter yields one group's items, stopping when the key changes.
type groupIter struct {
	parent *groupbyIter
	first  py.Object
	used   bool
	done   bool
}

var groupIterType = py.NewType("itertools._grouper", "The sub-iterator of one group.")

func (g *groupIter) Type() *py.Type { return groupIterType }

func (g *groupIter) M__iter__() (py.Object, error) { return g, nil }

func (g *groupIter) M__next__() (py.Object, error) {
	if g.done {
		return nil, py.StopIteration
	}
	if !g.used {
		g.used = true
		return g.first, nil
	}
	v, ok, err := nextOf(g.parent.it)
	if err != nil {
		return nil, err
	}
	if !ok {
		g.done = true
		return nil, py.StopIteration
	}
	key, err := g.parent.keyOf(v)
	if err != nil {
		return nil, err
	}
	eq, err := py.Eq(key, g.parent.current)
	if err != nil {
		return nil, err
	}
	if eq != py.True {
		// A different key: this item belongs to the next group.  It is
		// stashed by treating it as the parent's pending item, which the
		// parent picks up when this group is finished.
		g.parent.has = false
		g.parent.pending = v
		g.done = true
		return nil, py.StopIteration
	}
	return v, nil
}

var (
	_ py.I__iter__ = (*groupIter)(nil)
	_ py.I__next__ = (*groupIter)(nil)
)

// ---------------------------------------------------------------------------
// combinatoric iterators

func product(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	repeat := 1
	if v, ok := kwargs.Get("repeat"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		repeat = n
	}
	var pools [][]py.Object
	for _, a := range args {
		items, err := sequenceOf(a)
		if err != nil {
			return nil, err
		}
		for i := 0; i < repeat; i++ {
			pools = append(pools, items)
		}
	}
	return &productIter{pools: pools, index: nil}, nil
}

type productIter struct {
	pools [][]py.Object
	index []int
}

var productType = py.NewType("itertools.product", "An iterator of the cartesian product.")

func (p *productIter) Type() *py.Type { return productType }

func (p *productIter) M__iter__() (py.Object, error) { return p, nil }

func (p *productIter) M__next__() (py.Object, error) {
	if len(p.pools) == 0 {
		return nil, py.StopIteration
	}
	if p.index == nil {
		p.index = make([]int, len(p.pools))
	} else {
		// Advance the odometer, least significant digit first.
		i := len(p.index) - 1
		for ; i >= 0; i-- {
			p.index[i]++
			if p.index[i] < len(p.pools[i]) {
				break
			}
			p.index[i] = 0
		}
		if i < 0 {
			return nil, py.StopIteration
		}
	}
	row := make(py.Tuple, len(p.pools))
	for i, pool := range p.pools {
		row[i] = pool[p.index[i]]
	}
	return row, nil
}

var (
	_ py.I__iter__ = (*productIter)(nil)
	_ py.I__next__ = (*productIter)(nil)
)

func permutations(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "permutations() needs an iterable")
	}
	items, err := sequenceOf(args[0])
	if err != nil {
		return nil, err
	}
	r := len(items)
	if len(args) >= 2 && args[1] != py.None {
		v, err := py.IndexInt(args[1])
		if err != nil {
			return nil, err
		}
		r = v
	}
	return &permIter{items: items, r: r}, nil
}

type permIter struct {
	items []py.Object
	r     int
	built bool
	all   []py.Object
	pos   int
}

var permType = py.NewType("itertools.permutations", "An iterator of permutations.")

func (p *permIter) Type() *py.Type { return permType }

func (p *permIter) M__iter__() (py.Object, error) { return p, nil }

func (p *permIter) M__next__() (py.Object, error) {
	if !p.built {
		p.all = permutationsOf(p.items, p.r)
		p.built = true
	}
	if p.pos >= len(p.all) {
		return nil, py.StopIteration
	}
	v := p.all[p.pos]
	p.pos++
	return v, nil
}

// permutationsOf builds every r-length permutation of items.
func permutationsOf(items []py.Object, r int) []py.Object {
	if r < 0 || r > len(items) {
		return nil
	}
	out := []py.Object{}
	used := make([]bool, len(items))
	current := make([]py.Object, 0, r)
	var walk func()
	walk = func() {
		if len(current) == r {
			row := make(py.Tuple, r)
			copy(row, current)
			out = append(out, row)
			return
		}
		for i := range items {
			if used[i] {
				continue
			}
			used[i] = true
			current = append(current, items[i])
			walk()
			current = current[:len(current)-1]
			used[i] = false
		}
	}
	walk()
	return out
}

var (
	_ py.I__iter__ = (*permIter)(nil)
	_ py.I__next__ = (*permIter)(nil)
)

func combinations(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "combinations() needs an iterable and a length")
	}
	items, err := sequenceOf(args[0])
	if err != nil {
		return nil, err
	}
	r, err := py.IndexInt(args[1])
	if err != nil {
		return nil, err
	}
	return &combIter{items: items, r: r}, nil
}

type combIter struct {
	items []py.Object
	r     int
	built bool
	all   []py.Object
	pos   int
}

var combType = py.NewType("itertools.combinations", "An iterator of combinations.")

func (c *combIter) Type() *py.Type { return combType }

func (c *combIter) M__iter__() (py.Object, error) { return c, nil }

func (c *combIter) M__next__() (py.Object, error) {
	if !c.built {
		c.all = combinationsOf(c.items, c.r)
		c.built = true
	}
	if c.pos >= len(c.all) {
		return nil, py.StopIteration
	}
	v := c.all[c.pos]
	c.pos++
	return v, nil
}

func combinationsOf(items []py.Object, r int) []py.Object {
	if r < 0 || r > len(items) {
		return nil
	}
	out := []py.Object{}
	current := make([]py.Object, 0, r)
	var walk func(start int)
	walk = func(start int) {
		if len(current) == r {
			row := make(py.Tuple, r)
			copy(row, current)
			out = append(out, row)
			return
		}
		for i := start; i < len(items); i++ {
			current = append(current, items[i])
			walk(i + 1)
			current = current[:len(current)-1]
		}
	}
	walk(0)
	return out
}

var (
	_ py.I__iter__ = (*combIter)(nil)
	_ py.I__next__ = (*combIter)(nil)
)
