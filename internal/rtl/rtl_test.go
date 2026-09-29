package rtl

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/graphemes"
)

// cps builds a string from space-separated hex code points, as rtl.md writes them.
func cps(hex string) string {
	var b strings.Builder
	for _, f := range strings.Fields(hex) {
		n, err := strconv.ParseUint(f, 16, 32)
		if err != nil {
			panic(err)
		}
		b.WriteRune(rune(n))
	}
	return b.String()
}

func hexOf(s string) string {
	var parts []string
	for _, r := range s {
		parts = append(parts, fmt.Sprintf("%04X", r))
	}
	return strings.Join(parts, " ")
}

// golden are the vectors of rtl.md section 8 table A, produced by the Rust unicode-bidi and
// ar-reshaper pipeline. That pipeline keeps harakat in vectors 11 and 14; Shape deletes them,
// so their expected output is the Rust output with 064B, 064E, 064F and 0651 taken out.
var golden = []struct {
	name string
	in   string
	base Dir
	want string
}{
	{"1 greeting", cps("0633 0644 0627 0645 0020 062F 0646 06CC 0627"), RTL,
		cps("FE8E FBFF FEE7 FEA9 0020 FEE1 FEFC FEB3")},
	{"2 zwnj breaks joining", cps("067E 0631 0648 0698 0647 200C 0647 0627 06CC 0020 0645 0646"), RTL,
		cps("FEE6 FEE3 0020 FBFC FE8E FEEB FEE9 FB8A FEED FEAE FB58")},
	{"2b no zwnj joins", cps("067E 0631 0648 0698 0647 0647 0627 06CC 0020 0645 0646"), RTL,
		cps("FEE6 FEE3 0020 FBFC FE8E FEEC FEEB FB8A FEED FEAE FB58")},
	{"3 zwnj inside a word", cps("06AF 0641 062A 200C 0648 06AF 0648"), RTL,
		cps("FEEE FB94 FEED FE96 FED4 FB94")},
	{"4 lam-alef", cps("06A9 0644 0627 0633 0020 0644 0627 0644 0647"), RTL,
		cps("FEEA FEDF FEFB 0020 FEB1 FEFC FB90")},
	{"5 english inside persian", "Claude Code را باز کن", RTL,
		cps("FEE6 FB90 0020 FEAF FE8E FE91 0020 FE8D FEAD 0020") + "Claude Code"},
	{"6 persian version number", "نسخه ۲.۰.۳ منتشر شد", RTL,
		cps("FEAA FEB7 0020 FEAE FEB8 FE98 FEE8 FEE3 0020 06F2 002E 06F0 002E 06F3 0020 FEEA FEA8 FEB4 FEE7")},
	{"7 mirrored brackets", "۵۴ گفتگو (کار)", RTL,
		cps("0028 FEAD FE8E FB90 0029 0020 FEEE FB95 FE98 FED4 FB94 0020 06F5 06F4")},
	{"8 path", `مسیر C:\Users\pamir`, RTL,
		`C:\Users\pamir` + cps("0020 FEAE FBFF FEB4 FEE3")},
	{"9 auto picks ltr", "Chat: بررسی کد (v2)", DirAuto,
		"Chat: " + cps("FEAA FB90 0020 FBFD FEB3 FEAD FEAE FE91 0020") + "(v2)"},
	{"10 auto picks rtl", "سلام Hello دنیا", DirAuto,
		cps("FE8E FBFF FEE7 FEA9 0020") + "Hello" + cps("0020 FEE1 FEFC FEB3")},
	{"11 harakat dropped", cps("0645 064F 062D 064E 0645 064E 0651 062F"), RTL,
		cps("FEAA FEE4 FEA4 FEE3")},
	{"12 persian letters", "چای ژله گرگ کیک", RTL,
		cps("FB8F FBFF FB90 0020 FB92 FEAE FB94 0020 FEEA FEDF FB8A 0020 FBFC FE8E FB7C")},
	{"13 persian separators", "قیمت ۱۲٬۳۴۵ تومان", RTL,
		cps("FEE5 FE8E FEE3 FEEE FE97 0020 06F1 06F2 066C 06F3 06F4 06F5 0020 FE96 FEE4 FBFF FED7")},
	{"14 harakat dropped from lam-alef", cps("06A9 0627 0645 0644 0627 064B"), RTL,
		cps("FEFC FEE3 FE8E FB90")},
}

func TestVisualGolden(t *testing.T) {
	for _, v := range golden {
		t.Run(v.name, func(t *testing.T) {
			if got := Visual(v.in, v.base); got != v.want {
				t.Errorf("Visual = %s\n           want %s", hexOf(got), hexOf(v.want))
			}
		})
	}
}

func TestVisual(t *testing.T) {
	tests := []struct {
		name string
		in   string
		base Dir
		want string
	}{
		{"english unchanged", "Hello, world (v2) [x] {y} <z>!", DirAuto, "Hello, world (v2) [x] {y} <z>!"},
		{"english unchanged ltr", `C:\Users\pamir 1.2.3`, LTR, `C:\Users\pamir 1.2.3`},
		{"empty", "", RTL, ""},
		{"number after persian in an ltr line", "Title: گفتگو ۵۴", LTR,
			"Title: " + cps("06F5 06F4 0020 FEEE FB95 FE98 FED4 FB94")},
		// ٢٬٣ is at level 2 inside the level-0 run of x: runs split by direction lose that.
		{"level 2 inside a level 0 run", "x٢٬٣ بب", LTR, "x" + cps("FE90 FE91 0020 0662 066C 0663")},
		{"emoji zwj sequence kept", cps("0633 0644 0627 0645 0020 1F469 200D 1F4BB"), RTL,
			cps("1F469 200D 1F4BB 0020 FEE1 FEFC FEB3")},
		{"ascii numbers stay left to right", "فصل 12 از 30", RTL,
			"30 " + cps("FEAF FE8D") + " 12 " + cps("FEDE FEBC FED3")},
		{"guillemets mirrored", "«سلام»", RTL, "«" + cps("FEE1 FEFC FEB3") + "»"},
		{"mark stays after its base", cps("0628 0654 062A"), RTL, cps("FE96 FE91 0654")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Visual(tt.in, tt.base); got != tt.want {
				t.Errorf("Visual = %s\n           want %s", hexOf(got), hexOf(tt.want))
			}
		})
	}
}

func TestHasRTL(t *testing.T) {
	for s, want := range map[string]bool{"Hello 123": false, "": false, "a سلام": true, "۱۲۳": false, "שלום": true} {
		if got := HasRTL(s); got != want {
			t.Errorf("HasRTL(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestLine(t *testing.T) {
	in := "سلام دنیا"
	for _, m := range []Mode{Terminal, Off} {
		if got := m.Line(in, RTL); got != in {
			t.Errorf("%v.Line changed the text: %s", m, hexOf(got))
		}
	}
	for _, m := range []Mode{App, Auto} {
		if got, want := m.Line(in, RTL), Visual(in, RTL); got != want {
			t.Errorf("%v.Line = %s, want %s", m, hexOf(got), hexOf(want))
		}
	}
}

func TestWidth(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		app, terminal int
	}{
		{"english", "Hello", 5, 5},
		{"lam-alef is one cell when shaped, two in the terminal", "کلاس لاله", 7, 9},
		{"zwnj takes no cell", "پروژه" + cps("200C") + "های من", 11, 11},
		{"harakat take no cell", "مُحَمَّد", 4, 4},
		{"wide", "日本", 4, 4},
		{"arabic letter mark takes no cell", cps("061C 0633"), 1, 1},
		{"ambiguous width is narrow", "«…»", 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := App.Width(tt.in); got != tt.app {
				t.Errorf("App.Width = %d, want %d", got, tt.app)
			}
			if got := Terminal.Width(tt.in); got != tt.terminal {
				t.Errorf("Terminal.Width = %d, want %d", got, tt.terminal)
			}
			if got := Off.Width(tt.in); got != tt.terminal {
				t.Errorf("Off.Width = %d, want %d", got, tt.terminal)
			}
		})
	}
}

func TestFit(t *testing.T) {
	tests := []struct {
		name  string
		mode  Mode
		in    string
		width int
		base  Dir
		want  string
	}{
		{"zero width", App, "Hello", 0, LTR, ""},
		{"negative width", App, "Hello", -3, LTR, ""},
		{"fits", App, "Hello", 5, LTR, "Hello"},
		{"english", App, "Hello world", 8, LTR, "Hello w…"},
		{"only the ellipsis fits", App, "Hello", 1, LTR, "…"},
		{"fits after shaping", App, "کلاس لاله", 7, RTL, cps("FEEA FEDF FEFB 0020 FEB1 FEFC FB90")},
		// ف keeps its final form (FED2) instead of the medial one it has in the full word.
		{"last letter takes final form", App, "گفتگو", 3, RTL, cps("2026 FED2 FB94")},
		{"ellipsis at the logical end", App, "سلام دنیا", 6, RTL, cps("2026 FEA9 0020 FEE1 FEFC FEB3")},
		{"terminal cuts logical text", Terminal, "سلام دنیا", 6, RTL, "سلام …"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.Fit(tt.in, tt.width, tt.base); got != tt.want {
				t.Errorf("Fit = %s (%q)\n     want %s", hexOf(got), got, hexOf(tt.want))
			}
		})
	}
}

func TestFold(t *testing.T) {
	in := "كتاب ي ۱۲۳ ٤٥ ABC می" + cps("200C") + "خواهم مُحَمَّد بـزرگ"
	want := "کتاب ی 123 45 abc میخواهم محمد بزرگ"
	if got := Fold(in); got != want {
		t.Errorf("Fold = %q, want %q", got, want)
	}
}

// FuzzVisual checks that Visual never panics, that Width predicts the width of App output, and
// that Fit never overflows. Run it with: go test -fuzz FuzzVisual ./internal/rtl
func FuzzVisual(f *testing.F) {
	for _, v := range golden {
		f.Add(v.in)
	}
	f.Add(cps("1F44D 1F44D 200D 0020 0628 200D 1F469 200D 1F4BB"))
	f.Fuzz(func(t *testing.T, s string) {
		for _, base := range []Dir{DirAuto, LTR, RTL} {
			line := App.Line(s, base)
			// Invalid UTF-8 and control characters are outside the plain-text contract.
			if !utf8.ValidString(s) || strings.ContainsFunc(s, unicode.IsControl) {
				continue
			}
			if w, got := App.Width(s), ansi.StringWidth(line); w != got && !joinsAtEdge(Shape(s)) {
				t.Fatalf("base %d: App.Width = %d, App.Line is %d cells: %s", base, w, got, hexOf(s))
			}
			for _, width := range []int{1, 4, 12} {
				if got := ansi.StringWidth(App.Fit(s, width, base)); got > width {
					t.Fatalf("base %d: App.Fit(%d) is %d cells: %s", base, width, got, hexOf(s))
				}
				if got := ansi.StringWidthWc(Terminal.Fit(s, width, base)); got > width {
					t.Fatalf("base %d: Terminal.Fit(%d) is %d cells: %s", base, width, got, hexOf(s))
				}
			}
		}
	})
}

// joinsAtEdge reports whether s starts with a mark or ends with a prepended sign such as
// U+0600. Such a cluster merges with whatever reordering puts beside it, changing the width;
// titles and UI text never hold one.
func joinsAtEdge(s string) bool {
	last := ""
	for g := graphemes.FromString(s + "a"); g.Next(); {
		last = g.Value()
	}
	return graphemes.FromString("a"+s).First() != "a" || last != "a"
}
