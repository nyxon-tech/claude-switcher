// Package rtl makes Persian (and Arabic) readable in terminals, such as Windows Terminal, conhost
// and VS Code, that neither join Arabic-script letters nor lay out right-to-left text. In Mode
// App a line is first shaped in logical order: letters become their joined presentation forms,
// lam-alef one ligature, and ZWNJ, ZWJ, BOM and harakat are dropped once joining is decided.
// Then the Unicode Bidirectional Algorithm (UAX #9) gives every character a level, and
// right-to-left runs are reversed by grapheme cluster with brackets mirrored: cells left to right.
// Terminals that do both themselves (Mode Terminal) get the text unchanged; Detect picks the mode.
// Every function takes plain text (no ANSI escape codes) holding a single line.
package rtl

import (
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/graphemes"
	"golang.org/x/text/unicode/bidi"
)

// Mode says who lays out right-to-left text.
type Mode int

const (
	Auto     Mode = iota // decide from the terminal (see Detect)
	App                  // this package joins and reorders
	Terminal             // the terminal does it; text passes through unchanged
	Off                  // never touch text
)

// Dir is the base direction of a line of text.
type Dir int

const (
	DirAuto Dir = iota // from the first strong character (UAX #9 rules P2-P3)
	LTR
	RTL
)

const ellipsis = "…"

// HasRTL reports whether s holds any right-to-left letter.
func HasRTL(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		c := classOf(r)
		return c == bidi.R || c == bidi.AL
	})
}

// Visual shapes s and returns it in visual order for a terminal without BiDi: the Unicode
// Bidirectional Algorithm (UAX #9) with base direction base, right-to-left runs reversed by
// grapheme cluster and brackets mirrored.
func Visual(s string, base Dir) string {
	s = Shape(s)
	rs := []rune(s)
	classes := make([]bidi.Class, len(rs))
	reorders := false
	for i, r := range rs {
		c := classOf(r)
		classes[i] = c
		reorders = reorders || c == bidi.R || c == bidi.AL || c == bidi.AN
	}
	para := paragraphLevel(classes, base)
	if para == 0 && !reorders {
		return s
	}
	return reorder(string(rs), resolveLevels(rs, classes, para))
}

// Line prepares one line of text for display in mode m: Visual for App (and Auto), unchanged
// for Terminal and Off.
func (m Mode) Line(s string, base Dir) string {
	if m == Terminal || m == Off {
		return s
	}
	return Visual(s, base)
}

// Width is how many terminal cells Line(s, base) takes.
func (m Mode) Width(s string) int {
	if m == Terminal || m == Off {
		return m.cells(s)
	}
	// Reordering moves whole grapheme clusters, so the shaped text is as wide as the visual
	// one, whatever the base direction.
	return ansi.StringWidth(Shape(s))
}

// Fit shortens s at its logical end with "…" so that its displayed form fits width cells, then
// returns Line of it. Text that already fits is only passed through Line.
func (m Mode) Fit(s string, width int, base Dir) string {
	if width <= 0 {
		return ""
	}
	if line := m.Line(s, base); m.cells(line) <= width {
		return line
	}
	// Cut the logical text between grapheme clusters and shape afterwards, so the last kept
	// letter takes its final form.
	cuts := []int{0}
	for g := graphemes.FromString(s); g.Next(); {
		if g.End() < len(s) {
			cuts = append(cuts, g.End())
		}
	}
	// Search returns the first cut that no longer fits, having seen the one before it fit.
	// Measuring the prepared line, not Width, keeps even text whose width changes when
	// reordered (a lone prepended sign at the end) inside width.
	n := sort.Search(len(cuts), func(i int) bool {
		return m.cells(m.Line(s[:cuts[i]]+ellipsis, base)) > width
	})
	if n == 0 {
		return ""
	}
	return m.Line(s[:cuts[n-1]]+ellipsis, base)
}

// cells is the width of a line that Line has prepared. A terminal that joins letters itself
// still gives every rune its own cells (so lam and alef take two even when drawn as one
// ligature), and marks and format characters none. Like ansi.StringWidth, and unlike
// go-runewidth's default, this ignores the locale.
func (m Mode) cells(line string) int {
	if m == Terminal || m == Off {
		return ansi.StringWidthWc(line)
	}
	return ansi.StringWidth(line)
}

// Fold prepares s for search matching: lower case, Arabic yeh and kaf as their Persian forms,
// no ZWNJ, ZWJ, tatweel or harakat, and Persian and Arabic-Indic digits as ASCII digits.
func Fold(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 0x064A: // ي
			return 0x06CC // ی
		case r == 0x0643: // ك
			return 0x06A9 // ک
		case r == zwnj || r == zwj || r == tatweel || isHarakah(r):
			return -1
		case r >= 0x06F0 && r <= 0x06F9:
			return '0' + r - 0x06F0
		case r >= 0x0660 && r <= 0x0669:
			return '0' + r - 0x0660
		}
		return unicode.ToLower(r)
	}, s)
}
