package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

// doctorScreen lists ops.Doctor's checks under a summary: an icon by level, the title, the
// detail and what to do about it.
type doctorScreen struct {
	checks  []ops.Check
	loaded  bool
	loading bool
	spin    spinner.Model
	scroll  scroll
}

type doctorMsg []ops.Check

// doctorLevels in the order the summary counts them.
var doctorLevels = []ops.Level{ops.OK, ops.Info, ops.Warn, ops.Fail}

func (*doctorScreen) Title() string { return i18n.T("tab.doctor") }
func (*doctorScreen) Typing() bool  { return false }

func (s *doctorScreen) Keys(c *Ctx) []key.Binding {
	if !s.loaded {
		return nil
	}
	rerun := bind("r", "insights.key.rerun")
	if len(s.report(c, c.Width-2)) <= bodyRows(c)-3 {
		return []key.Binding{rerun}
	}
	return []key.Binding{scrollKeys(), rerun}
}

func (s *doctorScreen) Update(c *Ctx, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case shownMsg:
		if !s.loaded && !s.loading {
			return s.run(c)
		}
	case spinner.TickMsg:
		if s.loading {
			var cmd tea.Cmd
			s.spin, cmd = s.spin.Update(msg)
			return cmd
		}
	case doctorMsg:
		s.checks, s.loaded, s.loading = msg, true, false
	case tea.KeyPressMsg:
		switch k := msg.String(); {
		case k == "r" && !s.loading:
			return s.run(c)
		case s.loaded:
			s.scroll.key(k)
		}
	case tea.MouseWheelMsg:
		s.scroll.wheel(msg)
	}
	return nil
}

// run checks the setup in the background. A new spinner each run leaves the ticks of an earlier
// one to die out.
func (s *doctorScreen) run(c *Ctx) tea.Cmd {
	env := c.Env
	s.loading = true
	s.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(c.St.Accent))
	return tea.Batch(s.spin.Tick, func() tea.Msg { return doctorMsg(env.Doctor()) })
}

func (s *doctorScreen) View(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	switch {
	case !s.loaded:
		text := st.Muted.Render(l.Fit(i18n.T("insights.doctor.checking"), width-4))
		return middle(l, s.spin.View()+" "+text, width, height)
	case len(s.checks) == 0:
		return notice(c, st.Muted, i18n.T("insights.doctor.none"), "", width, height)
	}
	w := width - 2 // a cell of margin at each end
	lines := append([]string{"", s.summary(c, w), ""}, s.scroll.window(s.report(c, w), height-3)...)
	for i, line := range lines {
		lines[i] = " " + l.Pad(line, w) + " "
	}
	return strings.Join(lines, "\n")
}

// report is every check, a blank line apart.
func (s *doctorScreen) report(c *Ctx, width int) []string {
	var lines []string
	for i, ch := range s.checks {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, checkLines(c, ch, width)...)
	}
	return lines
}

// summary is the verdict ("Everything looks right", "2 things need attention") and, at the right
// edge, how many checks came out at each level.
func (s *doctorScreen) summary(c *Ctx, width int) string {
	l, st := c.L(), c.St
	count := map[ops.Level]int{}
	for _, ch := range s.checks {
		count[ch.Level]++
	}
	attention := count[ops.Warn] + count[ops.Fail]
	icon, text := levelIcon(st, ops.OK), st.Title.Render(l.Text(i18n.T("insights.doctor.fine")))
	switch {
	case s.loading:
		icon, text = s.spin.View(), st.Muted.Render(l.Text(i18n.T("insights.doctor.checking")))
	case count[ops.Fail] > 0:
		icon = levelIcon(st, ops.Fail)
	case attention > 0:
		icon = levelIcon(st, ops.Warn)
	}
	if attention > 0 && !s.loading {
		text = st.Title.Render(l.Text(i18n.T("insights.doctor.attention", "n", attention)))
	}
	var tally []string
	for _, lv := range doctorLevels {
		if n := count[lv]; n > 0 {
			tally = append(tally, levelIcon(st, lv)+" "+st.Muted.Render(l.Text(i18n.N(int64(n)))))
		}
	}
	return l.Row(icon+" "+text, strings.Join(tally, "   "), width)
}

// checkLines are one check wrapped to width: the icon and title, the detail in muted and the fix
// in the accent, both indented under the title.
func checkLines(c *Ctx, ch ops.Check, width int) []string {
	l, st := c.L(), c.St
	var out []string
	add := func(line string) { out = append(out, l.Pad(line, width)) }
	for i, t := range wrap(l, ch.Title, width-2) {
		lead := "  "
		if i == 0 {
			lead = levelIcon(st, ch.Level) + " "
		}
		add(lead + st.Text.Render(t))
	}
	for _, t := range wrap(l, ch.Detail, width-2) {
		add("  " + st.Muted.Render(t))
	}
	for i, t := range wrap(l, ch.Fix, width-4) {
		lead := "    "
		if i == 0 {
			lead = "  " + st.Accent.Render("›") + " "
		}
		add(lead + st.Accent.Render(t))
	}
	return out
}

// levelIcon marks a check's level: ✓ ok, i info, ! warn and ✗ fail, each in its colour.
func levelIcon(st Styles, lv ops.Level) string {
	switch lv {
	case ops.OK:
		return st.OK.Render("✓")
	case ops.Warn:
		return st.Warn.Render("!")
	case ops.Fail:
		return st.Fail.Render("✗")
	}
	return st.Accent.Render("i")
}
