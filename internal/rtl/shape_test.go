package rtl

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

// TestShapePositions puts every letter alone, after beh, before beh and between two behs.
// Presentation forms of a letter are consecutive: isolated, final, initial, medial (rtl.md
// section 9), except the initial and medial forms of alef maksura.
func TestShapePositions(t *testing.T) {
	const beh = "ب"
	behIsol, behFina, behInit := rune(0xFE8F), rune(0xFE90), rune(0xFE91)
	letters := []struct {
		letter string
		isol   rune
		join   byte // D, R or U
	}{
		{"ا", 0xFE8D, 'R'}, {"آ", 0xFE81, 'R'}, {"أ", 0xFE83, 'R'}, {"إ", 0xFE87, 'R'}, {"ؤ", 0xFE85, 'R'},
		{"ء", 0xFE80, 'U'}, {"ئ", 0xFE89, 'D'}, {"ب", 0xFE8F, 'D'}, {"پ", 0xFB56, 'D'}, {"ت", 0xFE95, 'D'},
		{"ث", 0xFE99, 'D'}, {"ج", 0xFE9D, 'D'}, {"چ", 0xFB7A, 'D'}, {"ح", 0xFEA1, 'D'}, {"خ", 0xFEA5, 'D'},
		{"د", 0xFEA9, 'R'}, {"ذ", 0xFEAB, 'R'}, {"ر", 0xFEAD, 'R'}, {"ز", 0xFEAF, 'R'}, {"ژ", 0xFB8A, 'R'},
		{"س", 0xFEB1, 'D'}, {"ش", 0xFEB5, 'D'}, {"ص", 0xFEB9, 'D'}, {"ض", 0xFEBD, 'D'}, {"ط", 0xFEC1, 'D'},
		{"ظ", 0xFEC5, 'D'}, {"ع", 0xFEC9, 'D'}, {"غ", 0xFECD, 'D'}, {"ف", 0xFED1, 'D'}, {"ق", 0xFED5, 'D'},
		{"ک", 0xFB8E, 'D'}, {"ك", 0xFED9, 'D'}, {"گ", 0xFB92, 'D'}, {"ل", 0xFEDD, 'D'}, {"م", 0xFEE1, 'D'},
		{"ن", 0xFEE5, 'D'}, {"و", 0xFEED, 'R'}, {"ه", 0xFEE9, 'D'}, {"ۀ", 0xFBA4, 'R'}, {"ة", 0xFE93, 'R'},
		{"ی", 0xFBFC, 'D'}, {"ي", 0xFEF1, 'D'}, {"ى", 0xFEEF, 'D'},
	}
	for _, l := range letters {
		isol, fina, init, medi := l.isol, l.isol+1, l.isol+2, l.isol+3
		if l.letter == "ى" {
			init, medi = 0xFBE8, 0xFBE9
		}
		var want [4][]rune // alone, after beh, before beh, between behs
		switch l.join {
		case 'D':
			want = [4][]rune{{isol}, {behInit, fina}, {init, behFina}, {behInit, medi, behFina}}
		case 'R':
			want = [4][]rune{{isol}, {behInit, fina}, {isol, behIsol}, {behInit, fina, behIsol}}
		case 'U':
			want = [4][]rune{{isol}, {behIsol, isol}, {isol, behIsol}, {behIsol, isol, behIsol}}
		}
		inputs := [4]string{l.letter, beh + l.letter, l.letter + beh, beh + l.letter + beh}
		for i, in := range inputs {
			if got := Shape(in); got != string(want[i]) {
				t.Errorf("Shape(%s) = %s, want %s", hexOf(in), hexOf(got), hexOf(string(want[i])))
			}
		}
	}
}

// TestFormsDecompose checks every table entry against Unicode: each presentation form
// decomposes (NFKD) to the letter it stands for.
func TestFormsDecompose(t *testing.T) {
	for letter, fs := range forms {
		for _, f := range fs {
			if f != 0 && norm.NFKD.String(string(f)) != norm.NFKD.String(string(letter)) {
				t.Errorf("%04X is not a form of %04X", f, letter)
			}
		}
	}
	for alef, ligs := range lamAlef {
		for _, lig := range ligs {
			if got := norm.NFKD.String(string(lig)); got != norm.NFKD.String(string([]rune{lam, alef})) {
				t.Errorf("%04X is not lam with %04X", lig, alef)
			}
		}
	}
}

func TestShapeJoiners(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"zwj forces joining after", cps("0628 200D"), cps("FE91")},
		{"zwj forces joining before", cps("200D 0628"), cps("FE90")},
		{"zwj between letters goes", cps("0628 200D 200D 0628"), cps("FE91 FE90")},
		{"emoji zwj sequence stays", cps("0628 0020 1F469 200D 1F4BB"), cps("FE8F 0020 1F469 200D 1F4BB")},
		{"zwj after an emoji alone goes", cps("1F44D 200D 0628"), cps("1F44D FE90")},
		{"tatweel joins and stays", cps("0628 0640 0628"), cps("FE91 0640 FE90")},
		{"zwnj breaks joining and goes", cps("0628 200C 0628"), cps("FE8F FE8F")},
		{"bom goes", cps("FEFF 0628"), cps("FE8F")},
		{"marks are transparent", cps("0628 0654 0628"), cps("FE91 0654 FE90")},
		{"lam-alef after a joining letter is final", cps("0628 0644 0622"), cps("FE91 FEF6")},
		{"lam-alef alone is isolated", cps("0644 0623 0020 0644 0625"), cps("FEF7 0020 FEF9")},
		{"letter after lam-alef starts again", cps("0644 0627 0628"), cps("FEFB FE8F")},
		{"latin unchanged", "abc 123", "abc 123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Shape(tt.in); got != tt.want {
				t.Errorf("Shape = %s, want %s", hexOf(got), hexOf(tt.want))
			}
		})
	}
}
