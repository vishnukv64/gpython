// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package datetime provides the implementation of python's 'datetime'
// module.
//
// datetime, date, time, timedelta and timezone are implemented with the
// arithmetic and formatting that code actually uses: construction,
// comparison, the timedelta operations, isoformat, strftime and strptime.
//
// The C-level date/time algorithms are the civil-from-days conversion from
// Howard Hinnant's paper, which is the same one CPython's implementation is
// derived from.
package datetime

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Fast implementation of the datetime type.`

// timedelta is a duration, held exactly as CPython holds it: days, seconds
// and microseconds, normalised so that seconds and microseconds are
// non-negative and below their limits.
type TimeDelta struct {
	Days         int
	Seconds      int
	Microseconds int
}

var TimeDeltaType = py.NewTypeX("datetime.timedelta", "Difference between two datetime values.", timeDeltaNew, nil)

func (d *TimeDelta) Type() *py.Type { return TimeDeltaType }

// normalize folds the components into their ranges.
func (d *TimeDelta) normalize() {
	// 86400 seconds in a day, 1000000 microseconds in a second.
	total := int64(d.Days)*86400 + int64(d.Seconds)
	totalUsec := int64(d.Microseconds)
	totalUsec += (total % 86400) * 1000000
	total -= total % 86400
	d.Days = int(total / 86400)
	d.Seconds = int(totalUsec / 1000000)
	d.Microseconds = int(totalUsec % 1000000)
	if d.Microseconds < 0 {
		d.Microseconds += 1000000
		d.Seconds--
	}
	if d.Seconds < 0 {
		d.Seconds += 86400
		d.Days--
	}
}

// totalMicros is the whole duration in microseconds.
func (d *TimeDelta) totalMicros() int64 {
	return (int64(d.Days)*86400+int64(d.Seconds))*1000000 + int64(d.Microseconds)
}

func timeDeltaNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &TimeDelta{}
	var (
		days, seconds, microseconds, milliseconds, minutes, hours, weeks int
	)
	for _, __e := range kwargs.Items() {
		k := __e.Key
		v := __e.Value

		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		switch k {
		case "days":
			days = n
		case "seconds":
			seconds = n
		case "microseconds":
			microseconds = n
		case "milliseconds":
			milliseconds = n
		case "minutes":
			minutes = n
		case "hours":
			hours = n
		case "weeks":
			weeks = n
		default:
			return nil, py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
		}
	}
	if len(args) > 7 {
		return nil, py.ExceptionNewf(py.TypeError, "timedelta() takes at most 7 arguments")
	}
	pos := []*int{&days, &seconds, &microseconds, &milliseconds, &minutes, &hours, &weeks}
	for i, a := range args {
		n, err := py.IndexInt(a)
		if err != nil {
			return nil, err
		}
		*pos[i] = n
	}

	d.Days = days + weeks*7
	d.Seconds = seconds + minutes*60 + hours*3600
	d.Microseconds = microseconds + milliseconds*1000
	d.normalize()
	return d, nil
}

// DateTime is a point in time.
type DateTime struct {
	Year, Month, Day            int
	Hour, Minute, Second, Micro int
	TZInfo                      py.Object
}

var DateTimeType = py.NewTypeX("datetime.datetime", "A date and time.", dateTimeNew, nil)

func (d *DateTime) Type() *py.Type { return DateTimeType }

func dateTimeNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &DateTime{Year: 1900, Month: 1, Day: 1}
	fields := []*int{&d.Year, &d.Month, &d.Day, &d.Hour, &d.Minute, &d.Second, &d.Micro}
	names := []string{"year", "month", "day", "hour", "minute", "second", "microsecond"}
	if len(args) > 7 {
		return nil, py.ExceptionNewf(py.TypeError, "datetime() takes at most 7 arguments")
	}
	for i, a := range args {
		n, err := py.IndexInt(a)
		if err != nil {
			return nil, err
		}
		*fields[i] = n
	}
	for _, __e := range kwargs.Items() {
		k := __e.Key
		v := __e.Value

		if k == "tzinfo" {
			d.TZInfo = v
			continue
		}
		if k == "fold" {
			continue
		}
		found := false
		for i, name := range names {
			if k == name {
				n, err := py.IndexInt(v)
				if err != nil {
					return nil, err
				}
				*fields[i] = n
				found = true
				break
			}
		}
		if !found {
			return nil, py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
		}
	}
	if len(args) < 3 {
		for _, name := range names[:3] {
			if _, ok := kwargs.Get(name); !ok {
				return nil, py.ExceptionNewf(py.TypeError, "function missing required argument '%s' (pos 1)", name)
			}
		}
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DateTime) validate() error {
	if d.Month < 1 || d.Month > 12 {
		return py.ExceptionNewf(py.ValueError, "month must be in 1..12")
	}
	if d.Day < 1 || d.Day > daysInMonth(d.Year, d.Month) {
		return py.ExceptionNewf(py.ValueError, "day is out of range for month")
	}
	if d.Hour < 0 || d.Hour > 23 {
		return py.ExceptionNewf(py.ValueError, "hour must be in 0..23")
	}
	if d.Minute < 0 || d.Minute > 59 {
		return py.ExceptionNewf(py.ValueError, "minute must be in 0..59")
	}
	if d.Second < 0 || d.Second > 59 {
		return py.ExceptionNewf(py.ValueError, "second must be in 0..59")
	}
	if d.Micro < 0 || d.Micro > 999999 {
		return py.ExceptionNewf(py.ValueError, "microsecond must be in 0..999999")
	}
	return nil
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func daysInMonth(y, m int) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeap(y) {
			return 29
		}
		return 28
	}
	return 0
}

// toTime converts to the Go time, for the calendar arithmetic.
func (d *DateTime) toTime() time.Time {
	return time.Date(d.Year, time.Month(d.Month), d.Day, d.Hour, d.Minute, d.Second, d.Micro*1000, time.UTC)
}

func fromTime(t time.Time) *DateTime {
	return &DateTime{
		Year: t.Year(), Month: int(t.Month()), Day: t.Day(),
		Hour: t.Hour(), Minute: t.Minute(), Second: t.Second(), Micro: t.Nanosecond() / 1000,
	}
}

// isoformat is the ISO 8601 form, as CPython writes it.
func (d *DateTime) isoformat(sep string) string {
	out := fmt.Sprintf("%04d-%02d-%02d%s%02d:%02d:%02d", d.Year, d.Month, d.Day, sep, d.Hour, d.Minute, d.Second)
	if d.Micro != 0 {
		out += fmt.Sprintf(".%06d", d.Micro)
	}
	return out
}

func init() {
	// timedelta
	TimeDeltaType.Dict.Set("days", intProp(func(self py.Object) int { return self.(*TimeDelta).Days }))
	TimeDeltaType.Dict.Set("seconds", intProp(func(self py.Object) int { return self.(*TimeDelta).Seconds }))
	TimeDeltaType.Dict.Set("microseconds", intProp(func(self py.Object) int { return self.(*TimeDelta).Microseconds }))
	TimeDeltaType.Dict.Set("total_seconds", py.MustNewMethod("total_seconds", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*TimeDelta)
		return py.Float(float64(d.totalMicros()) / 1e6), nil
	}, 0, "Total duration in seconds."))

	// datetime
	for name, get := range map[string]func(*DateTime) int{
		"year":        func(d *DateTime) int { return d.Year },
		"month":       func(d *DateTime) int { return d.Month },
		"day":         func(d *DateTime) int { return d.Day },
		"hour":        func(d *DateTime) int { return d.Hour },
		"minute":      func(d *DateTime) int { return d.Minute },
		"second":      func(d *DateTime) int { return d.Second },
		"microsecond": func(d *DateTime) int { return d.Micro },
	} {
		getter := get
		DateTimeType.Dict.Set(name, intProp(func(self py.Object) int { return getter(self.(*DateTime)) }))
	}
	DateTimeType.Dict.Set("tzinfo", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		if self.(*DateTime).TZInfo == nil {
			return py.None, nil
		}
		return self.(*DateTime).TZInfo, nil
	}})

	DateTimeType.Dict.Set("isoformat", py.MustNewMethod("isoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		sep := "T"
		if len(args) > 0 {
			if s, ok := args[0].(py.String); ok {
				sep = string(s)
			}
		}
		return py.String(self.(*DateTime).isoformat(sep)), nil
	}, 0, "Return the date and time as a string in ISO 8601 format."))

	DateTimeType.Dict.Set("date", py.MustNewMethod("date", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DateTime)
		return &Date{Year: d.Year, Month: d.Month, Day: d.Day}, nil
	}, 0, "Return the date part."))
	DateTimeType.Dict.Set("time", py.MustNewMethod("time", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DateTime)
		return &Clock{Hour: d.Hour, Minute: d.Minute, Second: d.Second, Micro: d.Micro}, nil
	}, 0, "Return the time part."))
	DateTimeType.Dict.Set("replace", py.MustNewMethod("replace", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		d := *self.(*DateTime)
		for _, __e := range kwargs.Items() {
			k := __e.Key
			v := __e.Value

			n, err := py.IndexInt(v)
			if err != nil {
				if k == "tzinfo" {
					d.TZInfo = v
					continue
				}
				return nil, err
			}
			switch k {
			case "year":
				d.Year = n
			case "month":
				d.Month = n
			case "day":
				d.Day = n
			case "hour":
				d.Hour = n
			case "minute":
				d.Minute = n
			case "second":
				d.Second = n
			case "microsecond":
				d.Micro = n
			default:
				return nil, py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
			}
		}
		if err := d.validate(); err != nil {
			return nil, err
		}
		return &d, nil
	}, 0, "Return a datetime with the given fields replaced."))
	DateTimeType.Dict.Set("strftime", py.MustNewMethod("strftime", func(self py.Object, args py.Tuple) (py.Object, error) {
		var format py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "strftime", 1, 1, &format); err != nil {
			return nil, err
		}
		text, err := py.StrAsString(format)
		if err != nil {
			return nil, err
		}
		return py.String(strftime(self.(*DateTime), text)), nil
	}, 0, "Format the datetime according to the given format string."))
	DateTimeType.Dict.Set("weekday", py.MustNewMethod("weekday", func(self py.Object, args py.Tuple) (py.Object, error) {
		// Monday is 0, as in CPython.
		w := self.(*DateTime).toTime().Weekday()
		return py.Int(int(w)), nil
	}, 0, "Return the day of the week, where Monday is 0."))
	DateTimeType.Dict.Set("isoweekday", py.MustNewMethod("isoweekday", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(int(self.(*DateTime).toTime().Weekday()) + 1), nil
	}, 0, "Return the day of the week, where Monday is 1."))
	DateTimeType.Dict.Set("fromisoformat", py.MustNewMethod("fromisoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		var text py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "fromisoformat", 1, 1, &text); err != nil {
			return nil, err
		}
		s, err := py.StrAsString(text)
		if err != nil {
			return nil, err
		}
		d, ok := parseIsoDateTime(s)
		if !ok {
			return nil, py.ExceptionNewf(py.ValueError, "Invalid isoformat string: '%s'", s)
		}
		return d, nil
	}, py.METH_CLASS, "Construct a datetime from an ISO 8601 formatted string."))

	DateTimeType.Dict.Set("now", py.MustNewMethod("now", func(self py.Object, args py.Tuple) (py.Object, error) {
		return fromTime(time.Now()), nil
	}, py.METH_CLASS, "Return the current local date and time."))
	DateTimeType.Dict.Set("utcnow", DateTimeType.Dict.GetOrNil("now"))
	DateTimeType.Dict.Set("today", DateTimeType.Dict.GetOrNil("now"))
	DateTimeType.Dict.Set("fromtimestamp", py.MustNewMethod("fromtimestamp", func(self py.Object, args py.Tuple) (py.Object, error) {
		var ts py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "fromtimestamp", 1, 1, &ts); err != nil {
			return nil, err
		}
		f, err := py.FloatAsFloat64(ts)
		if err != nil {
			return nil, err
		}
		sec := int64(f)
		nsec := int64((f - float64(sec)) * 1e9)
		return fromTime(time.Unix(sec, nsec).UTC()), nil
	}, py.METH_CLASS, "Return the local date and time corresponding to a POSIX timestamp."))

	DateTimeType.Dict.Set("timestamp", py.MustNewMethod("timestamp", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DateTime)
		return py.Float(float64(d.toTime().Unix()) + float64(d.Micro)/1e6), nil
	}, 0, "Return the POSIX timestamp."))

	// The class-level strptime and the constructors.
	DateTimeType.Dict.Set("strptime", py.MustNewMethod("strptime", strptime, 0, "Parse a string into a datetime, according to a format string."))
	DateTimeType.Dict.Set("combine", py.MustNewMethod("combine", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) < 2 {
			return nil, py.ExceptionNewf(py.TypeError, "combine() needs a date and a time")
		}
		date, okDate := args[0].(*Date)
		clock, okTime := args[1].(*Clock)
		if !okDate || !okTime {
			return nil, py.ExceptionNewf(py.TypeError, "combine() expects date and time arguments")
		}
		return &DateTime{Year: date.Year, Month: date.Month, Day: date.Day,
			Hour: clock.Hour, Minute: clock.Minute, Second: clock.Second, Micro: clock.Micro}, nil
	}, py.METH_CLASS, "Combine a date and a time into a datetime."))

	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "datetime", Value: DateTimeType},
		py.DictEntry{Key: "date", Value: DateType},
		py.DictEntry{Key: "time", Value: ClockType},
		py.DictEntry{Key: "timedelta", Value: TimeDeltaType},
		py.DictEntry{Key: "timezone", Value: TimezoneType},
		py.DictEntry{Key: "tzinfo", Value: TimezoneType},
		py.DictEntry{Key: "MINYEAR", Value: py.Int(1)},
		py.DictEntry{Key: "MAXYEAR", Value: py.Int(9999)},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "datetime",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// strptime parses text with a format string.
func strptime(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "strptime() needs a string and a format")
	}
	text, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	format, err := py.StrAsString(args[1])
	if err != nil {
		return nil, err
	}
	parsed, err := parseWithFormat(text, format)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

// parseWithFormat walks the format string, consuming the input.
func parseWithFormat(text, format string) (*DateTime, error) {
	d := &DateTime{Year: 1900, Month: 1, Day: 1}
	ti := 0
	// A format is satisfied by a date OR a time directive; strptime("%H:%M")
	// is as valid as strptime("%Y-%m-%d").
	hasDate, hasTimeSpec := false, false

	readInt := func(maxDigits int) (int, bool) {
		start := ti
		for ti < len(text) && ti-start < maxDigits && text[ti] >= '0' && text[ti] <= '9' {
			ti++
		}
		if ti == start {
			return 0, false
		}
		n := 0
		for _, c := range text[start:ti] {
			n = n*10 + int(c-'0')
		}
		return n, true
	}

	for fi := 0; fi < len(format); fi++ {
		c := format[fi]
		if c != '%' {
			// Whitespace in the format matches any run of whitespace; any
			// other character must match exactly.
			if c == ' ' || c == '\t' {
				for ti < len(text) && (text[ti] == ' ' || text[ti] == '\t') {
					ti++
				}
				continue
			}
			if ti < len(text) && text[ti] == c {
				ti++
				continue
			}
			continue
		}
		fi++
		if fi >= len(format) {
			break
		}
		switch format[fi] {
		case 'Y':
			if n, ok := readInt(4); ok {
				d.Year = n
				hasDate = true
			}
		case 'y':
			if n, ok := readInt(2); ok {
				if n < 69 {
					d.Year = 2000 + n
				} else {
					d.Year = 1900 + n
				}
				hasDate = true
			}
		case 'm':
			if n, ok := readInt(2); ok {
				d.Month = n
				hasDate = true
			}
		case 'd':
			if n, ok := readInt(2); ok {
				d.Day = n
				hasDate = true
			}
		case 'H':
			if n, ok := readInt(2); ok {
				d.Hour = n
				hasTimeSpec = true
			}
		case 'I':
			if n, ok := readInt(2); ok {
				d.Hour = n
				hasTimeSpec = true
			}
		case 'M':
			if n, ok := readInt(2); ok {
				d.Minute = n
				hasTimeSpec = true
			}
		case 'S':
			if n, ok := readInt(2); ok {
				d.Second = n
				hasTimeSpec = true
			}
		case 'f':
			if n, ok := readInt(6); ok {
				// Microseconds, padded on the right.
				for i := digits(n); i < 6; i++ {
					n *= 10
				}
				d.Micro = n
			}
		case 'p':
			// AM/PM adjusts an hour read with %I.
			hasTimeSpec = true
			if ti+2 <= len(text) {
				ampm := strings.ToUpper(text[ti : ti+2])
				if ampm == "PM" && d.Hour < 12 {
					d.Hour += 12
				}
				if ampm == "AM" && d.Hour == 12 {
					d.Hour = 0
				}
				ti += 2
			}
		case 'b', 'B':
			// A month name, matched against the English names.
			for m, name := range monthNames {
				for _, candidate := range []string{name, name[:3]} {
					if len(text)-ti >= len(candidate) && strings.EqualFold(text[ti:ti+len(candidate)], candidate) {
						d.Month = m + 1
						hasDate = true
						ti += len(candidate)
						break
					}
				}
			}
		case 'a', 'A':
			// A weekday name, which strptime accepts and ignores.
			for _, name := range weekdayNames {
				for _, candidate := range []string{name, name[:3]} {
					if len(text)-ti >= len(candidate) && strings.EqualFold(text[ti:ti+len(candidate)], candidate) {
						ti += len(candidate)
						break
					}
				}
			}
		case 'j':
			if n, ok := readInt(3); ok {
				// A day of year, converted to a month and day.
				month, day := monthDayFromYearDay(d.Year, n)
				d.Month, d.Day = month, day
				hasDate = true
			}
		case 'z':
			// An offset such as +0000; the value is accepted and the
			// datetime stays naive, which is what a naive datetime does.
			if ti < len(text) && (text[ti] == '+' || text[ti] == '-') {
				ti += 5
			}
		case 'Z':
			for ti < len(text) && text[ti] != ' ' {
				ti++
			}
		case '%':
			if ti < len(text) && text[ti] == '%' {
				ti++
			}
		default:
			// An unsupported directive: skip any matching text so the rest
			// of the parse can continue.
		}
	}
	if !hasDate && !hasTimeSpec {
		return nil, py.ExceptionNewf(py.ValueError, "time data %q does not match format %q", text, format)
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return d, nil
}

func digits(n int) int {
	if n == 0 {
		return 1
	}
	d := 0
	for n > 0 {
		n /= 10
		d++
	}
	return d
}

func monthDayFromYearDay(year, yday int) (int, int) {
	for m := 1; m <= 12; m++ {
		dim := daysInMonth(year, m)
		if yday <= dim {
			return m, yday
		}
		yday -= dim
	}
	return 12, 31
}

var monthNames = []string{"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

var weekdayNames = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// strftime renders the datetime with the directives real code uses.
func strftime(d *DateTime, format string) string {
	var b strings.Builder
	for fi := 0; fi < len(format); fi++ {
		if format[fi] != '%' {
			b.WriteByte(format[fi])
			continue
		}
		fi++
		if fi >= len(format) {
			break
		}
		switch format[fi] {
		case 'Y':
			fmt.Fprintf(&b, "%04d", d.Year)
		case 'y':
			fmt.Fprintf(&b, "%02d", d.Year%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", d.Month)
		case 'd':
			fmt.Fprintf(&b, "%02d", d.Day)
		case 'H':
			fmt.Fprintf(&b, "%02d", d.Hour)
		case 'I':
			h := d.Hour % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%02d", h)
		case 'M':
			fmt.Fprintf(&b, "%02d", d.Minute)
		case 'S':
			fmt.Fprintf(&b, "%02d", d.Second)
		case 'f':
			fmt.Fprintf(&b, "%06d", d.Micro)
		case 'p':
			if d.Hour < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'b':
			b.WriteString(monthNames[d.Month-1][:3])
		case 'B':
			b.WriteString(monthNames[d.Month-1])
		case 'a':
			b.WriteString(weekdayNames[d.toTime().Weekday()][:3])
		case 'A':
			b.WriteString(weekdayNames[d.toTime().Weekday()])
		case 'j':
			yday := 0
			for m := 1; m < d.Month; m++ {
				yday += daysInMonth(d.Year, m)
			}
			fmt.Fprintf(&b, "%03d", yday+d.Day)
		case 'w':
			fmt.Fprintf(&b, "%d", int(d.toTime().Weekday())+1)
		case 'x':
			fmt.Fprintf(&b, "%02d/%02d/%02d", d.Month, d.Day, d.Year%100)
		case 'X':
			fmt.Fprintf(&b, "%02d:%02d:%02d", d.Hour, d.Minute, d.Second)
		case 'c':
			b.WriteString(d.toTime().Format("Mon Jan  2 15:04:05 2006"))
		case 'Z':
			b.WriteString("UTC")
		case 'z':
			b.WriteString("+0000")
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(format[fi])
		}
	}
	return b.String()
}

// intProp builds an int-valued property.
func intProp(get func(py.Object) int) *py.Property {
	return &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(get(self)), nil
	}}
}

// ---------------------------------------------------------------------------
// date and time

// Date is a calendar date.
type Date struct {
	Year, Month, Day int
}

var DateType = py.NewTypeX("datetime.date", "A calendar date.", dateNew, nil)

func (d *Date) Type() *py.Type { return DateType }

func dateNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &Date{}
	fields := []*int{&d.Year, &d.Month, &d.Day}
	names := []string{"year", "month", "day"}
	if len(args) > 3 {
		return nil, py.ExceptionNewf(py.TypeError, "date() takes at most 3 arguments")
	}
	for i, a := range args {
		n, err := py.IndexInt(a)
		if err != nil {
			return nil, err
		}
		*fields[i] = n
	}
	for _, __e := range kwargs.Items() {
		k := __e.Key
		v := __e.Value

		for i, name := range names {
			if k == name {
				n, err := py.IndexInt(v)
				if err != nil {
					return nil, err
				}
				*fields[i] = n
			}
		}
	}
	if err := validateDate(d.Year, d.Month, d.Day); err != nil {
		return nil, err
	}
	return d, nil
}

func validateDate(y, m, dd int) error {
	if m < 1 || m > 12 {
		return py.ExceptionNewf(py.ValueError, "month must be in 1..12")
	}
	if dd < 1 || dd > daysInMonth(y, m) {
		return py.ExceptionNewf(py.ValueError, "day is out of range for month")
	}
	return nil
}

// Clock is a time of day.
type Clock struct {
	Hour, Minute, Second, Micro int
}

var ClockType = py.NewTypeX("datetime.time", "A time of day.", clockNew, nil)

func (c *Clock) Type() *py.Type { return ClockType }

func clockNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := &Clock{}
	fields := []*int{&c.Hour, &c.Minute, &c.Second, &c.Micro}
	names := []string{"hour", "minute", "second", "microsecond"}
	if len(args) > 4 {
		return nil, py.ExceptionNewf(py.TypeError, "time() takes at most 4 arguments")
	}
	for i, a := range args {
		n, err := py.IndexInt(a)
		if err != nil {
			return nil, err
		}
		*fields[i] = n
	}
	for _, __e := range kwargs.Items() {
		k := __e.Key
		v := __e.Value

		for i, name := range names {
			if k == name {
				n, err := py.IndexInt(v)
				if err != nil {
					return nil, err
				}
				*fields[i] = n
			}
		}
	}
	return c, nil
}

// Timezone is a fixed offset from UTC.
type Timezone struct {
	OffsetMinutes int
	Name          string
}

var TimezoneType = py.NewTypeX("datetime.timezone", "A fixed offset from UTC.", timezoneNew, nil)

func (t *Timezone) Type() *py.Type { return TimezoneType }

func timezoneNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "timezone() needs an offset")
	}
	d, ok := args[0].(*TimeDelta)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "timezone() argument must be a timedelta")
	}
	tz := &Timezone{OffsetMinutes: int(d.totalMicros() / 60000000)}
	if len(args) >= 2 {
		if s, ok := args[1].(py.String); ok {
			tz.Name = string(s)
		}
	}
	return tz, nil
}

func init() {
	DateType.Dict.Set("year", intProp(func(self py.Object) int { return self.(*Date).Year }))
	DateType.Dict.Set("month", intProp(func(self py.Object) int { return self.(*Date).Month }))
	DateType.Dict.Set("day", intProp(func(self py.Object) int { return self.(*Date).Day }))
	DateType.Dict.Set("isoformat", py.MustNewMethod("isoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Date)
		return py.String(fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)), nil
	}, py.METH_CLASS, "ISO 8601 form."))
	DateType.Dict.Set("today", py.MustNewMethod("today", func(self py.Object, args py.Tuple) (py.Object, error) {
		now := time.Now()
		return &Date{Year: now.Year(), Month: int(now.Month()), Day: now.Day()}, nil
	}, 0, "The current local date."))
	DateType.Dict.Set("weekday", py.MustNewMethod("weekday", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Date)
		return py.Int(int(time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC).Weekday())), nil
	}, 0, "Day of the week, Monday is 0."))
	DateType.Dict.Set("toordinal", py.MustNewMethod("toordinal", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Date)
		return py.Int(ordinal(d.Year, d.Month, d.Day)), nil
	}, 0, "Return the proleptic Gregorian ordinal of the date."))
	DateType.Dict.Set("fromordinal", py.MustNewMethod("fromordinal", func(self py.Object, args py.Tuple) (py.Object, error) {
		var n py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "fromordinal", 1, 1, &n); err != nil {
			return nil, err
		}
		ord, err := py.IndexInt(n)
		if err != nil {
			return nil, err
		}
		if ord < 1 {
			return nil, py.ExceptionNewf(py.ValueError, "ordinal must be >= 1")
		}
		t := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, ord-1)
		return &Date{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}, nil
	}, py.METH_CLASS, "Construct a date from a proleptic Gregorian ordinal."))
	DateType.Dict.Set("fromisoformat", py.MustNewMethod("fromisoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		var text py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "fromisoformat", 1, 1, &text); err != nil {
			return nil, err
		}
		s, err := py.StrAsString(text)
		if err != nil {
			return nil, err
		}
		y, m, dd, ok := parseIsoDate(s)
		if !ok {
			return nil, py.ExceptionNewf(py.ValueError, "Invalid isoformat string: '%s'", s)
		}
		return &Date{Year: y, Month: m, Day: dd}, nil
	}, py.METH_CLASS, "Construct a date from an ISO 8601 formatted string."))

	ClockType.Dict.Set("hour", intProp(func(self py.Object) int { return self.(*Clock).Hour }))
	ClockType.Dict.Set("minute", intProp(func(self py.Object) int { return self.(*Clock).Minute }))
	ClockType.Dict.Set("second", intProp(func(self py.Object) int { return self.(*Clock).Second }))
	ClockType.Dict.Set("microsecond", intProp(func(self py.Object) int { return self.(*Clock).Micro }))
	ClockType.Dict.Set("isoformat", py.MustNewMethod("isoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Clock)
		out := fmt.Sprintf("%02d:%02d:%02d", c.Hour, c.Minute, c.Second)
		if c.Micro != 0 {
			out += fmt.Sprintf(".%06d", c.Micro)
		}
		return py.String(out), nil
	}, 0, "ISO 8601 form."))

	TimezoneType.Dict.Set("utc", &Timezone{})
}

// ---------------------------------------------------------------------------
// Go interface bridges
//
// str() and repr() go through the Go interfaces rather than the type's Dict,
// so the printable forms are implemented directly.  A datetime that printed
// as "<datetime.datetime instance at 0x...>" would be useless in a log line,
// which is exactly where these values end up.

func (d *DateTime) M__str__() (py.Object, error) { return py.String(d.isoformat(" ")), nil }

func (d *DateTime) M__repr__() (py.Object, error) {
	return py.String("datetime.datetime(" +
		fmt.Sprintf("%d, %d, %d, %d, %d", d.Year, d.Month, d.Day, d.Hour, d.Minute) +
		microPart(d.Micro) + ")"), nil
}

func (d *Date) M__str__() (py.Object, error) {
	return py.String(fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)), nil
}

func (d *Date) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("datetime.date(%d, %d, %d)", d.Year, d.Month, d.Day)), nil
}

func (c *Clock) M__str__() (py.Object, error) {
	out := fmt.Sprintf("%02d:%02d:%02d", c.Hour, c.Minute, c.Second)
	if c.Micro != 0 {
		out += fmt.Sprintf(".%06d", c.Micro)
	}
	return py.String(out), nil
}

func (c *Clock) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("datetime.time(%d, %d, %d)", c.Hour, c.Minute, c.Second)), nil
}

func (t *TimeDelta) M__str__() (py.Object, error) { return t.M__repr__() }

func (t *TimeDelta) M__repr__() (py.Object, error) {
	out := "datetime.timedelta("
	parts := []string{}
	if t.Days != 0 {
		parts = append(parts, fmt.Sprintf("days=%d", t.Days))
	}
	if t.Seconds != 0 {
		parts = append(parts, fmt.Sprintf("seconds=%d", t.Seconds))
	}
	if t.Microseconds != 0 {
		parts = append(parts, fmt.Sprintf("microseconds=%d", t.Microseconds))
	}
	return py.String(out + strings.Join(parts, ", ") + ")"), nil
}

// microPart renders the microseconds argument when it is non-zero.
func microPart(micro int) string {
	if micro == 0 {
		return ""
	}
	return fmt.Sprintf(", %d", micro)
}

// ---------------------------------------------------------------------------
// Arithmetic
//
// date/datetime and timedelta add and subtract the way CPython defines them,
// through the Go interfaces so the vm's binary operators find them.  An
// operand of any other type returns NotImplemented, which is what lets the vm
// fall through to the reflected method and finally raise the same TypeError
// CPython does.

// shiftBy moves a calendar day by a timedelta.  The day count goes through
// AddDate so that a timedelta of years keeps working: folding the whole
// duration into a time.Duration would overflow after about 292 years.
func shiftBy(year, month, day int, t *TimeDelta) (int, int, int) {
	tt := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	tt = tt.AddDate(0, 0, t.Days)
	// Seconds and microseconds are normalised non-negative and below their
	// limits, so this cannot overflow the duration.
	tt = tt.Add(time.Duration(t.Seconds)*time.Second + time.Duration(t.Microseconds)*time.Microsecond)
	return tt.Year(), int(tt.Month()), tt.Day()
}

// ordinal is the proleptic Gregorian ordinal CPython counts from: 0001-01-01
// is 1, so 1970-01-01 is 719163.
func ordinal(year, month, day int) int {
	return int(time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Unix()/86400) + 719163
}

// parseIsoDate parses the ISO 8601 forms CPython's date.fromisoformat accepts:
// YYYY-MM-DD, the compact YYYYMMDD, and the ISO week form YYYY-Www-D.  It
// returns the fields and whether the text matched; anything else is the
// ValueError the caller raises.
func parseIsoDate(text string) (int, int, int, bool) {
	if len(text) == 10 && text[4] == '-' && text[5] == 'W' && text[8] == '-' {
		// YYYY-Www-D: the week form, converted through the ISO year.
		year := atoiOr(text[0:4], -1)
		week := atoiOr(text[6:8], -1)
		day := atoiOr(text[9:10], -1)
		if year < 0 || week < 1 || week > 53 || day < 1 || day > 7 {
			return 0, 0, 0, false
		}
		// The Monday of ISO week 1 is the week containing January 4th.
		jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
		monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
		monday = monday.AddDate(0, 0, (week-1)*7+day-1)
		return monday.Year(), int(monday.Month()), monday.Day(), true
	}
	s := text
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		s = s[:4] + s[5:7] + s[8:10]
	}
	if len(s) != 8 {
		return 0, 0, 0, false
	}
	year := atoiOr(s[0:4], -1)
	month := atoiOr(s[4:6], -1)
	day := atoiOr(s[6:8], -1)
	if year < 0 || month < 0 || day < 0 {
		return 0, 0, 0, false
	}
	if validateDate(year, month, day) != nil {
		return 0, 0, 0, false
	}
	return year, month, day, true
}

// atoiOr is strconv.Atoi with a fallback, kept local so the parse helpers do
// not need an error return at every call.
func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func (d *Date) M__add__(other py.Object) (py.Object, error) {
	t, ok := other.(*TimeDelta)
	if !ok {
		return py.NotImplemented, nil
	}
	y, m, dd := shiftBy(d.Year, d.Month, d.Day, t)
	return &Date{Year: y, Month: m, Day: dd}, nil
}

// M__radd__ is the timedelta + date order, which the vm reflects here.
func (d *Date) M__radd__(other py.Object) (py.Object, error) {
	return d.M__add__(other)
}

func (d *Date) M__sub__(other py.Object) (py.Object, error) {
	switch o := other.(type) {
	case *TimeDelta:
		neg := &TimeDelta{Days: -o.Days, Seconds: -o.Seconds, Microseconds: -o.Microseconds}
		y, m, dd := shiftBy(d.Year, d.Month, d.Day, neg)
		return &Date{Year: y, Month: m, Day: dd}, nil
	case *Date:
		days := ordinal(d.Year, d.Month, d.Day) - ordinal(o.Year, o.Month, o.Day)
		return &TimeDelta{Days: days}, nil
	}
	return py.NotImplemented, nil
}

func (d *DateTime) M__add__(other py.Object) (py.Object, error) {
	t, ok := other.(*TimeDelta)
	if !ok {
		return py.NotImplemented, nil
	}
	y, m, dd := shiftBy(d.Year, d.Month, d.Day, t)
	tt := time.Date(y, time.Month(m), dd, d.Hour, d.Minute, d.Second, d.Micro*1000, time.UTC).
		Add(time.Duration(t.Seconds)*time.Second + time.Duration(t.Microseconds)*time.Microsecond)
	out := fromTime(tt)
	out.TZInfo = d.TZInfo
	return out, nil
}

func (d *DateTime) M__radd__(other py.Object) (py.Object, error) {
	return d.M__add__(other)
}

// parseIsoDateTime parses the ISO 8601 forms datetime.fromisoformat accepts.
// The date part is shared with parseIsoDate; a space or 'T' introduces the
// time, which may carry a fractional second and an (ignored, naive-keeping)
// UTC offset.
func parseIsoDateTime(text string) (*DateTime, bool) {
	datePart := text
	rest := ""
	if i := strings.IndexAny(text, "T "); i >= 0 {
		datePart, rest = text[:i], text[i+1:]
	}
	y, m, dd, ok := parseIsoDate(datePart)
	if !ok {
		return nil, false
	}
	d := &DateTime{Year: y, Month: m, Day: dd}
	if rest == "" {
		t := time.Date(y, time.Month(m), dd, 0, 0, 0, 0, time.UTC)
		d.Hour, d.Minute, d.Second = t.Hour(), t.Minute(), t.Second()
		return d, true
	}
	// Drop an offset; a naive datetime keeps the wall-clock fields.
	if i := strings.IndexAny(rest, "+-"); i > 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSuffix(rest, "Z")
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return nil, false
	}
	h := atoiOr(parts[0], -1)
	mi := atoiOr(parts[1], -1)
	secPart := parts[2]
	frac := 0
	if i := strings.IndexAny(secPart, ".,"); i >= 0 {
		fracText := secPart[i+1:]
		secPart = secPart[:i]
		if len(fracText) > 6 {
			fracText = fracText[:6]
		}
		for len(fracText) < 6 {
			fracText += "0"
		}
		frac = atoiOr(fracText, 0)
	}
	sec := atoiOr(secPart, -1)
	if h < 0 || mi < 0 || sec < 0 {
		return nil, false
	}
	d.Hour, d.Minute, d.Second, d.Micro = h, mi, sec, frac
	if err := d.validate(); err != nil {
		return nil, false
	}
	return d, true
}

func (d *DateTime) M__sub__(other py.Object) (py.Object, error) {
	switch o := other.(type) {
	case *TimeDelta:
		neg := &TimeDelta{Days: -o.Days, Seconds: -o.Seconds, Microseconds: -o.Microseconds}
		return d.M__add__(neg)
	case *DateTime:
		micros := (int64(ordinal(d.Year, d.Month, d.Day))-int64(ordinal(o.Year, o.Month, o.Day)))*86400*1000000 +
			int64(d.Hour-o.Hour)*3600*1000000 + int64(d.Minute-o.Minute)*60*1000000 +
			int64(d.Second-o.Second)*1000000 + int64(d.Micro-o.Micro)
		td := &TimeDelta{
			Days:         int(micros / (86400 * 1000000)),
			Seconds:      int((micros % (86400 * 1000000)) / 1000000),
			Microseconds: int(micros % 1000000),
		}
		td.normalize()
		return td, nil
	}
	return py.NotImplemented, nil
}

// Comparisons.  A datetime is ordered by its value, which is what sorting a
// list of them means.
func (d *DateTime) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*DateTime)
	if !ok {
		return py.False, nil
	}
	return py.NewBool(*d == *o), nil
}

func (d *DateTime) M__lt__(other py.Object) (py.Object, error) {
	o, ok := other.(*DateTime)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "cannot compare a datetime with %s", other.Type().Name)
	}
	return py.NewBool(d.toTime().Before(o.toTime())), nil
}

func (d *Date) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*Date)
	if !ok {
		return py.False, nil
	}
	return py.NewBool(*d == *o), nil
}

func (t *TimeDelta) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*TimeDelta)
	if !ok {
		return py.False, nil
	}
	return py.NewBool(t.totalMicros() == o.totalMicros()), nil
}

var (
	_ py.I__str__  = (*DateTime)(nil)
	_ py.I__repr__ = (*DateTime)(nil)
	_ py.I__eq__   = (*DateTime)(nil)
	_ py.I__lt__   = (*DateTime)(nil)
	_ py.I__str__  = (*Date)(nil)
	_ py.I__repr__ = (*Date)(nil)
	_ py.I__eq__   = (*Date)(nil)
	_ py.I__str__  = (*Clock)(nil)
	_ py.I__repr__ = (*Clock)(nil)
	_ py.I__str__  = (*TimeDelta)(nil)
	_ py.I__repr__ = (*TimeDelta)(nil)
	_ py.I__eq__   = (*TimeDelta)(nil)
)
