package rtl

import (
	"strings"
	"unicode"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

const (
	zwnj    = 0x200C
	zwj     = 0x200D
	bom     = 0xFEFF
	tatweel = 0x0640
	lam     = 0x0644
)

// forms lists the presentation forms of each joining letter: isolated, final, initial, medial.
// Arabic letters map to Forms-B, Persian letters to Forms-A. Letters without initial and medial
// forms join only the letter before them (joining type R); hamza joins nothing (type U).
var forms = map[rune][4]rune{
	0x0621: {0xFE80},                         // ء
	0x0622: {0xFE81, 0xFE82},                 // آ
	0x0623: {0xFE83, 0xFE84},                 // أ
	0x0624: {0xFE85, 0xFE86},                 // ؤ
	0x0625: {0xFE87, 0xFE88},                 // إ
	0x0626: {0xFE89, 0xFE8A, 0xFE8B, 0xFE8C}, // ئ
	0x0627: {0xFE8D, 0xFE8E},                 // ا
	0x0628: {0xFE8F, 0xFE90, 0xFE91, 0xFE92}, // ب
	0x0629: {0xFE93, 0xFE94},                 // ة
	0x062A: {0xFE95, 0xFE96, 0xFE97, 0xFE98}, // ت
	0x062B: {0xFE99, 0xFE9A, 0xFE9B, 0xFE9C}, // ث
	0x062C: {0xFE9D, 0xFE9E, 0xFE9F, 0xFEA0}, // ج
	0x062D: {0xFEA1, 0xFEA2, 0xFEA3, 0xFEA4}, // ح
	0x062E: {0xFEA5, 0xFEA6, 0xFEA7, 0xFEA8}, // خ
	0x062F: {0xFEA9, 0xFEAA},                 // د
	0x0630: {0xFEAB, 0xFEAC},                 // ذ
	0x0631: {0xFEAD, 0xFEAE},                 // ر
	0x0632: {0xFEAF, 0xFEB0},                 // ز
	0x0633: {0xFEB1, 0xFEB2, 0xFEB3, 0xFEB4}, // س
	0x0634: {0xFEB5, 0xFEB6, 0xFEB7, 0xFEB8}, // ش
	0x0635: {0xFEB9, 0xFEBA, 0xFEBB, 0xFEBC}, // ص
	0x0636: {0xFEBD, 0xFEBE, 0xFEBF, 0xFEC0}, // ض
	0x0637: {0xFEC1, 0xFEC2, 0xFEC3, 0xFEC4}, // ط
	0x0638: {0xFEC5, 0xFEC6, 0xFEC7, 0xFEC8}, // ظ
	0x0639: {0xFEC9, 0xFECA, 0xFECB, 0xFECC}, // ع
	0x063A: {0xFECD, 0xFECE, 0xFECF, 0xFED0}, // غ
	0x0641: {0xFED1, 0xFED2, 0xFED3, 0xFED4}, // ف
	0x0642: {0xFED5, 0xFED6, 0xFED7, 0xFED8}, // ق
	0x0643: {0xFED9, 0xFEDA, 0xFEDB, 0xFEDC}, // ك
	0x0644: {0xFEDD, 0xFEDE, 0xFEDF, 0xFEE0}, // ل
	0x0645: {0xFEE1, 0xFEE2, 0xFEE3, 0xFEE4}, // م
	0x0646: {0xFEE5, 0xFEE6, 0xFEE7, 0xFEE8}, // ن
	0x0647: {0xFEE9, 0xFEEA, 0xFEEB, 0xFEEC}, // ه
	0x0648: {0xFEED, 0xFEEE},                 // و
	0x0649: {0xFEEF, 0xFEF0, 0xFBE8, 0xFBE9}, // ى
	0x064A: {0xFEF1, 0xFEF2, 0xFEF3, 0xFEF4}, // ي
	0x067E: {0xFB56, 0xFB57, 0xFB58, 0xFB59}, // پ
	0x0686: {0xFB7A, 0xFB7B, 0xFB7C, 0xFB7D}, // چ
	0x0698: {0xFB8A, 0xFB8B},                 // ژ
	0x06A9: {0xFB8E, 0xFB8F, 0xFB90, 0xFB91}, // ک
	0x06AF: {0xFB92, 0xFB93, 0xFB94, 0xFB95}, // گ
	0x06C0: {0xFBA4, 0xFBA5},                 // ۀ
	0x06CC: {0xFBFC, 0xFBFD, 0xFBFE, 0xFBFF}, // ی
}

// lamAlef holds the mandatory lam-alef ligatures (isolated, final) by the alef after lam.
var lamAlef = map[rune][2]rune{
	0x0622: {0xFEF5, 0xFEF6}, // لآ
	0x0623: {0xFEF7, 0xFEF8}, // لأ
	0x0625: {0xFEF9, 0xFEFA}, // لإ
	0x0627: {0xFEFB, 0xFEFC}, // لا
}

// joining is a joining type from Unicode's ArabicShaping.txt.
type joining uint8

const (
	nonJoining   joining = iota // U
	rightJoining                // R: joins only the letter before it
	dualJoining                 // D
	joinCausing                 // C: tatweel and ZWJ join both sides and keep their shape
	transparent                 // T: marks and format characters, skipped when joining
)

func joiningOf(r rune) joining {
	if f, ok := forms[r]; ok {
		switch {
		case f[2] != 0:
			return dualJoining
		case f[1] != 0:
			return rightJoining
		}
		return nonJoining
	}
	switch {
	case r == zwj || r == tatweel:
		return joinCausing
	case r == zwnj:
		return nonJoining
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return transparent
	}
	return nonJoining
}

// joins reports whether a letter of type a joins the letter of type b that follows it.
func joins(a, b joining) bool {
	return (a == dualJoining || a == joinCausing) &&
		(b == rightJoining || b == dualJoining || b == joinCausing)
}

// neighbour is the joining type of the nearest non-transparent rune from rs[i] in direction step.
func neighbour(rs []rune, i, step int) joining {
	for k := i + step; k >= 0 && k < len(rs); k += step {
		if j := joiningOf(rs[k]); j != transparent {
			return j
		}
	}
	return nonJoining
}

func isHarakah(r rune) bool { return r >= 0x064B && r <= 0x0652 || r == 0x0670 }

func needsShaping(r rune) bool {
	return r >= 0x0600 && r <= 0x06FF || r == zwnj || r == zwj || r == bom
}

// Shape replaces Arabic-script letters with their joined presentation forms (Persian letters
// included, lam-alef ligatures on), drops ZWNJ, ZWJ, BOM and harakat. Logical order in and out.
// A ZWJ that joins an emoji sequence stays.
func Shape(s string) string {
	if !strings.ContainsFunc(s, needsShaping) {
		return s
	}
	// Harakat go first so lam still meets alef in words like کاملاً; they would sit badly on
	// presentation-form glyphs anyway.
	rs := []rune(strings.Map(func(r rune) rune {
		if isHarakah(r) || r == bom {
			return -1
		}
		return r
	}, s))
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		f, ok := forms[r]
		if !ok {
			// ZWNJ has done its work on the neighbours' joining; drop it now.
			if r != zwnj {
				b.WriteRune(r)
			}
			continue
		}
		before := joins(neighbour(rs, i, -1), joiningOf(r))
		if r == lam && i+1 < len(rs) {
			if lig, ok := lamAlef[rs[i+1]]; ok {
				form := lig[0]
				if before {
					form = lig[1]
				}
				b.WriteRune(form)
				i++
				continue
			}
		}
		after := joins(joiningOf(r), neighbour(rs, i, 1))
		switch {
		case before && after:
			b.WriteRune(f[3])
		case before:
			b.WriteRune(f[1])
		case after:
			b.WriteRune(f[2])
		default:
			b.WriteRune(f[0])
		}
	}
	return dropLooseJoiners(b.String())
}

// dropLooseJoiners removes every ZWJ that ends a grapheme cluster: next to a letter it has done
// its work on the joining. A ZWJ inside a cluster joins an emoji sequence and stays; keeping only
// those also means reordering whole clusters can never glue two emoji together.
func dropLooseJoiners(s string) string {
	if !strings.ContainsRune(s, zwj) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for g := graphemes.FromString(s); g.Next(); {
		b.WriteString(strings.TrimRight(g.Value(), "\u200d"))
	}
	return b.String()
}
