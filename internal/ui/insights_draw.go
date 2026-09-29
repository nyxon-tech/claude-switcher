package ui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// Drawing shared by the Activity, Usage and Doctor screens: scrolling, bars, the column chart
// and centred notices.

// scroll keeps a body taller than the screen in view: the arrows, the page keys and the wheel
// move it a line or a page at a time.
type scroll struct{ top, max, page int }

// window returns the lines that fit in height from the current position.
func (s *scroll) window(lines []string, height int) []string {
	height = max(height, 0)
	s.page = max(1, height-1)
	s.max = max(0, len(lines)-height)
	s.top = min(max(s.top, 0), s.max)
	return lines[s.top:min(len(lines), s.top+height)]
}

// key moves for one key press; keys that do not scroll leave it where it is.
func (s *scroll) key(k string) {
	switch k {
	case "up", "k":
		s.top--
	case "down", "j":
		s.top++
	case "pgup":
		s.top -= s.page
	case "pgdown":
		s.top += s.page
	case "home":
		s.top = 0
	case "end":
		s.top = s.max
	}
	s.top = min(max(s.top, 0), s.max)
}

// wheel moves three lines a notch.
func (s *scroll) wheel(msg tea.MouseWheelMsg) {
	switch msg.Button {
	case tea.MouseWheelUp:
		s.top -= 3
	case tea.MouseWheelDown:
		s.top += 3
	}
	s.top = min(max(s.top, 0), s.max)
}

// bodyRows is the height of the body under a one-line footer: the window less the header, the
// divider, the toast line and the footer. Keys need it because the footer is drawn first.
func bodyRows(c *Ctx) int { return c.Height - 4 }

func scrollKeys() key.Binding {
	return bind("↑↓", "insights.key.scroll", "up", "down", "k", "j", "pgup", "pgdown", "home", "end")
}

// notice centres a short message: a title and, below it, muted paragraphs (one per line of body).
func notice(c *Ctx, title lipgloss.Style, head, body string, width, height int) string {
	l, st := c.L(), c.St
	textW := min(60, width-4)
	var lines []string
	add := func(style lipgloss.Style, text string) {
		for _, t := range l.Wrap(text, textW) {
			lines = append(lines, l.Center(style.Render(t), width))
		}
	}
	add(title, head)
	for _, para := range strings.Split(body, "\n") {
		if para != "" {
			lines = append(lines, "")
			add(st.Muted, para)
		}
	}
	return strings.Repeat("\n", max(0, (height-len(lines))/2)) + strings.Join(lines, "\n")
}

// wrap is Layout.Wrap that also breaks a word longer than a line, such as a path, instead of
// cutting it short: after its last separator that fits, else where the line ends.
func wrap(l Layout, s string, width int) []string {
	var words []string
	for _, w := range strings.Fields(s) {
		for width > 0 && l.Mode.Width(w) > width {
			head := ansi.Truncate(w, width, "")
			if i := strings.LastIndexAny(head, `/\`); i > 0 {
				head = head[:i+1]
			}
			if head == "" { // a single character wider than the line
				break
			}
			words, w = append(words, head), w[len(head):]
		}
		words = append(words, w)
	}
	return l.Wrap(strings.Join(words, " "), width)
}

// meter is a bar width cells wide, filled by frac in the brand gradient (accent to text) on a
// track in the line colour. A share too small for a cell of its own still gets one.
func meter(st Styles, frac float64, width int) string {
	width = max(width, 0)
	n := min(max(int(frac*float64(width)+0.5), 0), width)
	if frac > 0 && width > 0 {
		n = max(n, 1)
	}
	fill := strings.Repeat("█", n)
	if !st.Plain { // the plain theme has no colours to blend
		var b strings.Builder
		for _, col := range lipgloss.Blend1D(width, st.pal.accent, st.pal.text)[:n] {
			b.WriteString(lipgloss.NewStyle().Foreground(col).Render("█"))
		}
		fill = b.String()
	}
	return fill + st.Divider.Render(strings.Repeat("░", width-n))
}

// columnGlyphs draw a column an eighth of a cell at a time.
var columnGlyphs = []rune("▁▂▃▄▅▆▇█")

// chartShape fits days columns in width cells, as wide as they go: each column bar cells wide
// (up to 4) with gap cells between (1, or 2 once columns are 2 wide). A window too narrow for a
// gap between columns gets none.
func chartShape(days, width int) (cols, bar, gap int) {
	used := 0
	for b := 1; b <= 4; b++ {
		for g := 1; g <= min(b, 2); g++ {
			if w := days*b + (days-1)*g; w <= width && w > used {
				used, bar, gap = w, b, g
			}
		}
	}
	if used > 0 {
		return days, bar, gap
	}
	return max(0, min(days, width)), 1, 0
}

// perDay adds up the output tokens of the days ending today, oldest first, into cols columns.
func perDay(byDay map[string]transcript.Usage, today time.Time, days, cols int) []int64 {
	if cols <= 0 {
		return nil
	}
	out := make([]int64, cols)
	for d := range days {
		out[d*cols/days] += byDay[today.AddDate(0, 0, d-days+1).Format(time.DateOnly)].Output
	}
	return out
}

// columns draws values as a column chart rows cells tall, top row first, one glyph a value. The
// largest value fills every row; any value above zero shows at least ▁.
func columns(values []int64, rows int) []string {
	peak := int64(0)
	for _, v := range values {
		peak = max(peak, v)
	}
	lines := make([]string, rows)
	for r := range rows {
		below := (rows - 1 - r) * len(columnGlyphs) // eighths under this row
		var b strings.Builder
		for _, v := range values {
			h := 0
			if v > 0 {
				h = max(1, int((v*int64(rows*len(columnGlyphs))+peak/2)/peak))
			}
			switch f := h - below; {
			case f >= len(columnGlyphs):
				b.WriteRune(columnGlyphs[len(columnGlyphs)-1])
			case f > 0:
				b.WriteRune(columnGlyphs[f-1])
			default:
				b.WriteByte(' ')
			}
		}
		lines[r] = b.String()
	}
	return lines
}

// widen draws each glyph of a chart row bar cells wide, gap cells apart.
func widen(row string, bar, gap int) string {
	var b strings.Builder
	for i, r := range []rune(row) {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", gap))
		}
		b.WriteString(strings.Repeat(string(r), bar))
	}
	return b.String()
}

// shortModel names a model the way people say it: "claude-opus-4-1-20250805" is "opus 4.1" and
// "claude-3-5-sonnet-20241022" is "sonnet 3.5". Other names stay as they are.
func shortModel(id string) string {
	rest, ok := strings.CutPrefix(id, "claude-")
	if !ok {
		return id
	}
	var words, version []string
	for _, part := range strings.Split(rest, "-") {
		switch {
		case len(part) == 8 && digitsOnly(part): // the release date
		case digitsOnly(part):
			version = append(version, part)
		case part != "":
			words = append(words, part)
		}
	}
	if len(words) == 0 {
		return id
	}
	name := words[0]
	if len(version) > 0 {
		name += " " + strings.Join(version, ".")
	}
	return strings.Join(append([]string{name}, words[1:]...), " ")
}

func digitsOnly(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// folder is the last part of a path written on any OS: transcripts keep the cwd as it was.
func folder(path string) string {
	path = strings.TrimRight(path, `/\`)
	return path[strings.LastIndexAny(path, `/\`)+1:]
}
