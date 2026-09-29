package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// usageScreen is the token dashboard: tiles with the totals, tokens by model, output tokens per
// day and the busiest project folders, all counted from the transcripts on this computer.
type usageScreen struct {
	stats       transcript.Stats
	loaded      bool
	loading     bool
	err         error
	done, total int          // transcripts read so far, of all
	feed        chan tea.Msg // the read under way: its progress, then its result
	scroll      scroll
}

type (
	usageProgressMsg struct{ done, total int }
	usageMsg         struct {
		stats transcript.Stats
		err   error
	}
)

const (
	chartDays   = 30
	topProjects = 5
	topModels   = 8
	wideUsage   = 80 // from this window width the model and project bars sit side by side
)

func (*usageScreen) Title() string { return i18n.T("tab.usage") }
func (*usageScreen) Typing() bool  { return false }

func (s *usageScreen) Keys(c *Ctx) []key.Binding {
	if !s.loaded || s.loading {
		return nil
	}
	refresh := bind("r", "insights.key.refresh")
	if s.err != nil || totalUsage(s.stats).Total() == 0 || len(s.dashboard(c, c.Width, bodyRows(c))) <= bodyRows(c) {
		return []key.Binding{refresh}
	}
	return []key.Binding{scrollKeys(), refresh}
}

func (s *usageScreen) Update(c *Ctx, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case shownMsg:
		if !s.loaded && !s.loading {
			return s.read(c)
		}
	case usageProgressMsg:
		s.done, s.total = msg.done, msg.total
		feed := s.feed
		return func() tea.Msg { return <-feed }
	case usageMsg:
		s.stats, s.err, s.loaded, s.loading = msg.stats, msg.err, true, false
	case tea.KeyPressMsg:
		switch k := msg.String(); {
		case s.loading:
		case k == "r":
			return s.read(c)
		default:
			s.scroll.key(k)
		}
	case tea.MouseWheelMsg:
		s.scroll.wheel(msg)
	}
	return nil
}

// read runs ops.Usage in the background. Its progress comes back through feed as messages, each
// answered with a command that waits for the next one, until the result arrives.
func (s *usageScreen) read(c *Ctx) tea.Cmd {
	env, feed := c.Env, make(chan tea.Msg, 1)
	s.feed, s.loading, s.done, s.total = feed, true, 0, 0
	return func() tea.Msg {
		go func() {
			stats, err := env.Usage(func(done, total int) {
				select {
				case feed <- usageProgressMsg{done, total}:
				default: // the last one is not drawn yet; a newer one follows
				}
			})
			feed <- usageMsg{stats, err}
		}()
		return <-feed
	}
}

func (s *usageScreen) View(c *Ctx, width, height int) string {
	st := c.St
	switch {
	case !s.loaded || s.loading:
		return s.reading(c, width, height)
	case s.err != nil:
		return notice(c, st.Fail, i18n.T("insights.usage.error"), s.err.Error()+"\n"+i18n.T("insights.usage.retry"), width, height)
	case totalUsage(s.stats).Total() == 0:
		return notice(c, st.Title, i18n.T("insights.usage.empty.title"), i18n.T("insights.usage.empty.body"), width, height)
	}
	return strings.Join(s.scroll.window(s.dashboard(c, width, height), height), "\n")
}

// reading is "Reading 447 transcripts… 38%" over a bar that fills as they are read.
func (s *usageScreen) reading(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	label, frac := i18n.T("insights.usage.finding"), 0.0
	if s.total > 0 {
		frac = float64(s.done) / float64(s.total)
		label = i18n.T("insights.usage.reading", "n", s.total, "pct", i18n.Digits(fmt.Sprintf("%d%%", s.done*100/s.total)))
	}
	lines := []string{
		l.Center(st.Text.Render(l.Fit(label, width-4)), width),
		"",
		l.Center(meter(st, frac, min(48, width-8)), width),
	}
	return strings.Repeat("\n", max(0, (height-len(lines))/2)) + strings.Join(lines, "\n")
}

func totalUsage(st transcript.Stats) transcript.Usage {
	var all transcript.Usage
	for _, u := range st.ByModel {
		all = all.Add(u)
	}
	return all
}

// dashboard is every section, a cell in from each edge. From wideUsage columns the model and
// project bars share a row; below that the sections stack in the order they are listed. A tall
// window gets taller columns and a blank line under each title.
func (s *usageScreen) dashboard(c *Ctx, width, height int) []string {
	l, st := c.L(), c.St
	w, all, airy := width-2, totalUsage(s.stats), height >= 30
	models := shares(s.stats.ByModel, topModels, shortModel)
	projects := shares(s.stats.ByProject, topProjects, folder)
	byModel := func(w int) []string {
		return section(c, i18n.T("insights.usage.by_model"), shareRows(c, models, all.Total(), w), w, airy)
	}
	byProject := func(w int) []string {
		return section(c, i18n.T("insights.usage.projects"), shareRows(c, projects, all.Total(), w), w, airy)
	}

	lines := append([]string{""}, s.tiles(c, all, w)...)
	add := func(block []string) { lines = append(append(lines, ""), block...) }
	if half := (w - 4) / 2; width >= wideUsage {
		add(beside(byModel(half), byProject(w-4-half), half, 4))
		add(s.chart(c, w, airy))
	} else {
		add(byModel(w))
		add(s.chart(c, w, airy))
		add(byProject(w))
	}
	lines = append(lines, "")
	for _, t := range l.Wrap(i18n.T("insights.usage.note"), w) {
		lines = append(lines, st.Muted.Render(t))
	}
	for i, line := range lines {
		lines[i] = " " + l.Pad(line, w) + " "
	}
	return lines
}

// tiles are the totals, a label over each value, side by side in rounded strips: as many strips
// as the width needs, each stretched to the full width.
func (s *usageScreen) tiles(c *Ctx, all transcript.Usage, width int) []string {
	l, st := c.L(), c.St
	span := i18n.Date(s.stats.First.Local()) + " – " + i18n.Date(s.stats.Last.Local())
	items := [][2]string{
		{l.Text(i18n.T("insights.usage.total")), l.Text(i18n.Compact(all.Total()))},
		{l.Text(i18n.T("insights.usage.output")), l.Text(i18n.Compact(all.Output))},
		{l.Text(i18n.T("insights.usage.sessions")), l.Text(i18n.N(int64(s.stats.Sessions)))},
		{l.Text(i18n.T("insights.usage.span")), l.Text(span)},
	}
	text := func(t [2]string) int { return max(ansi.StringWidth(t[0]), ansi.StringWidth(t[1])) }
	var rows [][][2]string
	used := width // the first tile starts a strip
	for _, t := range items {
		if used+text(t)+3 > width { // a border and a space each side
			rows, used = append(rows, nil), 1 // the strip's closing border
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], t)
		used += text(t) + 3
	}
	var lines []string
	for _, row := range rows {
		spare := width - 1 // below zero only for a lone tile wider than the window
		for _, t := range row {
			spare -= text(t) + 3
		}
		var rules, labels, values []string
		for i, t := range row {
			w := max(0, text(t)+spare/len(row))
			if i >= len(row)-spare%len(row) {
				w++
			}
			rules = append(rules, strings.Repeat("─", w+2))
			labels = append(labels, " "+l.Pad(st.Muted.Render(l.Clip(t[0], w)), w)+" ")
			values = append(values, " "+l.Pad(st.Title.Render(l.Clip(t[1], w)), w)+" ")
		}
		bar := st.Divider.Render("│")
		lines = append(lines,
			st.Divider.Render("╭"+strings.Join(rules, "┬")+"╮"),
			bar+strings.Join(labels, bar)+bar,
			bar+strings.Join(values, bar)+bar,
			st.Divider.Render("╰"+strings.Join(rules, "┴")+"╯"))
	}
	return lines
}

// share is one bar: a name and its tokens.
type share struct {
	name   string
	tokens int64
}

// shares are the n biggest entries of m, biggest first, named by name.
func shares(m map[string]transcript.Usage, n int, name func(string) string) []share {
	keys := slices.SortedFunc(maps.Keys(m), func(x, y string) int {
		return cmp.Or(cmp.Compare(m[y].Total(), m[x].Total()), strings.Compare(x, y))
	})
	var out []share
	for _, k := range keys[:min(n, len(keys))] {
		out = append(out, share{cmp.Or(name(k), "—"), m[k].Total()})
	}
	return out
}

// shareRows draws a bar per entry: its name (user data: a model or a project folder), its share
// of all as a bar, then its tokens and the share in per cent, right-aligned in their columns.
func shareRows(c *Ctx, items []share, all int64, width int) []string {
	l, st := c.L(), c.St
	names, nums, pcts := make([]string, len(items)), make([]string, len(items)), make([]string, len(items))
	nameW, numW, pctW := 0, 0, 0
	for i, it := range items {
		names[i] = l.Data(it.name, max(8, width/3))
		nums[i] = l.Text(i18n.Compact(it.tokens))
		pcts[i] = l.Text(i18n.Digits(fmt.Sprintf("%.1f%%", float64(it.tokens)*100/float64(max(all, 1)))))
		nameW, numW, pctW = max(nameW, ansi.StringWidth(names[i])), max(numW, ansi.StringWidth(nums[i])), max(pctW, ansi.StringWidth(pcts[i]))
	}
	barW := max(1, width-nameW-numW-pctW-6)
	right := func(s string, w int) string { return lipgloss.PlaceHorizontal(w, lipgloss.Right, s) }
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = l.Pad(st.Text.Render(names[i]), nameW) + "  " + meter(st, float64(it.tokens)/float64(max(all, 1)), barW) +
			"  " + right(st.Text.Render(nums[i]), numW) + "  " + right(st.Muted.Render(pcts[i]), pctW)
	}
	return lines
}

// section is a titled block, with a blank line under the title when the window is airy.
func section(c *Ctx, title string, body []string, width int, airy bool) []string {
	l, st := c.L(), c.St
	head := []string{st.Title.Render(l.Fit(title, width))}
	if airy {
		head = append(head, "")
	}
	return append(head, body...)
}

// beside puts two blocks side by side, gap cells apart, the first widthA cells wide.
func beside(a, b []string, widthA, gap int) []string {
	out := make([]string, max(len(a), len(b)))
	for i := range out {
		var x, y string
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		out[i] = lipgloss.PlaceHorizontal(widthA+gap, lipgloss.Left, x) + y
	}
	return out
}

// chart is the output tokens of each of the last days as columns, eight rows tall in an airy
// window and four otherwise, on an axis with the first and the last date: today at the right.
func (s *usageScreen) chart(c *Ctx, width int, airy bool) []string {
	l, st := c.L(), c.St
	rows := 4
	if airy {
		rows = 8
	}
	today := c.Now().Local()
	cols, bar, gap := chartShape(chartDays, width)
	values := perDay(s.stats.ByDay, today, chartDays, cols)
	peak := int64(0)
	for _, v := range values {
		peak = max(peak, v)
	}
	title := st.Title.Render(l.Text(i18n.T("insights.usage.days", "n", chartDays))) + "  " +
		st.Muted.Render(l.Text(i18n.T("insights.usage.per_day")))
	var top string
	if peak > 0 {
		top = st.Muted.Render(l.Text(i18n.T("insights.usage.peak", "v", i18n.Compact(peak))))
	}
	chartW := cols*bar + max(cols-1, 0)*gap
	headW := chartW // the peak sits over the chart's end, when there is room
	if ansi.StringWidth(title)+2+ansi.StringWidth(top) > chartW {
		headW = width
	}
	lines := []string{l.Row(title, top, headW)}
	if airy {
		lines = append(lines, "")
	}
	for _, row := range columns(values, rows) {
		lines = append(lines, st.Accent.Render(widen(row, bar, gap)))
	}
	first, last := l.Text(i18n.Date(today.AddDate(0, 0, 1-chartDays))), l.Text(i18n.Date(today))
	rule := chartW - ansi.StringWidth(first) - ansi.StringWidth(last) - 2
	if rule < 2 { // no room for the dates
		return append(lines, st.Divider.Render(strings.Repeat("─", chartW)))
	}
	return append(lines, st.Muted.Render(first)+" "+st.Divider.Render(strings.Repeat("─", rule))+" "+st.Muted.Render(last))
}
