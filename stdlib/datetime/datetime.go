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
	for k, v := range kwargs {
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
	for k, v := range kwargs {
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
			if _, ok := kwargs[name]; !ok {
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
	TimeDeltaType.Dict["days"] = intProp(func(self py.Object) int { return self.(*TimeDelta).Days })
	TimeDeltaType.Dict["seconds"] = intProp(func(self py.Object) int { return self.(*TimeDelta).Seconds })
	TimeDeltaType.Dict["microseconds"] = intProp(func(self py.Object) int { return self.(*TimeDelta).Microseconds })
	TimeDeltaType.Dict["total_seconds"] = py.MustNewMethod("total_seconds", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*TimeDelta)
		return py.Float(float64(d.totalMicros()) / 1e6), nil
	}, 0, "Total duration in seconds.")

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
		DateTimeType.Dict[name] = intProp(func(self py.Object) int { return getter(self.(*DateTime)) })
	}
	DateTimeType.Dict["tzinfo"] = &py.Property{Fget: func(self py.Object) (py.Object, error) {
		if self.(*DateTime).TZInfo == nil {
			return py.None, nil
		}
		return self.(*DateTime).TZInfo, nil
	}}

	DateTimeType.Dict["isoformat"] = py.MustNewMethod("isoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		sep := "T"
		if len(args) > 0 {
			if s, ok := args[0].(py.String); ok {
				sep = string(s)
			}
		}
		return py.String(self.(*DateTime).isoformat(sep)), nil
	}, 0, "Return the date and time as a string in ISO 8601 format.")

	DateTimeType.Dict["date"] = py.MustNewMethod("date", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DateTime)
		return &Date{Year: d.Year, Month: d.Month, Day: d.Day}, nil
	}, 0, "Return the date part.")
	DateTimeType.Dict["time"] = py.MustNewMethod("time", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DateTime)
		return &Clock{Hour: d.Hour, Minute: d.Minute, Second: d.Second, Micro: d.Micro}, nil
	}, 0, "Return the time part.")
	DateTimeType.Dict["replace"] = py.MustNewMethod("replace", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		d := *self.(*DateTime)
		for k, v := range kwargs {
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
	}, 0, "Return a datetime with the given fields replaced.")
	DateTimeType.Dict["strftime"] = py.MustNewMethod("strftime", func(self py.Object, args py.Tuple) (py.Object, error) {
		var format py.Object
		if err := py.UnpackTuple(args, nil, "strftime", 1, 1, &format); err != nil {
			return nil, err
		}
		text, err := py.StrAsString(format)
		if err != nil {
			return nil, err
		}
		return py.String(strftime(self.(*DateTime), text)), nil
	}, 0, "Format the datetime according to the given format string.")
	DateTimeType.Dict["weekday"] = py.MustNewMethod("weekday", func(self py.Object, args py.Tuple) (py.Object, error) {
		// Monday is 0, as in CPython.
		w := self.(*DateTime).toTime().Weekday()
		return py.Int(int(w)), nil
	}, 0, "Return the day of the week, where Monday is 0.")
	DateTimeType.Dict["isoweekday"] = py.MustNewMethod("isoweekday", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(int(self.(*DateTime).toTime().Weekday()) + 1), nil
	}, 0, "Return the day of the week, where Monday is 1.")

	DateTimeType.Dict["now"] = py.MustNewMethod("now", func(self py.Object, args py.Tuple) (py.Object, error) {
		return fromTime(time.Now()), nil
	}, 0, "Return the current local date and time.")
	DateTimeType.Dict["utcnow"] = DateTimeType.Dict["now"]
	DateTimeType.Dict["today"] = DateTimeType.Dict["now"]
	DateTimeType.Dict["fromtimestamp"] = py.MustNewMethod("fromtimestamp", func(self py.Object, args py.Tuple) (py.Object, error) {
		var ts py.Object
		if err := py.UnpackTuple(args, nil, "fromtimestamp", 1, 1, &ts); err != nil {
			return nil, err
		}
		f, err := py.FloatAsFloat64(ts)
		if err != nil {
			return nil, err
		}
		sec := int64(f)
		nsec := int64((f - float64(sec)) * 1e9)
		return fromTime(time.Unix(sec, nsec).UTC()), nil
	}, 0, "Return the local date and time corresponding to a POSIX timestamp.")

	DateTimeType.Dict["timestamp"] = py.MustNewMethod("timestamp", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DateTime)
		return py.Float(float64(d.toTime().Unix()) + float64(d.Micro)/1e6), nil
	}, 0, "Return the POSIX timestamp.")

	// The class-level strptime and the constructors.
	DateTimeType.Dict["strptime"] = py.MustNewMethod("strptime", strptime, 0, "Parse a string into a datetime, according to a format string.")
	DateTimeType.Dict["combine"] = py.MustNewMethod("combine", func(self py.Object, args py.Tuple) (py.Object, error) {
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
	}, 0, "Combine a date and a time into a datetime.")

	globals := py.StringDict{
		"datetime":  DateTimeType,
		"date":      DateType,
		"time":      ClockType,
		"timedelta": TimeDeltaType,
		"timezone":  TimezoneType,
		"tzinfo":    TimezoneType,
		"MINYEAR":   py.Int(1),
		"MAXYEAR":   py.Int(9999),
	}

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
	for k, v := range kwargs {
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
	for k, v := range kwargs {
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
	DateType.Dict["year"] = intProp(func(self py.Object) int { return self.(*Date).Year })
	DateType.Dict["month"] = intProp(func(self py.Object) int { return self.(*Date).Month })
	DateType.Dict["day"] = intProp(func(self py.Object) int { return self.(*Date).Day })
	DateType.Dict["isoformat"] = py.MustNewMethod("isoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Date)
		return py.String(fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)), nil
	}, 0, "ISO 8601 form.")
	DateType.Dict["today"] = py.MustNewMethod("today", func(self py.Object, args py.Tuple) (py.Object, error) {
		now := time.Now()
		return &Date{Year: now.Year(), Month: int(now.Month()), Day: now.Day()}, nil
	}, 0, "The current local date.")
	DateType.Dict["weekday"] = py.MustNewMethod("weekday", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Date)
		return py.Int(int(time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC).Weekday())), nil
	}, 0, "Day of the week, Monday is 0.")

	ClockType.Dict["hour"] = intProp(func(self py.Object) int { return self.(*Clock).Hour })
	ClockType.Dict["minute"] = intProp(func(self py.Object) int { return self.(*Clock).Minute })
	ClockType.Dict["second"] = intProp(func(self py.Object) int { return self.(*Clock).Second })
	ClockType.Dict["microsecond"] = intProp(func(self py.Object) int { return self.(*Clock).Micro })
	ClockType.Dict["isoformat"] = py.MustNewMethod("isoformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Clock)
		out := fmt.Sprintf("%02d:%02d:%02d", c.Hour, c.Minute, c.Second)
		if c.Micro != 0 {
			out += fmt.Sprintf(".%06d", c.Micro)
		}
		return py.String(out), nil
	}, 0, "ISO 8601 form.")

	TimezoneType.Dict["utc"] = &Timezone{}
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
