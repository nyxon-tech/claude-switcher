package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

func layoutFor(t *testing.T, lang string) Layout {
	t.Helper()
	i18n.Load(lang)
	t.Cleanup(func() { i18n.Load("en") })
	return Layout{Mode: rtl.App, RTL: i18n.RTL()}
}

func TestLayoutPlacesAtReadingEdges(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#a8b2ff"))
	for _, tc := range []struct {
		lang                       string
		row, pad, end, inline, cut string
	}{
		{"en", "ab      cd", "ab   ", "   ab", "abc", "abcd…"},
		{"fa", "cd      ab", "   ab", "ab   ", "cba", "…ghij"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			l := layoutFor(t, tc.lang)
			checks := []struct{ name, got, want string }{
				{"Row", l.Row("ab", "cd", 10), tc.row},
				{"Row styled", ansi.Strip(l.Row(styled.Render("ab"), styled.Render("cd"), 10)), tc.row},
				{"Pad", l.Pad("ab", 5), tc.pad},
				{"End", l.End("ab", 5), tc.end},
				{"Inline", l.Inline("a", "b", "c"), tc.inline},
				{"Clip", l.Clip("abcdefghij", 5), tc.cut},
				{"Center", l.Center("ab", 6), "  ab  "},
				{"Row without room for trail", l.Row("abcdefgh", "xyz", 10), l.Pad("abcdefgh", 10)},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
				}
			}
		})
	}
}

// Persian text is prepared first and placed after: at the right edge in Persian, exact widths.
func TestLayoutPersianText(t *testing.T) {
	l := layoutFor(t, "fa")
	tab := l.Text(i18n.T("tab.accounts"))
	if w := ansi.StringWidth(tab); w != 6 {
		t.Fatalf("حساب‌ها takes %d cells, want 6 (ZWNJ has no width)", w)
	}
	row := l.Row(tab, "v3.0.0", 20)
	if ansi.StringWidth(row) != 20 || !strings.HasSuffix(row, tab) || !strings.HasPrefix(row, "v3.0.0") {
		t.Errorf("row = %q: want the tab at the right edge and the version at the left, 20 cells", row)
	}
	if got := l.Inline(l.Text("●"), " ", tab); got != tab+" ●" {
		t.Errorf("inline = %q: the dot leads, so it goes to the right", got)
	}
	if l.Pointer() != "‹" || l.BarGlyph() != "▐" {
		t.Errorf("pointer %q and bar %q should face the right edge", l.Pointer(), l.BarGlyph())
	}

	tagline := i18n.T("app.tagline")
	if w := ansi.StringWidth(l.Fit(tagline, 20)); w > 20 {
		t.Errorf("Fit(tagline, 20) is %d cells", w)
	}
	lines := l.Wrap(tagline, 20)
	if len(lines) < 2 {
		t.Fatalf("the tagline should wrap into several lines, got %q", lines)
	}
	for _, line := range lines {
		if w := ansi.StringWidth(line); w > 20 {
			t.Errorf("wrapped line %q is %d cells", line, w)
		}
	}
	// The first line of a wrapped Persian sentence starts it: its first word is at the right end.
	first := strings.Fields(tagline)[0]
	if !strings.HasSuffix(lines[0], l.Text(first)) {
		t.Errorf("first wrapped line %q should end with %q", lines[0], l.Text(first))
	}
}

func TestLayoutEnglish(t *testing.T) {
	l := layoutFor(t, "en")
	if l.Pointer() != "›" || l.BarGlyph() != "▌" {
		t.Errorf("pointer %q and bar %q should face the left edge", l.Pointer(), l.BarGlyph())
	}
	lines := l.Wrap("Switch Claude accounts without signing in again", 16)
	want := []string{"Switch Claude", "accounts without", "signing in again"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("Wrap = %q, want %q", lines, want)
	}
	block := l.Lines("one\ntwo is long\n", 6, 4)
	if len(block) != 4 {
		t.Fatalf("Lines gave %d rows, want 4", len(block))
	}
	for i, line := range block {
		if ansi.StringWidth(line) != 6 {
			t.Errorf("row %d = %q is not 6 cells", i, line)
		}
	}
	if block[1] != "two i…" {
		t.Errorf("a long row is clipped at its end: %q", block[1])
	}
}
