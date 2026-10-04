// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compile

import (
	"strconv"

	"github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/vm"
)

// matchStmt lowers a PEP 634 match statement into ordinary assignment and
// comparison code.
//
// The subject is evaluated once into a hidden local, and each case becomes a
// test built from the pattern.  A capture assigns to its name; a literal or
// dotted name compares equal to the subject; a class pattern checks the type
// and then recurses into the positional sub-patterns; an or-pattern tries
// each alternative; the wildcard always matches.
//
// The alternative would be new bytecode, and there is none to add: the
// existing opcodes express all of this, so nothing below is a special case
// for the VM.
func (c *compiler) matchStmt(node *ast.MatchStmt) {
	// The subject is evaluated once, into a local, because a case must be
	// able to test it more than once.
	subject := c.matchTmp()
	c.Expr(node.Subject)
	c.storeTemp(subject)

	end := new(Label)
	caseLabels := make([]*Label, len(node.Cases))
	for i := range node.Cases {
		caseLabels[i] = new(Label)
	}
	next := new(Label)

	for i, clause := range node.Cases {
		c.Label(caseLabels[i])
		pattern := clause.Pattern
		var guard ast.Expr
		if guarded, ok := pattern.(*ast.MatchGuard); ok {
			pattern = guarded.Pattern
			guard = guarded.Guard
		}

		// The pattern's test jumps to the next case when it fails.
		c.matchPattern(pattern, subject, next)
		if guard != nil {
			// A guard that is false also moves on to the next case.
			c.Expr(guard)
			c.Jump(vm.POP_JUMP_IF_FALSE, next)
		}
		c.Stmts(clause.Body)
		c.Jump(vm.JUMP_FORWARD, end)
		c.Label(next)
		next = new(Label)
	}
	// No case matched: the statement falls through, which is what a match
	// with no matching clause does.
	c.Label(end)
}

// matchTmp allocates a unique hidden local for the subject.
var matchCounter int

func (c *compiler) matchTmp() string {
	matchCounter++
	return ".match" + strconv.Itoa(matchCounter)
}

// storeTemp stores the value on the stack into a temporary local.
func (c *compiler) storeTemp(name string) {
	// A module or class scope has no fast locals; the name goes into the
	// namespace, which is what STORE_NAME does there.
	c.NameOp(name, ast.Store)
}

// loadTemp pushes a temporary back onto the stack.
func (c *compiler) loadTemp(name string) {
	c.NameOp(name, ast.Load)
}

// matchPattern emits the test for one pattern, jumping to fail when it does
// not match.  A capture binds as a side effect.
func (c *compiler) matchPattern(pattern ast.Expr, subject string, fail *Label) {
	switch p := pattern.(type) {
	case nil:
		// No pattern is the wildcard.
		return

	case *ast.MatchWildcard:
		// "_" matches anything and binds nothing.
		return

	case *ast.MatchCapture:
		// A bare name captures: it always matches and binds the subject.
		c.loadTemp(subject)
		c.NameOp(string(p.Name), ast.Store)
		return

	case *ast.MatchValue:
		// The grammar resolves a bare name through the dotted-name
		// alternative, so "a" and "_" arrive here as a MatchValue wrapping a
		// Name rather than as a MatchCapture or MatchWildcard.  A single
		// name means capture (or wildcard for "_"); a dotted name like
		// Color.RED is a value to compare against.
		if name, ok := p.Value.(*ast.Name); ok {
			if string(name.Id) == "_" {
				// "_" matches anything and binds nothing.
				return
			}
			// A bare name captures.
			c.loadTemp(subject)
			c.NameOp(string(name.Id), ast.Store)
			return
		}
		// A literal or dotted name: compare for equality.
		c.loadTemp(subject)
		c.Expr(p.Value)
		c.OpArg(vm.COMPARE_OP, vm.PyCmp_EQ)
		c.Jump(vm.POP_JUMP_IF_FALSE, fail)
		return

	case *ast.MatchAs:
		if p.Pattern != nil {
			c.matchPattern(p.Pattern, subject, fail)
		}
		c.loadTemp(subject)
		c.NameOp(string(p.Name), ast.Store)
		return

	case *ast.MatchOr:
		// An or-pattern: the whole pattern fails only when every
		// alternative fails.
		success := new(Label)
		for _, alt := range p.Patterns {
			inner := new(Label)
			c.matchPattern(alt, subject, inner)
			c.Jump(vm.JUMP_FORWARD, success)
			c.Label(inner)
		}
		c.Jump(vm.JUMP_FORWARD, fail)
		c.Label(success)
		return

	case *ast.MatchSequence:
		// A sequence pattern binds the subject to a hidden name, checks its
		// length, then tests each element.
		c.matchSequence(p, subject, fail)
		return

	case *ast.MatchClass:
		c.matchClass(p, subject, fail)
		return

	case *ast.MatchMapping:
		c.matchMapping(p, subject, fail)
		return
	}

	// An unrecognised pattern node: fail rather than match, so a mistake is
	// visible instead of silently taking the first case.
	c.Jump(vm.JUMP_FORWARD, fail)
}

// matchSequence checks the length and then each element pattern.
func (c *compiler) matchSequence(p *ast.MatchSequence, subject string, fail *Label) {
	// A star pattern makes the length a minimum rather than an exact count.
	hasStar := false
	starIndex := -1
	for i, sub := range p.Patterns {
		if _, ok := sub.(*ast.MatchStar); ok {
			hasStar = true
			starIndex = i
		}
	}

	subjectType := c.temp()
	c.loadTemp(subject)
	c.StoreVar(subjectType)

	// A sequence pattern applies only to a SEQUENCE, and it is not "has a
	// length": CPython accepts a list, a tuple, a range and anything registered
	// through collections.abc.Sequence - and REFUSES a str, bytes, dict or set.
	//
	// Testing hasattr(subject, "__len__") accepted a string, so
	// "case [x, y]" matched "ab" and bound x='a', y='b' where CPython falls
	// through to the next case.
	// The test is a helper rather than inline bytecode: "is a sequence for
	// pattern matching" is CPython's rule (list, tuple, range, or a registered
	// Sequence - but NOT str, bytes, dict or set) and expressing it as a single
	// well-named call keeps this compiler readable.
	c.NameOp("__match_is_sequence__", ast.Load)
	c.loadTemp(subjectType)
	c.OpArg(vm.CALL_FUNCTION, 1)
	c.Jump(vm.POP_JUMP_IF_FALSE, fail)

	// len(subject) == len(patterns), or >= the minimum with a star.
	c.NameOp("len", ast.Load)
	c.loadTemp(subjectType)
	c.OpArg(vm.CALL_FUNCTION, 1)
	if hasStar {
		c.LoadConst(py.Int(len(p.Patterns) - 1))
		c.OpArg(vm.COMPARE_OP, vm.PyCmp_GE)
	} else {
		c.LoadConst(py.Int(len(p.Patterns)))
		c.OpArg(vm.COMPARE_OP, vm.PyCmp_EQ)
	}
	c.Jump(vm.POP_JUMP_IF_FALSE, fail)

	// Each element is tested against its pattern, by index.  With a star,
	// elements after it are indexed from the end.
	for i, sub := range p.Patterns {
		if _, ok := sub.(*ast.MatchStar); ok {
			continue
		}
		index := i
		if hasStar && i > starIndex {
			index = i - len(p.Patterns)
		}
		c.loadTemp(subjectType)
		c.LoadConst(py.Int(index))
		c.Op(vm.BINARY_SUBSCR)

		element := c.temp()
		c.StoreVar(element)
		c.matchPattern(sub, element, fail)
	}

	// The star pattern captures the middle slice.
	if hasStar {
		star := p.Patterns[starIndex].(*ast.MatchStar)
		if star.Name != "" {
			// The star takes everything between the patterns before it and the
			// patterns after it, so its slice runs from starIndex to
			// "len(subject) - (patterns after the star)".
			//
			// Writing that stop as a NEGATIVE index - "starIndex + 1 -
			// len(patterns)" - is wrong when the star is LAST: the stop is
			// then "1 - len(patterns)", which is a negative index the slice
			// resolves differently from "end of sequence".  Computing it from
			// len(subject) is what CPython's star does, and it is the same
			// arithmetic for both cases.
			after := len(p.Patterns) - starIndex - 1
			c.loadTemp(subjectType)
			c.LoadConst(py.Int(starIndex))
			if after == 0 {
				// To the end: an explicit None, which is what a slice stop of
				// "end" means.
				c.LoadConst(py.None)
			} else {
				c.NameOp("len", ast.Load)
				c.loadTemp(subjectType)
				c.OpArg(vm.CALL_FUNCTION, 1)
				c.LoadConst(py.Int(after))
				c.Op(vm.BINARY_SUBTRACT)
			}
			c.OpArg(vm.BUILD_SLICE, 2)
			c.Op(vm.BINARY_SUBSCR)
			c.NameOp(string(star.Name), ast.Store)
		}
	}
}

// matchClass checks isinstance(subject, Cls) and then the positional
// sub-patterns against attributes of the instance.
func (c *compiler) matchClass(p *ast.MatchClass, subject string, fail *Label) {
	// isinstance(subject, Class) must hold.
	c.NameOp("isinstance", ast.Load)
	c.loadTemp(subject)
	c.Expr(p.Cls)
	c.OpArg(vm.CALL_FUNCTION, 2)
	c.Jump(vm.POP_JUMP_IF_FALSE, fail)

	// A class pattern's positional sub-patterns are tested against the
	// attributes the class exposes: CPython uses __match_args__, and falls
	// back to nothing when it is absent.
	instance := c.temp()
	c.loadTemp(subject)
	c.StoreVar(instance)

	// The instance is tested for its type name only; the sub-patterns below
	// use __match_args__ when the class provides it.
	// __match_args__ is optional: a class with positional sub-patterns must
	// provide it, but one without any positional patterns need not.  The
	// lookup goes through getattr so that its absence is a default rather
	// than an AttributeError.
	matchArgs := c.temp()
	c.NameOp("getattr", ast.Load)
	c.Expr(p.Cls)
	c.LoadConst(py.String("__match_args__"))
	c.LoadConst(py.Tuple{})
	c.OpArg(vm.CALL_FUNCTION, 3)
	c.StoreVar(matchArgs)

	for i, sub := range p.Patterns {
		// attribute = getattr(instance, __match_args__[i])
		c.loadTemp(instance)
		// The attribute name comes from __match_args__ at run time, so the
		// lookup goes through getattr rather than an inline attribute load.
		c.NameOp("getattr", ast.Load)
		c.loadTemp(instance)
		c.loadTemp(matchArgs)
		c.LoadConst(py.Int(i))
		c.Op(vm.BINARY_SUBSCR)
		c.OpArg(vm.CALL_FUNCTION, 2)

		element := c.temp()
		c.StoreVar(element)
		c.matchPattern(sub, element, fail)
	}
}

// matchMapping checks the keys are present and then each value pattern.
func (c *compiler) matchMapping(p *ast.MatchMapping, subject string, fail *Label) {
	mapping := c.temp()
	c.loadTemp(subject)
	c.StoreVar(mapping)

	// A mapping pattern only applies to a mapping: "key in 9" raises rather
	// than failing the case, so the type is checked first.
	c.loadTemp(mapping)
	c.NameOp("hasattr", ast.Load)
	c.loadTemp(mapping)
	c.LoadConst(py.String("__getitem__"))
	c.OpArg(vm.CALL_FUNCTION, 2)
	c.Jump(vm.POP_JUMP_IF_FALSE, fail)

	for i, key := range p.Keys {
		// A mapping pattern's key is a MatchValue wrapper, so the value
		// expression inside it is what belongs on the stack - passing the
		// pattern node itself to Expr is what produced "a match pattern
		// cannot be used as an expression".
		keyValue, err := matchKeyValue(key)
		if err != nil {
			c.panicSyntaxErrorf(key, "%s", err)
		}
		// The key must be present: key in subject.
		c.Expr(keyValue)
		c.loadTemp(mapping)
		c.OpArg(vm.COMPARE_OP, vm.PyCmp_IN)
		c.Jump(vm.POP_JUMP_IF_FALSE, fail)

		// And its value must match the sub-pattern.
		c.loadTemp(mapping)
		c.Expr(keyValue)
		c.Op(vm.BINARY_SUBSCR)
		element := c.temp()
		c.StoreVar(element)
		c.matchPattern(p.Patterns[i], element, fail)
	}
}

// matchKeyValue extracts the value expression from a mapping key pattern.
func matchKeyValue(pattern ast.Expr) (ast.Expr, error) {
	if mv, ok := pattern.(*ast.MatchValue); ok {
		return mv.Value, nil
	}
	return nil, py.ExceptionNewf(py.SyntaxError, "a mapping pattern key must be a literal")
}

// temp allocates a unique hidden local for an intermediate value.
func (c *compiler) temp() string {
	matchCounter++
	return ".matchtmp" + strconv.Itoa(matchCounter)
}

// StoreVar pushes a STORE for a name in the current scope.
func (c *compiler) StoreVar(name string) {
	c.NameOp(name, ast.Store)
}
