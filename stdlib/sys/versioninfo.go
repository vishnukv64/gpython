// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

import (
	"fmt"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// VersionInfo is sys.version_info.
//
// CPython models it as a struct sequence: a tuple whose elements also have
// names, so both "version_info[0]" and "version_info.major" work and it
// compares equal to a plain tuple of the same values.  Here it was a bare
// py.Tuple, so the sequence half worked and the named half did not -
// "sys.version_info.major" raised "'tuple' object has no attribute 'major'",
// which is how code written for 3.9+ spells the check.
//
// The shape follows time.struct_time (stdlib/time/structtime.go), which is the
// same kind of object already built this way.
type VersionInfo struct {
	Dict py.StringDict
	// The five sequence fields, in CPython's order.
	Major, Minor, Micro int
	ReleaseLevel        string
	Serial              int
}

// VersionInfoType is sys.version_info's type.
var VersionInfoType = py.NewTypeX("sys.version_info",
	"Version information as a named tuple.", nil, nil)

func (v *VersionInfo) Type() *py.Type { return VersionInfoType }

func (v *VersionInfo) GetDict() py.StringDict { return v.Dict }

var _ py.IGetDict = (*VersionInfo)(nil)

var versionInfoNames = []string{"major", "minor", "micro", "releaselevel", "serial"}

// fields is the five-item sequence, in CPython's order.  releaselevel is a
// string and the rest are integers, so the values are produced directly rather
// than through a []int like struct_time's.
func (v *VersionInfo) fields() []py.Object {
	return []py.Object{
		py.Int(v.Major), py.Int(v.Minor), py.Int(v.Micro),
		py.String(v.ReleaseLevel), py.Int(v.Serial),
	}
}

// The version this interpreter reports.  Kept beside the value it describes so
// sys.version, sys.hexversion, sys.version_info and the values below cannot
// drift apart.
const (
	pyVersionMajor  = 3
	pyVersionMinor  = 10
	pyVersionMicro  = 0
	pyVersionLevel  = "final"
	pyVersionSerial = 0
)

func (v *VersionInfo) M__len__() (py.Object, error) { return py.Int(len(versionInfoNames)), nil }

func (v *VersionInfo) M__iter__() (py.Object, error) {
	return py.NewListFromItems(v.fields()).M__iter__()
}

func (v *VersionInfo) M__getitem__(key py.Object) (py.Object, error) {
	vals := v.fields()
	if sl, ok := key.(*py.Slice); ok {
		start, _, step, slicelength, err := sl.GetIndices(len(vals))
		if err != nil {
			return nil, err
		}
		out := make(py.Tuple, slicelength)
		for i, j := start, 0; j < slicelength; i, j = i+step, j+1 {
			out[j] = vals[i]
		}
		return out, nil
	}
	n, err := py.IndexIntCheck(key, len(vals))
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "tuple indices must be integers or slices, not %s", key.Type().Name)
	}
	return vals[n], nil
}

// M__eq__ compares field by field, and against a PLAIN tuple as well: CPython's
// struct sequences are tuples, so "sys.version_info == (3, 10, 0, 'final', 0)"
// is True and code depends on it.  Comparing only against another VersionInfo
// would have made that False while looking correct.
func (v *VersionInfo) M__eq__(other py.Object) (py.Object, error) {
	seq, err := py.SequenceTuple(other)
	if err != nil {
		return py.NotImplemented, nil
	}
	return py.Bool(valuesEqual(v.fields(), seq)), nil
}

func valuesEqual(a []py.Object, b py.Tuple) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		eq, err := py.Eq(a[i], b[i])
		if err != nil || eq != py.True {
			return false
		}
	}
	return true
}

// M__lt__/le/gt/ge delegate to the tuple the values form, so comparing a
// version against a tuple works exactly as CPython's does.
func (v *VersionInfo) M__lt__(other py.Object) (py.Object, error) {
	return compareWithTuple(v, other, py.Lt)
}
func (v *VersionInfo) M__le__(other py.Object) (py.Object, error) {
	return compareWithTuple(v, other, py.Le)
}
func (v *VersionInfo) M__gt__(other py.Object) (py.Object, error) {
	return compareWithTuple(v, other, py.Gt)
}
func (v *VersionInfo) M__ge__(other py.Object) (py.Object, error) {
	return compareWithTuple(v, other, py.Ge)
}

func compareWithTuple(v *VersionInfo, other py.Object, op func(a, b py.Object) (py.Object, error)) (py.Object, error) {
	seq, err := py.SequenceTuple(other)
	if err != nil {
		return py.NotImplemented, nil
	}
	return op(py.Tuple(v.fields()), seq)
}

// M__hash__ hashes the tuple of values, as CPython's does - a struct sequence
// is a tuple, so it is hashable and equal objects hash alike.
func (v *VersionInfo) M__hash__() (py.Object, error) {
	if h, ok := py.HashValue(py.Tuple(v.fields())); ok {
		return py.Int(h), nil
	}
	return py.NotImplemented, nil
}

func (v *VersionInfo) M__repr__() (py.Object, error) {
	var b strings.Builder
	b.WriteString("sys.version_info(")
	for i, name := range versionInfoNames {
		if i > 0 {
			b.WriteString(", ")
		}
		repr, err := py.Repr(v.fields()[i])
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "%s=%s", name, repr.(py.String))
	}
	b.WriteString(")")
	return py.String(b.String()), nil
}

// newVersionInfo builds a version_info for the given components, filling in the
// named attributes.
func newVersionInfo(major, minor, micro int, level string, serial int) *VersionInfo {
	v := &VersionInfo{
		Dict:         py.NewStringDict(),
		Major:        major,
		Minor:        minor,
		Micro:        micro,
		ReleaseLevel: level,
		Serial:       serial,
	}
	for i, name := range versionInfoNames {
		v.Dict.Set(name, v.fields()[i])
	}
	// CPython also exposes n_fields and friends on a struct sequence, and code
	// (and the stdlib itself) reads them.
	v.Dict.Set("n_fields", py.Int(len(versionInfoNames)))
	v.Dict.Set("n_sequence_fields", py.Int(len(versionInfoNames)))
	v.Dict.Set("n_unnamed_fields", py.Int(0))
	return v
}

// currentVersionInfo is the interpreter's own version, used for sys.version_info
// and for sys.implementation.version (which CPython also makes a version_info).
func currentVersionInfo() *VersionInfo {
	return newVersionInfo(pyVersionMajor, pyVersionMinor, pyVersionMicro, pyVersionLevel, pyVersionSerial)
}
