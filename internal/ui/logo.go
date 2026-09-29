package ui

import (
	"image/color"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// The Nyxon wordmark drawn with quadrant block characters (U+2596-U+259F), traced from
// assets/brand/nyxon-logo.svg at two sizes. Every terminal font that has box drawing has these,
// and Windows Terminal draws them itself.

var logoLarge = []string{
	" ▄████████▄  ▐█▌       ██  ▀██▄    ▗▟█▛▘ ▄▟██████▙▖  ▗▟███████▄ ",
	"██▀      ▜█▙ ▐█▌       ██    ▀██▄▗▟█▛▘  ▐█▛      ▜█▖▗██▘     ▀██",
	"██       ▐██ ▐█▌      ▗██      █▛▝█▌    ▐█▌      ▐█▌▐█▌       ██",
	"██       ▐██ ▝██▖     ▟██    ▄██▀▝██▙▖  ▐█▙      ▟█▌▐█▌       ██",
	"██       ▐██  ▝▜█████████  ▄██▀    ▝██▙▖ ▀████████▀ ▐█▌       ██",
	"                     ▗▟█▛                                       ",
	"                   ▄██▛▘                                        ",
}

var logoSmall = []string{
	"▗██▀▀▜█▄ ▐█    ▐█ ▝▜▙▖  ▄█▀ ▄█▛▀▜█▙ ▗▟██▀▜█▖",
	"█▌    ▐█ ▐█    ▐█   ▝▜▌▜▛▘  █▌    █▌█▌    ▐█",
	"█▌    ▐█ ▐█▖   ▟█   ▄█▘▜█▄  █▌   ▗█▘█▌    ▐█",
	"▀▘    ▝▀  ▀▀▀▀▀▜█  ▀▀   ▝▀▘ ▝▀▀▀▀▀▘ ▀▘    ▝▀",
	"             ▗▟▛▘                          ",
}

// gradient colours text column by column, from one colour at the left to the other at the right.
// The logo is a brand mark, so it is never mirrored for right-to-left languages.
func gradient(lines []string, from, to color.Color, bold bool) string {
	width := 0
	for _, l := range lines {
		width = max(width, utf8.RuneCountInString(l))
	}
	base := lipgloss.NewStyle().Bold(bold)
	if from == nil || to == nil {
		return base.Render(strings.Join(lines, "\n"))
	}
	cols := lipgloss.Blend1D(width, from, to)
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		for x, r := range []rune(l) {
			if r == ' ' {
				b.WriteByte(' ')
				continue
			}
			b.WriteString(base.Foreground(cols[x]).Render(string(r)))
		}
	}
	return b.String()
}

// logos are the wordmarks a window has room for, the large one first.
func (s Styles) logos(width, height int) []string {
	var out []string
	if width >= 70 && height >= 30 {
		out = append(out, gradient(logoLarge, s.pal.accent, s.pal.text, false))
	}
	if width >= 50 && height >= 24 {
		out = append(out, gradient(logoSmall, s.pal.accent, s.pal.text, false))
	}
	return out
}

// mark is the compact brand for the header: "nyxon" in the logo's gradient, bold.
func (s Styles) mark() string {
	return gradient([]string{"nyxon"}, s.pal.accent, s.pal.text, true)
}
