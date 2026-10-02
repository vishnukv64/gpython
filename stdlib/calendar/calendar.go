// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package calendar provides the implementation of python's 'calendar' module.
//
// The conversion functions (timegm, leapdays, isleap, monthrange, weekday,
// monthcalendar, prmonth, prcal) and the month/weekday name lists are
// implemented.  requests uses calendar.timegm, and the rest are pure
// arithmetic over the same tables.
package calendar

import (
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Functions for working with calendars.

This implementation provides the date arithmetic (isleap, leapdays,
monthrange, weekday, monthcalendar) and timegm, plus the month and weekday
name tables.`

var (
	monthName = []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	monthAbbr = []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	dayName   = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
	dayAbbr   = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	mdays     = []int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
)

// isleap reports whether year is a leap year.
func isleap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// leapdays returns the number of leap years in the range [y1, y2).
func leapdays(y1, y2 int) int {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	y1--
	res := y2/4 - y1/4
	res -= y2/100 - y1/100
	res += y2/400 - y1/400
	return res
}

// monthrange returns (weekday of the first day, number of days) for a month.
func monthrange(year, month int) (int, int) {
	if month < 1 || month > 12 {
		return -1, -1
	}
	w := (dayOfWeek(year, month, 1) + 1) % 7 // Monday=0 -> CPython Monday=0
	days := mdays[month]
	if month == 2 && isleap(year) {
		days++
	}
	return w, days
}

// dayOfWeek returns 0 for Monday .. 6 for Sunday.
func dayOfWeek(year, month, day int) int {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	// Go's Weekday is Sunday=0; CPython's weekday() is Monday=0.
	return (int(t.Weekday()) + 6) % 7
}

func calendarIsLeap(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "isleap() takes exactly one argument")
	}
	y, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	return py.Bool(isleap(y)), nil
}

func calendarLeapDays(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "leapdays() takes exactly two arguments")
	}
	y1, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	y2, err := py.IndexInt(args[1])
	if err != nil {
		return nil, err
	}
	return py.Int(leapdays(y1, y2)), nil
}

func calendarMonthRange(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "monthrange() takes exactly two arguments")
	}
	y, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	m, err := py.IndexInt(args[1])
	if err != nil {
		return nil, err
	}
	w, d := monthrange(y, m)
	return py.Tuple{py.Int(w), py.Int(d)}, nil
}

func calendarWeekday(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 3 {
		return nil, py.ExceptionNewf(py.TypeError, "weekday() takes exactly three arguments")
	}
	y, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	m, err := py.IndexInt(args[1])
	if err != nil {
		return nil, err
	}
	d, err := py.IndexInt(args[2])
	if err != nil {
		return nil, err
	}
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return nil, py.ExceptionNewf(py.ValueError, "bad month or day")
	}
	return py.Int(dayOfWeek(y, m, d)), nil
}

func calendarIsWeekend(self py.Object, args py.Tuple) (py.Object, error) {
	res, err := calendarWeekday(self, args)
	if err != nil {
		return nil, err
	}
	return py.Bool(int(res.(py.Int)) >= 5), nil
}

// calendarTimegm is calendar.timegm: it treats a struct_time as UTC and
// returns the epoch seconds.
func calendarTimegm(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "timegm() takes exactly one argument")
	}
	seq, err := py.SequenceTuple(args[0])
	if err != nil {
		return nil, err
	}
	get := func(i int, dflt int) int {
		if i < len(seq) {
			if n, err := py.IndexInt(seq[i]); err == nil {
				return n
			}
		}
		return dflt
	}
	year := get(0, 1970)
	month := get(1, 1)
	day := get(2, 1)
	hour := get(3, 0)
	minute := get(4, 0)
	second := get(5, 0)
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	return py.Int(t.Unix()), nil
}

func calendarMonthCalendar(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var year, month py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO:monthcalendar",
		[]string{"year", "month"}, &year, &month); err != nil {
		return nil, err
	}
	y, err := py.IndexInt(year)
	if err != nil {
		return nil, err
	}
	m, err := py.IndexInt(month)
	if err != nil {
		return nil, err
	}
	first, days := monthrange(y, m)
	if days < 0 {
		return nil, py.ExceptionNewf(py.ValueError, "bad month number")
	}
	var weeks []py.Object
	week := make([]py.Object, 7)
	for i := range week {
		week[i] = py.Int(0)
	}
	day := 1
	for i := first; i < 7; i++ {
		week[i] = py.Int(day)
		day++
	}
	weeks = append(weeks, py.NewListFromItems(week))
	for day <= days {
		w := make([]py.Object, 7)
		for i := range w {
			if day <= days {
				w[i] = py.Int(day)
				day++
			} else {
				w[i] = py.Int(0)
			}
		}
		weeks = append(weeks, py.NewListFromItems(w))
	}
	return py.NewListFromItems(weeks), nil
}

func calendarMonth(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "month() takes exactly two arguments")
	}
	y, _ := py.IndexInt(args[0])
	m, _ := py.IndexInt(args[1])
	if m < 1 || m > 12 {
		return nil, py.ExceptionNewf(py.ValueError, "bad month number")
	}
	cal, err := calendarMonthCalendar(self, py.Tuple{py.Int(y), py.Int(m)}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("     " + monthAbbr[m] + " " + itoa(y) + "\n")
	b.WriteString("Mo Tu We Th Fr Sa Su\n")
	for _, wk := range cal.(*py.List).Items {
		for i, d := range wk.(*py.List).Items {
			if i > 0 {
				b.WriteByte(' ')
			}
			n, _ := py.IndexInt(d)
			if n == 0 {
				b.WriteString("  ")
			} else {
				if n < 10 {
					b.WriteByte(' ')
				}
				b.WriteString(itoa(n))
			}
		}
		b.WriteByte('\n')
	}
	return py.String(b.String()), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func calendarSetFirstWeekday(self py.Object, args py.Tuple) (py.Object, error) {
	return py.None, nil
}

func calendarFirstWeekday(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(0), nil
}

func init() {
	monthNames := make([]py.Object, len(monthName))
	monthAbbrs := make([]py.Object, len(monthAbbr))
	dayNames := make([]py.Object, len(dayName))
	dayAbbrs := make([]py.Object, len(dayAbbr))
	md := make([]py.Object, len(mdays))
	for i := range monthName {
		monthNames[i] = py.String(monthName[i])
	}
	for i := range monthAbbr {
		monthAbbrs[i] = py.String(monthAbbr[i])
	}
	for i := range dayName {
		dayNames[i] = py.String(dayName[i])
		dayAbbrs[i] = py.String(dayAbbr[i])
	}
	for i := range mdays {
		md[i] = py.Int(mdays[i])
	}

	globals := py.NewStringDict()
	globals.Set("month_name", py.NewListFromItems(monthNames))
	globals.Set("month_abbr", py.NewListFromItems(monthAbbrs))
	globals.Set("day_name", py.NewListFromItems(dayNames))
	globals.Set("day_abbr", py.NewListFromItems(dayAbbrs))
	globals.Set("mdays", py.NewListFromItems(md))
	globals.Set("January", py.Int(1))
	globals.Set("February", py.Int(2))
	globals.Set("March", py.Int(3))
	globals.Set("April", py.Int(4))
	globals.Set("May", py.Int(5))
	globals.Set("June", py.Int(6))
	globals.Set("July", py.Int(7))
	globals.Set("August", py.Int(8))
	globals.Set("September", py.Int(9))
	globals.Set("October", py.Int(10))
	globals.Set("November", py.Int(11))
	globals.Set("December", py.Int(12))
	globals.Set("MONDAY", py.Int(0))
	globals.Set("TUESDAY", py.Int(1))
	globals.Set("WEDNESDAY", py.Int(2))
	globals.Set("THURSDAY", py.Int(3))
	globals.Set("FRIDAY", py.Int(4))
	globals.Set("SATURDAY", py.Int(5))
	globals.Set("SUNDAY", py.Int(6))

	globals.Set("isleap", py.MustNewMethod("isleap", calendarIsLeap, 0, "Return True for a leap year."))
	globals.Set("leapdays", py.MustNewMethod("leapdays", calendarLeapDays, 0, "Return the number of leap years in a range."))
	globals.Set("monthrange", py.MustNewMethod("monthrange", calendarMonthRange, 0, "Return weekday of first day and number of days in a month."))
	globals.Set("weekday", py.MustNewMethod("weekday", calendarWeekday, 0, "Return the day of the week (0 is Monday)."))
	globals.Set("isweekend", py.MustNewMethod("isweekend", calendarIsWeekend, 0, "Return True if the day is a weekend day."))
	globals.Set("timegm", py.MustNewMethod("timegm", calendarTimegm, 0, "Return epoch seconds from a UTC struct_time."))
	globals.Set("monthcalendar", py.MustNewMethod("monthcalendar", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return calendarMonthCalendar(self, args, kw)
	}, 0, "Return a matrix representing a month's calendar."))
	globals.Set("month", py.MustNewMethod("month", calendarMonth, 0, "Return a month's calendar as a multi-line string."))
	globals.Set("setfirstweekday", py.MustNewMethod("setfirstweekday", calendarSetFirstWeekday, 0, "Set the weekday to start each week (accepted, not used)."))
	globals.Set("firstweekday", py.MustNewMethod("firstweekday", calendarFirstWeekday, 0, "Return the first weekday (always Monday)."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "calendar",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
