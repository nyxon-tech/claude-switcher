package ui

import (
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

// accountsScreen is home: the logo, then a card per saved account and a ghost card that adds one.
type accountsScreen struct {
	focus  int    // card index; len(profiles) is the add card
	placed bool   // focus has started on the profile in use
	follow string // a profile just saved, added or renamed: focus moves to it once it shows
	top    int    // first card row shown when they do not all fit
	geo    grid   // where the last View drew the cards, for clicks
}

// grid is where the cards landed in the body.
type grid struct {
	x, y        int // top-left cell of the first row shown
	cols, cardW int
	top, rows   int // first row shown and how many rows are shown
	n           int // cards, the add card included
}

const (
	cardH    = 5
	gapX     = 2
	gapY     = 1
	maxCardW = 30
	minCardW = 26
)

type mergePlanMsg struct {
	name string
	plan ops.Plan
	err  error
}

func (*accountsScreen) Title() string { return i18n.T("tab.accounts") }
func (*accountsScreen) Typing() bool  { return false }

// profiles are the saved profiles, plus the profile in use when its login is not saved yet
// (right after adding an account).
func (s *accountsScreen) profiles(c *Ctx) []ops.Profile {
	ps := slices.Clone(c.Status.Profiles)
	if cur := c.Status.Current; cur != "" {
		if _, ok := findProfile(c.Status, cur); !ok {
			ps = append(ps, ops.Profile{Name: cur, Current: true})
		}
	}
	return ps
}

func (s *accountsScreen) Keys(c *Ctx) []key.Binding {
	if !c.Loaded {
		return nil
	}
	if len(s.profiles(c)) == 0 {
		return []key.Binding{bind("s", "ui.key.save")}
	}
	return []key.Binding{
		bind("←↑↓→", "ui.key.move", "left", "right", "up", "down", "h", "j", "k", "l"),
		bind("enter", "ui.key.switch"),
		bind("n", "ui.key.add"),
		bind("s", "ui.key.save"),
		bind("r", "ui.key.rename"),
		bind("d", "ui.key.remove"),
		bind("m", "ui.key.merge"),
	}
}

func (s *accountsScreen) Update(c *Ctx, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case statusMsg:
		ps := s.profiles(c)
		if !s.placed {
			s.placed = true
			s.focus = max(0, slices.IndexFunc(ps, func(p ops.Profile) bool { return p.Current }))
		}
		if i := slices.IndexFunc(ps, func(p ops.Profile) bool { return strings.EqualFold(p.Name, s.follow) }); i >= 0 {
			s.focus, s.follow = i, ""
		}
	case mergePlanMsg:
		if c.runner != nil || c.dialog != nil { // the user moved on while the plan was worked out
			return nil
		}
		if msg.err != nil {
			return c.Toast(toastFail, msg.err.Error())
		}
		return s.confirmMerge(c, msg.name, msg.plan)
	case tea.KeyPressMsg:
		if c.Loaded {
			return s.key(c, msg.String())
		}
	case tea.MouseClickMsg:
		if i, ok := s.geo.hit(c.L(), msg.X, msg.Y); ok {
			if i == s.focus {
				return s.activate(c)
			}
			s.focus = i
		}
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

func (s *accountsScreen) key(c *Ctx, k string) tea.Cmd {
	ps := s.profiles(c)
	if len(ps) == 0 {
		if k == "s" {
			return s.save(c)
		}
		return nil
	}
	n, cols := len(ps)+1, max(s.geo.cols, 1)
	// Arrows move on screen: in Persian the cards are mirrored, so left is the next one.
	side := 1
	if c.L().RTL {
		side = -1
	}
	switch k {
	case "left", "h":
		s.focus = min(max(s.focus-side, 0), n-1)
	case "right", "l":
		s.focus = min(max(s.focus+side, 0), n-1)
	case "up", "k":
		if s.focus >= cols {
			s.focus -= cols
		}
	case "down", "j":
		if s.focus/cols < (n-1)/cols {
			s.focus = min(s.focus+cols, n-1)
		}
	case "enter":
		return s.activate(c)
	case "n":
		return s.add(c)
	case "s":
		return s.save(c)
	}
	if s.focus >= len(ps) {
		return nil
	}
	switch p := ps[s.focus]; k {
	case "r":
		return s.rename(c, p)
	case "d":
		return s.remove(c, p)
	case "m":
		return s.merge(c, p)
	}
	return nil
}

// activate is enter on a card: switch to its account, or add one on the add card.
func (s *accountsScreen) activate(c *Ctx) tea.Cmd {
	ps := s.profiles(c)
	if s.focus >= len(ps) {
		return s.add(c)
	}
	p := ps[s.focus]
	if p.Current {
		return c.Toast(toastInfo, i18n.T("ops.err.already-current", "name", p.Name))
	}
	var steps []string
	if cur := c.Status.Current; cur != "" {
		steps = append(steps, i18n.T("ui.step.saved", "name", cur))
	}
	env, done := c.Env, i18n.T("ui.switch.done", "name", p.Name)
	return c.Change(change{
		title: i18n.T("ui.switch.title", "name", p.Name), always: true,
		steps: append(steps, i18n.T("ui.step.restored", "name", p.Name)),
		run:   func() (string, error) { return done, env.Switch(p.Name) },
	})
}

func (s *accountsScreen) add(c *Ctx) tea.Cmd {
	env := c.Env
	return c.Prompt(i18n.T("ui.add.title"), i18n.T("ui.add.body"), "", i18n.T("ui.add.placeholder"),
		i18n.T("action.add"), freeName(c, ""), func(name string) tea.Cmd {
			var steps []string
			if cur := c.Status.Current; cur != "" {
				steps = append(steps, i18n.T("ui.step.saved", "name", cur))
			}
			done := i18n.T("ui.add.done", "name", name)
			s.follow = name
			return c.Change(change{
				title: i18n.T("ui.add.running", "name", name), always: true,
				steps: append(steps, i18n.T("ui.step.signed_out")),
				run:   func() (string, error) { return done, env.Add(name) },
			})
		})
}

// save stores the login Desktop is signed into, suggesting the name it already has.
func (s *accountsScreen) save(c *Ctx) tea.Cmd {
	st := c.Status
	if st.Account == "" {
		return c.Toast(toastWarn, i18n.T("ops.err.not-signed-in"))
	}
	suggest := st.Current
	for _, p := range st.Profiles {
		if p.Account == st.Account {
			suggest = p.Name
		}
	}
	if suggest == "" && len(st.Profiles) == 0 {
		suggest = "work"
	}
	return c.Prompt(i18n.T("ui.save.title"), i18n.T("ui.save.body"), suggest, "work", i18n.T("action.save"),
		validName, func(name string) tea.Cmd { return s.saveAs(c, name, false) })
}

// saveAs saves under name. A profile of that name from another account is only replaced after
// the user agrees.
func (s *accountsScreen) saveAs(c *Ctx, name string, replace bool) tea.Cmd {
	env, done := c.Env, i18n.T("ui.save.done", "name", name)
	s.follow = name
	return c.Change(change{
		title: i18n.T("ui.save.running", "name", name),
		steps: []string{i18n.T("ui.step.saved", "name", name)},
		run:   func() (string, error) { return done, env.Save(name, replace) },
		handle: func(err error) bool {
			if replace || !ops.Is(err, ops.OtherAccount) {
				return false
			}
			c.Confirm(i18n.T("ui.save.replace.title", "name", name), i18n.T("ui.save.replace.body", "name", name),
				i18n.T("ui.save.replace.yes"), true, func() tea.Cmd { return s.saveAs(c, name, true) })
			return true
		},
	})
}

func (s *accountsScreen) rename(c *Ctx, p ops.Profile) tea.Cmd {
	if p.Saved.IsZero() {
		return c.Toast(toastInfo, i18n.T("ui.rename.unsaved", "name", p.Name))
	}
	env := c.Env
	return c.Prompt(i18n.T("ui.rename.title", "name", p.Name), i18n.T("ui.rename.body"), p.Name, "",
		i18n.T("action.rename"), freeName(c, p.Name), func(name string) tea.Cmd {
			if name == p.Name {
				return nil
			}
			s.follow = name
			return c.Do(func() error { return env.Rename(p.Name, name) }, i18n.T("ui.rename.done", "old", p.Name, "name", name))
		})
}

func (s *accountsScreen) remove(c *Ctx, p ops.Profile) tea.Cmd {
	if p.Current {
		return c.Toast(toastWarn, i18n.T("ops.err.profile-in-use", "name", p.Name))
	}
	env := c.Env
	c.Confirm(i18n.T("ui.remove.title", "name", p.Name), i18n.T("ui.remove.body"), i18n.T("action.remove"), true,
		func() tea.Cmd {
			return c.Do(func() error { return env.Remove(p.Name) }, i18n.T("ui.remove.done", "name", p.Name))
		})
	return nil
}

// merge brings every chat into the profile's chat list, asking which one when its account has
// several (one per organization).
func (s *accountsScreen) merge(c *Ctx, p ops.Profile) tea.Cmd {
	var lists []ops.List
	for _, l := range c.Status.Lists {
		if p.Account != "" && l.Account == p.Account {
			lists = append(lists, l)
		}
	}
	switch len(lists) {
	case 0:
		c.Alert(i18n.T("ui.merge.title", "name", p.Name), i18n.T("ui.merge.no_list", "name", p.Name))
		return nil
	case 1:
		return planMerge(c, p.Name, lists[0])
	}
	labels := make([]string, len(lists))
	for i, l := range lists {
		labels[i] = l.Label
	}
	c.Choose(i18n.T("ui.merge.pick", "name", p.Name), "", labels, func(i int) tea.Cmd {
		return planMerge(c, p.Name, lists[i])
	})
	return nil
}

func planMerge(c *Ctx, name string, to ops.List) tea.Cmd {
	env := c.Env
	return func() tea.Msg {
		plan, err := env.PlanMerge(to)
		return mergePlanMsg{name, plan, err}
	}
}

func (s *accountsScreen) confirmMerge(c *Ctx, name string, plan ops.Plan) tea.Cmd {
	title := i18n.T("ui.merge.title", "name", name)
	add, upd := plan.Count(ops.Create), plan.Count(ops.Update)
	if add+upd == 0 {
		c.Alert(title, i18n.T("ui.merge.nothing", "name", name))
		return nil
	}
	counts := i18n.T("ui.merge.counts", "new", add, "updated", upd, "newer", plan.Count(ops.SkipNewer))
	env := c.Env
	c.Confirm(title, counts+"\n"+i18n.T("ui.merge.body"), i18n.T("action.merge"), false, func() tea.Cmd {
		return c.Change(change{
			title: i18n.T("ui.merge.running", "name", name),
			steps: []string{i18n.T("ui.step.merged", "name", name)},
			run: func() (string, error) {
				r, err := env.Apply(plan)
				return r.Summary, err
			},
		})
	})
	return nil
}

// validName checks a profile name against the rules ops enforces, before anything runs.
func validName(name string) string {
	if !store.ValidName(name) {
		return i18n.T("ops.err.bad-name", "name", name)
	}
	return ""
}

// freeName is validName plus: no other profile has the name. keep may keep its own.
func freeName(c *Ctx, keep string) func(string) string {
	return func(name string) string {
		if p, ok := findProfile(c.Status, name); ok && !strings.EqualFold(p.Name, keep) {
			return i18n.T("ops.err.profile-exists", "name", p.Name)
		}
		return validName(name)
	}
}

func (s *accountsScreen) View(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	s.geo = grid{} // no cards to click until the grid is drawn
	switch {
	case !c.Loaded:
		return middle(l, st.Muted.Render(l.Fit(i18n.T("state.loading"), width)), width, height)
	case c.Err != nil && len(c.Status.Profiles) == 0:
		return middle(l, st.Fail.Render(l.Fit(c.Err.Error(), width)), width, height)
	}
	ps := s.profiles(c)
	if len(ps) == 0 {
		return s.empty(c, width, height)
	}

	n := len(ps) + 1
	s.focus = min(s.focus, n-1)
	g := newGrid(width, n)
	total := (n + g.cols - 1) / g.cols
	head := heading(c, width, height-gridHeight(total))
	g.rows = min(total, max(1, (height-len(head)+gapY)/(cardH+gapY)))
	if fr := s.focus / g.cols; fr < s.top {
		s.top = fr
	} else if fr >= s.top+g.rows {
		s.top = fr - g.rows + 1
	}
	s.top = min(s.top, total-g.rows)
	g.top = s.top

	pad := max(0, (height-len(head)-gridHeight(g.rows))/2)
	g.y = pad + len(head)
	s.geo = g
	lines := make([]string, pad, height)
	lines = append(lines, head...)
	gridW := g.cols*g.cardW + (g.cols-1)*gapX
	for r := g.top; r < g.top+g.rows; r++ {
		if r > g.top {
			lines = append(lines, "")
		}
		var cells []string
		for i := r * g.cols; i < min(n, (r+1)*g.cols); i++ {
			if len(cells) > 0 {
				cells = append(cells, strings.Repeat(" ", gapX))
			}
			if i < len(ps) {
				cells = append(cells, profileCard(c, ps[i], i == s.focus, g.cardW))
			} else {
				cells = append(cells, addCard(c, i == s.focus, g.cardW))
			}
		}
		if l.RTL {
			slices.Reverse(cells)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, cells...)
		for _, line := range strings.Split(row, "\n") {
			lines = append(lines, l.Center(l.Pad(line, gridW), width))
		}
	}
	return strings.Join(lines, "\n")
}

// heading is the logo (when it fits) and the tagline, in at most height lines.
func heading(c *Ctx, width, height int) []string {
	l, st := c.L(), c.St
	tagline := []string{l.Center(st.Muted.Render(l.Fit(i18n.T("app.tagline"), width)), width), "", ""}
	for _, logo := range st.logos(c.Width, c.Height) {
		var lines []string
		for _, line := range strings.Split(logo, "\n") {
			lines = append(lines, l.Center(line, width))
		}
		if lines = append(append(lines, ""), tagline...); len(lines) <= height {
			return lines
		}
	}
	if len(tagline) <= height {
		return tagline
	}
	return nil
}

func newGrid(width, n int) grid {
	cols := min(3, n, max(1, (width+gapX)/(minCardW+gapX)))
	cardW := min(maxCardW, (width-(cols-1)*gapX)/cols)
	return grid{cols: cols, cardW: cardW, n: n, x: (width - cols*cardW - (cols-1)*gapX) / 2}
}

func gridHeight(rows int) int { return rows*(cardH+gapY) - gapY }

// hit finds the card under a body cell. In Persian the rows are mirrored and the last, shorter
// row keeps to the right edge.
func (g grid) hit(l Layout, x, y int) (int, bool) {
	x, y = x-g.x, y-g.y
	if g.cols == 0 || x < 0 || y < 0 || x%(g.cardW+gapX) >= g.cardW || y%(cardH+gapY) >= cardH {
		return 0, false
	}
	col, row := x/(g.cardW+gapX), y/(cardH+gapY)
	if col >= g.cols || row >= g.rows {
		return 0, false
	}
	if l.RTL {
		col = g.cols - 1 - col
	}
	i := (g.top+row)*g.cols + col
	return i, i < g.n
}

// profileCard is one account: its name and ACTIVE badge, its chats, when its login was saved and
// its account id.
func profileCard(c *Ctx, p ops.Profile, focused bool, width int) string {
	l, st := c.L(), c.St
	inner := width - 4 // border and one cell of padding each side
	dot, badge := st.Dim.Render("○"), ""
	if p.Current {
		dot = st.Warn.Render("●")
		if p.SignedIn {
			dot = st.OK.Render("●")
		}
		badge = st.Badge.Render(" " + l.Text(i18n.T("ui.badge.active")) + " ")
	}
	nameW := inner - 2
	if badge != "" {
		nameW -= ansi.StringWidth(badge) + 1
	}
	name := st.Bold.Render(l.Data(p.Name, nameW))
	chats := st.Muted.Render(l.Fit(i18n.T("count.chats", "n", p.Chats), inner-9))
	id := ""
	if p.Account != "" {
		id = st.Dim.Render(l.Path(p.Account[:min(8, len(p.Account))], 8))
	}
	saved := st.Warn.Render(l.Fit(i18n.T("ui.accounts.unsaved"), inner))
	if !p.Saved.IsZero() {
		saved = st.Muted.Render(l.Fit(i18n.T("ui.accounts.saved", "ago", i18n.Ago(p.Saved.Local(), c.Now())), inner))
	}
	body := strings.Join([]string{
		l.Row(l.Inline(dot, " ", name), badge, inner),
		l.Row(chats, id, inner),
		l.Pad(saved, inner),
	}, "\n")
	return st.Box(focused).Padding(0, 1).Render(body)
}

// addCard is the ghost card at the end of the grid.
func addCard(c *Ctx, focused bool, width int) string {
	l, st := c.L(), c.St
	inner := width - 4
	text := st.Muted
	if focused {
		text = st.Accent
	}
	label := l.Inline(st.Accent.Render("+"), " ", text.Render(l.Fit(i18n.T("ui.accounts.add"), inner-2)))
	blank := strings.Repeat(" ", inner)
	return st.Ghost(focused).Padding(0, 1).Render(strings.Join([]string{blank, l.Center(label, inner), blank}, "\n"))
}

// empty is the first-run screen: nothing is saved yet, so offer to save the signed-in account.
func (s *accountsScreen) empty(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	textW := min(60, width-4)
	var body []string
	add := func(style lipgloss.Style, text string) {
		for _, t := range l.Wrap(text, textW) {
			body = append(body, l.Center(style.Render(t), width))
		}
	}
	add(st.Title, i18n.T("ui.accounts.empty.title"))
	body = append(body, "")
	if c.Status.Account == "" {
		add(st.Muted, i18n.T("ui.accounts.empty.signed_out"))
	} else {
		add(st.Muted, i18n.T("ui.accounts.empty.body"))
		body = append(body, "", l.Center(l.Inline(st.Badge.Render(" s "), "  ", st.Text.Render(l.Fit(i18n.T("ui.accounts.empty.action"), textW-5))), width))
	}
	lines := append(heading(c, width, height-len(body)), body...)
	return strings.Repeat("\n", max(0, (height-len(lines))/2)) + strings.Join(lines, "\n")
}

// middle centres one line in a width by height area.
func middle(l Layout, line string, width, height int) string {
	return strings.Repeat("\n", height/2) + l.Center(line, width)
}
