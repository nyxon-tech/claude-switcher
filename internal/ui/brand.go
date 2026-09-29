package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
)

// The brand is typographic: the product's name on a pill of the accent colour or spaced out in
// capitals, then "by nyxon", the maker's name shading from the accent to the text colour.

// star marks the brand's title.
const star = "✦"

// gradient writes s in bold, each letter a step from one colour to the other; spaces stay
// unstyled. Without colours (the plain theme) it is only bold.
func gradient(s string, from, to color.Color) string {
	bold := lipgloss.NewStyle().Bold(true)
	if from == nil || to == nil {
		return bold.Render(s)
	}
	rs := []rune(s)
	colours := lipgloss.Blend1D(len(rs), from, to)
	var b strings.Builder
	for i, r := range rs {
		if r == ' ' {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(bold.Foreground(colours[i]).Render(string(r)))
	}
	return b.String()
}

// Nyxon is the maker's name as the brand writes it, in a palette's gradient, for the command line.
func Nyxon(p Palette) string { return gradient("nyxon", p.Accent, p.Text) }

// shade is text in the brand's gradient, from the accent to the text colour.
func (s Styles) shade(text string) string { return gradient(text, s.pal.accent, s.pal.text) }

// byNyxon is "by nyxon": "by" muted, the name in the gradient.
func (s Styles) byNyxon() string { return s.Muted.Render(i18n.T("app.by")+" ") + s.shade("nyxon") }

// brand is the header's lockup: the name on a pill of the accent (reverse video in the plain
// theme), then " by nyxon" when full.
func (s Styles) brand(full bool) string {
	b := s.Badge.Render(" " + i18n.T("app.name") + " ")
	if full {
		b += " " + s.byNyxon()
	}
	return b
}

// hero is the brand as a title block, each line centred in width: the star, the name spaced out
// in capitals, "by nyxon", a blank line and the tagline. Below 40 cells the star and the name
// share a line, unspaced.
func (s Styles) hero(l Layout, width int) []string {
	name := i18n.T("app.name")
	lines := []string{s.Accent.Render(star), s.shade(spaced(strings.ToUpper(name)))}
	if width < 40 {
		lines = []string{s.Accent.Render(star) + " " + s.shade(name)}
	}
	lines = append(lines, s.byNyxon(), "")
	for _, t := range l.Wrap(i18n.T("app.tagline"), width) {
		lines = append(lines, s.Muted.Render(t))
	}
	for i, line := range lines {
		lines[i] = l.Center(line, width)
	}
	return lines
}

// spaced writes s with a space between letters and three between words:
// "C L A U D E   S W I T C H E R".
func spaced(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.Join(strings.Split(w, ""), " ")
	}
	return strings.Join(words, "   ")
}
