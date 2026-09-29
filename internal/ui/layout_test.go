package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

func TestLayoutPlaces(t *testing.T) {
	l := Layout{Mode: rtl.App}
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#a8b2ff"))
	for _, c := range []struct{ name, got, want string }{
		{"Row", l.Row("ab", "cd", 10), "ab      cd"},
		{"Row styled", ansi.Strip(l.Row(styled.Render("ab"), styled.Render("cd"), 10)), "ab      cd"},
		{"Row without room for trail", l.Row("abcdefgh", "xyz", 10), "abcdefgh  "},
		{"Pad", l.Pad("ab", 5), "ab   "},
		{"End", l.End("ab", 5), "   ab"},
		{"Clip", l.Clip("abcdefghij", 5), "abcd…"},
		{"Center", l.Center("ab", 6), "  ab  "},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestLayoutWrapAndLines(t *testing.T) {
	l := Layout{Mode: rtl.App}
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

// User data takes its direction from its first letter, so a Persian title reads right to left;
// interface text reads left to right, a Persian name in it laid out in place. Left to the
// terminal, both pass through as typed.
func TestLayoutUserData(t *testing.T) {
	word := rtl.Visual("کار", rtl.DirAuto) // joined, its letters right to left
	app, term := Layout{Mode: rtl.App}, Layout{Mode: rtl.Terminal}
	for _, c := range []struct{ name, got, want string }{
		{"Data", app.Data("کار work", 20), "work " + word},
		{"Text", app.Text("Switched to کار"), "Switched to " + word},
		{"Data in the terminal", term.Data("کار work", 20), "کار work"},
		{"Text in the terminal", term.Text("Switched to کار"), "Switched to کار"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
