package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// Layout places text for the current language. Every visible string goes through it: first the
// rtl mode prepares the text (joining and reordering Persian when the app does it), then it is
// measured, padded and aligned with the reading-start edge on the left in English and on the
// right in Persian. Text is prepared before it is styled; the placing helpers take styled text.
type Layout struct {
	Mode rtl.Mode
	RTL  bool // the interface language is written right to left: mirror the layout
}

// dir is the base direction of interface text.
func (l Layout) dir() rtl.Dir {
	if l.RTL {
		return rtl.RTL
	}
	return rtl.LTR
}

// Text prepares a line of interface text (from i18n) for display.
func (l Layout) Text(s string) string { return l.Mode.Line(s, l.dir()) }

// Fit prepares a line of interface text, shortened with "…" to width cells.
func (l Layout) Fit(s string, width int) string { return l.Mode.Fit(s, width, l.dir()) }

// Data prepares user data (a profile name, a chat title, a folder) shortened to width cells.
// Its direction comes from its own first strong letter, whatever the interface language.
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

// Inline joins styled pieces given in reading order into one line: left to right in English,
// right to left in Persian. Separators between pieces are pieces too.
func (l Layout) Inline(parts ...string) string {
	if l.RTL {
		parts = slices.Clone(parts)
		slices.Reverse(parts)
	}
	return strings.Join(parts, "")
}

// Row fills a line of width cells with lead at the reading-start edge and trail at the
// reading-end edge. When both do not fit with a space between them, trail is dropped and lead
// is clipped.
func (l Layout) Row(lead, trail string, width int) string {
	lw, tw := ansi.StringWidth(lead), ansi.StringWidth(trail)
	if tw > 0 && lw+1+tw > width {
		trail, tw = "", 0
	}
	if lw+tw > width {
		lead = l.Clip(lead, width)
		lw = ansi.StringWidth(lead)
	}
	gap := strings.Repeat(" ", width-lw-tw)
	if l.RTL {
		return trail + gap + lead
	}
	return lead + gap + trail
}

// Pad fills styled text to width cells so it lines up at the reading-start edge.
func (l Layout) Pad(s string, width int) string { return l.Row(s, "", width) }

// End fills styled text to width cells so it lines up at the reading-end edge.
func (l Layout) End(s string, width int) string { return l.Row("", s, width) }

// Center puts styled text in the middle of width cells.
func (l Layout) Center(s string, width int) string {
	s = l.Clip(s, width)
	w := ansi.StringWidth(s)
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-w-left)
}

// Clip cuts styled text to width cells at its reading end (the right in English, the left in
// Persian), marking the cut with "…". Prepare text with Fit or Data where you can; Clip is the
// safety net for composed lines.
func (l Layout) Clip(s string, width int) string {
	w := ansi.StringWidth(s)
	switch {
	case w <= width:
		return s
	case width <= 0:
		return ""
	case l.RTL:
		return ansi.TruncateLeft(s, w-width+1, "…")
	}
	return ansi.Truncate(s, width, "…")
}

// Pointer points from the reading-start edge at the item it marks: › in English, ‹ in Persian.
func (l Layout) Pointer() string {
	if l.RTL {
		return "‹"
	}
	return "›"
}

// BarGlyph is the selection bar, drawn against the reading-start edge.
func (l Layout) BarGlyph() string {
	if l.RTL {
		return "▐"
	}
	return "▌"
}

// Lines fits text to a block of exactly width by height cells: every line clipped and padded
// at its reading-end side, missing lines blank.
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
