// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unicodedata

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

// ucd320 is unicodedata.ucd_3_2_0, the Unicode 3.2.0 database that stringprep
// (RFC 3454) is defined against.  category and bidirectional use real 3.2.0
// tables (ucd320.go).  normalize uses the current composition data, which
// agrees with 3.2.0 for every code point 3.2.0 assigns that is not one of the
// few whose decomposition changed later.
type ucd320 struct{}

var ucd320Type = py.ObjectType.NewType("unicodedata.UCD", "Unicode character database, version 3.2.0", nil, nil)

func (*ucd320) Type() *py.Type { return ucd320Type }

type verRange struct {
	lo, hi rune
	val    string
}

var (
	verOnce sync.Once
	verCat  []verRange
	verBidi []verRange
)

func parseVerRanges(s string) []verRange {
	parts := strings.Split(s, ";")
	out := make([]verRange, 0, len(parts))
	for _, p := range parts {
		f := strings.SplitN(p, ",", 3)
		lo, _ := strconv.ParseInt(f[0], 16, 32)
		hi, _ := strconv.ParseInt(f[1], 16, 32)
		out = append(out, verRange{rune(lo), rune(hi), f[2]})
	}
	return out
}

func lookupVer(rs []verRange, r rune) string {
	i := sort.Search(len(rs), func(i int) bool { return rs[i].hi >= r })
	if i < len(rs) && rs[i].lo <= r {
		return rs[i].val
	}
	return ""
}

func ucd320Char(name string, which *[]verRange) func(py.Object, py.Tuple) (py.Object, error) {
	return func(self py.Object, args py.Tuple) (py.Object, error) {
		verOnce.Do(func() {
			verCat, verBidi = parseVerRanges(ucd320Cat), parseVerRanges(ucd320Bidi)
		})
		var o py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, name, 1, 1, &o); err != nil {
			return nil, err
		}
		r, err := charArg(o, name)
		if err != nil {
			return nil, err
		}
		return py.String(lookupVer(*which, r)), nil
	}
}

func init() {
	d := &ucd320Type.Dict
	d.Set("category", py.MustNewMethod("category", ucd320Char("category", &verCat), 0, ""))
	d.Set("bidirectional", py.MustNewMethod("bidirectional", ucd320Char("bidirectional", &verBidi), 0, ""))
	d.Set("normalize", py.MustNewMethod("normalize", normalizeMethod, 0, ""))
	d.Set("unidata_version", py.String("3.2.0"))
}
