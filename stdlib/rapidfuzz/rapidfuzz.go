// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rapidfuzz provides a Go-native 'rapidfuzz' package: the subset of
// the API that cleo (hence poetry) and common callers use.
//
// Real rapidfuzz is a C++ extension built with CMake, which gpython cannot
// build.  This covers rapidfuzz.distance.Levenshtein (distance, similarity,
// normalized_distance, normalized_similarity with weights, processor,
// score_cutoff, score_hint), rapidfuzz.fuzz.ratio and
// rapidfuzz.utils.default_process, with the semantics of the compiled
// rapidfuzz 3.14.6 (checked against a CPython-generated golden).
//
// Not provided: the other distance metrics, the other fuzz scorers,
// rapidfuzz.process and editops/opcodes.  Neither cleo nor poetry uses them.
package rapidfuzz

import (
	"math"
	"strings"
	"unicode"

	"github.com/vishnukv64/gpython/py"
)

// Version is the rapidfuzz release whose behaviour this reproduces.  The
// dist-info that pip sees must carry the same version.
const Version = "3.14.6"

type fn = func(py.Object, py.Tuple, py.StringDict) (py.Object, error)

func init() {
	reg := func(name, doc string, fns map[string]fn, extra map[string]py.Object) {
		g := py.NewStringDict()
		for n, f := range fns {
			g.Set(n, py.MustNewMethod(n, f, 0, ""))
		}
		for n, v := range extra {
			g.Set(n, v)
		}
		py.RegisterModule(&py.ModuleImpl{Info: py.ModuleInfo{Name: name, Doc: doc}, Globals: g})
	}
	reg("rapidfuzz", "rapid string matching library (Go-native subset)", nil, map[string]py.Object{
		"__version__": py.String(Version),
		"__author__":  py.String("Max Bachmann"),
		"__license__": py.String("MIT"),
	})
	reg("rapidfuzz.distance", "string distance metrics", nil, nil)
	reg("rapidfuzz.distance.Levenshtein", "Levenshtein distance", map[string]fn{
		"distance":              func(_ py.Object, a py.Tuple, k py.StringDict) (py.Object, error) { return lev(a, k, kDist) },
		"similarity":            func(_ py.Object, a py.Tuple, k py.StringDict) (py.Object, error) { return lev(a, k, kSim) },
		"normalized_distance":   func(_ py.Object, a py.Tuple, k py.StringDict) (py.Object, error) { return lev(a, k, kNDist) },
		"normalized_similarity": func(_ py.Object, a py.Tuple, k py.StringDict) (py.Object, error) { return lev(a, k, kNSim) },
	}, nil)
	reg("rapidfuzz.fuzz", "fuzzy string matching scorers", map[string]fn{"ratio": ratio}, nil)
	reg("rapidfuzz.utils", "string preprocessing", map[string]fn{"default_process": defaultProcess}, nil)
}

type kind int

const (
	kDist kind = iota
	kSim
	kNDist
	kNSim
)

var builtinHash py.Object

// seq converts a sequence the way rapidfuzz does: str and bytes compare by
// code point / byte, any other iterable by element (a 1-char str by its code
// point, -1 as itself, everything else by hash()).
func seq(o py.Object) ([]int64, error) {
	switch v := o.(type) {
	case py.String:
		out := make([]int64, 0, len(v))
		for _, r := range string(v) {
			out = append(out, int64(r))
		}
		return out, nil
	case py.Bytes:
		out := make([]int64, len(v))
		for i, b := range v {
			out[i] = int64(b)
		}
		return out, nil
	}
	var out []int64
	var herr error
	err := py.Iterate(o, func(e py.Object) bool {
		switch x := e.(type) {
		case py.String:
			if r := []rune(string(x)); len(r) == 1 {
				out = append(out, int64(r[0]))
				return false
			}
		case py.Int:
			if x == -1 {
				out = append(out, -1)
				return false
			}
		}
		if builtinHash == nil {
			builtinHash, _ = py.GetModuleImplOrNil("builtins").Globals.Get("hash")
		}
		h, err := py.Call(builtinHash, py.Tuple{e}, py.NewStringDict())
		if err != nil {
			herr = err
			return true
		}
		n, _ := h.(py.Int)
		out = append(out, int64(n))
		return false
	})
	if err == nil {
		err = herr
	}
	return out, err
}

func isNone(o py.Object) bool {
	if o == py.None {
		return true
	}
	f, ok := o.(py.Float)
	return ok && math.IsNaN(float64(f))
}

// weighted computes the edit distance with the given (insert, delete,
// replace) costs.
func weighted(s1, s2 []int64, ins, del, rep int64) int64 {
	for len(s1) > 0 && len(s2) > 0 && s1[0] == s2[0] {
		s1, s2 = s1[1:], s2[1:]
	}
	for len(s1) > 0 && len(s2) > 0 && s1[len(s1)-1] == s2[len(s2)-1] {
		s1, s2 = s1[:len(s1)-1], s2[:len(s2)-1]
	}
	cache := make([]int64, len(s1)+1)
	for i := range cache {
		cache[i] = int64(i) * del
	}
	for _, c2 := range s2 {
		temp := cache[0]
		cache[0] += ins
		for i, c1 := range s1 {
			x := temp
			if c1 != c2 {
				x = min(cache[i]+del, cache[i+1]+ins, temp+rep)
			}
			temp = cache[i+1]
			cache[i+1] = x
		}
	}
	return cache[len(s1)]
}

func maximum(l1, l2, ins, del, rep int64) int64 {
	m := l1*del + l2*ins
	if l1 >= l2 {
		return min(m, l2*rep+(l1-l2)*del)
	}
	return min(m, l1*rep+(l2-l1)*ins)
}

// prep applies the optional processor and converts to a comparable sequence.
func prep(proc, s py.Object) ([]int64, error) {
	if proc != py.None {
		var err error
		if s, err = py.Call(proc, py.Tuple{s}, py.NewStringDict()); err != nil {
			return nil, err
		}
	}
	return seq(s)
}

func parseWeights(w py.Object) (ins, del, rep int64, err error) {
	if w == py.None {
		return 1, 1, 1, nil
	}
	var v []int64
	if err = py.Iterate(w, func(e py.Object) bool {
		n, ierr := py.IndexInt(e)
		if ierr != nil {
			err = ierr
			return true
		}
		v = append(v, int64(n))
		return false
	}); err != nil {
		return
	}
	if len(v) != 3 {
		err = py.ExceptionNewf(py.ValueError, "weights must be a tuple of 3 ints")
		return
	}
	for _, x := range v {
		if x < 0 {
			err = py.ExceptionNewf(py.OverflowError, "can't convert negative value to size_t")
			return
		}
	}
	return v[0], v[1], v[2], nil
}

func lev(args py.Tuple, kwargs py.StringDict, k kind) (py.Object, error) {
	var o1, o2 py.Object
	weights, proc, cutoff, hint := py.Object(py.None), py.Object(py.None), py.Object(py.None), py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|$OOOO:levenshtein", []string{"s1", "s2", "weights", "processor", "score_cutoff", "score_hint"}, &o1, &o2, &weights, &proc, &cutoff, &hint); err != nil {
		return nil, err
	}
	if isNone(o1) || isNone(o2) {
		switch k {
		case kNDist:
			return py.Float(1), nil
		case kNSim:
			return py.Float(0), nil
		}
	}
	ins, del, rep, err := parseWeights(weights)
	if err != nil {
		return nil, err
	}
	s1, err := prep(proc, o1)
	if err != nil {
		return nil, err
	}
	s2, err := prep(proc, o2)
	if err != nil {
		return nil, err
	}
	dist := weighted(s1, s2, ins, del, rep)
	max := maximum(int64(len(s1)), int64(len(s2)), ins, del, rep)
	switch k {
	case kDist, kSim:
		var c int
		if cutoff != py.None {
			// size_t conversion: a float cutoff is truncated.
			if f, ok := cutoff.(py.Float); ok {
				c = int(f)
			} else if c, err = py.IndexInt(cutoff); err != nil {
				return nil, err
			}
		}
		if k == kDist {
			if cutoff != py.None && dist > int64(c) {
				return py.Int(c + 1), nil
			}
			return py.Int(dist), nil
		}
		if sim := max - dist; cutoff == py.None || sim >= int64(c) {
			return py.Int(sim), nil
		}
		return py.Int(0), nil
	}
	nd := 0.0
	if max != 0 {
		nd = float64(dist) / float64(max)
	}
	c := 0.0
	for _, o := range []py.Object{cutoff, hint} {
		if o == py.None {
			continue
		}
		f, err := py.FloatAsFloat64(o)
		if err != nil {
			return nil, err
		}
		if f < 0 || f > 1 {
			return nil, py.ExceptionNewf(py.TypeError, "score_cutoff has to be in the range of 0.0 - 1.0")
		}
		if o == cutoff {
			c = f
		}
	}
	if k == kNSim {
		if ns := 1.0 - nd; cutoff == py.None || ns >= c {
			return py.Float(ns), nil
		}
		return py.Float(0), nil
	}
	if cutoff != py.None && nd > c {
		return py.Float(1), nil
	}
	return py.Float(nd), nil
}

func ratio(_ py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var o1, o2 py.Object
	proc, cutoff := py.Object(py.None), py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|$OO:ratio", []string{"s1", "s2", "processor", "score_cutoff"}, &o1, &o2, &proc, &cutoff); err != nil {
		return nil, err
	}
	if isNone(o1) || isNone(o2) {
		return py.Int(0), nil
	}
	s1, err := prep(proc, o1)
	if err != nil {
		return nil, err
	}
	s2, err := prep(proc, o2)
	if err != nil {
		return nil, err
	}
	// Indel distance is Levenshtein with a substitution cost of 2.
	dist := weighted(s1, s2, 1, 1, 2)
	score := 100.0
	if lensum := len(s1) + len(s2); lensum > 0 {
		score = 100 * (1 - float64(dist)/float64(lensum))
	}
	if cutoff != py.None {
		c, err := py.FloatAsFloat64(cutoff)
		if err != nil {
			return nil, err
		}
		if score < c {
			return py.Float(0), nil
		}
	}
	return py.Float(score), nil
}

func defaultProcess(_ py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var o py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:default_process", []string{"sentence"}, &o); err != nil {
		return nil, err
	}
	s, ok := o.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "sentence must be a str")
	}
	out := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, string(s))
	return py.String(strings.TrimFunc(out, unicode.IsSpace)), nil
}
