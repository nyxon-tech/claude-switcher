package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// Layout places text on screen. Every visible string goes through it: first the rtl mode
// prepares the text, joining and reordering Persian, Arabic or Hebrew user data when the app
// does it, then it is measured, padded and aligned. Text is prepared before it is styled; the
// placing helpers take styled text.
type Layout struct{ Mode rtl.Mode }

// Text prepares a line of interface text (from i18n), which may hold user data such as a name.
func (l Layout) Text(s string) string { return l.Mode.Line(s, rtl.LTR) }

// Fit prepares a line of interface text, shortened with "…" to width cells.
func (l Layout) Fit(s string, width int) string { return l.Mode.Fit(s, width, rtl.LTR) }

// Data prepares user data (a profile name, a chat title, a folder) shortened to width cells.
// Its direction comes from its own first strong letter.
func (l Layout) Data(s string, width int) string { return l.Mode.Fit(s, width, rtl.DirAuto) }

// Path prepares a path or an id, always left to right, shortened to width cells.
func (l Layout) Path(s string, width int) string { return l.Mode.Fit(s, width, rtl.LTR) }

// Wrap breaks interface text into prepared lines of at most width cells.
func (l Layout) Wrap(s string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case l.Mode.Width(line+" "+word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	for i, t := range lines {
		lines[i] = l.Fit(t, width)
	}
	return lines
}

// Row fills a line of width cells with lead at the left edge and trail at the right. When both
// do not fit with a space between them, trail is dropped and lead is clipped.
func (l Layout) Row(lead, trail string, width int) string {
	lw, tw := ansi.StringWidth(lead), ansi.StringWidth(trail)
	if tw > 0 && lw+1+tw > width {
		trail, tw = "", 0
	}
	if lw+tw > width {
		lead = l.Clip(lead, width)
		lw = ansi.StringWidth(lead)
	}
	return lead + strings.Repeat(" ", width-lw-tw) + trail
}

// Pad fills styled text to width cells, lined up at the left edge.
func (l Layout) Pad(s string, width int) string { return l.Row(s, "", width) }

// End fills styled text to width cells, lined up at the right edge.
func (l Layout) End(s string, width int) string { return l.Row("", s, width) }

// Center puts styled text in the middle of width cells.
func (l Layout) Center(s string, width int) string {
	s = l.Clip(s, width)
	w := ansi.StringWidth(s)
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-w-left)
}

// Clip cuts styled text to width cells at its end, marking the cut with "…". Prepare text with
// Fit or Data where you can; Clip is the safety net for composed lines.
func (l Layout) Clip(s string, width int) string {
	switch {
	case ansi.StringWidth(s) <= width:
		return s
	case width <= 0:
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// Lines fits text to a block of exactly width by height cells: every line clipped and padded,
// missing lines blank.
func (l Layout) Lines(s string, width, height int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	out := make([]string, height)
	for i := range out {
		if i < len(lines) {
			out[i] = l.Pad(lines[i], width)
		} else {
			out[i] = strings.Repeat(" ", width)
		}
	}
	return out
}
