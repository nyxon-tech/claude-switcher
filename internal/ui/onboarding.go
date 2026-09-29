package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

// onboarding is the welcome, on the first run and from Settings. It covers the whole window:
// the brand, then a name for the account Claude Desktop is signed into (when no profile holds
// it yet). It ends on Accounts.
type onboarding struct {
	waiting bool // enter came before the state was read, which the next step depends on
	saving  bool // on the second step: naming the signed-in account
	input   textinput.Model
	problem string
}

// Welcome opens the onboarding.
func (c *Ctx) Welcome() { c.onboard = &onboarding{} }

// update takes the messages that are not keys.
func (o *onboarding) update(a *app, msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case statusMsg:
		if o.waiting {
			return o.next(a)
		}
	default:
		if o.saving { // the field's cursor blinks
			var cmd tea.Cmd
			o.input, cmd = o.input.Update(msg)
			return cmd
		}
	}
	return nil
}

func (o *onboarding) key(a *app, msg tea.KeyPressMsg) tea.Cmd {
	if o.saving {
		return o.saveKey(a, msg)
	}
	switch msg.String() {
	case "enter":
		if !o.waiting {
			return o.next(a)
		}
	case "esc":
		return o.finish(a, "")
	case "q":
		return tea.Quit
	}
	return nil
}

func (o *onboarding) saveKey(a *app, msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc": // later
		return o.finish(a, "")
	case "enter":
		name := strings.TrimSpace(o.input.Value())
		if o.problem = validName(name); o.problem != "" {
			return nil
		}
		return o.finish(a, name)
	}
	var cmd tea.Cmd
	o.input, cmd = o.input.Update(msg)
	o.problem = ""
	return cmd
}

// next leaves the brand: for a name for the signed-in account when no profile holds it, else
// for the end. That depends on the state, so until it is read, next waits for it.
func (o *onboarding) next(a *app) tea.Cmd {
	if o.waiting = !a.Loaded; o.waiting {
		return nil
	}
	if a.Err == nil && unsavedAccount(a.Status) {
		o.saving = true
		return o.ask(a.Ctx)
	}
	return o.finish(a, "")
}

// unsavedAccount reports whether Claude Desktop is signed into an account no profile holds.
func unsavedAccount(st ops.Status) bool {
	return st.Account != "" && !slices.ContainsFunc(st.Profiles, func(p ops.Profile) bool { return p.Account == st.Account })
}

// ask opens the name field, suggesting "work" unless a profile has that name.
func (o *onboarding) ask(c *Ctx) tea.Cmd {
	o.input = textinput.New()
	o.input.Prompt = ""
	o.input.Placeholder = "work"
	o.input.CharLimit = 40
	o.input.SetStyles(c.St.input())
	if _, taken := findProfile(c.Status, "work"); !taken {
		o.input.SetValue("work")
		o.input.CursorEnd()
	}
	return o.input.Focus()
}

// finish ends the welcome on Accounts and remembers it was seen. Given a name, it saves the
// signed-in account under it as Accounts does, closing Desktop around the save if it runs.
func (o *onboarding) finish(a *app, name string) tea.Cmd {
	a.onboard = nil
	s := a.Settings
	s.Onboarded = true
	cmds := []tea.Cmd{a.Apply(s), a.show(0)}
	if name == "" {
		return tea.Batch(append(cmds, a.Toast(toastOK, i18n.T("welcome.done")))...)
	}
	return tea.Batch(append(cmds, a.screens[0].(*accountsScreen).saveAs(a.Ctx, name, false))...)
}

// view draws the step in the middle of the window, its key hints on the bottom line.
func (o *onboarding) view(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	body := st.hero(l, width)
	keys := []key.Binding{bind("enter", "welcome.key.continue"), bind("esc", "welcome.key.skip")}
	if o.saving {
		body = o.save(c, width)
		keys = []key.Binding{bind("enter", "welcome.key.save"), bind("esc", "welcome.key.later")}
	}
	foot := centredHints(c, keys, width)
	if o.waiting {
		foot = l.Center(st.Muted.Render(l.Fit(i18n.T("state.loading"), width)), width)
	}
	lines := make([]string, max(0, (height-1-len(body))/2), height)
	lines = append(lines, body...)
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	return strings.Join(append(lines[:height-1], foot), "\n")
}

// save asks for a name for the account Claude Desktop is signed into.
func (o *onboarding) save(c *Ctx, width int) []string {
	l, st := c.L(), c.St
	textW := min(64, width-8)
	lines := []string{l.Center(st.Title.Render(l.Fit(i18n.T("welcome.save.title"), textW)), width), ""}
	for _, t := range l.Wrap(i18n.T("welcome.save.body"), textW) {
		lines = append(lines, l.Center(st.Muted.Render(t), width))
	}
	lines = append(lines, "")
	o.input.SetWidth(min(textW, 40) - 5)
	for _, t := range strings.Split(st.Box(true).Padding(0, 1).Render(o.input.View()), "\n") {
		lines = append(lines, l.Center(t, width))
	}
	for _, t := range l.Wrap(o.problem, textW) {
		lines = append(lines, l.Center(st.Fail.Render(t), width))
	}
	return lines
}

// centredHints are key hints in the middle of a line.
func centredHints(c *Ctx, keys []key.Binding, width int) string {
	items := make([]string, len(keys))
	for i, b := range keys {
		items[i] = hintItem(c, b)
	}
	return c.L().Center(strings.Join(items, "   "), width)
}
