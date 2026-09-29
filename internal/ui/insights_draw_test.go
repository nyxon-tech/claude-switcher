package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// The chart and its columns stay inside any width, down to none at all.
func TestChartFitsWidth(t *testing.T) {
	for width := range 200 {
		cols, bar, gap := chartShape(30, width)
		values := perDay(map[string]transcript.Usage{}, now, 30, cols)
		if len(values) != cols {
			t.Fatalf("width %d: %d values for %d columns", width, len(values), cols)
		}
		for _, row := range columns(values, 8) {
			if w := ansi.StringWidth(widen(row, bar, gap)); w > width {
				t.Fatalf("width %d: a chart row is %d cells", width, w)
			}
		}
	}
	for _, tc := range []struct{ width, bar, gap int }{
		{58, 1, 0}, {59, 1, 1}, {78, 1, 1}, {98, 2, 1}, {118, 2, 2}, {138, 3, 1}, {198, 4, 2},
	} {
		if cols, bar, gap := chartShape(30, tc.width); cols != 30 || bar != tc.bar || gap != tc.gap {
			t.Errorf("%d cells give %d columns of %d, %d apart; want 30 of %d, %d apart", tc.width, cols, bar, gap, tc.bar, tc.gap)
		}
	}
}

func TestColumns(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []int64
		want   []string
	}{
		{"zero", []int64{0, 0, 0}, []string{"   ", "   "}},
		{"one day", []int64{0, 0, 500}, []string{"  █", "  █"}},
		{"levels", []int64{16, 8, 1, 0}, []string{"█   ", "██▁ "}},
		{"half a cell", []int64{16, 5}, []string{"█ ", "█▅"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := columns(tc.values, 2); !slices.Equal(got, tc.want) {
				t.Errorf("columns(%v) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// perDay puts each day in its column, today last, and folds days together when columns are few.
func TestPerDay(t *testing.T) {
	today := time.Date(2026, 9, 29, 12, 0, 0, 0, time.Local)
	byDay := map[string]transcript.Usage{
		"2026-09-29": {Output: 5}, "2026-09-28": {Output: 3}, "2026-08-31": {Output: 7}, "2026-08-30": {Output: 100},
	}
	days := perDay(byDay, today, 30, 30)
	if days[29] != 5 || days[28] != 3 || days[0] != 7 {
		t.Errorf("per day: %v", days)
	}
	if got := perDay(byDay, today, 30, 3); !slices.Equal(got, []int64{7, 0, 8}) {
		t.Errorf("in 3 columns: %v, want [7 0 8]", got)
	}
}

// A bar fills from the left by its share half a cell at a time, a share too small for half a cell
// still gets one, and it never grows past its width, in colour and in the plain theme alike.
func TestMeter(t *testing.T) {
	for _, theme := range []string{"dark", "plain"} {
		st := newStyles(theme, true)
		for _, tc := range []struct {
			frac  float64
			width int
			want  string
		}{
			{0, 4, "────"}, {0.001, 4, "╸───"}, {0.25, 12, "━━━─────────"}, {1, 4, "━━━━"},
			{1.5, 4, "━━━━"}, {-1, 4, "────"}, {0.5, 0, ""}, {0.5, 1, "╸"},
			{0.3, 4, "━───"}, {0.4, 4, "━╸──"},
		} {
			if got := ansi.Strip(meter(st, tc.frac, tc.width)); got != tc.want {
				t.Errorf("%s: meter(%v, %d) = %q, want %q", theme, tc.frac, tc.width, got, tc.want)
			}
		}
	}
}

func TestShortModel(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5":              "opus 5",
		"claude-opus-4-1-20250805":   "opus 4.1",
		"claude-sonnet-4-5-20250929": "sonnet 4.5",
		"claude-3-5-sonnet-20241022": "sonnet 3.5",
		"claude-3-7-sonnet-latest":   "sonnet 3.7 latest",
		"claude-haiku-4-5":           "haiku 4.5",
		"gpt-4o":                     "gpt-4o",
		"claude-20250101":            "claude-20250101",
	} {
		if got := shortModel(in); got != want {
			t.Errorf("shortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFolder(t *testing.T) {
	for in, want := range map[string]string{
		"/work/api": "api", `C:\Users\me\site\`: "site", "notes": "notes", "": "",
	} {
		if got := folder(in); got != want {
			t.Errorf("folder(%q) = %q, want %q", in, got, want)
		}
	}
}

// Scrolling stops at both ends and pages by the height shown.
func TestScroll(t *testing.T) {
	lines := strings.Split(strings.Repeat("x\n", 29)+"x", "\n")
	var s scroll
	if got := len(s.window(lines, 10)); got != 10 || s.max != 20 {
		t.Fatalf("window shows %d lines with max %d, want 10 and 20", got, s.max)
	}
	s.key("up")
	if s.top != 0 {
		t.Errorf("up at the top moved to %d", s.top)
	}
	s.key("pgdown")
	s.key("pgdown")
	s.key("pgdown")
	if s.top != 20 {
		t.Errorf("three pages down reached %d, want the end (20)", s.top)
	}
	if s.key("x"); s.top != 20 {
		t.Errorf("x is not a scrolling key, yet it moved to %d", s.top)
	}
}
