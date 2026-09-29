package rtl

import (
	"bufio"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	gobidi "github.com/go-text/typesetting/bidi"
	"golang.org/x/text/unicode/bidi"
)

// TestConformance checks the resolved levels against BidiCharacterTest.txt, which go-text
// ships. Lines with explicit embeddings or isolates, a paragraph separator inside the line, or
// brackets other than ( ) [ ] { } are outside this package and skipped.
func TestConformance(t *testing.T) {
	dir, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/go-text/typesetting").Output()
	if err != nil {
		t.Skipf("go-text module not found: %v", err)
	}
	f, err := os.Open(filepath.Join(strings.TrimSpace(string(dir)), "bidi", "test", "BidiCharacterTest.txt"))
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	dirs := map[string]Dir{"0": LTR, "1": RTL, "2": DirAuto}
	checked := 0
	for sc := bufio.NewScanner(f); sc.Scan(); {
		fields := strings.Split(sc.Text(), ";")
		if len(fields) != 5 {
			continue
		}
		rs := []rune(cps(fields[0]))
		if !conformanceSupported(rs) {
			continue
		}
		classes := make([]bidi.Class, len(rs))
		for i, r := range rs {
			classes[i] = classOf(r)
		}
		para := paragraphLevel(classes, dirs[fields[1]])
		levels := resolveLevels(rs, classes, para)
		ok := strconv.Itoa(int(para)) == fields[2]
		for i, want := range strings.Fields(fields[3]) {
			ok = ok && (want == "x" || want == strconv.Itoa(int(levels[i])))
		}
		if !ok {
			t.Errorf("%s: paragraph %d, levels %v", sc.Text(), para, levels)
		}
		checked++
	}
	if checked < 90000 {
		t.Errorf("checked only %d lines", checked)
	}
}

func conformanceSupported(rs []rune) bool {
	for i, r := range rs {
		p, _ := bidi.LookupRune(r)
		_, mirrored := mirrors[r]
		if p.Class() >= bidi.Control || p.Class() == bidi.B && i < len(rs)-1 || p.IsBracket() && !mirrored {
			return false
		}
	}
	return true
}

// TestLevelsMatchGoText compares the resolved levels with go-text/typesetting/bidi, which
// passes the Unicode conformance tests but only exports runs split by direction. Random lines
// from an alphabet covering every bidi class this package handles must give the same runs.
func TestLevelsMatchGoText(t *testing.T) {
	alphabet := []rune{
		'a', 'Z', 0x200E, // L, LRM
		0x05D0, 0x200F, // R, RLM
		0x0628, 0x0627, 0x061C, // AL, ALM
		'1', 0x06F1, // EN
		0x0661,             // AN
		'+', '-', '$', '%', // ES, ET
		'.', ',', ':', '/', 0x00A0, // CS
		' ', '\t', // WS, S
		'(', ')', '[', ']', '!', '«', // ON
		0x0301, // NSM
		0x200D, // BN
	}
	dirs := map[Dir]gobidi.Direction{DirAuto: gobidi.Neutral, LTR: gobidi.LeftToRight, RTL: gobidi.RightToLeft}
	rng := rand.New(rand.NewPCG(1, 2))
	var p gobidi.Paragraph
	for range 20000 {
		rs := make([]rune, rng.IntN(14))
		for i := range rs {
			rs[i] = alphabet[rng.IntN(len(alphabet))]
		}
		for base, dir := range dirs {
			classes := make([]bidi.Class, len(rs))
			for i, r := range rs {
				classes[i] = classOf(r)
			}
			got := runsOf(resolveLevels(rs, classes, paragraphLevel(classes, base)))
			out := p.Segment(rs, dir)
			var want []gobidi.Run
			for i := range out.NumRuns() {
				want = append(want, out.Run(i))
			}
			if !slices.Equal(got, want) {
				t.Fatalf("%s base %d: runs %v, go-text %v", hexOf(string(rs)), base, got, want)
			}
		}
	}
}

func runsOf(levels []uint8) []gobidi.Run {
	var runs []gobidi.Run
	for i, l := range levels {
		if i == 0 || l%2 != levels[i-1]%2 {
			runs = append(runs, gobidi.Run{Start: i, Level: gobidi.Level(l)})
		}
		runs[len(runs)-1].End = i + 1
	}
	return runs
}
