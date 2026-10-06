// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// struct_time, and the conversion and formatting built on it.
//
// CPython's struct_time is a PyStructSequence: a tuple of nine integers with
// named attributes.  It is modelled here as a Go type carrying the fields and
// implementing the sequence protocol by hand, because the interpreter has no
// struct-sequence machinery and nine named fields are the whole interface.

package time

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

// StructTime is time.struct_time.
type StructTime struct {
	Dict py.StringDict
	// The nine sequence fields, in the order CPython exposes them.
	Year, Mon, Mday, Hour, Min, Sec, Wday, Yday, Isdst int
	// Zone and Gmtoff are the extra attributes only gmtime/localtime set.
	Zone   py.Object
	Gmtoff int
}

// StructTimeType is time.struct_time.
var StructTimeType = py.NewTypeX("struct_time",
	"The time type represents a calendar time broken down into its components.", nil, nil)

func (s *StructTime) Type() *py.Type { return StructTimeType }

func (s *StructTime) GetDict() py.StringDict { return s.Dict }

var _ py.IGetDict = (*StructTime)(nil)

// fields is the nine-item sequence, in order.
func (s *StructTime) fields() []int {
	return []int{s.Year, s.Mon, s.Mday, s.Hour, s.Min, s.Sec, s.Wday, s.Yday, s.Isdst}
}

var structTimeNames = []string{"tm_year", "tm_mon", "tm_mday", "tm_hour", "tm_min", "tm_sec", "tm_wday", "tm_yday", "tm_isdst"}

func (s *StructTime) M__len__() (py.Object, error) { return py.Int(9), nil }

func (s *StructTime) M__iter__() (py.Object, error) {
	items := make([]py.Object, 9)
	for i, v := range s.fields() {
		items[i] = py.Int(v)
	}
	return py.NewListFromItems(items).M__iter__()
}

func (s *StructTime) M__getitem__(key py.Object) (py.Object, error) {
	vals := s.fields()
	if sl, ok := key.(*py.Slice); ok {
		start, _, step, slicelength, err := sl.GetIndices(9)
		if err != nil {
			return nil, err
		}
		out := make(py.Tuple, slicelength)
		for i, j := start, 0; j < slicelength; i, j = i+step, j+1 {
			out[j] = py.Int(vals[i])
		}
		return out, nil
	}
	n, err := py.IndexIntCheck(key, 9)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "tuple indices must be integers or slices, not %s", key.Type().Name)
	}
	return py.Int(vals[n]), nil
}

// M__eq__ compares the nine fields, and against a PLAIN tuple as well:
// time.struct_time IS a tuple in CPython, so "localtime() == tuple(localtime())"
// is True.  Comparing only against another struct_time made it False, and left
// the object unusable as a set member or dict key.
func (s *StructTime) M__eq__(other py.Object) (py.Object, error) {
	seq, err := py.SequenceTuple(other)
	if err != nil {
		return py.NotImplemented, nil
	}
	return py.Bool(equalFields(s, seq)), nil
}

func equalFields(a *StructTime, b py.Tuple) bool {
	fa := a.fields()
	if len(fa) != len(b) {
		return false
	}
	for i := range fa {
		eq, err := py.Eq(py.Int(fa[i]), b[i])
		if err != nil || eq != py.True {
			return false
		}
	}
	return true
}

// M__lt__/le/gt/ge delegate to the tuple of values, as CPython's does: a
// struct_time IS a tuple, so sorted() over a list of them works.  Without these
// "sorted([localtime(), localtime()])" raised
// "'<' not supported between instances of 'time.struct_time' and
// 'time.struct_time'".
func (s *StructTime) M__lt__(other py.Object) (py.Object, error) {
	return compareTime(s, other, py.Lt)
}
func (s *StructTime) M__le__(other py.Object) (py.Object, error) {
	return compareTime(s, other, py.Le)
}
func (s *StructTime) M__gt__(other py.Object) (py.Object, error) {
	return compareTime(s, other, py.Gt)
}
func (s *StructTime) M__ge__(other py.Object) (py.Object, error) {
	return compareTime(s, other, py.Ge)
}

func compareTime(s *StructTime, other py.Object, op func(a, b py.Object) (py.Object, error)) (py.Object, error) {
	seq, err := py.SequenceTuple(other)
	if err != nil {
		return py.NotImplemented, nil
	}
	fields := s.fields()
	items := make(py.Tuple, len(fields))
	for i, v := range fields {
		items[i] = py.Int(v)
	}
	return op(items, seq)
}

// M__hash__ hashes the tuple of values, as CPython's does.  Without it
// hash(localtime()) raised "descriptor '__hash__' requires a 'type' object",
// which also made a struct_time unusable as a dict key or set member.
func (s *StructTime) M__hash__() (py.Object, error) {
	fields := s.fields()
	items := make(py.Tuple, len(fields))
	for i, v := range fields {
		items[i] = py.Int(v)
	}
	if h, ok := py.HashValue(items); ok {
		return py.Int(h), nil
	}
	return py.NotImplemented, nil
}

func (s *StructTime) M__repr__() (py.Object, error) {
	var b strings.Builder
	b.WriteString("time.struct_time(")
	for i, name := range structTimeNames {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%d", name, s.fields()[i])
	}
	b.WriteString(")")
	return py.String(b.String()), nil
}

// newStructTime fills the attributes and the two extras.
func newStructTime(t time.Time, zone py.Object, gmtoff int, isdst int) *StructTime {
	s := &StructTime{
		Dict:   py.NewStringDict(),
		Year:   t.Year(),
		Mon:    int(t.Month()),
		Mday:   t.Day(),
		Hour:   t.Hour(),
		Min:    t.Minute(),
		Sec:    t.Second(),
		Wday:   (int(t.Weekday()) + 6) % 7, // Monday is 0
		Yday:   t.YearDay(),
		Isdst:  isdst,
		Zone:   zone,
		Gmtoff: gmtoff,
	}
	for i, name := range structTimeNames {
		s.Dict.Set(name, py.Int(s.fields()[i]))
	}
	s.Dict.Set("tm_zone", zone)
	s.Dict.Set("tm_gmtoff", py.Int(gmtoff))
	s.Dict.Set("n_fields", py.Int(9))
	s.Dict.Set("n_sequence_fields", py.Int(9))
	s.Dict.Set("n_unnamed_fields", py.Int(0))
	return s
}

// gmStructTime builds the UTC breakdown of an epoch second.
func gmStructTime(sec int64) *StructTime {
	t := time.Unix(sec, 0).UTC()
	return newStructTime(t, py.String("UTC"), 0, 0)
}

// localStructTime builds the local breakdown of an epoch second.  The zone
// name and offset come from Go's time package, which reads the system zone
// database the same way libc does.
func localStructTime(sec int64) *StructTime {
	t := time.Unix(sec, 0)
	name, off := t.Zone()
	isdst := 0
	if t.IsDST() {
		isdst = 1
	}
	return newStructTime(t, py.String(name), off, isdst)
}

// ---------------------------------------------------------------------------
// Parsing a struct_time argument

// timeTuple is the nine integers a struct_time or plain tuple carries.
type timeTuple struct {
	year, mon, mday, hour, min, sec, wday, yday, isdst int
}

func getTimeTuple(v py.Object) (*timeTuple, error) {
	seq, err := py.SequenceTuple(v)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "Tuple or struct_time argument required")
	}
	if len(seq) < 9 {
		return nil, py.ExceptionNewf(py.TypeError, "Tuple or struct_time argument required")
	}
	tt := &timeTuple{}
	dst := []*int{&tt.year, &tt.mon, &tt.mday, &tt.hour, &tt.min, &tt.sec, &tt.wday, &tt.yday, &tt.isdst}
	for i, p := range dst {
		n, err := py.IndexInt(seq[i])
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "an integer is required (got type %s)", seq[i].Type().Name)
		}
		*p = n
	}
	// A struct_time carries its zone in attributes; a plain tuple does not.
	if st, ok := v.(*StructTime); ok {
		tt.hour = st.Hour
	}
	return tt, nil
}

// checkTimeTuple is CPython's checktm: the fields must be in range before a
// conversion, so that formatting cannot index blindly into a table.
func checkTimeTuple(tt *timeTuple) error {
	switch {
	case tt.year < 1 || tt.year > 9999:
		return py.ExceptionNewf(py.ValueError, "year out of range")
	case tt.mon < 1 || tt.mon > 12:
		return py.ExceptionNewf(py.ValueError, "month out of range")
	case tt.mday < 1 || tt.mday > 31:
		return py.ExceptionNewf(py.ValueError, "day of month out of range")
	case tt.hour < 0 || tt.hour > 23:
		return py.ExceptionNewf(py.ValueError, "hour out of range")
	case tt.min < 0 || tt.min > 59:
		return py.ExceptionNewf(py.ValueError, "minute out of range")
	case tt.sec < 0 || tt.sec > 61:
		return py.ExceptionNewf(py.ValueError, "second out of range")
	case tt.wday < 0 || tt.wday > 6:
		return py.ExceptionNewf(py.ValueError, "day of week out of range")
	case tt.yday < 0 || tt.yday > 366:
		return py.ExceptionNewf(py.ValueError, "day of year out of range")
	}
	return nil
}

// epochSeconds turns a local breakdown back into an epoch second.
func (tt *timeTuple) epochSeconds(local bool) int64 {
	loc := time.UTC
	if local {
		loc = time.Local
	}
	return time.Date(tt.year, time.Month(tt.mon), tt.mday, tt.hour, tt.min, tt.sec, 0, loc).Unix()
}

// ---------------------------------------------------------------------------
// strftime

var weekdaysFull = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
var weekdaysAbbr = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
var monthsFull = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
var monthsAbbr = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// formatTime renders a format string against a breakdown.  The directives are
// the real ones; anything else is left verbatim, which is what CPython does
// on platforms whose strftime has no replacement for it.
func formatTime(format string, tt *timeTuple, zone string, gmtoff int, hasZone bool) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' || i+1 >= len(format) {
			b.WriteByte(c)
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			fmt.Fprintf(&b, "%d", tt.year)
		case 'y':
			fmt.Fprintf(&b, "%02d", ((tt.year%100)+100)%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", tt.mon)
		case 'd':
			fmt.Fprintf(&b, "%02d", tt.mday)
		case 'H':
			fmt.Fprintf(&b, "%02d", tt.hour)
		case 'I':
			h := tt.hour % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%02d", h)
		case 'M':
			fmt.Fprintf(&b, "%02d", tt.min)
		case 'S':
			fmt.Fprintf(&b, "%02d", tt.sec)
		case 'j':
			fmt.Fprintf(&b, "%03d", tt.yday)
		case 'A':
			b.WriteString(weekdaysFull[tt.wday])
		case 'a':
			b.WriteString(weekdaysAbbr[tt.wday])
		case 'B':
			b.WriteString(monthsFull[tt.mon-1])
		case 'b', 'h':
			b.WriteString(monthsAbbr[tt.mon-1])
		case 'p':
			if tt.hour < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'Z':
			// On this platform %Z reports the zone the struct carries, and only
			// a breakdown with a known zone has one; isdst == -1 means "ask the
			// system", which CPython leaves empty here too.
			if hasZone && tt.isdst != -1 {
				b.WriteString(zone)
			} else if tt.isdst != -1 {
				b.WriteString(zoneName(time.Now().Unix()))
			}
		case 'z':
			// The measured CPython reports the process's local offset here even
			// for a UTC breakdown, because the C library's strftime answers
			// %z, so the struct's own gmtoff is not used.
			if tt.isdst != -1 {
				off := localGmtoff(time.Now().Unix())
				sign := "+"
				if off < 0 {
					sign = "-"
					off = -off
				}
				fmt.Fprintf(&b, "%s%02d%02d", sign, off/3600, (off%3600)/60)
			}
		case 'w':
			// Sunday is 0 here, while the struct's tm_wday has Monday as 0.
			fmt.Fprintf(&b, "%d", (tt.wday+1)%7)
		case 'u':
			d := tt.wday
			if d == 0 {
				d = 7
			}
			fmt.Fprintf(&b, "%d", d)
		case 'U':
			// Week number with the first Sunday as day 1 of week 1.
			fmt.Fprintf(&b, "%02d", (tt.yday+6-tt.wday)/7)
		case 'W':
			// Week number with the first Monday as day 1 of week 1.
			wd := (tt.wday + 6) % 7
			fmt.Fprintf(&b, "%02d", (tt.yday+6-wd)/7)
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// strptime

// parseTime is a small strptime: the numeric directives the time module
// documents, plus the named ones the same way CPython's _strptime handles
// them.  It returns the fields it saw; the rest keep CPython's defaults.
func parseTime(text, format string) (*timeTuple, error) {
	badType := func() error {
		return py.ExceptionNewf(py.TypeError, "strptime() argument 1 must be str, not %s", "")
	}
	_ = badType

	tt := &timeTuple{year: 1900, mon: 1, mday: 1, wday: -1, yday: -1, isdst: -1}
	var ampm string
	ti, fi := 0, 0
	readInt := func(maxDigits int) (int, bool) {
		start := ti
		for ti < len(text) && ti-start < maxDigits && text[ti] >= '0' && text[ti] <= '9' {
			ti++
		}
		if ti == start {
			return 0, false
		}
		n, _ := strconv.Atoi(text[start:ti])
		return n, true
	}

	for fi < len(format) {
		c := format[fi]
		fi++
		if c != '%' {
			// Literal whitespace in the format matches a run of whitespace.
			if c == ' ' || c == '\t' || c == '\n' {
				for ti < len(text) && (text[ti] == ' ' || text[ti] == '\t' || text[ti] == '\n') {
					ti++
				}
				continue
			}
			if ti >= len(text) || text[ti] != c {
				return nil, py.ExceptionNewf(py.ValueError,
					"time data '%s' does not match format '%s'", text, format)
			}
			ti++
			continue
		}
		if fi >= len(format) {
			break
		}
		d := format[fi]
		fi++
		ok := true
		switch d {
		case 'Y':
			tt.year, ok = readInt(4)
		case 'y':
			var y int
			y, ok = readInt(2)
			tt.year = y + 1900
			if y >= 69 {
				tt.year = y + 1900
			} else {
				tt.year = y + 2000
			}
		case 'm':
			tt.mon, ok = readInt(2)
		case 'd':
			tt.mday, ok = readInt(2)
		case 'H':
			tt.hour, ok = readInt(2)
		case 'I':
			tt.hour, ok = readInt(2)
		case 'M':
			tt.min, ok = readInt(2)
		case 'S':
			tt.sec, ok = readInt(2)
		case 'j':
			tt.yday, ok = readInt(3)
		case 'w':
			tt.wday, ok = readInt(1)
		case 'f':
			// Fractional second: read the digits, keep microseconds truncated.
			start := ti
			for ti < len(text) && text[ti] >= '0' && text[ti] <= '9' {
				ti++
			}
			ok = ti > start
		case 'p':
			// AM/PM: match the longest of the two the locale uses.
			switch {
			case strings.HasPrefix(text[ti:], "AM") || strings.HasPrefix(text[ti:], "am"):
				ampm = "AM"
				ti += 2
			case strings.HasPrefix(text[ti:], "PM") || strings.HasPrefix(text[ti:], "pm"):
				ampm = "PM"
				ti += 2
			default:
				ok = false
			}
		case 'b', 'B', 'a', 'A':
			names := monthsAbbr
			if d == 'B' {
				names = monthsFull
			} else if d == 'a' {
				names = weekdaysAbbr
			} else if d == 'A' {
				names = weekdaysFull
			}
			matched := false
			for i, n := range names {
				if len(text)-ti >= len(n) && strings.EqualFold(text[ti:ti+len(n)], n) {
					ti += len(n)
					if d == 'b' || d == 'B' {
						tt.mon = i + 1
					} else {
						tt.wday = i
					}
					matched = true
					break
				}
			}
			ok = matched
		case '%':
			if ti < len(text) && text[ti] == '%' {
				ti++
			} else {
				ok = false
			}
		default:
			// Unsupported directive: consume nothing rather than guess.
			ok = false
		}
		if !ok {
			return nil, py.ExceptionNewf(py.ValueError,
				"time data '%s' does not match format '%s'", text, format)
		}
	}
	if ti != len(text) {
		return nil, py.ExceptionNewf(py.ValueError,
			"unconverted data remains: %s", text[ti:])
	}
	if ampm == "PM" && tt.hour < 12 {
		tt.hour += 12
	} else if ampm == "AM" && tt.hour == 12 {
		tt.hour = 0
	}
	// Fill in the weekday and day of year the format did not give, which
	// CPython's strptime does so the result is a complete struct_time.
	t := time.Date(tt.year, time.Month(tt.mon), tt.mday, tt.hour, tt.min, tt.sec, 0, time.UTC)
	if tt.yday == -1 {
		tt.yday = t.YearDay()
	} else {
		// A day-of-year fixes the month and day, the way CPython's _strptime
		// does when %j is the only date directive present.
		t = time.Date(tt.year, 1, 1, tt.hour, tt.min, tt.sec, 0, time.UTC).AddDate(0, 0, tt.yday-1)
		tt.mon, tt.mday = int(t.Month()), t.Day()
	}
	if tt.wday == -1 {
		tt.wday = (int(t.Weekday()) + 6) % 7
	}
	return tt, nil
}

// The module goes in __module__, not in the name: CPython's type(x).__name__ is the bare name and type(x).__module__ is 'time'.
func init() {
	StructTimeType.Dict.Set("__module__", py.String("time"))
}
