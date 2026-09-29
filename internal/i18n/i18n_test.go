package i18n

import (
	"testing"
	"time"
)

// set switches the package state for one test and restores the defaults afterwards.
func set(t *testing.T, lang string, persianDigits, jalali bool) {
	t.Helper()
	Load(lang)
	SetPersianDigits(persianDigits)
	SetJalali(jalali)
	t.Cleanup(func() {
		Load("en")
		SetPersianDigits(true)
		SetJalali(true)
	})
}

func TestT(t *testing.T) {
	en := &catalog{lang: "en", text: map[string]string{
		"hello":       "Hello, {name}",
		"chats.one":   "{n} chat",
		"chats.other": "{n} chats",
		"only.en":     "English only",
	}}
	fa := &catalog{lang: "fa", rules: langRules["fa"], fallback: en, text: map[string]string{
		"hello":       "سلام {name}",
		"chats.one":   "یک گفتگو",
		"chats.other": "{n} گفتگو",
		"messages":    "{n} پیام",
	}}
	tests := []struct {
		c    *catalog
		key  string
		args []any
		want string
	}{
		{en, "hello", []any{"name", "Sara"}, "Hello, Sara"},
		{en, "chats", []any{"n", 1}, "1 chat"},
		{en, "chats", []any{"n", 0}, "0 chats"},
		{en, "chats", []any{"n", int64(1234)}, "1,234 chats"},
		{en, "chats", []any{"n", "1.2k"}, "1.2k chats"},
		{en, "chats", []any{"n", "1"}, "1 chat"},
		{en, "missing.key", nil, "missing.key"},
		{fa, "chats", []any{"n", 1}, "۱ گفتگو"},
		{fa, "chats", []any{"n", uint8(3)}, "۳ گفتگو"},
		{fa, "messages", []any{"n", 2}, "۲ پیام"},
		{fa, "chats", []any{"n", "۱٫۲ هزار"}, "۱٫۲ هزار گفتگو"},
		{fa, "only.en", nil, "English only"},
		{fa, "missing.key", nil, "missing.key"},
		{fa, "hello", []any{"name", "{n}", "n", 5}, "سلام {n}"},
	}
	for _, tt := range tests {
		if got := tt.c.t(tt.key, tt.args...); got != tt.want {
			t.Errorf("%s T(%q, %v) = %q, want %q", tt.c.lang, tt.key, tt.args, got, tt.want)
		}
	}
}

func TestLoad(t *testing.T) {
	set(t, "fa_IR.UTF-8", true, true)
	if Lang() != "fa" || !RTL() {
		t.Fatalf("Lang() = %q, RTL() = %v after Load(fa_IR.UTF-8)", Lang(), RTL())
	}
	if got := T("tab.chats"); got != "گفتگوها" {
		t.Errorf("fa tab.chats = %q", got)
	}
	if got := T("count.chats", "n", 5); got != "۵ گفتگو" {
		t.Errorf("fa count.chats = %q", got)
	}
	Load("de")
	if Lang() != "en" || RTL() {
		t.Errorf("Load(de) gives %q, want en", Lang())
	}
	if got := T("count.chats", "n", 1); got != "1 chat" {
		t.Errorf("en count.chats = %q", got)
	}
}

func TestN(t *testing.T) {
	tests := []struct {
		lang    string
		persian bool
		v       int64
		want    string
	}{
		{"en", true, 0, "0"},
		{"en", true, 999, "999"},
		{"en", true, 1234, "1,234"},
		{"en", true, 100000, "100,000"},
		{"en", true, 1234567, "1,234,567"},
		{"en", true, -1234, "-1,234"},
		{"fa", true, 0, "۰"},
		{"fa", true, 1234, "۱٬۲۳۴"},
		{"fa", true, 1234567, "۱٬۲۳۴٬۵۶۷"},
		{"fa", false, 1234, "1,234"},
	}
	for _, tt := range tests {
		set(t, tt.lang, tt.persian, true)
		if got := N(tt.v); got != tt.want {
			t.Errorf("%s N(%d) = %q, want %q", tt.lang, tt.v, got, tt.want)
		}
	}
}

func TestCompact(t *testing.T) {
	tests := []struct {
		lang    string
		persian bool
		v       int64
		want    string
	}{
		{"en", true, 999, "999"},
		{"en", true, 1000, "1k"},
		{"en", true, 1234, "1.2k"},
		{"en", true, 34_567, "34k"},
		{"en", true, 999_999, "999k"},
		{"en", true, 1_234_567, "1.2M"},
		{"en", true, 3_456_789_012, "3.4B"},
		{"en", true, 1_234_000_000_000, "1,234B"},
		{"fa", true, 999, "۹۹۹"},
		{"fa", true, 1234, "۱٫۲ هزار"},
		{"fa", true, 34_567, "۳۴ هزار"},
		{"fa", true, 1_234_567, "۱٫۲ میلیون"},
		{"fa", true, 3_456_789_012, "۳٫۴ میلیارد"},
		{"fa", false, 1234, "1.2 هزار"},
	}
	for _, tt := range tests {
		set(t, tt.lang, tt.persian, true)
		if got := Compact(tt.v); got != tt.want {
			t.Errorf("%s Compact(%d) = %q, want %q", tt.lang, tt.v, got, tt.want)
		}
	}
}

func TestDigits(t *testing.T) {
	tests := []struct {
		lang    string
		persian bool
		s, want string
	}{
		{"en", true, "1,234.5 (12%)", "1,234.5 (12%)"},
		{"fa", true, "1,234.5 (12%)", "۱٬۲۳۴٫۵ (۱۲٪)"},
		{"fa", false, "12.5%", "12.5%"},
	}
	for _, tt := range tests {
		set(t, tt.lang, tt.persian, true)
		if got := Digits(tt.s); got != tt.want {
			t.Errorf("%s Digits(%q) = %q, want %q", tt.lang, tt.s, got, tt.want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		ago    time.Duration
		en, fa string
	}{
		{-5 * time.Minute, "just now", "همین حالا"},
		{30 * time.Second, "just now", "همین حالا"},
		{5 * time.Minute, "5m ago", "۵ دقیقه پیش"},
		{3 * time.Hour, "3h ago", "۳ ساعت پیش"},
		{30 * time.Hour, "yesterday", "دیروز"},
		{4 * 24 * time.Hour, "4d ago", "۴ روز پیش"},
		{45 * 24 * time.Hour, "Aug 15, 2026", "۲۴ مرداد ۱۴۰۵"},
	}
	for _, tt := range tests {
		for lang, want := range map[string]string{"en": tt.en, "fa": tt.fa} {
			set(t, lang, true, true)
			if got := Ago(now.Add(-tt.ago), now); got != want {
				t.Errorf("%s Ago(%v) = %q, want %q", lang, tt.ago, got, want)
			}
		}
	}
}

func TestDate(t *testing.T) {
	day := time.Date(2026, 9, 29, 23, 30, 0, 0, time.UTC)
	tests := []struct {
		lang            string
		persian, jalali bool
		t               time.Time
		want            string
	}{
		{"en", true, true, day, "Sep 29, 2026"},
		{"fa", true, true, day, "۷ مهر ۱۴۰۵"},
		{"fa", true, false, day, "۲۹ سپتامبر ۲۰۲۶"},
		{"fa", false, true, day, "7 مهر 1405"},
		{"fa", true, true, time.Time{}, "۱ ژانویه ۱"}, // outside the Jalali range
	}
	for _, tt := range tests {
		set(t, tt.lang, tt.persian, tt.jalali)
		if got := Date(tt.t); got != tt.want {
			t.Errorf("%s Date(%v) = %q, want %q", tt.lang, tt.t, got, tt.want)
		}
	}
}

func TestJalali(t *testing.T) {
	tests := []struct {
		g          string
		jy, jm, jd int
	}{
		{"2026-03-21", 1405, 1, 1},
		{"2026-09-23", 1405, 7, 1},
		{"2026-09-29", 1405, 7, 7},
		{"2026-03-20", 1404, 12, 29}, // 1404 is not leap
		{"2025-03-21", 1404, 1, 1},
		{"2025-03-20", 1403, 12, 30}, // 1403 is leap
		{"2024-03-20", 1403, 1, 1},
		{"2021-03-20", 1399, 12, 30}, // 1399 is leap
		{"2021-03-21", 1400, 1, 1},
		{"2029-03-19", 1407, 12, 29}, // 1407 is not leap, four years after 1403
		{"2029-03-20", 1408, 1, 1},
		{"2030-03-20", 1408, 12, 30}, // 1408 is leap
		{"2030-03-21", 1409, 1, 1},
		{"1979-02-11", 1357, 11, 22},
	}
	for _, tt := range tests {
		g, _ := time.Parse(time.DateOnly, tt.g)
		jy, jm, jd, ok := jalali(g.Date())
		if !ok || jy != tt.jy || jm != tt.jm || jd != tt.jd {
			t.Errorf("jalali(%s) = %d/%d/%d %v, want %d/%d/%d", tt.g, jy, jm, jd, ok, tt.jy, tt.jm, tt.jd)
		}
	}
}

// Walking day by day, the Jalali date must step through valid month lengths without gaps.
func TestJalaliIsContinuous(t *testing.T) {
	day := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	py, pm, pd, _ := jalali(day.Date())
	for day.Year() < 2200 {
		day = day.AddDate(0, 0, 1)
		y, m, d, ok := jalali(day.Date())
		next := ok && (y == py && m == pm && d == pd+1 ||
			y == py && m == pm+1 && d == 1 && pd == monthEnd(pm) ||
			y == py+1 && m == 1 && d == 1 && pm == 12 && (pd == 29 || pd == 30))
		if !next {
			t.Fatalf("%s: %d/%d/%d follows %d/%d/%d", day.Format(time.DateOnly), y, m, d, py, pm, pd)
		}
		py, pm, pd = y, m, d
	}
}

func monthEnd(m int) int {
	if m <= 6 {
		return 31
	}
	return 30
}

func TestDetect(t *testing.T) {
	tests := []struct {
		switcher, lcAll, lcMessages, lang string
		want                              string
	}{
		{"", "", "", "fa_IR.UTF-8", "fa"},
		{"", "en_US.UTF-8", "", "fa_IR.UTF-8", "en"},
		{"", "", "fa_IR", "en_US.UTF-8", "fa"},
		{"FA-ir", "en_US.UTF-8", "", "", "fa"},
		{"", "", "", "C", "en"},
		{"", "", "", "de_DE.UTF-8", "en"},
	}
	for _, tt := range tests {
		t.Setenv("CLAUDE_SWITCHER_LANG", tt.switcher)
		t.Setenv("LC_ALL", tt.lcAll)
		t.Setenv("LC_MESSAGES", tt.lcMessages)
		t.Setenv("LANG", tt.lang)
		if got := Detect(); got != tt.want {
			t.Errorf("Detect() with %+v = %q, want %q", tt, got, tt.want)
		}
	}

	t.Setenv("LANG", "")
	if got := Detect(); got != "en" && got != "fa" {
		t.Errorf("Detect() from the OS = %q", got)
	}
}
