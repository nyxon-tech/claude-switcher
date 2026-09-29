package i18n

import (
	"strconv"
	"strings"
	"time"
)

// digits rewrites an ASCII number ("1,234.5", "12%") with the language's digits, separators
// and percent sign.
func (c *catalog) digits(s string) string {
	r := c.rules
	if r.zero == 0 || asciiDigits.Load() {
		return s
	}
	return strings.Map(func(ch rune) rune {
		switch {
		case ch >= '0' && ch <= '9':
			return r.zero + ch - '0'
		case ch == ',':
			return r.group
		case ch == '.':
			return r.decimal
		case ch == '%':
			return r.percent
		}
		return ch
	}, s)
}

func (c *catalog) n(v int64) string {
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
	return c.digits(b.String())
}

var compactUnits = []struct {
	size int64
	key  string
}{
	{1_000_000_000, "num.billion"},
	{1_000_000, "num.million"},
	{1_000, "num.thousand"},
}

func (c *catalog) compact(v int64) string {
	for _, u := range compactUnits {
		if v >= u.size {
			return c.t(u.key, "v", c.scaled(v, u.size))
		}
	}
	return c.n(v)
}

// scaled writes v/size with one decimal below 10 (1.2, 9.9) and none above (34, 999).
// It truncates, so a count never reads larger than it is.
func (c *catalog) scaled(v, size int64) string {
	if v >= 10*size {
		return c.n(v / size)
	}
	tenths := v / (size / 10)
	s := strconv.FormatInt(tenths/10, 10)
	if tenths%10 != 0 {
		s += "." + strconv.FormatInt(tenths%10, 10)
	}
	return c.digits(s)
}

func (c *catalog) ago(t, now time.Time) string {
	const day = 24 * time.Hour
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return c.t("time.just_now")
	case d < time.Hour:
		return c.t("time.minutes_ago", "n", int64(d/time.Minute))
	case d < day:
		return c.t("time.hours_ago", "n", int64(d/time.Hour))
	case d < 2*day:
		return c.t("time.yesterday")
	case d < 30*day:
		return c.t("time.days_ago", "n", int64(d/day))
	}
	return c.date(t)
}

func (c *catalog) date(t time.Time) string {
	y, m, d := t.Date()
	month := "month." + strconv.Itoa(int(m))
	if c.rules.jalali && !gregorian.Load() {
		if jy, jm, jd, ok := jalali(y, m, d); ok {
			y, d, month = jy, jd, "month.jalali."+strconv.Itoa(jm)
		}
	}
	return c.t("date.format",
		"day", c.digits(strconv.Itoa(d)),
		"month", c.t(month),
		"year", c.digits(strconv.Itoa(y)))
}
