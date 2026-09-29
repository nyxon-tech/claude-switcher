package rtl

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
	"golang.org/x/text/unicode/bidi"
)

// This file implements the Unicode Bidirectional Algorithm (UAX #9) for one paragraph without
// explicit embeddings: rules P2-P3, W1-W7, N0-N2, I1-I2 and L1-L2, plus L4 for brackets.
// go-text/typesetting/bidi resolves the same levels but only exports runs split by direction,
// each with the level of its first character, and L2 needs every level: in the left-to-right
// line "x٢٬٣ بب" the Arabic number is at level 2 inside the level-0 run of x, yet must move to
// the right of the Arabic word after it. go-text and the Unicode conformance file it ships serve
// as oracles in the tests instead.

// mirrors are the characters rule L4 swaps inside right-to-left runs.
var mirrors = map[rune]rune{
	'(': ')', ')': '(', '[': ']', ']': '[', '{': '}', '}': '{',
	'<': '>', '>': '<', '«': '»', '»': '«', '‹': '›', '›': '‹',
}

func classOf(r rune) bidi.Class {
	p, _ := bidi.LookupRune(r)
	if c := p.Class(); c < bidi.Control {
		return c
	}
	// Explicit embeddings and isolates are not supported; they are skipped like BN.
	return bidi.BN
}

// paragraphLevel applies rules P2-P3 when base is DirAuto: the first strong letter decides.
func paragraphLevel(classes []bidi.Class, base Dir) uint8 {
	switch base {
	case LTR:
		return 0
	case RTL:
		return 1
	}
	for _, c := range classes {
		switch c {
		case bidi.L:
			return 0
		case bidi.R, bidi.AL:
			return 1
		}
	}
	return 0
}

// resolveLevels returns the embedding level of every rune after rules W1-W7, N0-N2, I1-I2
// and L1. Runes that rule X9 removes (BN) take the level of the rune before them.
func resolveLevels(rs []rune, classes []bidi.Class, para uint8) []uint8 {
	var seq []int // indexes of the runes X9 keeps
	for i, c := range classes {
		if c != bidi.BN {
			seq = append(seq, i)
		}
	}
	t := make([]bidi.Class, len(seq))
	for k, i := range seq {
		t[k] = classes[i]
	}
	e := bidi.L // embedding direction; with one level it is also sos and eos
	if para == 1 {
		e = bidi.R
	}
	resolveWeak(t, e)
	resolveBrackets(t, rs, seq, classes, e)
	resolveNeutrals(t, e)

	levels := make([]uint8, len(rs))
	lvl, k := para, 0
	for i := range levels {
		if k < len(seq) && seq[k] == i {
			lvl = implicitLevel(t[k], para)
			k++
		}
		levels[i] = lvl
	}
	resetWhitespace(levels, classes, para)
	return levels
}

// resolveWeak applies rules W1-W7.
func resolveWeak(t []bidi.Class, sos bidi.Class) {
	prev := sos
	for k, c := range t { // W1
		if c == bidi.NSM {
			t[k] = prev
		}
		prev = t[k]
	}
	strong := sos
	for k, c := range t { // W2, W3
		switch c {
		case bidi.L, bidi.R:
			strong = c
		case bidi.AL:
			strong, t[k] = c, bidi.R
		case bidi.EN:
			if strong == bidi.AL {
				t[k] = bidi.AN
			}
		}
	}
	for k := 1; k+1 < len(t); k++ { // W4
		before, after := t[k-1], t[k+1]
		if before == after && (t[k] == bidi.ES && before == bidi.EN ||
			t[k] == bidi.CS && (before == bidi.EN || before == bidi.AN)) {
			t[k] = before
		}
	}
	for k := 0; k < len(t); { // W5
		end := k
		for end < len(t) && t[end] == bidi.ET {
			end++
		}
		if end > k && (k > 0 && t[k-1] == bidi.EN || end < len(t) && t[end] == bidi.EN) {
			fill(t[k:end], bidi.EN)
		}
		k = max(end, k+1)
	}
	strong = sos
	for k, c := range t { // W6, W7
		switch c {
		case bidi.ES, bidi.ET, bidi.CS:
			t[k] = bidi.ON
		case bidi.L, bidi.R:
			strong = c
		case bidi.EN:
			if strong == bidi.L {
				t[k] = bidi.L
			}
		}
	}
}

type bracketPair struct{ open, close int }

// resolveBrackets applies rule N0: a bracket pair takes the direction of its content, or of
// the text before it when the content only holds the opposite direction.
func resolveBrackets(t []bidi.Class, rs []rune, seq []int, classes []bidi.Class, e bidi.Class) {
	for _, p := range bracketPairs(t, rs, seq) {
		dir := pairDirection(t, p, e)
		if dir == bidi.ON {
			continue
		}
		for _, b := range []int{p.open, p.close} {
			t[b] = dir
			// Marks that followed the bracket took its old type in W1; they follow it again.
			for k := b + 1; k < len(t) && classes[seq[k]] == bidi.NSM; k++ {
				t[k] = dir
			}
		}
	}
}

// bracketPairs finds the pairs of rule BD16, sorted by opening bracket. Brackets without an
// entry in mirrors (outside ASCII) never pair.
func bracketPairs(t []bidi.Class, rs []rune, seq []int) []bracketPair {
	type opener struct {
		pos    int
		closer rune
	}
	var stack []opener
	var pairs []bracketPair
	for k, i := range seq {
		p, _ := bidi.LookupRune(rs[i])
		if t[k] != bidi.ON || !p.IsBracket() {
			continue
		}
		if p.IsOpeningBracket() {
			if len(stack) == 63 { // BD16 gives up at this depth
				break
			}
			stack = append(stack, opener{k, mirrors[rs[i]]})
			continue
		}
		for j := len(stack) - 1; j >= 0; j-- {
			if stack[j].closer == rs[i] {
				pairs = append(pairs, bracketPair{stack[j].pos, k})
				stack = stack[:j]
				break
			}
		}
	}
	slices.SortFunc(pairs, func(a, b bracketPair) int { return a.open - b.open })
	return pairs
}

func pairDirection(t []bidi.Class, p bracketPair, e bidi.Class) bidi.Class {
	opposite := false
	for _, c := range t[p.open+1 : p.close] {
		switch strongDir(c) {
		case e:
			return e
		case bidi.ON:
		default:
			opposite = true
		}
	}
	if !opposite {
		return bidi.ON
	}
	for k := p.open - 1; k >= 0; k-- {
		if s := strongDir(t[k]); s != bidi.ON {
			return s
		}
	}
	return e
}

// resolveNeutrals applies rules N1-N2: neutrals between two runs of one direction take it,
// all others take the embedding direction.
func resolveNeutrals(t []bidi.Class, e bidi.Class) {
	for k := 0; k < len(t); {
		end := k
		for end < len(t) && isNeutral(t[end]) {
			end++
		}
		if end == k {
			k++
			continue
		}
		before, after := e, e
		if k > 0 {
			before = strongDir(t[k-1])
		}
		if end < len(t) {
			after = strongDir(t[end])
		}
		dir := e
		if before == after {
			dir = before
		}
		fill(t[k:end], dir)
		k = end
	}
}

func isNeutral(c bidi.Class) bool {
	return c == bidi.B || c == bidi.S || c == bidi.WS || c == bidi.ON
}

// strongDir is L or R for a resolved type (numbers count as R), ON for anything else.
func strongDir(c bidi.Class) bidi.Class {
	switch c {
	case bidi.L:
		return bidi.L
	case bidi.R, bidi.AL, bidi.EN, bidi.AN:
		return bidi.R
	}
	return bidi.ON
}

// implicitLevel applies rules I1-I2 to a type resolved to L, R, EN or AN.
func implicitLevel(c bidi.Class, para uint8) uint8 {
	switch {
	case para%2 == 1 && c != bidi.R:
		return para + 1
	case para%2 == 0 && c == bidi.R:
		return para + 1
	case para%2 == 0 && (c == bidi.EN || c == bidi.AN):
		return para + 2
	}
	return para
}

// resetWhitespace applies rule L1: separators, and whitespace before them or at the end of the
// line, return to the paragraph level. Runes removed by X9 count as whitespace.
func resetWhitespace(levels []uint8, classes []bidi.Class, para uint8) {
	trailing := true
	for i := len(levels) - 1; i >= 0; i-- {
		switch classes[i] {
		case bidi.S, bidi.B:
			levels[i], trailing = para, true
		case bidi.WS, bidi.BN:
			if trailing {
				levels[i] = para
			}
		default:
			trailing = false
		}
	}
}

// reorder applies rules L2 and L4 to s: from the highest level down to 1 it reverses every
// sequence at that level or above, one grapheme cluster at a time so marks stay after their
// base, then mirrors brackets at odd levels.
func reorder(s string, levels []uint8) string {
	type cluster struct {
		text  string
		level uint8
	}
	var cs []cluster
	var high uint8
	i := 0
	for g := graphemes.FromString(s); g.Next(); {
		cs = append(cs, cluster{g.Value(), levels[i]})
		high = max(high, levels[i])
		i += utf8.RuneCountInString(g.Value())
	}
	for lvl := high; lvl >= 1; lvl-- {
		for k := 0; k < len(cs); {
			end := k
			for end < len(cs) && cs[end].level >= lvl {
				end++
			}
			slices.Reverse(cs[k:end])
			k = max(end, k+1)
		}
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range cs {
		r, size := utf8.DecodeRuneInString(c.text)
		if m, ok := mirrors[r]; ok && c.level%2 == 1 {
			b.WriteRune(m)
			b.WriteString(c.text[size:])
			continue
		}
		b.WriteString(c.text)
	}
	return b.String()
}

func fill(t []bidi.Class, c bidi.Class) {
	for k := range t {
		t[k] = c
	}
}
