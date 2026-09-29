package ui

import (
	"context"
	"errors"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
)

// change is a write to Claude's data, carried out by the runner so that Claude Desktop is closed
// around it. Every screen writes through one (see Ctx.Change).
type change struct {
	title  string                 // what is happening: "Switching to personal"
	steps  []string               // what the ops call does, shown as one checklist entry each
	run    func() (string, error) // the ops call; returns the summary toasted when it is done
	always bool                   // reopen Desktop even when it was closed before (switch, add)
	handle func(err error) bool   // optional: takes over an error the caller can act on
}

type phase int

const (
	running   phase = iota // the ops call is under way
	waiting                // Desktop was open: asked to quit, waiting until it has
	launching              // reopening Desktop
	failed                 // the error is shown until the user goes back
)

// runner shows a change's steps with a spinner. It first tries the ops call: ops refuses
// before writing anything while Desktop runs, and every other refusal (a bad name, another
// account) comes back before the user is asked to quit Desktop. On desktop-running it asks
// Desktop to quit (Windows: the user quits from the tray), waits with ops.WaitClosed, and tries
// again. Then it reopens Desktop when the change or the earlier state calls for it.
type runner struct {
	ch       change
	phase    phase
	waited   bool // Desktop was open, so it was closed for the change
	manual   bool // Desktop can only be quit by hand (its tray icon)
	cancel   context.CancelFunc
	spin     spinner.Model
	ran      bool // the ops call succeeded
	launched bool
	err      error
	note     string // a problem that does not stop the change (quit or force quit failed)
	summary  string
	ask      *dialog // the force-close question, while it is open
}

type (
	runDoneMsg struct {
		summary string
		err     error
	}
	quitMsg     struct{ err error }
	closedMsg   struct{ err error }
	forceMsg    struct{ err error }
	launchedMsg struct{ err error }
)

// Change starts the runner for a write. Only one change runs at a time: while one does, a
// second is dropped (its messages would reach the wrong runner).
func (c *Ctx) Change(ch change) tea.Cmd {
	if c.runner != nil {
		return nil
	}
	r := &runner{ch: ch, spin: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(c.St.Accent))}
	c.runner = r
	return tea.Batch(r.spin.Tick, r.try())
}

func (r *runner) try() tea.Cmd {
	r.phase = running
	run := r.ch.run
	return func() tea.Msg {
		summary, err := run()
		return runDoneMsg{summary, err}
	}
}

// relaunch reports whether Desktop is reopened after the change: always after switching or
// adding an account, otherwise only when it was open before, and never with --no-launch.
func (r *runner) relaunch(c *Ctx) bool {
	return !c.Opts.NoLaunch && (r.waited || r.ch.always && r.err == nil)
}

// update handles the runner's own messages and keys; it closes the runner through c.
func (r *runner) update(c *Ctx, msg tea.Msg) tea.Cmd {
	env := c.Env
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		r.spin, cmd = r.spin.Update(msg)
		return cmd
	case runDoneMsg:
		if ops.Is(msg.err, ops.DesktopRunning) {
			ctx, cancel := context.WithCancel(context.Background())
			r.phase, r.waited, r.cancel = waiting, true, cancel
			return tea.Sequence(
				func() tea.Msg { return quitMsg{env.Desktop.Quit()} },
				func() tea.Msg { return closedMsg{env.WaitClosed(ctx)} },
			)
		}
		r.summary, r.err, r.ran = msg.summary, msg.err, msg.err == nil
		// Once Desktop was closed for the change, the error is shown here so Desktop reopens.
		if r.err != nil && !r.waited && r.ch.handle != nil && r.ch.handle(r.err) {
			c.runner = nil
			return c.Refresh()
		}
		if r.relaunch(c) {
			r.phase = launching
			return func() tea.Msg { return launchedMsg{env.Desktop.Launch()} }
		}
		return r.finish(c)
	case quitMsg:
		r.manual = errors.Is(msg.err, platform.ErrManualQuit)
		if msg.err != nil && !r.manual {
			r.note = msg.err.Error()
		}
	case closedMsg:
		r.cancel()
		if c.dialog != nil && c.dialog == r.ask { // Desktop closed on its own: no need to force it
			c.dialog = nil
		}
		if msg.err == nil {
			return r.try()
		}
		// Cancelled, or Desktop's state could not be read: nothing was written either way.
		c.runner = nil
		toast := c.Toast(toastInfo, i18n.T("ui.run.cancelled"))
		if !errors.Is(msg.err, context.Canceled) {
			toast = c.Toast(toastFail, msg.err.Error())
		}
		return tea.Batch(toast, c.Refresh())
	case forceMsg:
		if msg.err != nil {
			r.note = msg.err.Error()
		}
	case launchedMsg:
		r.launched = true
		if msg.err != nil {
			r.note = i18n.T("ui.run.launch_failed", "error", msg.err.Error())
		}
		return r.finish(c)
	case tea.KeyPressMsg:
		return r.key(c, msg)
	}
	return nil
}

func (r *runner) key(c *Ctx, msg tea.KeyPressMsg) tea.Cmd {
	env := c.Env
	switch {
	case r.phase == waiting && msg.String() == "esc":
		r.cancel()
	case r.phase == waiting && msg.String() == "f":
		c.Confirm(i18n.T("ui.run.force.title"), i18n.T("ui.run.force.body"), i18n.T("ui.run.force.yes"), true, func() tea.Cmd {
			if c.runner != r || r.phase != waiting { // closed meanwhile, maybe reopened: never kill it now
				return nil
			}
			return func() tea.Msg { return forceMsg{env.Desktop.ForceQuit()} }
		})
		r.ask = c.dialog
	case r.phase == failed && (msg.String() == "esc" || msg.String() == "enter"):
		c.runner = nil
	}
	return nil
}

// finish ends a change: on success the runner closes and a toast says what happened (or what
// went wrong around it); an error stays on screen until the user goes back.
func (r *runner) finish(c *Ctx) tea.Cmd {
	if r.err != nil {
		r.phase = failed
		return c.Refresh()
	}
	c.runner = nil
	toast := c.Toast(toastOK, r.summary)
	if r.note != "" { // the change is done, but Desktop did not quit or open as asked
		toast = c.Toast(toastWarn, r.note)
	}
	return tea.Batch(toast, c.Refresh())
}

func (r *runner) keys() []key.Binding {
	switch r.phase {
	case waiting:
		return []key.Binding{bind("f", "ui.key.force"), bind("esc", "ui.key.cancel")}
	case failed:
		return []key.Binding{bind("esc", "ui.key.back")}
	}
	return nil
}

// view draws the runner's box, at most width by height cells: its title, the checklist and
// what the user can do.
func (r *runner) view(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	inner := boxInner(width)
	lines := []string{l.Pad(st.Title.Render(l.Fit(r.ch.title, inner)), inner), ""}
	step := func(done, active, bad bool, text string) {
		icon := st.Dim.Render("·")
		switch {
		case done:
			icon = st.OK.Render("✓")
		case bad:
			icon = st.Fail.Render("✗")
		case active:
			icon = r.spin.View()
		}
		lines = append(lines, l.Pad(l.Inline(icon, " ", st.Text.Render(l.Fit(text, inner-2))), inner))
	}
	block := func(style func(...string) string, text string, indent string) {
		for _, t := range l.Wrap(text, inner-len(indent)) {
			lines = append(lines, l.Pad(l.Inline(indent, style(t)), inner))
		}
	}
	if r.waited {
		step(r.phase != waiting, r.phase == waiting, false, i18n.T("ui.step.wait"))
		if r.phase == waiting && r.manual {
			block(st.Muted.Render, i18n.T("ui.step.tray"), "  ")
		}
	}
	for i, s := range r.ch.steps {
		first := i == 0
		step(r.ran, first && r.phase == running, first && r.err != nil, s)
	}
	if r.launched || r.relaunch(c) {
		step(r.launched, r.phase == launching, false, i18n.T("ui.step.open"))
	}
	if r.note != "" {
		lines = append(lines, "")
		block(st.Warn.Render, r.note, "")
	}
	if r.err != nil {
		lines = append(lines, "")
		block(st.Fail.Render, r.err.Error(), "")
	}
	if ks := r.keys(); len(ks) > 0 {
		lines = append(lines, "", hintLine(c, ks, inner))
	}
	return boxed(st, lines, height)
}
