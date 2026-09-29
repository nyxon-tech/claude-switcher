package i18n

import (
	"strconv"
	"strings"
	"time"
)

// N writes an integer with its thousands grouped: 1,234.
func N(v int64) string {
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	if v < 0 {
		b.WriteByte('-')
		s = s[1:]
	}
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var compactUnits = []struct {
	size int64
	key  string
}{
	{1_000_000_000, "num.billion"},
	{1_000_000, "num.million"},
	{1_000, "num.thousand"},
}

// Compact writes a large count briefly: 1.2M, 34k.
func Compact(v int64) string {
	for _, u := range compactUnits {
		if v >= u.size {
			return T(u.key, "v", scaled(v, u.size))
		}
	}
	return N(v)
}

// scaled writes v/size with one decimal below 10 (1.2, 9.9) and none above (34, 999).
// It truncates, so a count never reads larger than it is.
func scaled(v, size int64) string {
	if v >= 10*size {
		return N(v / size)
	}
	tenths := v / (size / 10)
	s := strconv.FormatInt(tenths/10, 10)
	if tenths%10 != 0 {
		s += "." + strconv.FormatInt(tenths%10, 10)
	}
	return s
}

// Ago is a relative time such as "5m ago"; older than 30 days gives Date(t).
func Ago(t, now time.Time) string {
	const day = 24 * time.Hour
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return T("time.just_now")
	case d < time.Hour:
		return T("time.minutes_ago", "n", int64(d/time.Minute))
	case d < day:
		return T("time.hours_ago", "n", int64(d/time.Hour))
	case d < 2*day:
		return T("time.yesterday")
	case d < 30*day:
		return T("time.days_ago", "n", int64(d/day))
	}
	return Date(t)
}

// Date is t's calendar date, such as "Sep 29, 2026". It uses t's own location, so pass t.Local()
// for the user's day. The day and the year go in as text, so T does not group the year's digits.
func Date(t time.Time) string {
	y, m, d := t.Date()
	return T("date.format", "day", strconv.Itoa(d), "month", T("month."+strconv.Itoa(int(m))), "year", strconv.Itoa(y))
}
