package ui

import (
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

// onboarding is the welcome, on the first run and from Settings. It covers the whole window:
// the brand and a greeting, then a name for the account Claude Desktop is signed into (when no
// profile holds it yet). It ends on Accounts.
type onboarding struct {
	fade    int  // how far the greeting has faded in; fadeSteps once it is done
	waiting bool // enter came before the state was read, which the next step depends on
	saving  bool // on the second step: naming the signed-in account
	input   textinput.Model
	problem string
}

type fadeMsg struct{}

const (
	fadeSteps = 8
	fadeEvery = 40 * time.Millisecond // the greeting takes a third of a second to fade in
)

// Welcome opens the onboarding. Its greeting fades in, unless the theme has no colours.
func (c *Ctx) Welcome() tea.Cmd {
	c.onboard = &onboarding{}
	if c.St.Plain {
		c.onboard.fade = fadeSteps
		return nil
	}
	return fadeTick()
}

func fadeTick() tea.Cmd {
	return tea.Tick(fadeEvery, func(time.Time) tea.Msg { return fadeMsg{} })
}

// update takes the messages that are not keys.
func (o *onboarding) update(a *app, msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case fadeMsg:
		if o.fade < fadeSteps {
			o.fade++
		}
		if o.fade < fadeSteps {
			return fadeTick()
		}
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
	if o.fade < fadeSteps { // any key skips the fade, and does nothing else
		o.fade = fadeSteps
		return nil
	}
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

// next leaves the greeting: for a name for the signed-in account when no profile holds it, else
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
	body := o.hello(c, width, height)
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

// hello is the brand, when the window has room for it, and the greeting.
func (o *onboarding) hello(c *Ctx, width, height int) []string {
	l, st := c.L(), c.St
	var lines []string
	if logos := st.logos(width, height); len(logos) > 0 {
		for _, line := range strings.Split(logos[0], "\n") {
			lines = append(lines, l.Center(line, width)) // a brand mark: centred, never mirrored
		}
		lines = append(lines, "", "")
	}
	textW := min(64, width-8)
	lines = append(lines, l.Center(o.faded(st, st.Title).Render(l.Fit(i18n.T("welcome.title"), textW)), width), "")
	for _, t := range l.Wrap(i18n.T("app.tagline"), textW) {
		lines = append(lines, l.Center(o.faded(st, st.Muted).Render(t), width))
	}
	return lines
}

// faded is style part way through the greeting's fade-in: its colour mixed with the void, the
// palette's stand-in for the terminal's background.
func (o *onboarding) faded(st Styles, style lipgloss.Style) lipgloss.Style {
	if o.fade >= fadeSteps || st.Plain {
		return style
	}
	return style.Foreground(lipgloss.Blend1D(fadeSteps+1, st.pal.void, style.GetForeground())[o.fade])
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

// centredHints are key hints in reading order, in the middle of a line.
func centredHints(c *Ctx, keys []key.Binding, width int) string {
	var parts []string
	for i, b := range keys {
		if i > 0 {
			parts = append(parts, "   ")
		}
		parts = append(parts, hintItem(c, b))
	}
	l := c.L()
	return l.Center(l.Inline(parts...), width)
}
