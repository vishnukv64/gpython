// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// A set's |=, &=, -= and ^= update it in place.  Without these the operators
// fell back to the copying forms, so "a |= b" inside a function never changed
// the set the caller passed (lark's update_set, hence the LALR analysis in
// poetry-core's requirement parser, looped forever).
func (s *Set) inplace(method string, other Object) (Object, error) {
	if _, ok := other.(*Set); !ok {
		return NotImplemented, nil
	}
	if _, err := SetType.Dict.GetOrNil(method).(*Method).Call(s, Tuple{other}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Set) M__ior__(other Object) (Object, error)  { return s.inplace("update", other) }
func (s *Set) M__iand__(other Object) (Object, error) { return s.inplace("intersection_update", other) }
func (s *Set) M__isub__(other Object) (Object, error) { return s.inplace("difference_update", other) }
func (s *Set) M__ixor__(other Object) (Object, error) {
	return s.inplace("symmetric_difference_update", other)
}

// A frozenset embeds Set but is immutable: its augmented forms rebind to a new
// frozenset through the binary operator.
func (*FrozenSet) M__ior__(Object) (Object, error)  { return NotImplemented, nil }
func (*FrozenSet) M__iand__(Object) (Object, error) { return NotImplemented, nil }
func (*FrozenSet) M__isub__(Object) (Object, error) { return NotImplemented, nil }
func (*FrozenSet) M__ixor__(Object) (Object, error) { return NotImplemented, nil }
