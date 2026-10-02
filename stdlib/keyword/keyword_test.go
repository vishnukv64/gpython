// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package keyword

import (
	"testing"
)

// cpythonKwlist is what "python3 -c 'import keyword; print(keyword.kwlist)'"
// prints on CPython 3.14.7.  It is spelled out here rather than derived so
// that a change to the module's data fails this test instead of agreeing with
// itself.
var cpythonKwlist = []string{
	"False", "None", "True", "and", "as", "assert", "async", "await",
	"break", "class", "continue", "def", "del", "elif", "else", "except",
	"finally", "for", "from", "global", "if", "import", "in", "is",
	"lambda", "nonlocal", "not", "or", "pass", "raise", "return", "try",
	"while", "with", "yield",
}

var cpythonSoftkwlist = []string{"_", "case", "match", "type"}

// TestKwlistMatchesCPython pins the list and its ORDER: code reflects on it
// positionally, so a reordering is a behaviour change even though the set is
// the same.
func TestKwlistMatchesCPython(t *testing.T) {
	if len(kwlist) != len(cpythonKwlist) {
		t.Fatalf("kwlist has %d entries, CPython 3.14 has %d", len(kwlist), len(cpythonKwlist))
	}
	for i, want := range cpythonKwlist {
		if kwlist[i] != want {
			t.Errorf("kwlist[%d] = %q, want %q", i, kwlist[i], want)
		}
	}
	if len(softkwlist) != len(cpythonSoftkwlist) {
		t.Fatalf("softkwlist has %d entries, CPython 3.14 has %d", len(softkwlist), len(cpythonSoftkwlist))
	}
	for i, want := range cpythonSoftkwlist {
		if softkwlist[i] != want {
			t.Errorf("softkwlist[%d] = %q, want %q", i, softkwlist[i], want)
		}
	}
}

// TestKeywordNoOverlap checks the two lists are disjoint: a soft keyword is
// by definition not a real one, and code that unions them relies on it.
func TestKeywordNoOverlap(t *testing.T) {
	hard := map[string]bool{}
	for _, w := range kwlist {
		hard[w] = true
	}
	for _, w := range softkwlist {
		if hard[w] {
			t.Errorf("%q is in both kwlist and softkwlist", w)
		}
	}
}
