package i18n

import (
	"testing"
	"time"
)

func TestT(t *testing.T) {
	c := catalog{
		"hello":       "Hello, {name}",
		"chats.one":   "{n} chat",
		"chats.other": "{n} chats",
		"messages":    "{n} messages",
	}
	tests := []struct {
		key  string
		args []any
		want string
	}{
		{"hello", []any{"name", "Sara"}, "Hello, Sara"},
		{"hello", []any{"name", "{n}", "n", 5}, "Hello, {n}"},
		{"chats", []any{"n", 1}, "1 chat"},
		{"chats", []any{"n", 0}, "0 chats"},
		{"chats", []any{"n", uint8(3)}, "3 chats"},
		{"chats", []any{"n", int64(1234)}, "1,234 chats"},
		{"chats", []any{"n", "1.2k"}, "1.2k chats"},
		{"chats", []any{"n", "1"}, "1 chat"},
		{"messages", []any{"n", 1}, "1 messages"}, // without forms, the one text
		{"missing.key", nil, "missing.key"},
	}
	for _, tt := range tests {
		if got := c.t(tt.key, tt.args...); got != tt.want {
			t.Errorf("T(%q, %v) = %q, want %q", tt.key, tt.args, got, tt.want)
		}
	}
	if got := T("count.chats", "n", 1); got != "1 chat" {
		t.Errorf("the catalog's count.chats = %q", got)
	}
}

func TestN(t *testing.T) {
	for v, want := range map[int64]string{0: "0", 999: "999", 1234: "1,234", 100000: "100,000", 1234567: "1,234,567", -1234: "-1,234"} {
		if got := N(v); got != want {
			t.Errorf("N(%d) = %q, want %q", v, got, want)
		}
	}
}

func TestCompact(t *testing.T) {
	for v, want := range map[int64]string{
		999: "999", 1000: "1k", 1234: "1.2k", 34_567: "34k", 999_999: "999k",
		1_234_567: "1.2M", 3_456_789_012: "3.4B", 1_234_000_000_000: "1,234B",
	} {
		if got := Compact(v); got != want {
			t.Errorf("Compact(%d) = %q, want %q", v, got, want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for ago, want := range map[time.Duration]string{
		-5 * time.Minute:    "just now",
		30 * time.Second:    "just now",
		5 * time.Minute:     "5m ago",
		3 * time.Hour:       "3h ago",
		30 * time.Hour:      "yesterday",
		4 * 24 * time.Hour:  "4d ago",
		45 * 24 * time.Hour: "Aug 15, 2026",
	} {
		if got := Ago(now.Add(-ago), now); got != want {
			t.Errorf("Ago(%v) = %q, want %q", ago, got, want)
		}
	}
}

func TestDate(t *testing.T) {
	for d, want := range map[time.Time]string{
		time.Date(2026, 9, 29, 23, 30, 0, 0, time.UTC): "Sep 29, 2026",
		time.Date(12026, 1, 1, 0, 0, 0, 0, time.UTC):   "Jan 1, 12026", // the year is not grouped
		{}: "Jan 1, 1",
	} {
		if got := Date(d); got != want {
			t.Errorf("Date(%v) = %q, want %q", d, got, want)
		}
	}
}
