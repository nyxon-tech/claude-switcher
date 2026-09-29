package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// Rows of the body: the chips, the search field, a blank line, then the panes.
const (
	chatsChipsY  = 0
	chatsSearchY = 1
	chatsListY   = 3
	chatsWide    = 100 // from this width the preview sits beside the list, not under it
)

// chatsGeo is where the last View drew things, for clicks.
type chatsGeo struct {
	chips    []span
	chipKeys []string
	listW    int // the list starts at the left edge
	rows     int // rows drawn from s.top
}

type chatChip struct {
	key, label string
	n          int
	data       bool // the label is a list's name, not interface text
}

// chips are All, one per list and Lost (while there are lost chats, or it is open).
func (s *chatsScreen) chips() []chatChip {
	chips := []chatChip{{key: chipAll, label: i18n.T("chats.chip.all"), n: len(s.all)}}
	for _, l := range s.lists {
		chips = append(chips, chatChip{key: l.Dir, label: l.Label, n: len(s.byList[l.Dir]), data: true})
	}
	if len(s.lost) > 0 || s.chip == chipLost {
		chips = append(chips, chatChip{key: chipLost, label: i18n.T("chats.chip.lost"), n: len(s.lost)})
	}
	return chips
}

func (s *chatsScreen) View(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	s.geo = chatsGeo{}
	switch {
	case !s.loaded && s.err != nil:
		return middle(l, st.Fail.Render(l.Fit(s.err.Error(), width)), width, height)
	case !s.loaded:
		return middle(l, l.Inline(s.spinner(c), " ", st.Muted.Render(l.Fit(i18n.T("chats.loading"), width-2))), width, height)
	case s.full:
		return s.fullView(c, width, height)
	}
	lines := []string{s.chipsLine(c, width), s.searchLine(c, width), ""}
	h := height - len(lines)
	if len(s.shown) == 0 {
		lines = append(lines, s.emptyLines(c, width, h)...)
	} else {
		lines = append(lines, s.panes(c, width, h)...)
	}
	return strings.Join(lines, "\n")
}

// chipsLine draws the chips from the left edge, the open one on the accent. When they do not
// all fit, the first ones make way so the open chip shows.
func (s *chatsScreen) chipsLine(c *Ctx, width int) string {
	l, st := c.L(), c.St
	chips := s.chips()
	parts := make([]string, len(chips))
	open := 0
	for i, ch := range chips {
		name := l.Text(ch.label)
		if ch.data {
			name = l.Data(ch.label, 24)
		}
		n := i18n.N(int64(ch.n))
		if ch.key == s.chip {
			open = i
			parts[i] = st.Badge.Render(" " + l.Inline(name, " ", n) + " ")
			continue
		}
		count := st.Dim
		if ch.key == chipLost {
			count = st.Warn
		}
		parts[i] = l.Inline(" ", st.Muted.Render(name), " ", count.Render(n), " ")
	}
	first := 0
	for first < open && spanWidth(parts[first:open+1]) > width {
		first++
	}
	x := 0
	var shown []string
	for i := first; i < len(parts); i++ {
		w := ansi.StringWidth(parts[i])
		if x+w > width {
			break
		}
		s.geo.chips = append(s.geo.chips, span{x, x + w})
		s.geo.chipKeys = append(s.geo.chipKeys, chips[i].key)
		shown = append(shown, parts[i], " ")
		x += w + 1
	}
	return l.Pad(l.Inline(shown...), width)
}

// spanWidth is how wide chips are side by side, one cell apart.
func spanWidth(parts []string) int {
	w := len(parts) - 1
	for _, p := range parts {
		w += ansi.StringWidth(p)
	}
	return w
}

// searchLine is the search field at the left edge and the count at the right. The caret sits
// where typing goes on: at the left of a query written right to left.
func (s *chatsScreen) searchLine(c *Ctx, width int) string {
	l, st := c.L(), c.St
	q, total := s.input.Value(), len(s.rows())
	count := i18n.T("count.chats", "n", total)
	if q != "" {
		count = i18n.T("chats.search.count", "shown", len(s.shown), "total", total)
	}
	trail := st.Muted.Render(l.Text(count))
	room := max(1, width-ansi.StringWidth(trail)-5)
	icon := st.Dim.Render("/")
	if s.typing {
		icon = st.Accent.Render("/")
	}
	caret := ""
	if s.typing {
		caret = st.Badge.Render(" ")
	}
	var field string
	switch {
	case q == "":
		field = l.Inline(caret, st.Dim.Render(l.Fit(i18n.T("chats.search.placeholder"), room)))
	case dirOf(q) == rtl.RTL:
		field = l.Inline(caret, st.Text.Render(l.Data(q, room)))
	default:
		field = l.Inline(st.Text.Render(l.Data(q, room)), caret)
	}
	return l.Row(l.Inline(icon, " ", field), trail, width)
}

// panes are the list and the preview: side by side, or the preview under the list in a narrow
// window.
func (s *chatsScreen) panes(c *Ctx, width, height int) []string {
	st := c.St
	if width < chatsWide {
		listH := max(1, height*11/20)
		prevH := max(0, height-listH-1)
		out := s.listLines(c, width, listH)
		out = append(out, st.Divider.Render(strings.Repeat("─", width)))
		return append(out, s.previewPane(c, width, prevH)...)
	}
	listW := width * 3 / 5
	prevW := width - listW - 3
	list, prev := s.listLines(c, listW, height), s.previewPane(c, prevW, height)
	sep := " " + st.Divider.Render("│") + " "
	out := make([]string, height)
	for i := range out {
		out[i] = list[i] + sep + prev[i]
	}
	return out
}

// listLines draws the rows that fit, exactly height lines of width cells. In Lost, the last
// line says how many older parts were left out.
func (s *chatsScreen) listLines(c *Ctx, width, height int) []string {
	l, st := c.L(), c.St
	rowsH, note := height, ""
	if s.chip == chipLost && s.hidden > 0 && height > 3 {
		note = st.Muted.Render(l.Fit(i18n.T("chats.lost.hidden", "n", s.hidden), width))
		rowsH -= 2
	}
	if s.cursor < s.top {
		s.top = s.cursor
	} else if s.cursor >= s.top+rowsH {
		s.top = s.cursor - rowsH + 1
	}
	s.top = max(0, min(s.top, len(s.shown)-rowsH))
	end := min(len(s.shown), s.top+rowsH)
	rows, vis := s.rows(), make([]chatRow, 0, end-s.top)
	for _, i := range s.shown[s.top:end] {
		vis = append(vis, rows[i])
	}
	s.geo.listW, s.geo.rows = width, len(vis)
	out := l.Lines(strings.Join(s.rowLines(c, vis, width), "\n"), width, height)
	if note != "" {
		out[height-1] = l.Pad(note, width)
	}
	return out
}

// chatCells are a row's details, prepared for display.
type chatCells struct {
	proj, when, label string
	warn              bool // the chat has no history left
}

// rowLines draws rows: the selection bar and checkbox, the title, then the project, when it
// was last used and (in All) its list, in columns. Columns that leave the title too little room
// are dropped: the project first, then the list, then the time.
func (s *chatsScreen) rowLines(c *Ctx, vis []chatRow, width int) []string {
	l := c.L()
	now := c.Now()
	cells := make([]chatCells, len(vis))
	var w [3]int // project, time, list
	for i, r := range vis {
		cl := &cells[i]
		cl.proj = l.Data(r.Project, max(8, width/5))
		switch {
		case !r.HasHistory:
			cl.when, cl.warn = l.Text(i18n.T("chats.row.no_history")), true
		case !r.Last.IsZero():
			cl.when = l.Text(i18n.Ago(r.Last.Local(), now))
		}
		if s.chip == chipAll {
			cl.label = l.Data(r.List.Label, max(8, width/6))
		}
		for j, t := range []string{cl.proj, cl.when, cl.label} {
			w[j] = max(w[j], ansi.StringWidth(t))
		}
	}
	if anyWarn(cells) {
		w[1] += 2 // "! "
	}
	lead := 2
	if len(s.selected) > 0 {
		lead += 2
	}
	for _, drop := range []int{-1, 0, 2, 1} {
		if drop >= 0 {
			w[drop] = 0
		}
		if width-lead-2-trailWidth(w) >= 16 {
			break
		}
	}
	out := make([]string, len(vis))
	for i, r := range vis {
		out[i] = s.rowLine(c, r, cells[i], w, s.top+i == s.cursor, width-trailWidth(w)-2, width)
	}
	return out
}

func anyWarn(cells []chatCells) bool {
	for _, cl := range cells {
		if cl.warn {
			return true
		}
	}
	return false
}

// trailWidth is the width of the detail columns, two cells apart.
func trailWidth(w [3]int) int {
	total, n := 0, 0
	for _, x := range w {
		if x > 0 {
			total, n = total+x, n+1
		}
	}
	return total + 2*max(0, n-1)
}

func (s *chatsScreen) rowLine(c *Ctx, r chatRow, cl chatCells, w [3]int, cursor bool, leadW, width int) string {
	l, st := c.L(), c.St
	text, muted, accent := st.Text, st.Muted, st.Accent
	if r.Archived {
		text, muted, accent = st.Dim, st.Dim, st.Dim
	}
	if cursor {
		text = text.Bold(true)
	}
	parts := []string{" ", " "}
	if cursor {
		parts[0] = st.Bar.Render(l.BarGlyph())
	}
	if len(s.selected) > 0 {
		box := st.Dim.Render("☐")
		if s.selected[r.key] {
			box = st.Accent.Render("☑")
		}
		parts = append(parts, box, " ")
	}
	titleW := max(1, leadW-ansi.StringWidth(strings.Join(parts, "")))
	title := text.Render(l.Data(r.Title, titleW))
	if r.Title == "" {
		title = st.Dim.Render(l.Fit(i18n.T("chats.untitled"), titleW))
	}
	lead := l.Inline(append(parts, title)...)

	when := muted.Render(cl.when)
	if cl.warn {
		when = l.Inline(st.Warn.Render("!"), " ", st.Warn.Render(cl.when))
	}
	var cols []string
	for j, t := range []string{muted.Render(cl.proj), when, accent.Render(cl.label)} {
		if w[j] == 0 {
			continue
		}
		if len(cols) > 0 {
			cols = append(cols, "  ")
		}
		cols = append(cols, l.Pad(t, w[j]))
	}
	return l.Row(lead, l.Inline(cols...), width)
}

// emptyLines explain an empty list: no match, no lost chats, or no chats at all.
func (s *chatsScreen) emptyLines(c *Ctx, width, height int) []string {
	l, st := c.L(), c.St
	var title, body string
	switch q := s.input.Value(); {
	case q != "":
		title, body = i18n.T("chats.nomatch.title", "query", q), i18n.T("chats.nomatch.body")
	case s.chip == chipLost:
		title, body = i18n.T("chats.lost.empty.title"), i18n.T("chats.lost.empty.body")
	case len(s.all) == 0:
		title, body = i18n.T("chats.empty.title"), i18n.T("chats.empty.body")
	default:
		title, body = i18n.T("chats.empty.list"), i18n.T("chats.empty.list_body")
	}
	textW := min(60, width-4)
	lines := []string{l.Center(st.Title.Render(l.Fit(title, textW)), width), ""}
	for _, t := range l.Wrap(body, textW) {
		lines = append(lines, l.Center(st.Muted.Render(t), width))
	}
	pad := max(0, (height-len(lines))/2)
	return l.Lines(strings.Repeat("\n", pad)+strings.Join(lines, "\n"), width, height)
}
