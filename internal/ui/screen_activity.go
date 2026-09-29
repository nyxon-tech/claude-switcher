package ui

import (
	"cmp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// activityScreen is the timeline of changes to the chat lists, newest first, with undo.
type activityScreen struct {
	js     []store.Journal
	loaded bool
	err    error
	cursor int
	top    int  // first row shown
	rows   int  // rows the last View had room for, for paging and clicks
	open   bool // the details of the entry under the cursor are shown
	dirty  bool // an undo is under way: read the history again once it is over
}

type historyMsg struct {
	js  []store.Journal
	err error
}

// listTop is the blank line between the divider and the first row.
const listTop = 1

// actionIcons mark each kind of change.
var actionIcons = map[string]string{ops.Copy: "+", ops.Move: "→", ops.Merge: "»", ops.Rescue: "↑"}

func (*activityScreen) Title() string { return i18n.T("tab.activity") }
func (*activityScreen) Typing() bool  { return false }

func (s *activityScreen) Keys(*Ctx) []key.Binding {
	switch {
	case s.open:
		return []key.Binding{bind("esc", "insights.key.close", "esc", "enter")}
	case len(s.js) == 0:
		return nil
	}
	undo := bind("u", "insights.key.undo")
	undo.SetEnabled(slices.ContainsFunc(s.js, func(j store.Journal) bool { return !j.Undone }))
	return []key.Binding{
		bind("↑↓", "ui.key.move", "up", "down", "k", "j", "pgup", "pgdown", "home", "end"),
		bind("enter", "insights.key.details"),
		undo,
	}
}

func (s *activityScreen) Update(c *Ctx, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case shownMsg:
		s.open = false
		return s.read(c)
	case statusMsg:
		if s.dirty && c.runner == nil { // the undo is over, done or not
			s.dirty = false
			return s.read(c)
		}
	case historyMsg:
		s.js, s.err, s.loaded = msg.js, msg.err, true
		s.cursor = min(s.cursor, max(0, len(s.js)-1))
		s.open = s.open && len(s.js) > 0
	case tea.KeyPressMsg:
		if s.loaded {
			return s.key(c, msg.String())
		}
	case tea.MouseClickMsg:
		s.click(msg.Y)
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			return s.key(c, "up")
		case tea.MouseWheelDown:
			return s.key(c, "down")
		}
	}
	return nil
}

func (s *activityScreen) read(c *Ctx) tea.Cmd {
	env := c.Env
	return func() tea.Msg {
		js, err := env.History()
		return historyMsg{js, err}
	}
}

func (s *activityScreen) key(c *Ctx, k string) tea.Cmd {
	if s.open {
		s.open = k != "esc" && k != "enter"
		return nil
	}
	if k == "u" {
		return s.undo(c)
	}
	if len(s.js) == 0 {
		return nil
	}
	switch k {
	case "up", "k":
		s.cursor--
	case "down", "j":
		s.cursor++
	case "pgup":
		s.cursor -= max(1, s.rows-1)
	case "pgdown":
		s.cursor += max(1, s.rows-1)
	case "home":
		s.cursor = 0
	case "end":
		s.cursor = len(s.js) - 1
	case "enter":
		s.open = true
	}
	s.cursor = min(max(s.cursor, 0), len(s.js)-1)
	return nil
}

// click selects the row under the pointer and opens the details of a row already selected. A
// click anywhere closes them.
func (s *activityScreen) click(y int) {
	if s.open {
		s.open = false
		return
	}
	i := s.top + y - listTop
	if y < listTop || i >= min(len(s.js), s.top+s.rows) {
		return
	}
	s.open = i == s.cursor
	s.cursor = i
}

// undo asks before undoing the newest change that is not undone yet, then undoes it with Claude
// Desktop closed.
func (s *activityScreen) undo(c *Ctx) tea.Cmd {
	i := slices.IndexFunc(s.js, func(j store.Journal) bool { return !j.Undone })
	if i < 0 {
		return c.Toast(toastInfo, i18n.T("ops.err.nothing-to-undo"))
	}
	j, env := s.js[i], c.Env
	body := i18n.T("insights.undo.body", "summary", j.Summary, "ago", i18n.Ago(j.At.Local(), c.Now()))
	c.Confirm(i18n.T("insights.undo.title"), body, i18n.T("action.undo"), false, func() tea.Cmd {
		s.dirty = true
		return c.Change(change{
			title: i18n.T("insights.undo.running"),
			steps: []string{i18n.T("insights.undo.step")},
			run: func() (string, error) {
				j, err := env.Undo()
				return i18n.T("insights.undo.done", "summary", j.Summary), err
			},
		})
	})
	return nil
}

func (s *activityScreen) View(c *Ctx, width, height int) string {
	st := c.St
	switch {
	case !s.loaded:
		return notice(c, st.Muted, i18n.T("state.loading"), "", width, height)
	case s.err != nil:
		return notice(c, st.Fail, i18n.T("insights.activity.error"), s.err.Error(), width, height)
	case len(s.js) == 0:
		return notice(c, st.Title, i18n.T("insights.activity.empty.title"), i18n.T("insights.activity.empty.body"), width, height)
	}
	list := s.list(c, width, height)
	if !s.open {
		return list
	}
	return overlay(st, list, s.details(c, width, height), width, height)
}

// list is one row per change, scrolled so the cursor shows.
func (s *activityScreen) list(c *Ctx, width, height int) string {
	s.rows = max(1, height-listTop)
	if s.cursor < s.top {
		s.top = s.cursor
	} else if s.cursor >= s.top+s.rows {
		s.top = s.cursor - s.rows + 1
	}
	s.top = max(0, min(s.top, len(s.js)-s.rows))
	lines := make([]string, listTop, height)
	for i := s.top; i < min(len(s.js), s.top+s.rows); i++ {
		lines = append(lines, s.row(c, s.js[i], i == s.cursor, width))
	}
	return strings.Join(lines, "\n")
}

// row is one change: the selection bar, its icon and summary (struck through once undone), and
// at the right edge how long ago it was.
func (s *activityScreen) row(c *Ctx, j store.Journal, focused bool, width int) string {
	l, st := c.L(), c.St
	width -= 2 // a cell of margin at each end
	bar, text, tag := " ", st.Text, ""
	if focused {
		bar, text = st.Bar.Render("▌"), st.Accent
	}
	if j.Undone {
		text = st.Dim.Strikethrough(true)
		tag = "  " + st.Dim.Render(l.Text(i18n.T("insights.activity.undone")))
	}
	when := st.Muted.Render(l.Text(i18n.Ago(j.At.Local(), c.Now())))
	room := width - 5 - ansi.StringWidth(tag) - 2 - ansi.StringWidth(when) // the bar and the icon, and a gap before the time
	lead := bar + " " + actionIcon(st, j) + "  " + text.Render(l.Data(j.Summary, max(room, 1))) + tag
	return " " + l.Row(lead, when, width) + " "
}

func actionIcon(st Styles, j store.Journal) string {
	style := st.Accent
	if j.Undone {
		style = st.Dim
	}
	return style.Render(cmp.Or(actionIcons[j.Action], "·"))
}

// details is the box over the list for the change under the cursor: what kind of change it was,
// when, how many files it created and replaced or removed, and their names.
func (s *activityScreen) details(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	j := s.js[s.cursor]
	inner := boxInner(width)
	var names []string
	created := 0
	for _, e := range j.Entries {
		if e.Op == store.OpCreated {
			created++
		}
		names = append(names, folder(e.Path))
	}
	at := j.At.Local()
	when := i18n.Date(at) + " · " + i18n.Digits(at.Format("15:04"))
	if ago := i18n.Ago(at, c.Now()); ago != i18n.Date(at) {
		when += " · " + ago
	}
	kind := j.Action // a kind this version does not know, from a newer one
	if _, ok := actionIcons[j.Action]; ok {
		kind = i18n.T("insights.action." + j.Action)
	}
	fields := [][2]string{
		{"insights.details.kind", kind},
		{"insights.details.when", when},
		{"insights.details.files", i18n.T("insights.details.counts", "created", created, "removed", len(j.Entries)-created)},
	}
	if j.Undone {
		fields = append(fields, [2]string{"insights.details.status", i18n.T("insights.details.undone")})
	}
	labelW := 0
	for _, f := range fields {
		labelW = max(labelW, ansi.StringWidth(l.Text(i18n.T(f[0])))+2)
	}
	lines := []string{l.Pad(st.Title.Render(l.Data(j.Summary, inner)), inner), ""}
	for _, f := range fields {
		label := l.Pad(st.Muted.Render(l.Text(i18n.T(f[0]))), labelW)
		lines = append(lines, l.Pad(label+st.Text.Render(l.Fit(f[1], inner-labelW)), inner))
	}

	// The names get the rows the window has left, after the border, the padding and the button.
	room := height - 4 - len(lines) - 3
	shown, more := names, 0
	if len(names) > room {
		shown = names[:max(room-1, 0)]
		more = len(names) - len(shown)
	}
	if len(shown) > 0 {
		lines = append(lines, "")
	}
	for _, name := range shown {
		lines = append(lines, l.Pad(st.Dim.Render(l.Path(name, inner)), inner))
	}
	if more > 0 && room > 0 {
		lines = append(lines, l.Pad(st.Muted.Render(l.Fit(i18n.T("insights.details.more", "n", more), inner)), inner))
	}
	button := st.Badge.Render(" " + l.Text(i18n.T("action.close")) + " ")
	lines = append(lines, "", l.End(button, inner))
	return boxed(st, lines, height)
}

// overlay dims the body behind a box and centres the box over it in a clear ring, as the app
// does for dialogs.
func overlay(st Styles, body, box string, width, height int) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = st.Dim.Render(ansi.Strip(line))
	}
	ring := 1
	if lipgloss.Height(box)+2 > height {
		ring = 0
	}
	box = lipgloss.NewStyle().Margin(ring, 2).Render(box)
	x, y := place(box, width, height)
	return lipgloss.NewCanvas(width, height).Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(strings.Join(lines, "\n")), lipgloss.NewLayer(box).X(x).Y(y).Z(1))).Render()
}
